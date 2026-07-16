package main

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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func verifyArchive(repoRoot, archive, binaryName, binaryDigest string) ([]evidenceFile, error) {
	expected, err := expectedArchiveMembers(repoRoot, binaryName, binaryDigest)
	if err != nil {
		return nil, err
	}
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

func expectedArchiveMembers(repoRoot, binaryName, binaryDigest string) (map[string]string, error) {
	expected := map[string]string{binaryName: binaryDigest}
	for _, relative := range []string{"LICENSE", "THIRD_PARTY_LICENSES.md"} {
		digest, err := fileSHA256(filepath.Join(repoRoot, relative))
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", relative, err)
		}
		expected[relative] = digest
	}
	licenses := filepath.Join(repoRoot, "third_party", "licenses")
	err := filepath.WalkDir(licenses, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("license tree contains a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("license tree contains a non-regular file: %s", path)
		}
		relative, err := filepath.Rel(licenses, path)
		if err != nil {
			return err
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return err
		}
		expected[filepath.ToSlash(filepath.Join("third_party", "licenses", relative))] = digest
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read complete license tree: %w", err)
	}
	if len(expected) == 3 {
		return nil, errors.New("license tree contains no regular files")
	}
	return expected, nil
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

func addArchiveMember(members map[string]string, name string, body io.Reader) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "../") || name == ".." {
		return fmt.Errorf("archive member path is unsafe: %q", name)
	}
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
