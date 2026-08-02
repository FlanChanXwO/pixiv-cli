//go:build linux

package secret

import "context"

// SecretService 提供 Linux secret-service 解密入口。当前构建不可用；
// 原生 CI 将实现真实解密。
type SecretService struct{}

// Decrypt 返回 ErrNotAvailableOnBuild。
func (SecretService) Decrypt(ctx context.Context, blob []byte) ([]byte, error) {
	return nil, ErrNotAvailableOnBuild
}
