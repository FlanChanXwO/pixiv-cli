//go:build windows

package secret

import "context"

// DPAPI 提供 Windows DPAPI 解密入口。当前构建不可用；原生 CI 将实现真实解密。
type DPAPI struct{}

// Unprotect 返回 ErrNotAvailableOnBuild。
func (DPAPI) Unprotect(ctx context.Context, blob []byte) ([]byte, error) {
	return nil, ErrNotAvailableOnBuild
}
