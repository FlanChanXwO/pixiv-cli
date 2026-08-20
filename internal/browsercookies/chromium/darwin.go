//go:build darwin

package chromium

import (
	"bytes"
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/secret"
)

func (k kind) keychainService() string {
	if k == kindEdge {
		return "Microsoft Edge Safe Storage"
	}
	return "Chrome Safe Storage"
}

func (k kind) keychainAccount() string {
	if k == kindEdge {
		return "Microsoft Edge"
	}
	return "Chrome"
}

// keychainPassword 读取浏览器 Safe Storage 密码。测试可通过
// keychainKeyOverride 注入已经派生的 key，避免调用真实 `security` 命令。
func (p *provider) keychainPassword(ctx context.Context) ([]byte, error) {
	password, err := secret.Keychain{}.GetPassword(ctx, p.kind.keychainService(), p.kind.keychainAccount())
	if err != nil {
		if errors.Is(err, secret.ErrItemNotFound) {
			return nil, browsercookies.ErrKeychainItemNotFound
		}
		if errors.Is(err, secret.ErrNotAvailableOnBuild) {
			return nil, browsercookies.ErrEncryptedValueUnsupported
		}
		return nil, browsercookies.ErrKeychainAccess
	}
	return password, nil
}

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	if p.encryptionKeyOverride != nil {
		return p.encryptionKeyOverride(ctx)
	}
	if p.keychainKeyOverride != nil {
		key, err := p.keychainKeyOverride(ctx)
		if err != nil {
			return nil, err
		}
		return [][]byte{key}, nil
	}
	password, err := p.keychainPassword(ctx)
	if err != nil {
		return nil, err
	}
	legacyKeys := chromiumKeyCandidates(password)
	localStateKey, present, err := p.localStateEncryptedKey()
	if err != nil {
		return nil, err
	}
	if !present {
		return legacyKeys, nil
	}
	if len(localStateKey) >= chromiumPrefixLength && hasChromiumPrefix(localStateKey) {
		for _, key := range legacyKeys {
			if unwrapped, err := decryptChromiumGCM(localStateKey, key); err == nil && len(unwrapped) > 0 {
				return [][]byte{unwrapped}, nil
			}
		}
		return nil, browsercookies.ErrEncryptedMalformed
	}
	if bytes.HasPrefix(localStateKey, []byte("DPAPI")) {
		return nil, browsercookies.ErrEncryptedFormatUnknown
	}
	return [][]byte{localStateKey}, nil
}

func (p *provider) decryptLegacy(ctx context.Context, blob []byte) ([]byte, error) {
	keys, err := p.encryptionKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		plain, err := decryptCookieValue(blob, key)
		if err != nil {
			continue
		}
		if value, err := stripChromiumPrefix(plain); err == nil {
			return value, nil
		}
	}
	return nil, browsercookies.ErrEncryptedMalformed
}
