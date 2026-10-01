package settings_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件是 goal-1 的 t10 行为层：保护 input.md §3.5 的两个结构性约束，并锁定
// 高级可选配置组（服务级网络 / FlareSolverr / 账号池 / 默认账号选择）的既有语义。
//
// 与 t2/t4 的分工：t2 锁定普通配置表面，t4 锁定标签声明；本文件专门覆盖
// **标签无法表达、必须由领域规则处理**的部分，因此这些绑定继续留在 snapshot.go。

// diskFileStore 把配置端口接到真实磁盘路径，供需要验证 auth 表与普通配置在同一份
// 文件中共存的测试使用（注入式内存 store 不便于同时断言 auth.go 的读写与 TOML 结构）。
type diskFileStore struct{ path string }

func (s diskFileStore) Path() (string, error) { return s.path, nil }

func (s diskFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (s diskFileStore) WritePrivateFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o600)
}

func (s diskFileStore) EnsurePrivateFile(path string, body []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, body, 0o600)
}

// hasField 报告一个值类型是否声明了指定名称的导出字段。用于在编译期保证之外
// 断言“窄类型确实没有该字段”（例如 PixivNetworkConfig 不得有 UserAgent）。
func hasField(value any, name string) bool {
	typeOf := reflect.TypeOf(value)
	for i := 0; i < typeOf.NumField(); i++ {
		if typeOf.Field(i).Name == name {
			return true
		}
	}
	return false
}

// mustParseValue 把一个 Go 字符串按既有约定序列化为 TOML 字面量，供 SetConfigValue
// 使用。与既有的 parser.ParseValue("'./new'") 用法一致：config set 的输入总是字符串。
func mustParseValue(t *testing.T, raw string) parser.Value {
	t.Helper()
	value, err := parser.ParseValue("'" + raw + "'")
	require.NoError(t, err)
	return value
}

// ---------------------------------------------------------------- 约束 1：Pixiv 不接受 user_agent

// TestPixivNetworkRejectsUserAgentWhileFanboxKeepsIt 是 §3.5 第一个结构性保护的守门：
// Pixiv 与 FANBOX 的网络字段并不相同。给共享结构体统一加标签会让 Pixiv 无意中开始
// 接受 user_agent —— 那是新增用户可配置能力，必须被拒绝。
//
// 本测试同时从两个方向证明这一点：
//   - Pixiv 侧写了 user_agent 也不生效（保持未设置）；
//   - FANBOX/反搜侧写了 user_agent 必须生效（否则就是把差异抹平了）。
func TestPixivNetworkRejectsUserAgentWhileFanboxKeepsIt(t *testing.T) {
	withoutProxyEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[pixiv.network]
proxy_url = "http://pixiv-proxy"
user_agent = "must-not-be-accepted"

[fanbox.network]
user_agent = "fanbox-agent"

[reverse_search.network]
user_agent = "reverse-search-agent"
`), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)

	// Pixiv：代理生效，user_agent 必须**不**生效。
	// 窄类型 PixivNetworkConfig 没有 UserAgent 字段，因此这一点由**编译器**保证：
	// 尝试读取 runtime.PixivNetwork.UserAgent 会直接编译失败。这里在行为层再确认
	// 写了 user_agent 也不会影响任何 Pixiv 侧可观察状态。
	assert.True(t, runtime.PixivNetwork.ProxyURL.Present)
	assert.Equal(t, "http://pixiv-proxy", runtime.PixivNetwork.ProxyURL.Value)
	require.False(t, hasField(runtime.PixivNetwork, "UserAgent"),
		"PixivNetworkConfig must not expose a UserAgent field: that would be a new user-configurable capability")

	// FANBOX / 反搜：user_agent 必须生效（不是把差异抹平）。
	assert.True(t, runtime.FanboxNetwork.UserAgent.Present)
	assert.Equal(t, "fanbox-agent", runtime.FanboxNetwork.UserAgent.Value)
	assert.True(t, runtime.ReverseSearchNetwork.UserAgent.Present)
	assert.Equal(t, "reverse-search-agent", runtime.ReverseSearchNetwork.UserAgent.Value)
}

// TestPixivNetworkKeepsStrictTypeChecking 断言 Pixiv 窄类型没有因为拆分而放松校验：
// 非法类型仍必须报错，而不是被静默忽略。
func TestPixivNetworkKeepsStrictTypeChecking(t *testing.T) {
	withoutProxyEnvironment(t)
	for name, body := range map[string]string{
		"proxy must be a string":       "[pixiv.network]\nproxy_url = 42\n",
		"proxy rejects a bool":         "[pixiv.network]\nproxy_url = true\n",
		"proxy rejects a nested table": "[pixiv.network.proxy_url]\nvalue = \"x\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
			state, err := config.LoadSnapshotAt(path)
			require.NoError(t, err)
			_, err = state.Runtime()
			require.Error(t, err, "malformed pixiv.network values must stay rejected")
		})
	}
}

// TestPixivNetworkDistinguishesAbsentFromExplicitEmpty 断言服务级代理的
// "缺失 vs 显式空串"语义在窄类型里保持：缺失表示可继承上级代理，显式空串表示直连。
func TestPixivNetworkDistinguishesAbsentFromExplicitEmpty(t *testing.T) {
	withoutProxyEnvironment(t)

	t.Run("absent means inherit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://global\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.False(t, runtime.PixivNetwork.ProxyURL.Present, "absent pixiv proxy must stay unset so the global proxy applies")
	})

	t.Run("explicit empty means direct", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://global\"\n\n[pixiv.network]\nproxy_url = \"\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.True(t, runtime.PixivNetwork.ProxyURL.Present, "an explicit empty pixiv proxy means direct access, not inheritance")
		assert.Empty(t, runtime.PixivNetwork.ProxyURL.Value)
	})
}

// ---------------------------------------------------------------- 约束 2：default_user_id 不进 Runtime

// TestInvalidDefaultUserIDDoesNotBreakOrdinaryRuntimeReads 是 §3.5 第二个结构性保护的
// 守门：默认账号选择不是普通运行时设置。若把它并入 Runtime()，一个残留的非法
// default_user_id 会让**所有**普通命令失败，并改变错误触发时机。
func TestInvalidDefaultUserIDDoesNotBreakOrdinaryRuntimeReads(t *testing.T) {
	withoutProxyEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[pixiv.auth]
default_user_id = -5

[fanbox.auth]
default_user_id = "not-a-number"

[download]
path = "./ok"
`), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)

	// 普通配置读取与 Runtime() 必须完全不受非法 default_user_id 影响。
	runtime, err := state.Runtime()
	require.NoError(t, err, "an invalid default_user_id must not break ordinary runtime reads")
	assert.Equal(t, "./ok", runtime.DownloadPath)

	for _, alias := range config.ValidSettingAliases() {
		_, err := state.Effective(alias)
		require.NoErrorf(t, err, "Effective(%q) must not be affected by default_user_id", alias)
	}

	// 只有按需读取默认账号时才失败——错误触发时机保持在原处。
	store := config.Store{Files: diskFileStore{path: path}}
	_, _, err = store.ReadPixivDefaultUserID()
	require.Error(t, err, "reading the default user id must still surface the invalid value")
	assert.Contains(t, err.Error(), "must be a positive integer")

	_, _, err = store.ReadFanboxDefaultUserID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a positive integer")
}

// TestDefaultUserIDRoundTripsWithoutEnteringRuntime 断言默认账号选择可读写，但它的
// 值不会出现在 RuntimeConfig 中（该 DTO 不承载账号选择）。
func TestDefaultUserIDRoundTripsWithoutEnteringRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store := config.Store{Files: diskFileStore{path: path}}

	// 未设置时 ok=false。
	_, ok, err := store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, store.SetPixivDefaultUserID(12345))
	userID, ok, err := store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(12345), userID)

	// 该值必须留在 auth 表中，且不影响 Runtime()。
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
	for _, alias := range config.ValidSettingAliases() {
		assert.NotEqualf(t, "default_user_id", alias, "default_user_id must not become a普通 config alias")
	}

	// 清理后恢复未设置。
	require.NoError(t, store.ClearPixivDefaultUserID())
	_, ok, err = store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	assert.False(t, ok)
}

// ---------------------------------------------------------------- 约束 3：退役键 unset 仍可用

// TestRemovedKeysStayClearableWhileAdvancedTablesExist 断言退役键的检查没有前移：
// 即使配置里同时存在高级表，`unset` 仍能清理退役键，普通读取也仍能正常进行
// （只有读到时才报 removed_setting）。
func TestRemovedKeysStayClearableWhileAdvancedTablesExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[web]
fallback_enabled = true

[pixiv.network]
proxy_url = "http://p"

[fanbox.flaresolverr]
url = "http://solver"
`), 0o600))

	// 写入侧不受退役键阻挡（否则无法修复配置）。
	require.NoError(t, config.SetConfigValue(path, "download_path", mustParseValue(t, "./x")))

	// 清理退役键可行。
	removed, err := config.UnsetConfigValue(path, "web_fallback_enabled")
	require.NoError(t, err)
	assert.True(t, removed)

	// 清理后 Runtime() 不再因退役键失败，且高级表仍生效。
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	assert.Equal(t, "http://p", runtime.PixivNetwork.ProxyURL.Value)
	require.NotNil(t, runtime.FanboxFlareSolverr)
	assert.Equal(t, "http://solver", runtime.FanboxFlareSolverr.URL)
}

// ---------------------------------------------------------------- 约束 4：FlareSolverr 可选性

// TestFlareSolverrGroupsStayOptional 断言缺少相关配置时保持未启用，不会因为绑定逻辑
// 自动创建可用实例；同时保留 "有表但 url 空白 → 报错" 的严格规则。
func TestFlareSolverrGroupsStayOptional(t *testing.T) {
	withoutProxyEnvironment(t)

	t.Run("absent tables stay nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[download]\npath = \"./x\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.Nil(t, runtime.FanboxFlareSolverr, "no solver dependency may be created implicitly")
		assert.Nil(t, runtime.ReverseSearchFlareSolverr)
	})

	t.Run("present table without url is rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[fanbox.flaresolverr]\nproxy_url = \"socks5://p\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		_, err = state.Runtime()
		require.EqualError(t, err, "fanbox.flaresolverr.url must be set when fanbox.flaresolverr is configured")
	})

	t.Run("blank url is rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[fanbox.flaresolverr]\nurl = \"   \"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		_, err = state.Runtime()
		require.EqualError(t, err, "fanbox.flaresolverr.url must be set when fanbox.flaresolverr is configured")
	})
}

// ---------------------------------------------------------------- 约束 5：账号池领域校验

// TestAccountPoolKeepsItsDomainValidation 断言账号池的严格校验没有被标签化绕过：
// 类型错误与非法策略必须报错，合法值必须生效，且 `account_pool_accounts` 墓碑仍生效。
func TestAccountPoolKeepsItsDomainValidation(t *testing.T) {
	withoutProxyEnvironment(t)

	t.Run("valid values apply", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[account_pool]\nenabled = true\nstrategy = \"random\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.True(t, runtime.AccountPool.Enabled)
		assert.Equal(t, config.AccountPoolStrategyRandom, runtime.AccountPool.Strategy)
	})

	t.Run("enabled must be a boolean", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[account_pool]\nenabled = \"yes\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		_, err = state.Runtime()
		require.EqualError(t, err, "account_pool.enabled must be a boolean")
	})

	t.Run("strategy must be a known value", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[account_pool]\nstrategy = \"weighted\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		_, err = state.Runtime()
		require.EqualError(t, err, "account_pool.strategy must be one of: round_robin, random")
	})

	t.Run("accounts tombstone still fails when present", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[account_pool]\naccounts = \"1,2\"\n"), 0o600))
		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		_, err = state.Runtime()
		require.ErrorIs(t, err, config.ErrRemovedSetting)
	})
}
