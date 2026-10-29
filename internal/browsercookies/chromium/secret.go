package chromium

import "errors"

// ErrNotAvailableOnBuild 等 sentinel 是 secret_*.go 平台秘密后端的稳定错误分类；
// 消息是静态文本，不包含密码、命令输出或路径。
var (
	ErrNotAvailableOnBuild = errors.New("browsercookies/chromium: not available on this build")
	ErrItemNotFound        = errors.New("browsercookies/chromium: keychain item not found")
	ErrKeychainCommand     = errors.New("browsercookies/chromium: keychain command failed")
	ErrEmptyPassword       = errors.New("browsercookies/chromium: keychain item has an empty password")
	ErrInvalidItem         = errors.New("browsercookies/chromium: invalid keychain item name")
	ErrInvalidBlob         = errors.New("browsercookies/chromium: invalid encrypted blob")
	ErrDPAPI               = errors.New("browsercookies/chromium: DPAPI unprotect failed")
	ErrSecretService       = errors.New("browsercookies/chromium: Secret Service lookup failed")
)
