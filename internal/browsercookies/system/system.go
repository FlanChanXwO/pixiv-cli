// Package system 提供浏览器 cookie 的系统/browser integration 入口。
//
// 导入本包即注册全部 provider 子包（chrome、edge、firefox、safari），
// 使 protocol-neutral root core 的 New 可分发所有浏览器。本包只做集成与
// re-export：共享类型、错误与核心逻辑定义在 internal/browsercookies（root）。
package system

import (
	// 导入 provider 子包触发 init 注册，使 New 可分发全部浏览器。
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/chromium"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/firefox"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/safari"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// DefaultQuery 是唯一允许 FANBOX adapter 使用的 cookie query。
var DefaultQuery = browsercookies.DefaultQuery

// 类型别名：与 root core 保持同一标识。
type (
	Secret      = browsercookies.Secret
	CookieQuery = browsercookies.CookieQuery
	Snapshot    = browsercookies.Snapshot
	Cookie      = browsercookies.Cookie
	Profile     = browsercookies.Profile
	Provider    = browsercookies.Provider
)

// 错误分类（与 root core 同一标识，errors.Is 可直接匹配）。
var (
	ErrUnknownBrowser             = browsercookies.ErrUnknownBrowser
	ErrQueryInvalid               = browsercookies.ErrQueryInvalid
	ErrProfileNotFound            = browsercookies.ErrProfileNotFound
	ErrMultipleProfiles           = browsercookies.ErrMultipleProfiles
	ErrInvalidProfileID           = browsercookies.ErrInvalidProfileID
	ErrDatabaseLocked             = browsercookies.ErrDatabaseLocked
	ErrDatabaseNotFound           = browsercookies.ErrDatabaseNotFound
	ErrEncryptedValueUnsupported  = browsercookies.ErrEncryptedValueUnsupported
	ErrEncryptedCookieUnsupported = browsercookies.ErrEncryptedCookieUnsupported
	ErrEncryptedFormatUnknown     = browsercookies.ErrEncryptedFormatUnknown
	ErrEncryptedMalformed         = browsercookies.ErrEncryptedMalformed
	ErrNotInstalled               = browsercookies.ErrNotInstalled
	ErrSQLiteUnavailable          = browsercookies.ErrSQLiteUnavailable
	ErrQueryFailed                = browsercookies.ErrQueryFailed
	ErrInvalidFormat              = browsercookies.ErrInvalidFormat
	ErrKeychainAccess             = browsercookies.ErrKeychainAccess
	ErrKeychainItemNotFound       = browsercookies.ErrKeychainItemNotFound
	ErrPermissionDenied           = browsercookies.ErrPermissionDenied
	ErrSecretServiceUnavailable   = browsercookies.ErrSecretServiceUnavailable
	ErrSecretServiceAccess        = browsercookies.ErrSecretServiceAccess
	ErrDPAPI                      = browsercookies.ErrDPAPI
)

// New 按 browser 名称创建 provider：支持 "chrome"、"edge"、
// "firefox"、"safari"；未知名称返回 ErrUnknownBrowser。本包导入已触发
// provider 注册，root core 因此可分发全部浏览器。
func New(browser string) (Provider, error) { return browsercookies.New(browser) }

// SelectProfile 按稳定 identifier 选择 profile；未指定且存在多个 profile 时
// 返回 ErrMultipleProfiles，错误只列出安全 identifier。
func SelectProfile(profiles []Profile, profileID string) (Profile, error) {
	return browsercookies.SelectProfile(profiles, profileID)
}

// SetProviderFixtureForTest 仅为测试拦截指定 provider 的数据库读取，使测试注入
// fixture SQLite 而不触碰真实浏览器；调用方必须用 t.Cleanup 执行返回的恢复函数。
func SetProviderFixtureForTest(providerName string, read func(string) ([]byte, error)) func() {
	return browsercookies.SetProviderFixtureForTest(providerName, read)
}
