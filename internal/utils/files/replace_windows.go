//go:build windows

package files

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var replaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

// ReplaceFile 用源文件替换目标路径。临时源文件必须已完整写入并关闭；Windows 在目标存在时
// 必须使用 ReplaceFileW 这一单次替换原语，不能把 MoveFileEx(REPLACE_EXISTING) 当作等价物。
// ReplaceFileW 不支持 WRITE_THROUGH，故不能擅自传入该标志；同步由调用方在替换前完成。
func ReplaceFile(sourcePath, targetPath string) error {
	from, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(targetPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// 仅在目标确实不存在时创建；若并发创建则由 MoveFileEx 原样报告冲突。
		return windows.MoveFileEx(from, to, 0)
	}
	if err := replaceFileW.Find(); err != nil {
		return err
	}
	success, _, lastErr := replaceFileW.Call(
		uintptr(unsafe.Pointer(to)),
		uintptr(unsafe.Pointer(from)),
		0,
		0,
		0,
		0,
	)
	if success == 0 {
		// LazyProc.Call 已捕获此 API 调用的 GetLastError。
		return lastErr
	}
	return nil
}
