//go:build windows

package loginhelper

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const seeMaskClassName = 0x00000001

// shellExecuteInfo 保留 ShellExecuteExW 到 hProcess 的完整 ABI 布局。这里用
// class name 而非默认 protocol association，才能在 pixiv-cli 当前是默认 handler
// 时把非白名单 URL 定向交回先前的 handler。
type shellExecuteInfo struct {
	CBSize     uint32
	FMask      uint32
	HWND       uintptr
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	HInstApp   uintptr
	IDList     uintptr
	Class      *uint16
	HKeyClass  uintptr
	HotKey     uint32
	Icon       uintptr
	Process    uintptr
}

var shell32DLL = windows.NewLazySystemDLL("shell32.dll")
var shellExecuteExW = shell32DLL.NewProc("ShellExecuteExW")

func windowsShellOpenClass(ctx context.Context, className, rawURL string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	class, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(rawURL)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		CBSize: uint32(unsafe.Sizeof(shellExecuteInfo{})),
		FMask:  seeMaskClassName,
		File:   file,
		Class:  class,
		Show:   1,
	}
	r1, _, callErr := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
			return callErr
		}
		return errors.New("ShellExecuteExW failed")
	}
	return nil
}
