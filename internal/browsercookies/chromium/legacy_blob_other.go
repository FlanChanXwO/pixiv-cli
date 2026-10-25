//go:build !windows

package chromium

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// 非 Windows 的受支持格式都在密文外层携带 v10/v11 标记。
func legacyBlobSupported([]byte) bool { return false }

func (p *provider) decryptLegacy(context.Context, []byte) ([]byte, error) {
	return nil, browsercookies.ErrEncryptedFormatUnknown
}
