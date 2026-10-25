// Chromium 密文格式识别与 Local State 密钥解包；平台文件只读取系统秘密后端。

package chromium

import (
	"bytes"
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

func (p *provider) profileKeys(legacyKeys [][]byte) ([][]byte, error) {
	localStateKey, present, err := p.localStateEncryptedKey()
	if err != nil {
		return nil, err
	}
	if !present {
		return legacyKeys, nil
	}
	if hasChromiumPrefix(localStateKey) {
		for _, key := range legacyKeys {
			if unwrapped, err := decryptChromiumGCM(localStateKey, key); err == nil && len(unwrapped) > 0 {
				return [][]byte{unwrapped}, nil
			}
		}
		return nil, browsercookies.ErrEncryptedMalformed
	}
	if bytes.HasPrefix(localStateKey, []byte("DPAPI")) {
		return nil, browsercookies.ErrEncryptedFormatUnknown
	}
	return [][]byte{localStateKey}, nil
}

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
