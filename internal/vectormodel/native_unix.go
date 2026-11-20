//go:build darwin || linux

package vectormodel

import (
	"github.com/ebitengine/purego"
	"runtime"
	"unsafe"
)

func loadNative(path string) (uintptr, error) {
	// 不卸载运行库：purego 注册的函数地址在进程生命周期内有效。
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return 0, err
	}
	return purego.Dlsym(handle, "OrtGetApiBase")
}

func nativePath(path string) (unsafe.Pointer, func(), error) {
	encoded := append([]byte(path), 0)
	var pin runtime.Pinner
	pin.Pin(&encoded[0])
	return unsafe.Pointer(&encoded[0]), pin.Unpin, nil
}
