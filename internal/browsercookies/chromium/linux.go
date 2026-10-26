//go:build linux

package chromium

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/secret"
)

func (k kind) secretApplication() string {
	if k == kindEdge {
		return "microsoft-edge"
	}
	return "chrome"
}

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	password, err := (secret.SecretService{}).GetPassword(ctx, p.kind.secretApplication())
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
	case errors.Is(err, secret.ErrNotAvailableOnBuild):
		return browsercookies.ErrSecretServiceUnavailable
	case errors.Is(err, secret.ErrItemNotFound):
		return browsercookies.ErrSecretServiceAccess
	case errors.Is(err, secret.ErrEmptyPassword), errors.Is(err, secret.ErrInvalidItem), errors.Is(err, secret.ErrSecretService):
		return browsercookies.ErrSecretServiceAccess
	default:
		return err
	}
}
