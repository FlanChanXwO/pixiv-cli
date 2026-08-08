// Package browsercookies 提供从已安装浏览器读取受限 cookie 的 provider 入口。
//
// 它是协议无关的：不知道 FANBOX/Pixiv/CLI/MCP/账号 store；只读取调用方请求的
// 受约束 CookieQuery，产出脱敏 Secret。导入本包即注册全部 provider 子包
// （chrome、edge、firefox、safari）。本包不识别 Cloudflare challenge，
// 也不运行任何 challenge 绕过；它只读取 cookie。
package browsercookies

import (
	// 导入 provider 子包触发 init 注册，使 core.New 可分发全部浏览器。
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/chromium"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/firefox"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/safari"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

// DefaultQuery 是唯一允许 FANBOX adapter 使用的 cookie query。
var DefaultQuery = core.DefaultQuery

// 类型别名：与 core 保持同一标识。
type (
	Secret      = core.Secret
	CookieQuery = core.CookieQuery
	Snapshot    = core.Snapshot
	Cookie      = core.Cookie
	Profile     = core.Profile
	Provider    = core.Provider
)

// 错误分类（与 core 同一标识，errors.Is 可直接匹配）。
var (
	ErrUnknownBrowser             = core.ErrUnknownBrowser
	ErrQueryInvalid               = core.ErrQueryInvalid
	ErrProfileNotFound            = core.ErrProfileNotFound
	ErrMultipleProfiles           = core.ErrMultipleProfiles
	ErrInvalidProfileID           = core.ErrInvalidProfileID
	ErrDatabaseLocked             = core.ErrDatabaseLocked
	ErrDatabaseNotFound           = core.ErrDatabaseNotFound
	ErrEncryptedValueUnsupported  = core.ErrEncryptedValueUnsupported
	ErrEncryptedCookieUnsupported = core.ErrEncryptedCookieUnsupported
	ErrEncryptedFormatUnknown     = core.ErrEncryptedFormatUnknown
	ErrEncryptedMalformed         = core.ErrEncryptedMalformed
	ErrNotInstalled               = core.ErrNotInstalled
	ErrSQLiteUnavailable          = core.ErrSQLiteUnavailable
	ErrQueryFailed                = core.ErrQueryFailed
	ErrInvalidFormat              = core.ErrInvalidFormat
	ErrKeychainAccess             = core.ErrKeychainAccess
	ErrKeychainItemNotFound       = core.ErrKeychainItemNotFound
	ErrPermissionDenied           = core.ErrPermissionDenied
	ErrSecretServiceUnavailable   = core.ErrSecretServiceUnavailable
	ErrSecretServiceAccess        = core.ErrSecretServiceAccess
	ErrDPAPI                      = core.ErrDPAPI
)

// New 按浏览器名称创建 provider：支持 "chrome"、"edge"、
// "firefox"、"safari"；未知名称返回 ErrUnknownBrowser。
func New(browser string) (Provider, error) { return core.New(browser) }

// SelectProfile 按稳定 identifier 选择 profile；未指定且存在多个 profile 时
// 返回 ErrMultipleProfiles，错误只列出安全 identifier。
func SelectProfile(profiles []Profile, profileID string) (Profile, error) {
	return core.SelectProfile(profiles, profileID)
}

// SetProviderFixtureForTest 仅为测试拦截指定 provider 的数据库读取，使测试注入
// fixture SQLite 而不触碰真实浏览器；调用方必须用 t.Cleanup 执行返回的恢复函数。
func SetProviderFixtureForTest(providerName string, read func(string) ([]byte, error)) func() {
	return core.SetProviderFixtureForTest(providerName, read)
}
