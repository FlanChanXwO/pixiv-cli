//go:build darwin

package chromium

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
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
	password, err := Keychain{}.GetPassword(ctx, p.kind.keychainService(), p.kind.keychainAccount())
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrItemNotFound) {
			return nil, browsercookies.ErrKeychainItemNotFound
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
	legacyKeys, err := chromiumKeyCandidates(password)
	if err != nil {
		return nil, err
	}
	return p.profileKeys(legacyKeys)
}
