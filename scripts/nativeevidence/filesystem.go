package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/downloader/staticlib"
)

func writeFreshFile(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func requireDirectory(path, label string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s must be a directory", label)
	}
	return filepath.Clean(path), nil
}

func requireSecureDirectory(path, label string) (string, error) {
	directory, err := requireDirectory(path, label)
	if err != nil {
		return "", err
	}
	for current := directory; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("inspect %s ancestor: %w", label, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s contains a symlink ancestor: %s", label, current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return directory, nil
}

func calculateSourceDigest(repoRoot string) (string, error) {
	digest, err := staticlib.CalculateRustSourceDigest(
		filepath.Join(repoRoot, "internal", "downloader", "ugoira_rs"),
		filepath.Join(repoRoot, "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		return "", fmt.Errorf("calculate Rust source digest: %w", err)
	}
	return digest, nil
}

func requireRegularFile(path, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a non-symlink regular file", label)
	}
	return nil
}

func requireNewOutput(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("output already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output: %w", err)
	}
	parent := filepath.Dir(path)
	if _, err := requireDirectory(parent, "output directory"); err != nil {
		return err
	}
	for directory := filepath.Clean(parent); ; directory = filepath.Dir(directory) {
		info, err := os.Lstat(directory)
		if err != nil {
			return fmt.Errorf("inspect output directory ancestor: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output directory contains a symlink ancestor: %s", directory)
		}
		parentDirectory := filepath.Dir(directory)
		if parentDirectory == directory {
			break
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	if err := requireRegularFile(path, "file"); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func findRepositoryRoot(t interface {
	Helper()
	Fatalf(string, ...any)
}) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("find repository root from %s", directory)
		}
		directory = parent
	}
}
