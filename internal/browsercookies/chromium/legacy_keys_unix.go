//go:build darwin || linux

package chromium

import (
	"crypto/aes"
	"crypto/pbkdf2"
	"crypto/sha1"
)

// chromiumKeyCandidates 返回已知 Chromium legacy key derivation 的有限候选。
// 每个候选都来自明确的 OS/profile 格式，而不是重试策略：GCM/CBC 认证失败
// 最终仍返回 ErrEncryptedMalformed。
func chromiumKeyCandidates(password []byte) ([][]byte, error) {
	var keys [][]byte
	for _, format := range []struct {
		salt       string
		iterations int
	}{{"saltysalt", 1003}, {"peanuts", 1}} {
		key, err := pbkdf2.Key(sha1.New, string(password), []byte(format.salt), format.iterations, aes.BlockSize)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}
