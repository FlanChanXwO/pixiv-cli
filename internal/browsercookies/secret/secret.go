// Package secret 提供平台秘密后端。
//
// 每个平台只暴露自己的系统凭据边界：darwin 使用 Keychain，windows 使用 DPAPI，
// linux 使用 Secret Service。后端不接收用户提供的命令或路径，命令输出只在进程
// 内存中短暂存在，错误不会携带秘密内容。
//
// 包的组成：darwin.go、linux.go、windows.go 各提供一个平台实现，secret.go 声明
// 共用的错误分类。
package secret

import "errors"

// ErrNotAvailableOnBuild 等 sentinel 是平台秘密后端的稳定错误分类；消息是静态文本，
// 不包含密码、命令输出或路径。
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
