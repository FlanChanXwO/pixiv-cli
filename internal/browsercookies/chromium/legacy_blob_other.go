//go:build !windows

package chromium

import "crypto/aes"

// macOS/Linux legacy cookie value 是项目支持的 v10/v11 明文前缀 + AES-CBC
// 结构；先检查其客观布局，避免明显短输入触发平台 secret backend。
func legacyBlobSupported(blob []byte) bool {
	return len(blob) >= prefixLength && len(blob)%aes.BlockSize == 0
}
