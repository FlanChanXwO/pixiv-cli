//go:build windows

package chromium

import (
	"bytes"
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
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
	key, err := (DPAPI{}).Unprotect(ctx, localStateKey)
	if err != nil {
		return nil, mapDPAPIError(err)
	}
	return [][]byte{key}, nil
}

func (p *provider) decryptLegacy(ctx context.Context, blob []byte) ([]byte, error) {
	value, err := (DPAPI{}).Unprotect(ctx, blob)
	if err != nil {
		return nil, mapDPAPIError(err)
	}
	return value, nil
}

func mapDPAPIError(err error) error {
	if errors.Is(err, ErrInvalidBlob) || errors.Is(err, ErrDPAPI) {
		return browsercookies.ErrDPAPI
	}
	return err
}
