//go:build linux

package chromium

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

func (k kind) secretApplication() string {
	if k == kindEdge {
		return "microsoft-edge"
	}
	return "chrome"
}

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	password, err := (SecretService{}).GetPassword(ctx, p.kind.secretApplication())
	if err != nil {
		return nil, mapSecretError(err)
	}
	legacyKeys, err := chromiumKeyCandidates(password)
	if err != nil {
		return nil, err
	}
	return p.profileKeys(legacyKeys)
}

func mapSecretError(err error) error {
	switch {
	case errors.Is(err, ErrNotAvailableOnBuild):
		return browsercookies.ErrSecretServiceUnavailable
	case errors.Is(err, ErrItemNotFound):
		return browsercookies.ErrSecretServiceAccess
	case errors.Is(err, ErrEmptyPassword), errors.Is(err, ErrInvalidItem), errors.Is(err, ErrSecretService):
		return browsercookies.ErrSecretServiceAccess
	default:
		return err
	}
}
