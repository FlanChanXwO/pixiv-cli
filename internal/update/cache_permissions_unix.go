//go:build !windows

package update

import (
	"fmt"
	"os"
)

// ensurePrivateReleaseCacheDirectory 在写入前收紧已存在的用户 cache 目录权限。
func ensurePrivateReleaseCacheDirectory(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("create GitHub Releases cache directory %q: %w", cacheDir, err)
	}
	if err := os.Chmod(cacheDir, 0o700); err != nil {
		return fmt.Errorf("set GitHub Releases cache directory permissions to 0700 %q: %w", cacheDir, err)
	}
	return nil
}
