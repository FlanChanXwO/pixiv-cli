package chromium

import (
	"bytes"
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

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

const (
	// chromiumPrefixLength 是真实 encrypted_value 的外层版本标记长度。
	chromiumPrefixLength = 3
	chromiumNonceLength  = 12
)

// decryptChromiumGCM 解密 v10/v11 AES-GCM encrypted_value。版本标记之后是
// 12 字节 nonce 与带 16 字节 authentication tag 的 ciphertext。
func decryptChromiumGCM(encrypted, key []byte) ([]byte, error) {
	if len(encrypted) < chromiumPrefixLength+chromiumNonceLength+16 || !hasChromiumPrefix(encrypted) {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	nonceStart := chromiumPrefixLength
	nonceEnd := nonceStart + chromiumNonceLength
	plain, err := gcm.Open(nil, encrypted[nonceStart:nonceEnd], encrypted[nonceEnd:], nil)
	if err != nil {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	return plain, nil
}

// decryptChromiumCBC 解密旧版真实 encrypted_value：v10/v11 在密文外层，
// 密文采用 AES-CBC、空格 IV，并使用标准 PKCS#7 padding。
func decryptChromiumCBC(encrypted, key []byte) ([]byte, error) {
	if len(encrypted) < chromiumPrefixLength+aes.BlockSize || !hasChromiumPrefix(encrypted) {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	ciphertext := encrypted[chromiumPrefixLength:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	block, err := aes.NewCipher(key[:min(len(key), 32)])
	if err != nil {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	plain := make([]byte, len(ciphertext))
	iv := bytes.Repeat([]byte{' '}, aes.BlockSize)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	return unpadCBC(plain)
}

func unpadCBC(plain []byte) ([]byte, error) {
	if len(plain) == 0 || len(plain)%aes.BlockSize != 0 {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return nil, browsercookies.ErrEncryptedMalformed
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return nil, browsercookies.ErrEncryptedMalformed
		}
	}
	return plain[:len(plain)-padding], nil
}

// stripChromiumHostDigest 去除现代 Chromium Cookies 数据库在加密 cookie
// 明文前加入的 SHA-256(host_key) 摘要。旧数据库没有该摘要，保持原值。
func stripChromiumHostDigest(plain []byte, host string) []byte {
	if len(plain) < sha256.Size {
		return plain
	}
	expected := sha256.Sum256([]byte(host))
	if !bytes.Equal(plain[:sha256.Size], expected[:]) {
		return plain
	}
	return plain[sha256.Size:]
}

// decryptChromiumValue 尝试同一 profile 支持的真实 Chromium formats。GCM
// 认证失败后再检查 legacy CBC 是格式兼容，不是网络或 provider fallback。
func decryptChromiumValue(encrypted, key []byte) ([]byte, error) {
	if !hasChromiumPrefix(encrypted) {
		return nil, browsercookies.ErrEncryptedFormatUnknown
	}
	if plain, err := decryptChromiumGCM(encrypted, key); err == nil {
		return plain, nil
	}
	return decryptChromiumCBC(encrypted, key)
}

// hasChromiumPrefix 报告 encrypted_value 是否以 v10/v11 开头。
func hasChromiumPrefix(plain []byte) bool {
	return len(plain) >= chromiumPrefixLength && (string(plain[:chromiumPrefixLength]) == "v10" || string(plain[:chromiumPrefixLength]) == "v11")
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
			return nil, true, browsercookies.ErrPermissionDenied
		}
		return nil, true, browsercookies.ErrEncryptedFormatUnknown
	}
	var state localStateFile
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, true, browsercookies.ErrEncryptedFormatUnknown
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
		return nil, true, browsercookies.ErrEncryptedFormatUnknown
	}
	return key, true, nil
}
