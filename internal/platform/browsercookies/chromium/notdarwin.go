//go:build !darwin

package chromium

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
)

// decryptEncrypted 在非 darwin 平台上把加密 cookie value 标记为不支持。
// 本包不尝试 DPAPI 或 secret-service；真实解密由原生 CI 工程负责。
func (p *provider) decryptEncrypted(ctx context.Context, blob []byte) ([]byte, error) {
	return nil, core.ErrEncryptedValueUnsupported
}
