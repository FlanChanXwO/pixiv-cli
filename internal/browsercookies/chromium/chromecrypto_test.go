package chromium

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

// TestDeriveChromeKeyVectors 用独立实现（python hashlib）预计算的已知向量
// 验证 deriveChromeKey，不调用 `security` 命令。
func TestDeriveChromeKeyVectors(t *testing.T) {
	vectors := []struct{ password, wantHex string }{
		{"test-password", "d25978bd65affb5353a83a363102e57311db9de1b51f02441bff7f86a41cc93e"},
		{"hunter2", "b85e8ec4afbc1ecbb974a5a584e24c40d1109494f0236b4672e25e6a65334764"},
		{"", "af35c5650cdb234c7ae16fb90bf3c68c08668ab21865062c6c7a9da2b26598cf"},
	}
	for _, v := range vectors {
		got := hex.EncodeToString(deriveChromeKey([]byte(v.password)))
		if got != v.wantHex {
			t.Fatalf("deriveChromeKey(%q) = %s, want %s", v.password, got, v.wantHex)
		}
	}
}

// buildV10Blob 按本包规范构造加密 blob：明文 = "v10" + 29 个零字节（32 字节
// 前缀）+ value，AES-128-CBC 加密，IV = 16 个零字节，无 padding。
func buildV10Blob(t *testing.T, key, value []byte) []byte {
	t.Helper()
	prefix := make([]byte, prefixLength)
	copy(prefix, "v10")
	plain := append(prefix, value...)
	if len(plain)%aes.BlockSize != 0 {
		t.Fatalf("plaintext not block-aligned: len=%d", len(plain))
	}
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(blob, plain)
	return blob
}

func buildModernV10CBCBlob(t *testing.T, key []byte, host string, value []byte) []byte {
	t.Helper()
	hostDigest := sha256.Sum256([]byte(host))
	plain := append(append([]byte(nil), hostDigest[:]...), value...)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, bytes.Repeat([]byte{' '}, aes.BlockSize)).CryptBlocks(ciphertext, plain)
	return append([]byte("v10"), ciphertext...)
}

func TestDecryptCookieValueRoundtrip(t *testing.T) {
	key := deriveChromeKey([]byte("test-password"))
	value := []byte("fanbox-session12") // 16 字节，保证 32+16 对齐
	blob := buildV10Blob(t, key, value)

	plain, err := decryptCookieValue(blob, key)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChromiumPrefix(plain) {
		t.Fatal("expected v10 prefix in decrypted output")
	}
	got, err := stripChromiumPrefix(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("decrypted value = %q, want %q", got, value)
	}
}

func TestDecryptChromiumGCM(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := []byte("0123456789ab")
	ciphertext := gcm.Seal(nil, nonce, []byte("gcm-session"), nil)
	blob := append([]byte("v10"), nonce...)
	blob = append(blob, ciphertext...)

	got, err := decryptChromiumValue(blob, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "gcm-session" {
		t.Fatalf("decrypted value = %q, want gcm-session", got)
	}
}

func TestDecryptChromiumCBCRoundtrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	value := []byte("cbc-session")
	padding := aes.BlockSize - len(value)%aes.BlockSize
	plain := append(append([]byte(nil), value...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, bytes.Repeat([]byte{' '}, aes.BlockSize)).CryptBlocks(ciphertext, plain)
	blob := append([]byte("v10"), ciphertext...)

	got, err := decryptChromiumValue(blob, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(value) {
		t.Fatalf("decrypted value = %q, want %q", got, value)
	}
}

func TestUnpadCBCRejectsInvalidPadding(t *testing.T) {
	cases := [][]byte{
		{1, 2, 3},
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0},
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 2},
	}
	for _, plain := range cases {
		if _, err := unpadCBC(plain); !errors.Is(err, core.ErrEncryptedMalformed) {
			t.Fatalf("unpadCBC(len=%d) err = %v, want ErrEncryptedMalformed", len(plain), err)
		}
	}
}

func TestDecryptCookieValueMalformed(t *testing.T) {
	key := deriveChromeKey([]byte("test-password"))
	cases := [][]byte{
		nil,
		{},
		{1, 2, 3}, // 非 16 对齐
		append([]byte("v10"), make([]byte, 13)...), // 不足 32 字节
	}
	for _, blob := range cases {
		if _, err := decryptCookieValue(blob, key); err == nil {
			t.Fatalf("decryptCookieValue(len=%d) = nil, want error", len(blob))
		}
	}
	// 短 key。
	if _, err := decryptCookieValue(make([]byte, 32), []byte("short")); !errors.Is(err, core.ErrEncryptedMalformed) {
		t.Fatalf("short key err = %v", err)
	}
}

func TestStripChromiumPrefixUnknownFormat(t *testing.T) {
	if _, err := stripChromiumPrefix(make([]byte, 32)); !errors.Is(err, core.ErrEncryptedFormatUnknown) {
		t.Fatalf("err = %v, want ErrEncryptedFormatUnknown", err)
	}
	if _, err := stripChromiumPrefix([]byte("v11")); !errors.Is(err, core.ErrEncryptedMalformed) {
		t.Fatalf("short prefix err = %v, want ErrEncryptedMalformed", err)
	}
}
