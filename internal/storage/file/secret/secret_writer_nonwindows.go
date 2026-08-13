//go:build !windows

package secret

import "os"

func openSecretFileExclusive(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func applySecretFileProtection(file *os.File) error {
	// umask 可能进一步收紧创建 mode；最终契约要求精确恢复为 0600。
	return file.Chmod(0o600)
}

func applySecretPathProtection(string) error { return nil }
