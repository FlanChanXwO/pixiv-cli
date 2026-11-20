package vectormodel

import (
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

func loadNative(path string) (uintptr, error) {
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
	if err != nil {
		return 0, err
	}
	// 与 Unix 一样保留库句柄，防止已注册的函数地址失效。
	return windows.GetProcAddress(handle, "OrtGetApiBase")
}

func nativePath(path string) (unsafe.Pointer, func(), error) {
	encoded, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, nil, err
	}
	var pin runtime.Pinner
	pin.Pin(&encoded[0])
	return unsafe.Pointer(&encoded[0]), pin.Unpin, nil
}
