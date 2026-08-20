// Package secret 提供平台秘密后端。
//
// 每个平台只暴露自己的系统凭据边界：darwin 使用 Keychain，windows 使用
// DPAPI，linux 使用 Secret Service。后端不接收用户提供的命令或路径，且所有
// 命令输出只在进程内存中短暂存在，错误不会携带秘密内容。
package secret

import "errors"

var (
	ErrNotAvailableOnBuild = errors.New("browsercookies/secret: not available on this build")
	ErrItemNotFound        = errors.New("browsercookies/secret: keychain item not found")
	ErrKeychainCommand     = errors.New("browsercookies/secret: keychain command failed")
	ErrEmptyPassword       = errors.New("browsercookies/secret: keychain item has an empty password")
	ErrInvalidItem         = errors.New("browsercookies/secret: invalid keychain item name")
	ErrInvalidBlob         = errors.New("browsercookies/secret: invalid encrypted blob")
	ErrDPAPI               = errors.New("browsercookies/secret: DPAPI unprotect failed")
	ErrSecretService       = errors.New("browsercookies/secret: Secret Service lookup failed")
)
