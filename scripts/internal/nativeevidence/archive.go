package nativeevidence

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func verifyArchive(archive, binaryName, binaryDigest string) ([]evidenceFile, error) {
	expected := map[string]string{binaryName: binaryDigest}
	var err error
	var actual map[string]string
	if strings.HasSuffix(archive, ".tar.gz") {
		actual, err = readTarGzMembers(archive)
	} else if strings.HasSuffix(archive, ".zip") {
		actual, err = readZIPMembers(archive)
	} else {
		return nil, errors.New("archive must end with .tar.gz or .zip")
	}
	if err != nil {
		return nil, err
	}
	if len(actual) != len(expected) {
		return nil, fmt.Errorf("archive regular members = %d, want complete set of %d", len(actual), len(expected))
	}
	members := make([]evidenceFile, 0, len(expected))
	for name, want := range expected {
		got, ok := actual[name]
		if !ok || got != want {
			return nil, fmt.Errorf("archive member %q does not match expected binary or license content", name)
		}
		members = append(members, evidenceFile{Name: name, SHA256: got})
	}
	sort.Slice(members, func(left, right int) bool { return members[left].Name < members[right].Name })
	return members, nil
}

func readTarGzMembers(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open tar.gz: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	members := make(map[string]string)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar member: %w", err)
		}
		if err := validateArchiveMemberPath(header.Name); err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("archive contains a non-regular member %q", header.Name)
		}
		if err := addArchiveMember(members, header.Name, reader); err != nil {
			return nil, err
		}
	}
	return members, nil
}

func readZIPMembers(path string) (map[string]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	members := make(map[string]string)
	for _, member := range reader.File {
		if err := validateArchiveMemberPath(member.Name); err != nil {
			return nil, err
		}
		info := member.FileInfo()
		if info.IsDir() {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("archive contains a non-regular member %q", member.Name)
		}
		body, err := member.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip member %q: %w", member.Name, err)
		}
		err = addArchiveMember(members, member.Name, body)
		closeErr := body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close zip member %q: %w", member.Name, closeErr)
		}
	}
	return members, nil
}

// 目录同样需要校验；drive 前缀不随运行主机的平台放行。
func validateArchiveMemberPath(name string) error {
	hasDrive := len(name) >= 2 && name[1] == ':'
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "../") || name == ".." || strings.HasSuffix(name, "/..") || hasDrive {
		return fmt.Errorf("archive member path is unsafe: %q", name)
	}
	return nil
}

func addArchiveMember(members map[string]string, name string, body io.Reader) error {
	if _, duplicate := members[name]; duplicate {
		return fmt.Errorf("archive contains duplicate regular member %q", name)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, body); err != nil {
		return fmt.Errorf("hash archive member %q: %w", name, err)
	}
	members[name] = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

func writeNewJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence JSON: %w", err)
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".native-evidence-*.json")
	if err != nil {
		return fmt.Errorf("create evidence staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set evidence staging mode: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write evidence JSON: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close evidence JSON: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish evidence JSON: %w", err)
	}
	return nil
}
