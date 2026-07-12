//go:build !windows

package update_test

import "os"

// readCacheForConcurrentReader 在非 Windows 平台用普通打开语义读取缓存。
func readCacheForConcurrentReader(path string) ([]byte, error) {
	return os.ReadFile(path)
}
