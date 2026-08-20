//go:build !darwin

package secret

import "context"

// Keychain 在非 darwin 构建上不可用。
type Keychain struct{}

// GetPassword 返回 ErrNotAvailableOnBuild。
func (Keychain) GetPassword(ctx context.Context, service, account string) ([]byte, error) {
	return nil, ErrNotAvailableOnBuild
}
