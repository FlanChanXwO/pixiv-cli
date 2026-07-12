//go:build windows

package update_test

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// readCacheForConcurrentReader 显式允许删除共享。ReplaceFileW 需要该共享模式；普通
// os.ReadFile 在 Windows 仅共享读/写，会把本测试的读者错误地变成替换操作的写入障碍。
func readCacheForConcurrentReader(path string) ([]byte, error) {
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
