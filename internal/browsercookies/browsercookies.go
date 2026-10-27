// Package browsercookies 定义浏览器 cookie provider 的共享类型与接口。
//
// 本包只接受受约束的 [CookieQuery]，产出脱敏的 [credential.Secret]；它不识别
// Cloudflare challenge，也不运行 challenge 绕过，只读取 cookie。本包不导入 provider
// 子包，单独导入不会注册任何 provider；需要完整分发的调用方导入
// internal/browsercookies/system。
//
// 包的组成：[Provider] 是各浏览器实现的读取接口，[Profile] 描述可选择的 profile，
// [Register] 与 [New] 维护名称到 provider 工厂的注册表，[CookieQuery] 约束查询字段。
package browsercookies

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

// ErrUnknownBrowser 等 sentinel 是 provider 的稳定错误分类。所有错误消息都是静态
// 字符串，绝不包含 cookie 值、加密密钥、绝对路径或 profile 内容。
var (
	ErrUnknownBrowser             = errors.New("browsercookies: unknown browser")
	ErrQueryInvalid               = errors.New("browsercookies: invalid cookie query")
	ErrProfileNotFound            = errors.New("browsercookies: profile not found")
	ErrMultipleProfiles           = errors.New("browsercookies: multiple profiles match and no profile was specified")
	ErrInvalidProfileID           = errors.New("browsercookies: invalid profile identifier")
	ErrDatabaseLocked             = errors.New("browsercookies: browser cookie database is locked (browser may be running)")
	ErrDatabaseNotFound           = errors.New("browsercookies: browser cookie database not found")
	ErrEncryptedValueUnsupported  = errors.New("browsercookies: cookie value is encrypted and decryption is not supported on this platform")
	ErrEncryptedCookieUnsupported = errors.New("browsercookies: cookie value is encrypted and decryption is not supported by this provider")
	ErrEncryptedFormatUnknown     = errors.New("browsercookies: cookie value uses an unknown encryption format")
	ErrEncryptedMalformed         = errors.New("browsercookies: encrypted cookie value is malformed")
	ErrNotInstalled               = errors.New("browsercookies: browser is not installed")
	ErrSQLiteUnavailable          = errors.New("browsercookies: the sqlite3 command-line tool is required but was not found")
	ErrQueryFailed                = errors.New("browsercookies: cookie database query failed")
	ErrInvalidFormat              = errors.New("browsercookies: cookie storage file has an invalid format")
	ErrKeychainAccess             = errors.New("browsercookies: keychain access failed")
	ErrKeychainItemNotFound       = errors.New("browsercookies: keychain item not found")
	ErrPermissionDenied           = errors.New("browsercookies: browser storage permission denied")
	ErrSecretServiceUnavailable   = errors.New("browsercookies: Linux Secret Service is unavailable")
	ErrSecretServiceAccess        = errors.New("browsercookies: Linux Secret Service access failed")
	ErrDPAPI                      = errors.New("browsercookies: Windows DPAPI access failed")
)

// CookieQuery 约束 cookie 查询：只允许 host 与 name 两个精确字段。
// 不允许任意 SQL、数据库路径或完整 Cookie header。
type CookieQuery struct {
	Host string
	Name string
}

// Valid 校验 query：Host 与 Name 必须非空且只包含保守的安全字符，
// 防止把任意内容带入 provider 的固定查询参数。
func (q CookieQuery) Valid() error {
	if !safeToken(q.Host) || !safeToken(q.Name) {
		return ErrQueryInvalid
	}
	return nil
}

// safeToken 限制 token 字符集为字母、数字、点、连字符与下划线。
func safeToken(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// DefaultQuery 保存唯一允许 FANBOX adapter 使用的 cookie query。
var DefaultQuery = CookieQuery{Host: ".fanbox.cc", Name: "FANBOXSESSID"}

// Profile 描述一个稳定、安全的浏览器 profile 标识与路径。ID 用于 --profile
// 参数匹配，必须是安全 identifier；Path 是解析后的绝对路径，绝不进入日志、
// 错误、JSON 或 MCP。
type Profile struct {
	ID   string
	Name string
	Path string
}

// ValidProfileID 只接受非隐藏的目录标识，拦截空值、路径分隔符与父目录跳转。
func ValidProfileID(id string) bool {
	return id != "" && !strings.HasPrefix(id, ".") && filepath.Base(id) == id
}

// Provider 定义浏览器 cookie provider 接口。DiscoverProfiles 与 Read 接收
// context 用于取消与锁等待控制；Read 只接受受约束的 CookieQuery。
type Provider interface {
	// Name 返回 provider 的稳定名称（"chrome"、"edge"、"firefox"、"safari"）。
	Name() string
	// DiscoverProfiles 返回已安装浏览器的 profile 列表；浏览器未安装时
	// 返回 ErrNotInstalled。绝不启动或修改浏览器。
	DiscoverProfiles(ctx context.Context) ([]Profile, error)
	// Read 读取 profileID 下匹配 query 的 cookie secret。
	Read(ctx context.Context, query CookieQuery, profileID string) ([]credential.Secret, error)
	Close() error
}

// SelectProfile 按稳定 identifier 选择 profile。未指定 identifier 且存在多个
// profile 时返回 ErrMultipleProfiles，错误只列出安全 identifier。
func SelectProfile(profiles []Profile, profileID string) (Profile, error) {
	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		ids = append(ids, p.ID)
	}
	if profileID == "" {
		if len(profiles) == 0 {
			return Profile{}, ErrProfileNotFound
		}
		if len(profiles) > 1 {
			return Profile{}, fmt.Errorf("%w: %s", ErrMultipleProfiles, strings.Join(ids, ", "))
		}
		return profiles[0], nil
	}
	for _, p := range profiles {
		if p.ID == profileID {
			return p, nil
		}
	}
	// 未知 identifier 时错误不包含调用方输入，防止把任意路径带入错误。
	return Profile{}, ErrProfileNotFound
}

// ProviderFactory 构造一个 provider。注册只发生在本包树内部。
type ProviderFactory func() (Provider, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]ProviderFactory{}
)

// Register 注册一个 browser 名称到 provider 工厂。由 provider 子包的 init 调用。
// 同一名称重复注册会 panic。
func Register(name string, factory ProviderFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic("browsercookies: duplicate provider registration: " + name)
	}
	registry[name] = factory
}

// New 按 browser 名称创建 provider：支持 "chrome"、"edge"、
// "firefox"、"safari"；未知名称返回 ErrUnknownBrowser。
// provider 子包通过 init 完成注册；集成入口导入 internal/browsercookies/system
// 获得完整分发。单独导入本包不会注册任何 provider。
func New(browser string) (Provider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := registry[strings.ToLower(strings.TrimSpace(browser))]
	if !ok {
		return nil, ErrUnknownBrowser
	}
	return factory()
}
