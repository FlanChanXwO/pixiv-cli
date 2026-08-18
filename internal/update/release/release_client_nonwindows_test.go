//go:build !windows

package release_test

import "os"

// readCacheForConcurrentReader 在非 Windows 平台用普通打开语义读取缓存。
// stopReaders 只统一 Windows/非 Windows 测试辅助函数的调用形状。
func readCacheForConcurrentReader(path string, _ <-chan struct{}) ([]byte, error) {
	return os.ReadFile(path)
}
