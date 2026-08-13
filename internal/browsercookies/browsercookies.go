// Package browsercookies 定义浏览器 cookie provider 的共享类型与接口。
//
// 本包是协议无关的：它不知道 FANBOX、Pixiv、CLI、MCP 或账号 store。
// 它只接受受约束的 CookieQuery，产出脱敏的 Secret，绝不接受任意 SQL、
// 任意数据库路径或完整 Cookie header。本包不识别 Cloudflare challenge，
// 也不运行任何 challenge 绕过；它只读取 cookie。
//
// 本包不导入任何 provider 子包，因此单独导入本包不会注册浏览器 provider。
// 系统/browser integration（导入并注册 chromium、firefox、safari）位于
// internal/browsercookies/system。
package browsercookies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// 错误分类。所有错误消息都是静态字符串，绝不包含 cookie 值、加密密钥、
// 绝对路径或 profile 内容。
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
	ErrTempSnapshot               = errors.New("browsercookies: could not create a private temporary snapshot")
	ErrKeychainAccess             = errors.New("browsercookies: keychain access failed")
	ErrKeychainItemNotFound       = errors.New("browsercookies: keychain item not found")
	ErrPermissionDenied           = errors.New("browsercookies: browser storage permission denied")
	ErrSecretServiceUnavailable   = errors.New("browsercookies: Linux Secret Service is unavailable")
	ErrSecretServiceAccess        = errors.New("browsercookies: Linux Secret Service access failed")
	ErrDPAPI                      = errors.New("browsercookies: Windows DPAPI access failed")
)

// redactedMarker 是所有 Secret 格式化输出的唯一可见内容。
const redactedMarker = "<redacted>"

// Secret 是一个脱敏的 cookie 秘密值。任何格式化输出（%v、%s、%q、%+v、%x 等）
// 与 JSON 序列化都不会泄露底层 value。只有受信任的调用方（本包树内的 provider
// 与上层保存流程）应当通过 Value() 读取实际值。
type Secret struct {
	value string
}

// NewSecret 构造一个 Secret。仅本包树内部使用。
func NewSecret(value string) Secret { return Secret{value: value} }

// Value 返回底层秘密值。此访问器只供受信任的调用方使用；日志、错误、JSON
// 或格式化路径都不得调用它。
func (s Secret) Value() string { return s.value }

// String 保证不泄露 value。
func (s Secret) String() string { return redactedMarker }

// GoString 保证 %#v 不泄露 value。
func (s Secret) GoString() string { return redactedMarker }

// Format 实现 fmt.Formatter，确保所有 verb 都不泄露 value。
func (s Secret) Format(f fmt.State, verb rune) {
	switch verb {
	case 'q':
		_, _ = io.WriteString(f, `"`+redactedMarker+`"`)
	default:
		_, _ = io.WriteString(f, redactedMarker)
	}
}

// CookieQuery 是受约束的 cookie 查询：只允许 host 与 name 两个精确字段。
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

// DefaultQuery 是唯一允许 FANBOX adapter 使用的 cookie query。
var DefaultQuery = CookieQuery{Host: ".fanbox.cc", Name: "FANBOXSESSID"}

// Profile 是一个稳定、安全的浏览器 profile 标识与路径。ID 用于 --profile
// 参数匹配，必须是安全 identifier；Path 是解析后的绝对路径，绝不进入日志、
// 错误、JSON 或 MCP。
type Profile struct {
	ID   string
	Name string
	Path string
}

// Cookie 是一条脱敏的 cookie 行。
type Cookie struct {
	Name     string
	Value    Secret
	Path     string
	Secure   bool
	HTTPOnly bool
}

// Snapshot 是进程私有的、内存中的解密 cookie 行副本；绝不持久化。
type Snapshot struct {
	ProfileID string
	Cookies   []Cookie
}

// Provider 是浏览器 cookie provider 接口。DiscoverProfiles 与 Read 接收
// context 用于取消与锁等待控制；Read 只接受受约束的 CookieQuery。
type Provider interface {
	// Name 返回 provider 的稳定名称（"chrome"、"edge"、"firefox"、"safari"）。
	Name() string
	// DiscoverProfiles 返回已安装浏览器的 profile 列表；浏览器未安装时
	// 返回 ErrNotInstalled。绝不启动或修改浏览器。
	DiscoverProfiles(ctx context.Context) ([]Profile, error)
	// Read 读取 profileID 下匹配 query 的 cookie secret。
	Read(ctx context.Context, query CookieQuery, profileID string) ([]Secret, error)
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

// dbReadHook 是测试 fixture 的全局读拦截状态，串行化方式与
// 测试调用方可用同样的路径级 seam 验证本地 cookie 读取错误。
type dbReadHook struct {
	providerName string
	read         func(string) ([]byte, error)
}

var (
	dbReadHookMu    sync.Mutex
	dbReadHookState atomic.Pointer[dbReadHook]
)

// SetProviderFixtureForTest 仅为测试拦截指定 provider 的数据库读取，使测试
// 注入 fixture SQLite 而不触碰真实浏览器。同一时间只允许一个 hook，互斥锁
// 串行化；调用方必须用 t.Cleanup 执行返回的恢复函数，且不得并行使用同一
// provider。
func SetProviderFixtureForTest(providerName string, read func(string) ([]byte, error)) func() {
	if strings.TrimSpace(providerName) == "" || read == nil {
		panic("browsercookies: provider fixture hook requires a provider name and reader")
	}
	dbReadHookMu.Lock()
	dbReadHookState.Store(&dbReadHook{providerName: providerName, read: read})
	var once sync.Once
	return func() {
		once.Do(func() {
			dbReadHookState.Store(nil)
			dbReadHookMu.Unlock()
		})
	}
}

// HookBytes 返回 provider 的数据库读取结果。命中测试 hook 时返回 hook 数据与
// hooked=true；否则返回 hooked=false，调用方应使用真实路径。
func HookBytes(providerName, path string) (data []byte, hooked bool, err error) {
	if hook := dbReadHookState.Load(); hook != nil && hook.providerName == providerName {
		b, err := hook.read(path)
		return b, true, err
	}
	return nil, false, nil
}

// WriteTempSnapshot 把字节写入进程私有临时目录（0700）下的私有文件（0600）。
// 返回文件路径与清理函数；调用方必须执行清理。任何错误都不包含路径或内容。
func WriteTempSnapshot(data []byte) (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "pixiv-browsercookies-*")
	if err != nil {
		return "", nil, fmt.Errorf("%w: create private dir", ErrTempSnapshot)
	}
	file, err := os.CreateTemp(dir, "snapshot-*.db")
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("%w: create private file", ErrTempSnapshot)
	}
	path = file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("%w: write snapshot", ErrTempSnapshot)
	}
	if err := file.Close(); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("%w: close snapshot", ErrTempSnapshot)
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}
