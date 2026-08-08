package chromium

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

// peanuts 是 Chromium Linux 旧版 key derivation 使用的固定盐；它也保留为
// fixture 兼容路径，不会出现在日志或错误中。
var peanuts = []byte("peanuts")

const (
	// prefixLength 是旧版 fixture 中解密结果的 v10/v11 前缀长度。
	prefixLength = 32
	// chromiumPrefixLength 是真实 encrypted_value 的外层版本标记长度。
	chromiumPrefixLength = 3
	chromiumNonceLength  = 12
)

// deriveChromeKey 保留项目既有的旧 fixture 向量。真实 Chromium profile 使用
// 的 PBKDF2 候选由 chromiumKeyCandidates 一并生成；这样旧 profile 与新 profile
// 都能得到明确的格式/解密错误，而不会静默换浏览器 provider。
func deriveChromeKey(password []byte) []byte {
	key := append([]byte(nil), password...)
	for range 1000 {
		h := sha256.New()
		_, _ = h.Write(key)
		_, _ = h.Write(peanuts)
		key = h.Sum(nil)
	}
	return key
}

// decryptCookieValue 解密项目保留的旧 fixture：AES-128-CBC，IV 为零，
// 明文自身包含 32 字节 v10/v11 前缀。真实 profile 则使用下面的
// decryptChromiumValue 处理外层版本标记。
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

// decryptChromiumGCM 解密 v10/v11 AES-GCM encrypted_value。版本标记之后是
// 12 字节 nonce 与带 16 字节 authentication tag 的 ciphertext。
func decryptChromiumGCM(encrypted, key []byte) ([]byte, error) {
	if len(encrypted) < chromiumPrefixLength+chromiumNonceLength+16 || !hasChromiumPrefix(encrypted) {
		return nil, core.ErrEncryptedMalformed
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, core.ErrEncryptedMalformed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, core.ErrEncryptedMalformed
	}
	nonceStart := chromiumPrefixLength
	nonceEnd := nonceStart + chromiumNonceLength
	plain, err := gcm.Open(nil, encrypted[nonceStart:nonceEnd], encrypted[nonceEnd:], nil)
	if err != nil {
		return nil, core.ErrEncryptedMalformed
	}
	return plain, nil
}

// decryptChromiumCBC 解密旧版真实 encrypted_value：v10/v11 在密文外层，
// 密文采用 AES-CBC、空格 IV，并使用标准 PKCS#7 padding。
func decryptChromiumCBC(encrypted, key []byte) ([]byte, error) {
	if len(encrypted) < chromiumPrefixLength+aes.BlockSize || !hasChromiumPrefix(encrypted) {
		return nil, core.ErrEncryptedMalformed
	}
	ciphertext := encrypted[chromiumPrefixLength:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, core.ErrEncryptedMalformed
	}
	block, err := aes.NewCipher(key[:min(len(key), 32)])
	if err != nil {
		return nil, core.ErrEncryptedMalformed
	}
	plain := make([]byte, len(ciphertext))
	iv := bytesRepeat(' ', aes.BlockSize)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	return unpadCBC(plain)
}

func bytesRepeat(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func unpadCBC(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, core.ErrEncryptedMalformed
	}
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return plain, nil
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return plain, nil
		}
	}
	return plain[:len(plain)-padding], nil
}

// decryptChromiumValue 尝试同一 profile 支持的真实 Chromium formats。GCM
// 认证失败后再检查 legacy CBC 是格式兼容，不是网络或 provider fallback。
func decryptChromiumValue(encrypted, key []byte) ([]byte, error) {
	if !hasChromiumPrefix(encrypted) {
		return nil, core.ErrEncryptedFormatUnknown
	}
	if plain, err := decryptChromiumGCM(encrypted, key); err == nil {
		return plain, nil
	}
	return decryptChromiumCBC(encrypted, key)
}

// hasChromiumPrefix 报告 encrypted_value 或旧 fixture 明文是否以 v10/v11 开头。
func hasChromiumPrefix(plain []byte) bool {
	return len(plain) >= chromiumPrefixLength && (string(plain[:chromiumPrefixLength]) == "v10" || string(plain[:chromiumPrefixLength]) == "v11")
}

// stripChromiumPrefix 校验旧 fixture 的 32 字节前缀并返回 cookie value。
func stripChromiumPrefix(plain []byte) ([]byte, error) {
	if !hasChromiumPrefix(plain) {
		return nil, core.ErrEncryptedFormatUnknown
	}
	if len(plain) < prefixLength {
		return nil, core.ErrEncryptedMalformed
	}
	return plain[prefixLength:], nil
}

type localStateFile struct {
	OSCrypt struct {
		EncryptedKey string `json:"encrypted_key"`
	} `json:"os_crypt"`
}

// localStateEncryptedKey 读取 Chromium Local State 中的 encrypted_key。文件不
// 存在表示旧版 profile，可使用平台 legacy key；文件存在但 JSON/值损坏则显露
// 格式错误，不把损坏状态伪装成未安装。
func (p *provider) localStateEncryptedKey() ([]byte, bool, error) {
	body, err := os.ReadFile(filepath.Join(p.root, "Local State"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		if errors.Is(err, fs.ErrPermission) {
			return nil, true, core.ErrPermissionDenied
		}
		return nil, true, core.ErrEncryptedFormatUnknown
	}
	var state localStateFile
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, true, core.ErrEncryptedFormatUnknown
	}
	encoded := strings.TrimSpace(state.OSCrypt.EncryptedKey)
	if encoded == "" {
		return nil, false, nil
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(key) == 0 {
		return nil, true, core.ErrEncryptedFormatUnknown
	}
	return key, true, nil
}
