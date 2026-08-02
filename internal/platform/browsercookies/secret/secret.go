// Package secret 提供平台秘密后端。
//
// darwin 通过 `security` 命令读取 Keychain generic password item，输出被严格
// 解析并在错误时脱敏；windows DPAPI 与 linux secret-service 只保留编译期结构
// 与 "not available on this build" 分类错误，由原生 CI 工程实现真实解密。
package secret

import "errors"

var (
	ErrNotAvailableOnBuild = errors.New("browsercookies/secret: not available on this build")
	ErrItemNotFound        = errors.New("browsercookies/secret: keychain item not found")
	ErrKeychainCommand     = errors.New("browsercookies/secret: keychain command failed")
	ErrEmptyPassword       = errors.New("browsercookies/secret: keychain item has an empty password")
	ErrInvalidItem         = errors.New("browsercookies/secret: invalid keychain item name")
)
