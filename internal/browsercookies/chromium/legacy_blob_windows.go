//go:build windows

package chromium

// Windows legacy encrypted_value 是 DPAPI blob，没有 AES block-alignment 或
// v10/v11 前缀可供公共层判断；非空 blob 必须交给 CryptUnprotectData 分类。
func legacyBlobSupported(blob []byte) bool { return len(blob) > 0 }
