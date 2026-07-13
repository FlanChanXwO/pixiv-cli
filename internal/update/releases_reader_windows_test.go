//go:build windows

package update_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

// readCacheForConcurrentReader 在测试写入仍活跃时读取完整缓存。ReplaceFileW 的官方文档说明它
// 组合“旧文件改名、新文件取得目标名、删除旧文件”等多个步骤；真实 Windows runner 已在该命名
// 空间切换窗口观察到 ERROR_FILE_NOT_FOUND。仅此瞬时错误可重试，其他打开/读取错误一律原样
// 返回；stopReaders 关闭后立即停止重试，因此不会用任意次数或时长掩盖意外的持久错误。
//
// https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-replacefilew
func readCacheForConcurrentReader(path string, stopReaders <-chan struct{}) ([]byte, error) {
	for {
		cacheBytes, err := readCacheForConcurrentReaderOnce(path)
		if err == nil {
			return cacheBytes, nil
		}
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil, err
		}
		select {
		case <-stopReaders:
			return nil, err
		default:
			// 让替换 goroutine 推进到新目标名可见的步骤，而不引入猜测性的睡眠时长。
			runtime.Gosched()
		}
	}
}

// readCacheForConcurrentReaderOnce 显式允许删除共享。ReplaceFileW 需要该共享模式；普通
// os.ReadFile 在 Windows 仅共享读/写，会把本测试的读者错误地变成替换操作的写入障碍。
func readCacheForConcurrentReaderOnce(path string) ([]byte, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode cache path %q as UTF-16: %w", path, err)
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("wrap cache reader handle for %q", path)
	}
	defer file.Close()
	return io.ReadAll(file)
}
