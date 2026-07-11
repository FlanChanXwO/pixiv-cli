//go:build windows

package update

import (
	"fmt"
	"os"
)

// Windows ACL 不等同于 POSIX mode，保留既有目录创建语义。
func ensurePrivateReleaseCacheDirectory(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("create GitHub Releases cache directory %q: %w", cacheDir, err)
	}
	return nil
}
