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

// keychainPassword 读取浏览器 Safe Storage 密码。
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
