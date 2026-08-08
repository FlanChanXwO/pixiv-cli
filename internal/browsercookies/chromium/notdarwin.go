//go:build !darwin && !linux && !windows

package chromium

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

func (p *provider) encryptionKeys(ctx context.Context) ([][]byte, error) {
	return nil, core.ErrEncryptedValueUnsupported
}

func (p *provider) decryptLegacy(ctx context.Context, blob []byte) ([]byte, error) {
	return nil, core.ErrEncryptedValueUnsupported
}
