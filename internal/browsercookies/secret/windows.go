//go:build windows

package secret

import (
	"context"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI 提供 Windows 当前用户 DPAPI 解密入口。Chromium 的 Local State
// encrypted_key 与旧版 cookie blob 都由当前 Windows 用户上下文保护。
type DPAPI struct{}

// Unprotect 使用 CryptUnprotectData 解密 blob，并复制出由 Windows 分配的
// 内存后立即释放。DPAPI 调用本身不可异步取消，因此在进入和返回处检查
// context，而不凭空加入固定超时。
func (DPAPI) Unprotect(ctx context.Context, blob []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		return nil, ErrInvalidBlob
	}
	// Windows DataBlob.Size 是 uint32；这是 API 的客观边界，不是对 cookie
	// 内容施加的展示层长度限制。
	if uint64(len(blob)) > uint64(^uint32(0)) {
		return nil, ErrInvalidBlob
	}
	in := windows.DataBlob{Size: uint32(len(blob)), Data: &blob[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, ErrDPAPI
	}
	if out.Data == nil || out.Size == 0 {
		return nil, ErrDPAPI
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := unsafe.Slice(out.Data, int(out.Size))
	result := append([]byte(nil), plain...)
	runtime.KeepAlive(blob)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
