package chromium

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// decryptEncrypted 是所有 Chromium provider 共用的解密入口。平台文件只负责
// 取得受 OS 保护的候选 key 或解包旧版平台 blob；格式判断、GCM/CBC 处理和
// 错误分类集中在这里，避免不同平台出现不同的安全边界。
func (p *provider) decryptEncrypted(ctx context.Context, blob []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !hasChromiumPrefix(blob) {
		if !legacyBlobSupported(blob) {
			return nil, browsercookies.ErrEncryptedFormatUnknown
		}
		return p.decryptLegacy(ctx, blob)
	}
	keys, err := p.encryptionKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if value, err := decryptChromiumValue(blob, key); err == nil {
			return value, nil
		}
	}
	return nil, browsercookies.ErrEncryptedMalformed
}
