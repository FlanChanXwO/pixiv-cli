package chromium

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
)

// peanuts 是 Chrome key derivation 的固定盐。
var peanuts = []byte("peanuts")

// prefixLength 是解密结果中 v10/v11 前缀的长度。
const prefixLength = 32

// deriveChromeKey 实现 Chrome 的 key derivation：
// key0 = password；每次迭代 key_{i+1} = SHA256(key_i || "peanuts")，共 1000 次。
// 纯函数，测试无需调用 `security` 命令。
func deriveChromeKey(password []byte) []byte {
	key := append([]byte(nil), password...)
	for i := 0; i < 1000; i++ {
		h := sha256.New()
		h.Write(key)
		h.Write(peanuts)
		key = h.Sum(nil)
	}
	return key
}

// decryptCookieValue 按本包规范解密 Chromium cookie value：
// AES-128-CBC，key 取 deriveChromeKey 结果的前 16 字节，IV = 16 个零字节，
// 无 padding 期望。返回完整解密结果。
func decryptCookieValue(encrypted, key []byte) ([]byte, error) {
	if len(key) < aes.BlockSize {
		return nil, core.ErrEncryptedMalformed
	}
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, core.ErrEncryptedMalformed
	}
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		return nil, core.ErrEncryptedMalformed
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, encrypted)
	if len(plain) < prefixLength {
		return nil, core.ErrEncryptedMalformed
	}
	return plain, nil
}

// hasChromiumPrefix 报告解密结果是否以 v10/v11 前缀开头。
func hasChromiumPrefix(plain []byte) bool {
	return len(plain) >= 3 && (string(plain[:3]) == "v10" || string(plain[:3]) == "v11")
}

// stripChromiumPrefix 校验 v10/v11 前缀并返回前缀之后的 cookie value。
// 按规范：解密结果前 32 字节为 v10/v11 前缀（保留或跳过均可），目标值位于
// decrypted[32 : 32+length]，length 为剩余字节数（无 padding 期望）。
func stripChromiumPrefix(plain []byte) ([]byte, error) {
	if !hasChromiumPrefix(plain) {
		return nil, core.ErrEncryptedFormatUnknown
	}
	if len(plain) < prefixLength {
		return nil, core.ErrEncryptedMalformed
	}
	return plain[prefixLength:], nil
}
