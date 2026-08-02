//go:build darwin

package chromium

import (
	"context"
	"crypto/aes"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/secret"
)

// keychainKey 读取浏览器 Safe Storage 密码并派生解密密钥。测试可通过
// keychainKeyOverride 注入，避免调用真实 `security` 命令。
func (p *provider) keychainKey(ctx context.Context) ([]byte, error) {
	if p.keychainKeyOverride != nil {
		return p.keychainKeyOverride(ctx)
	}
	password, err := secret.Keychain{}.GetPassword(ctx, p.kind.keychainService(), p.kind.keychainAccount())
	if err != nil {
		if errors.Is(err, secret.ErrItemNotFound) {
			return nil, core.ErrKeychainItemNotFound
		}
		if errors.Is(err, secret.ErrNotAvailableOnBuild) {
			return nil, core.ErrEncryptedValueUnsupported
		}
		return nil, core.ErrKeychainAccess
	}
	return deriveChromeKey(password), nil
}

// decryptEncrypted 解密 Chromium 加密 cookie value（macOS 路径）。
func (p *provider) decryptEncrypted(ctx context.Context, blob []byte) ([]byte, error) {
	// 先做无需 key 的格式检查（块对齐与最小长度），避免为明显损坏的 blob
	// 无谓调用 Keychain。
	if len(blob) < prefixLength || len(blob)%aes.BlockSize != 0 {
		return nil, core.ErrEncryptedFormatUnknown
	}
	key, err := p.keychainKey(ctx)
	if err != nil {
		return nil, err
	}
	plain, err := decryptCookieValue(blob, key)
	if err != nil {
		return nil, err
	}
	return stripChromiumPrefix(plain)
}
