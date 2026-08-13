//go:build windows

package chromium

import (
	"bytes"
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/secret"
)

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	if p.encryptionKeyOverride != nil {
		return p.encryptionKeyOverride(ctx)
	}
	localStateKey, present, err := p.localStateEncryptedKey()
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, browsercookies.ErrEncryptedValueUnsupported
	}
	if bytes.HasPrefix(localStateKey, []byte("DPAPI")) {
		localStateKey = localStateKey[len("DPAPI"):]
	}
	key, err := (secret.DPAPI{}).Unprotect(ctx, localStateKey)
	if err != nil {
		return nil, mapDPAPIError(err)
	}
	return [][]byte{key}, nil
}

func (p *provider) decryptLegacy(ctx context.Context, blob []byte) ([]byte, error) {
	value, err := (secret.DPAPI{}).Unprotect(ctx, blob)
	if err != nil {
		return nil, mapDPAPIError(err)
	}
	return value, nil
}

func mapDPAPIError(err error) error {
	if errors.Is(err, secret.ErrInvalidBlob) || errors.Is(err, secret.ErrDPAPI) {
		return browsercookies.ErrDPAPI
	}
	return err
}
