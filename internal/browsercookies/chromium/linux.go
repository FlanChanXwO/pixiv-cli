//go:build linux

package chromium

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/secret"
)

func (k kind) secretApplication() string {
	if k == kindEdge {
		return "microsoft-edge"
	}
	return "chrome"
}

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	if p.encryptionKeyOverride != nil {
		return p.encryptionKeyOverride(ctx)
	}
	password, err := (secret.SecretService{}).GetPassword(ctx, p.kind.secretApplication())
	if err != nil {
		return nil, mapSecretError(err)
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
		return nil, core.ErrEncryptedMalformed
	}
	if len(localStateKey) >= 5 && string(localStateKey[:5]) == "DPAPI" {
		return nil, core.ErrEncryptedFormatUnknown
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
	return nil, core.ErrEncryptedMalformed
}

func mapSecretError(err error) error {
	switch {
	case errors.Is(err, secret.ErrNotAvailableOnBuild):
		return core.ErrSecretServiceUnavailable
	case errors.Is(err, secret.ErrItemNotFound):
		return core.ErrSecretServiceAccess
	case errors.Is(err, secret.ErrEmptyPassword), errors.Is(err, secret.ErrInvalidItem), errors.Is(err, secret.ErrSecretService):
		return core.ErrSecretServiceAccess
	default:
		return err
	}
}
