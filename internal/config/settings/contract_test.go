package settings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 以独立的已发布配置预期检查公开 API，防止声明与实现同时改错仍自洽通过。
// 这些断言不从当前生产标签计算预期。

// authoritativeEnvNames 是"哪个别名能读取哪个环境变量"的权威清单，覆盖全部 8 个可读
// 环境的别名（与迁移前 EnvValue 的行为逐条一致）。顺序即优先级：https_proxy 先于
// HTTPS_PROXY。
//
// 断言策略：给每个变量设置"不相关的哨兵值"，使绑定与否产生不同的可观察结果——
// 绑定到 env 的别名会交付哨兵值（download/filename/directory 成功交付；log_level /
// log_format / request_interval 因哨兵非法而以自身校验错误暴露），未绑定 env 的别名则
// 仍报告 default/unset。因此删除或改错任一映射都会让对应断言失败，无需依赖开发机是否
// 恰好导出了某个变量。
var authoritativeEnvNames = map[string][]string{
	"download_path":      {"DOWNLOAD_PATH"},
	"filename_template":  {"FILENAME_TEMPLATE"},
	"directory_template": {"DIRECTORY_TEMPLATE"},
	"request_interval":   {"PIXIV_REQUEST_INTERVAL"},
	"log_level":          {"PIXIV_LOG_LEVEL"},
	"log_format":         {"PIXIV_LOG_FORMAT"},
	"https_proxy":        {"https_proxy", "HTTPS_PROXY"},
	"saucenao_api_key":   {"SAUCENAO_API_KEY"},
}

// envProbeSentinels 是每个可读环境变量使用的哨兵值：既不是合法枚举也不是合法 duration，
// 因此"被 env 接管"与"未接管"会产生不同的可观察结果。
var envProbeSentinels = map[string]string{
	"DOWNLOAD_PATH":          "./__env__/downloads",
	"FILENAME_TEMPLATE":      "__env__{id}",
	"DIRECTORY_TEMPLATE":     "__env__/{author}",
	"PIXIV_REQUEST_INTERVAL": "__env__not-a-duration",
	"PIXIV_LOG_LEVEL":        "__env__not-a-level",
	"PIXIV_LOG_FORMAT":       "__env__not-a-format",
	"SAUCENAO_API_KEY":       "__env__api-key",
}

// envControlledVariables 列出会左右配置结果的环境变量，供需要"无环境"视图的断言先清空。
var envControlledVariables = []string{
	"DOWNLOAD_PATH",
	"FILENAME_TEMPLATE",
	"DIRECTORY_TEMPLATE",
	"PIXIV_REQUEST_INTERVAL",
	"PIXIV_LOG_LEVEL",
	"PIXIV_LOG_FORMAT",
	"https_proxy",
	"HTTPS_PROXY",
	"SAUCENAO_API_KEY",
}

// clearSettingEnvironment 移除全部配置环境变量，使断言只反映声明与文件。它不改变任何
// 优先级契约，只把测试与运行它的机器隔离开；t.Setenv 保证测试结束后原始值被恢复。
func clearSettingEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range envControlledVariables {
		if value, present := os.LookupEnv(name); present {
			t.Setenv(name, value)
		}
		require.NoError(t, os.Unsetenv(name))
	}
}

// defaultSurface 记录每个别名在空配置下的默认表面：Effective 的 Value/HasValue/Source，
// 以及 Runtime() 中对应字段的最终值。
var defaultSurface = map[string]struct {
	effective any
	hasValue  bool
	runtime   any
}{
	"download_path":             {effective: "./downloads", hasValue: true, runtime: "./downloads"},
	"filename_template":         {effective: "{author} - {title}_{id}", hasValue: true, runtime: "{author} - {title}_{id}"},
	"directory_template":        {effective: nil, hasValue: false, runtime: ""},
	"output_json":               {effective: false, hasValue: true, runtime: false},
	"login_open_browser":        {effective: true, hasValue: true, runtime: true},
	"login_use_after_login":     {effective: false, hasValue: true, runtime: false},
	"update_check_enabled":      {effective: true, hasValue: true, runtime: true},
	"log_level":                 {effective: "info", hasValue: true, runtime: "info"},
	"log_format":                {effective: "text", hasValue: true, runtime: "text"},
	"https_proxy":               {effective: nil, hasValue: false, runtime: ""},
	"request_interval":          {effective: time.Duration(0), hasValue: true, runtime: time.Duration(0)},
	"reverse_search_provider":   {effective: "saucenao", hasValue: true, runtime: "saucenao"},
	"reverse_search_pixiv_only": {effective: true, hasValue: true, runtime: true},
	"saucenao_api_key":          {effective: nil, hasValue: false, runtime: ""},
	"login_relay_public_url":    {effective: nil, hasValue: false, runtime: ""},
	"login_relay_listen_addr":   {effective: nil, hasValue: false, runtime: ""},
	"login_relay_tls_cert_file": {effective: nil, hasValue: false, runtime: ""},
	"login_relay_tls_key_file":  {effective: nil, hasValue: false, runtime: ""},
}

// expectedAliases 是 ValidSettingAliases 的完整集合（排序后），不含退役墓碑。
var expectedAliases = []string{
	"account_pool_enabled",
	"account_pool_strategy",
	"directory_template",
	"download_path",
	"filename_template",
	"https_proxy",
	"log_format",
	"log_level",
	"login_open_browser",
	"login_relay_listen_addr",
	"login_relay_public_url",
	"login_relay_tls_cert_file",
	"login_relay_tls_key_file",
	"login_use_after_login",
	"output_json",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
	"update_check_enabled",
}

// allResolvableAliases 额外包含退役墓碑，用于 SettingSpecByAlias 的存在性断言。
var allResolvableAliases = []string{
	"account_pool_accounts",
	"account_pool_enabled",
	"account_pool_strategy",
	"directory_template",
	"download_path",
	"filename_template",
	"https_proxy",
	"log_format",
	"log_level",
	"login_open_browser",
	"login_relay_listen_addr",
	"login_relay_public_url",
	"login_relay_tls_cert_file",
	"login_relay_tls_key_file",
	"login_use_after_login",
	"output_json",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
	"update_check_enabled",
	"web_fallback_enabled",
}

// expectedCLIManagedAliases 是 pixiv config get/set/unset 暴露的键集合。
var expectedCLIManagedAliases = []string{
	"account_pool_enabled",
	"account_pool_strategy",
	"directory_template",
	"download_path",
	"filename_template",
	"https_proxy",
	"log_format",
	"log_level",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
}

// expectedRemovedAliases 是保留的迁移墓碑：仍可被 SettingSpecByAlias 查到（供 config unset
// 清理），但不在 Valid/CLI 别名集合中，显式出现在配置里时返回 removed_setting。
var expectedRemovedAliases = []string{"account_pool_accounts", "web_fallback_enabled"}

func TestSettingAliasSurfaceIsStable(t *testing.T) {
	aliases := config.ValidSettingAliases()
	assert.Equal(t, expectedAliases, aliases, "ValidSettingAliases changed")

	for _, removed := range expectedRemovedAliases {
		assert.NotContains(t, aliases, removed, "removed alias must not be offered as a valid key")
	}

	for _, alias := range allResolvableAliases {
		spec, ok := config.SettingSpecByAlias(alias)
		require.Truef(t, ok, "alias %q must stay resolvable", alias)
		assert.Equal(t, alias, spec.Alias)
	}

	managed := config.CLISettingAliases()
	assert.Equal(t, expectedCLIManagedAliases, managed, "CLISettingAliases changed")

	for _, removed := range expectedRemovedAliases {
		assert.NotContains(t, managed, removed, "removed alias must not be CLI managed")
	}
	for _, alias := range managed {
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok, "CLI managed alias %q must be resolvable", alias)
		assert.True(t, spec.CLIManaged, "CLI managed alias %q must declare CLIManaged", alias)
		assert.False(t, spec.Removed, "CLI managed alias %q must not be removed", alias)
	}
}

func TestSettingSpecCarriesTheDocumentedContract(t *testing.T) {
	type contract struct {
		key        string
		table      []string
		tomlKey    string
		hasDefault bool
		defaultIn  bool
		cliManaged bool
		sensitive  bool
	}
	want := map[string]contract{
		"download_path":             {key: "download.path", table: []string{"download"}, tomlKey: "path", hasDefault: true, defaultIn: true, cliManaged: true},
		"filename_template":         {key: "download.filename_template", table: []string{"download"}, tomlKey: "filename_template", hasDefault: true, defaultIn: true, cliManaged: true},
		"directory_template":        {key: "download.directory_template", table: []string{"download"}, tomlKey: "directory_template", cliManaged: true},
		"output_json":               {key: "output.json", table: []string{"output"}, tomlKey: "json", hasDefault: true, defaultIn: true},
		"login_open_browser":        {key: "login.open_browser", table: []string{"login"}, tomlKey: "open_browser", hasDefault: true, defaultIn: true},
		"login_use_after_login":     {key: "login.use_after_login", table: []string{"login"}, tomlKey: "use_after_login", hasDefault: true, defaultIn: true},
		"update_check_enabled":      {key: "update.check_enabled", table: []string{"update"}, tomlKey: "check_enabled", hasDefault: true, defaultIn: true},
		"log_level":                 {key: "logging.level", table: []string{"logging"}, tomlKey: "level", hasDefault: true, defaultIn: true, cliManaged: true},
		"log_format":                {key: "logging.format", table: []string{"logging"}, tomlKey: "format", hasDefault: true, defaultIn: true, cliManaged: true},
		"https_proxy":               {key: "network.https_proxy", table: []string{"network"}, tomlKey: "https_proxy", cliManaged: true},
		"request_interval":          {key: "network.request_interval", table: []string{"network"}, tomlKey: "request_interval", hasDefault: true, cliManaged: true},
		"reverse_search_provider":   {key: "reverse_search.provider", table: []string{"reverse_search"}, tomlKey: "provider", hasDefault: true, defaultIn: true, cliManaged: true},
		"reverse_search_pixiv_only": {key: "reverse_search.pixiv_only", table: []string{"reverse_search"}, tomlKey: "pixiv_only", hasDefault: true, defaultIn: true, cliManaged: true},
		"saucenao_api_key":          {key: "reverse_search.saucenao_api_key", table: []string{"reverse_search"}, tomlKey: "saucenao_api_key", cliManaged: true, sensitive: true},
		"web_fallback_enabled":      {key: "web.fallback_enabled", table: []string{"web"}, tomlKey: "fallback_enabled"},
		"login_relay_public_url":    {key: "login.relay_public_url", table: []string{"login"}, tomlKey: "relay_public_url"},
		"login_relay_listen_addr":   {key: "login.relay_listen_addr", table: []string{"login"}, tomlKey: "relay_listen_addr"},
		"login_relay_tls_cert_file": {key: "login.relay_tls_cert_file", table: []string{"login"}, tomlKey: "relay_tls_cert_file"},
		"login_relay_tls_key_file":  {key: "login.relay_tls_key_file", table: []string{"login"}, tomlKey: "relay_tls_key_file"},
		"account_pool_enabled":      {key: "account_pool.enabled", table: []string{"account_pool"}, tomlKey: "enabled", hasDefault: true, cliManaged: true},
		"account_pool_strategy":     {key: "account_pool.strategy", table: []string{"account_pool"}, tomlKey: "strategy", hasDefault: true, cliManaged: true},
		"account_pool_accounts":     {key: "account_pool.accounts", table: []string{"account_pool"}, tomlKey: "accounts"},
	}

	for alias, expected := range want {
		t.Run(alias, func(t *testing.T) {
			spec, ok := config.SettingSpecByAlias(alias)
			require.True(t, ok, "alias %q must resolve", alias)
			assert.Equal(t, alias, spec.Alias)
			assert.Equal(t, expected.key, spec.KoanfKey, "koanf/TOML path changed")
			assert.Equal(t, expected.table, spec.Table, "TOML table changed")
			assert.Equal(t, expected.tomlKey, spec.Key, "TOML key changed")
			assert.Equal(t, expected.hasDefault, spec.HasDefault, "default presence changed")
			assert.Equal(t, expected.defaultIn, spec.DefaultInFile, "DefaultInFile changed")
			assert.Equal(t, expected.cliManaged, spec.CLIManaged, "CLIManaged changed")
			assert.Equal(t, expected.sensitive, spec.Sensitive, "Sensitive changed")
		})
	}
}

func TestRemovedAliasTombstonesKeepTheirBehaviour(t *testing.T) {
	for _, removed := range expectedRemovedAliases {
		spec, ok := config.SettingSpecByAlias(removed)
		require.True(t, ok, "removed alias %q must stay resolvable for config unset", removed)
		assert.True(t, spec.Removed, "alias %q must stay a tombstone", removed)

		_, _, err := config.ParseSettingInput(removed, "true")
		require.ErrorIs(t, err, config.ErrRemovedSetting, "writing a removed key must fail with removed_setting")
		require.ErrorContains(t, err, "pixiv config unset "+removed)
	}

	if _, ok := config.SettingSpecByAlias("premium_status_cache_ttl"); ok {
		t.Fatal("retired setting premium_status_cache_ttl must not be reachable")
	}

	// 未出现在配置里的退役键不报错，因此常规读取与 config unset 都不会被它卡住。
	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)
	for _, removed := range expectedRemovedAliases {
		value, err := state.Effective(removed)
		require.NoErrorf(t, err, "absent removed key %q must not fail the read", removed)
		assert.Equal(t, "unset", value.Source)
	}
}

func TestRemovedAliasPresentInFileReportsRemovedSetting(t *testing.T) {
	for alias, table := range map[string]string{
		"web_fallback_enabled":  "[web]\nfallback_enabled = true\n",
		"account_pool_accounts": "[account_pool]\naccounts = \"1,2\"\n",
	} {
		t.Run(alias, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(table), 0o600))
			state, err := config.LoadSnapshotAt(path)
			require.NoError(t, err)

			_, err = state.Effective(alias)
			require.ErrorIs(t, err, config.ErrRemovedSetting)
			require.ErrorContains(t, err, "pixiv config unset "+alias)

			_, err = state.Runtime()
			require.ErrorIs(t, err, config.ErrRemovedSetting, "Runtime must surface the migration tombstone")
		})
	}
}

// TestDeclaredEnvironmentBindingsAreExactlyTheDocumentedSet 是"环境来源由声明决定"的
// 机械守门：删除某个绑定或错误地新增一个都会失败。
func TestDeclaredEnvironmentBindingsAreExactlyTheDocumentedSet(t *testing.T) {
	clearSettingEnvironment(t)
	for name, sentinel := range envProbeSentinels {
		t.Setenv(name, sentinel)
	}

	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	for _, alias := range config.ValidSettingAliases() {
		if _, declared := authoritativeEnvNames[alias]; declared {
			continue
		}
		// 未声明 env 的别名：哨兵值不得被读取。
		value, err := state.Effective(alias)
		require.NoErrorf(t, err, "alias %q must not read any environment variable", alias)
		assert.NotEqualf(t, "env", value.Source, "alias %q must not read any environment variable", alias)
	}

	// 合法哨兵：必须原样交付，并报告 env 来源。
	for alias, want := range map[string]any{
		"download_path":      envProbeSentinels["DOWNLOAD_PATH"],
		"filename_template":  envProbeSentinels["FILENAME_TEMPLATE"],
		"directory_template": envProbeSentinels["DIRECTORY_TEMPLATE"],
	} {
		value, err := state.Effective(alias)
		require.NoErrorf(t, err, "Effective(%q)", alias)
		assert.Equalf(t, want, value.Value, "alias %q did not deliver its declared environment value", alias)
		assert.Equal(t, "env", value.Source)
		assert.True(t, value.HasValue)
	}

	// 非法哨兵：必须因该别名自身的校验而失败，证明它确实读了环境变量而不是静默回退。
	for alias, wantErr := range map[string]string{
		"log_level":        "log_level must be one of: info, debug",
		"log_format":       "log_format must be one of: text, json",
		"request_interval": `request_interval: time: invalid duration "__env__not-a-duration"`,
	} {
		_, err := state.Effective(alias)
		require.EqualErrorf(t, err, wantErr, "alias %q must take its value from the declared environment variable", alias)
	}

	_, err = state.Runtime()
	require.Error(t, err, "Runtime must reject the sentinel environment values rather than falling back to defaults")
}

// TestEnvironmentPrecedenceKeepsDeclaredOrder 验证 https_proxy 的声明顺序：小写变量
// 先于大写变量，且"小写存在但为空"仍被选中（不回退到大写）。
func TestEnvironmentPrecedenceKeepsDeclaredOrder(t *testing.T) {
	t.Run("lowercase proxy wins over uppercase", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n"), 0o600))
		t.Setenv("https_proxy", "http://lower-proxy")
		t.Setenv("HTTPS_PROXY", "http://upper-proxy")

		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		value, err := state.Effective("https_proxy")
		require.NoError(t, err)
		assert.Equal(t, "http://lower-proxy", value.Value)
		assert.Equal(t, "env", value.Source)

		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.Equal(t, "http://lower-proxy", runtime.HTTPSProxy)
	})

	// 小写存在但为空：来源变成 env（命中），但空字符串按既有语义不算"已配置"，
	// 因此 HasValue=false、Value=nil；关键是它**不**回退到 HTTPS_PROXY。
	t.Run("empty lowercase proxy wins over uppercase", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		t.Setenv("https_proxy", "")
		t.Setenv("HTTPS_PROXY", "http://upper-proxy")

		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		value, err := state.Effective("https_proxy")
		require.NoError(t, err)
		assert.Equal(t, "env", value.Source, "an explicitly present lowercase proxy must be selected, not skipped")
		assert.False(t, value.HasValue, "an empty value is present but not configured")
		assert.Nil(t, value.Value, "an empty proxy must not resolve to the uppercase fallback")

		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.Equal(t, "", runtime.HTTPSProxy, "the uppercase variable must not leak into runtime")
	})

	t.Run("environment overrides file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n"), 0o600))
		t.Setenv("https_proxy", "http://env-proxy")

		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		value, err := state.Effective("https_proxy")
		require.NoError(t, err)
		assert.Equal(t, "http://env-proxy", value.Value)
		assert.Equal(t, "env", value.Source)
	})
}

func TestEffectiveDefaultsAndRuntimeSurfaceAreStable(t *testing.T) {
	clearSettingEnvironment(t)
	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	runtime, err := state.Runtime()
	require.NoError(t, err)

	assert.Equal(t, defaultSurface["download_path"].runtime, runtime.DownloadPath)
	assert.Equal(t, defaultSurface["filename_template"].runtime, runtime.FilenameTemplate)
	assert.Equal(t, defaultSurface["directory_template"].runtime, runtime.DirectoryTemplate)
	assert.Equal(t, defaultSurface["https_proxy"].runtime, runtime.HTTPSProxy)
	assert.Equal(t, defaultSurface["log_level"].runtime, runtime.LogLevel)
	assert.Equal(t, defaultSurface["log_format"].runtime, runtime.LogFormat)
	assert.Equal(t, defaultSurface["output_json"].runtime, runtime.OutputJSON)
	assert.Equal(t, defaultSurface["login_open_browser"].runtime, runtime.LoginOpenBrowser)
	assert.Equal(t, defaultSurface["login_use_after_login"].runtime, runtime.LoginUseAfterLogin)
	assert.Equal(t, defaultSurface["update_check_enabled"].runtime, runtime.UpdateCheckEnabled)
	assert.Equal(t, defaultSurface["reverse_search_provider"].runtime, runtime.ReverseSearchProvider)
	assert.Equal(t, defaultSurface["reverse_search_pixiv_only"].runtime, runtime.ReverseSearchPixivOnly)
	assert.Equal(t, defaultSurface["saucenao_api_key"].runtime, runtime.SauceNAOAPIKey)
	assert.Equal(t, defaultSurface["request_interval"].runtime, runtime.RequestInterval)
	assert.Equal(t, defaultSurface["login_relay_public_url"].runtime, runtime.LoginRelayPublicURL)
	assert.Equal(t, defaultSurface["login_relay_listen_addr"].runtime, runtime.LoginRelayListenAddr)
	assert.Equal(t, defaultSurface["login_relay_tls_cert_file"].runtime, runtime.LoginRelayTLSCertFile)
	assert.Equal(t, defaultSurface["login_relay_tls_key_file"].runtime, runtime.LoginRelayTLSKeyFile)
	assert.False(t, runtime.AccountPool.Enabled, "account pool must stay disabled by default")
	assert.Equal(t, config.AccountPoolStrategyRoundRobin, runtime.AccountPool.Strategy)

	for alias, want := range defaultSurface {
		value, err := state.Effective(alias)
		require.NoErrorf(t, err, "Effective(%q)", alias)
		if !want.hasValue {
			assert.Falsef(t, value.HasValue, "alias %q must stay unset by default", alias)
			assert.Equalf(t, "unset", value.Source, "alias %q must stay unset by default", alias)
			continue
		}
		assert.Truef(t, value.HasValue, "alias %q must keep a default", alias)
		assert.Equalf(t, "default", value.Source, "alias %q must be reported as default-sourced", alias)
		assert.Equalf(t, want.effective, value.Value, "default value of %q changed", alias)
	}
}

func TestEnvironmentEmptyValueStillCountsAsConfigured(t *testing.T) {
	t.Run("empty env override is not treated as absent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[download]\npath = \"./from-file\"\n"), 0o600))
		t.Setenv("DOWNLOAD_PATH", "")

		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		value, err := state.Effective("download_path")
		require.NoError(t, err)
		assert.Equal(t, "", value.Value)
		assert.Equal(t, "env", value.Source, "an explicitly empty env value must win over the file")
	})

	t.Run("explicit empty file string is preserved", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[download]\ndirectory_template = \"\"\n"), 0o600))

		state, err := config.LoadSnapshotAt(path)
		require.NoError(t, err)
		value, err := state.Effective("directory_template")
		require.NoError(t, err)
		// directory_template 没有默认值，因此空串在文件中被视为未设置；这正是既有语义。
		assert.False(t, value.HasValue)
		assert.Equal(t, "file", value.Source)
	})
}

func TestDefaultDeclarationDistinguishesFalseZeroAndEmpty(t *testing.T) {
	clearSettingEnvironment(t)

	// 默认值必须能表达 false 与 0s，而不是把它们当成"没有默认值"。
	cases := []struct {
		alias string
		want  any
		text  string
	}{
		{alias: "output_json", want: false, text: "false"},
		{alias: "login_use_after_login", want: false, text: "false"},
		{alias: "account_pool_enabled", want: false, text: "false"},
		{alias: "request_interval", want: time.Duration(0), text: "0s"},
	}
	for _, test := range cases {
		t.Run(test.alias, func(t *testing.T) {
			state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
			require.NoError(t, err)
			value, err := state.Effective(test.alias)
			require.NoError(t, err)
			assert.True(t, value.HasValue, "explicit false/0s default must be declared, not inferred from a non-zero check")
			assert.Equal(t, "default", value.Source)
			assert.Equal(t, test.want, value.Value)
			assert.Equal(t, test.text, value.Text)
		})
	}

	// 反过来：没有默认值的别名保持 unset，不被零值冒充。
	for _, alias := range []string{"directory_template", "https_proxy", "saucenao_api_key"} {
		state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
		require.NoError(t, err)
		value, err := state.Effective(alias)
		require.NoError(t, err)
		assert.Falsef(t, value.HasValue, "%q has no declared default and must stay unset", alias)
		assert.Nilf(t, value.Value, "%q must not expose a zero value as if it were configured", alias)
	}
}

func TestGeneratedBaselineFileSurfaceIsStable(t *testing.T) {
	path := "injected/config.toml"
	files := &injectedFileStore{path: path, files: make(map[string][]byte)}
	store := config.Store{Files: files}
	require.NoError(t, store.EnsureDefaultConfigFile())

	body := string(files.files[path])
	_, err := tomledit.Parse(bytes.NewReader([]byte(body)))
	require.NoError(t, err)

	// 进入初始文件的键（DefaultInFile）。
	for _, fragment := range []string{
		"[download]", `path = "./downloads"`, `filename_template = "{author} - {title}_{id}"`,
		"[output]", "json = false",
		"[login]", "open_browser = true", "use_after_login = false",
		"[update]", "check_enabled = true",
		"[logging]", `level = "info"`, `format = "text"`,
		"[reverse_search]", `provider = "saucenao"`, "pixiv_only = true",
	} {
		assert.Containsf(t, body, fragment, "generated baseline lost %q", fragment)
	}

	// 不进入初始文件的键：未声明 example、无默认值或敏感。
	for _, fragment := range []string{
		"directory_template",
		"request_interval",
		"https_proxy",
		"saucenao_api_key",
		"account_pool",
		"relay_",
	} {
		assert.NotContainsf(t, body, fragment, "generated baseline must stay compact: %q leaked in", fragment)
	}
	assert.NotContains(t, body, "api_key")

	// 已有文件不被覆盖。
	before := append([]byte(nil), files.files[path]...)
	files.files[path] = []byte("# user content\n")
	require.NoError(t, store.EnsureDefaultConfigFile())
	assert.Equal(t, "# user content\n", string(files.files[path]), "existing config file must never be overwritten")
	files.files[path] = before
}

func TestOnlySauceNAOKeyIsSensitiveAndStaysOutOfPublicOutput(t *testing.T) {
	for _, alias := range config.ValidSettingAliases() {
		wantSensitive := alias == "saucenao_api_key"
		assert.Equalf(t, wantSensitive, config.IsSensitiveSetting(alias), "sensitivity of %q changed", alias)
	}

	assert.Equal(t, "<redacted>", config.PublicSettingText("saucenao_api_key", "must-not-leak"))
	assert.Equal(t, "", config.PublicSettingText("saucenao_api_key", ""))
	assert.Equal(t, "info", config.PublicSettingText("log_level", "info"))

	// 敏感值不进入公开输出的 Store 路径：环境覆盖只报告"有覆盖"而不报告值。
	t.Setenv("SAUCENAO_API_KEY", "must-not-leak")
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	result, err := (config.Store{Files: files}).Set("saucenao_api_key", "file-key")
	require.NoError(t, err)
	assert.True(t, result.HasOverride)
	assert.Empty(t, result.EnvOverride)

	result, err = (config.Store{Files: files}).Unset("saucenao_api_key")
	require.NoError(t, err)
	assert.True(t, result.HasOverride)
	assert.Empty(t, result.EnvOverride)
}

func TestParseSettingInputNormalizesExactlyAsBefore(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		value, _, err := config.ParseSettingInput("download_path", "  ./out  ")
		require.NoError(t, err)
		assert.Equal(t, "./out", value.Value)
		assert.Equal(t, "cli", value.Source)
	})

	t.Run("bool accepts only parseable values", func(t *testing.T) {
		value, _, err := config.ParseSettingInput("output_json", "true")
		require.NoError(t, err)
		assert.Equal(t, true, value.Value)
		_, _, err = config.ParseSettingInput("output_json", "yes")
		require.Error(t, err)
	})

	t.Run("duration is normalized to its canonical string", func(t *testing.T) {
		value, _, err := config.ParseSettingInput("request_interval", "90s")
		require.NoError(t, err)
		assert.Equal(t, 90*time.Second, value.Value)
		assert.Equal(t, "1m30s", value.Text)
	})

	t.Run("negative interval is rejected", func(t *testing.T) {
		_, _, err := config.ParseSettingInput("request_interval", "-1s")
		require.EqualError(t, err, "request_interval must not be negative")
	})

	t.Run("unknown key is rejected", func(t *testing.T) {
		_, _, err := config.ParseSettingInput("not_a_key", "x")
		require.EqualError(t, err, `unknown config key "not_a_key"`)
	})
}
