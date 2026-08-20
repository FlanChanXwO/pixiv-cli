//go:build darwin || linux

package chromium

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
)

// derivePBKDF2SHA1 是 Chromium legacy CBC 需要的 PBKDF2-HMAC-SHA1；实现放在
// 标准库内，避免 browser provider 引入额外 crypto 依赖。
func derivePBKDF2SHA1(password, salt []byte, iterations, keyLength int) []byte {
	if iterations <= 0 || keyLength <= 0 {
		return nil
	}
	result := make([]byte, 0, keyLength)
	for blockNumber := uint32(1); len(result) < keyLength; blockNumber++ {
		mac := hmac.New(sha1.New, password)
		_, _ = mac.Write(salt)
		var counter [4]byte
		binary.BigEndian.PutUint32(counter[:], blockNumber)
		_, _ = mac.Write(counter[:])
		u := mac.Sum(nil)
		block := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha1.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := range block {
				block[j] ^= u[j]
			}
		}
		result = append(result, block...)
	}
	return result[:keyLength]
}

// chromiumKeyCandidates 返回已知 Chromium legacy key derivation 的有限候选。
// 每个候选都来自明确的 OS/profile 格式，而不是重试策略：GCM/CBC 认证失败
// 最终仍返回 ErrEncryptedMalformed。
func chromiumKeyCandidates(password []byte) [][]byte {
	candidates := [][]byte{
		deriveChromeKey(password),
		derivePBKDF2SHA1(password, []byte("saltysalt"), 1003, aes.BlockSize),
		derivePBKDF2SHA1(password, peanuts, 1, aes.BlockSize),
	}
	seen := make(map[string]struct{}, len(candidates))
	result := make([][]byte, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		key := string(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}
