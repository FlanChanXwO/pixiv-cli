package installer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

func extractReleaseBinary(archive []byte, archiveName, destination, binaryName string) error {
	if strings.HasSuffix(archiveName, ".tar.gz") {
		return extractTarGzBinary(archive, destination, binaryName)
	}
	if strings.HasSuffix(archiveName, ".zip") {
		return extractZipBinary(archive, destination, binaryName)
	}
	return fmt.Errorf("unsupported release archive %q", archiveName)
}

func extractTarGzBinary(archive []byte, destination, binaryName string) error {
	if err := validateTarGzArchivePaths(archive); err != nil {
		return err
	}
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open gzip release archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	found := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar release archive: %w", err)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("release archive contains link entry %q", header.Name)
		}
		if path.Base(header.Name) != binaryName {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("release archive binary %q is not a regular file", header.Name)
		}
		if found {
			return fmt.Errorf("release archive contains duplicate binary %q", binaryName)
		}
		if err := writeExtractedBinary(destination, reader); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return fmt.Errorf("release archive has no regular binary %q", binaryName)
	}
	return nil
}

// validateTarGzArchivePaths 在任何候选文件写入前扫描整个 tar stream，防止后置的
// 不安全路径在合法二进制已经写出后才被发现。
func validateTarGzArchivePaths(archive []byte) error {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open gzip release archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar release archive: %w", err)
		}
		if err := validateArchivePath(header.Name); err != nil {
			return err
		}
	}
}

func extractZipBinary(archive []byte, destination, binaryName string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open zip release archive: %w", err)
	}
	for _, entry := range reader.File {
		if err := validateArchivePath(entry.Name); err != nil {
			return err
		}
	}
	found := false
	for _, entry := range reader.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("release archive contains link entry %q", entry.Name)
		}
		if path.Base(entry.Name) != binaryName {
			continue
		}
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return fmt.Errorf("release archive binary %q is not a regular file", entry.Name)
		}
		if found {
			return fmt.Errorf("release archive contains duplicate binary %q", binaryName)
		}
		body, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open release archive binary %q: %w", entry.Name, err)
		}
		writeErr := writeExtractedBinary(destination, body)
		closeErr := body.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return fmt.Errorf("close release archive binary %q: %w", entry.Name, closeErr)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("release archive has no regular binary %q", binaryName)
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("release archive has unsafe path %q", name)
	}
	// 必须在 path.Clean 前检查原始 segment，避免 `nested/../pixiv` 被归一化后
	// 伪装成可接受的候选二进制路径。
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return fmt.Errorf("release archive has unsafe path %q", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("release archive has unsafe path %q", name)
	}
	return nil
}

func writeExtractedBinary(destination string, source io.Reader) (err error) {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return fmt.Errorf("create extracted release binary %q: %w", destination, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close extracted release binary %q: %w", destination, closeErr)
		}
	}()
	if _, err := io.Copy(file, source); err != nil {
		return fmt.Errorf("write extracted release binary %q: %w", destination, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync extracted release binary %q: %w", destination, err)
	}
	if err := file.Chmod(0o755); err != nil {
		return fmt.Errorf("set extracted release binary permissions %q: %w", destination, err)
	}
	return nil
}
