package chromium

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
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
