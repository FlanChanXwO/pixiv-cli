package settings_test

import (
	"encoding/json"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRuntimeAccountPoolUsesOnlyCurrentSettings(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
		want    config.AccountPoolConfig
	}{
		{
			name: "disabled by default",
			want: config.AccountPoolConfig{Strategy: config.AccountPoolStrategyRoundRobin},
		},
		{name: "enabled round robin", body: "[account_pool]\nenabled = true\nstrategy = 'round_robin'\n", want: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin}},
		{name: "random", body: "[account_pool]\nenabled = true\nstrategy = 'random'\n", want: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRandom}},
		{name: "enabled without accounts uses all local accounts", body: "[account_pool]\nenabled = true\n", want: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin}},
		{name: "unknown strategy", body: "[account_pool]\nstrategy = 'weighted'\n", wantErr: "account_pool.strategy must be one of: round_robin, random"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if test.body != "" {
				require.NoError(t, os.WriteFile(path, []byte(test.body), 0o600))
			}
			state, err := config.LoadSnapshotAt(path)
			require.NoError(t, err)
			runtime, err := state.Runtime()
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, runtime.AccountPool)
		})
	}
}

func TestRuntimeDefaultsReverseSearchSettings(t *testing.T) {
	// 隔离环境变量：若测试进程已导出 SAUCENAO_API_KEY，LoadSnapshotAt 会将其写入快照，
	// 导致下方 SauceNAOAPIKey 空值断言失败。
	t.Setenv("SAUCENAO_API_KEY", "")
	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "saucenao", runtimeConfig.ReverseSearchProvider)
	require.True(t, runtimeConfig.ReverseSearchPixivOnly)
	require.Empty(t, runtimeConfig.SauceNAOAPIKey)
}

func TestReverseSearchProviderAcceptsOnlySupportedValues(t *testing.T) {
	for _, provider := range []string{"saucenao", "ascii2d-color", "ascii2d-bovw", "all"} {
		t.Run(provider, func(t *testing.T) {
			value, _, err := config.ParseSettingInput("reverse_search_provider", provider)
			require.NoError(t, err)
			require.Equal(t, provider, value.Value)
		})
	}

	_, _, err := config.ParseSettingInput("reverse_search_provider", "unknown")
	require.EqualError(t, err, "reverse_search_provider must be one of: saucenao, ascii2d-color, ascii2d-bovw, all")
}

func TestSauceNAOAPIKeyEnvironmentOverridesFileInSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[reverse_search]\nsaucenao_api_key = 'file-key'\n"), 0o600))
	t.Setenv("SAUCENAO_API_KEY", "environment-key")

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	t.Setenv("SAUCENAO_API_KEY", "changed-after-snapshot")

	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "environment-key", runtimeConfig.SauceNAOAPIKey)
}

func TestAccountPoolAliasesExposeOnlyRuntimeSettings(t *testing.T) {
	for _, alias := range []string{"account_pool_enabled", "account_pool_strategy"} {
		_, exists := config.SettingSpecByAlias(alias)
		require.Truef(t, exists, "%s must be a supported config alias", alias)
	}
	spec, exists := config.SettingSpecByAlias("account_pool_accounts")
	require.True(t, exists, "legacy UID allowlist must remain discoverable for cleanup")
	require.True(t, spec.Removed, "legacy UID allowlist must be a removed setting")
}

func TestRemovedLegacyAccountPoolKeyFailsAndCanBeUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[account_pool]\nenabled=true\naccounts=[11,22]\nstrategy='random'\n"), 0o600))
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.ErrorIs(t, err, config.ErrRemovedSetting)
	require.Contains(t, err.Error(), "account_pool_accounts")

	removed, err := config.UnsetConfigValue(path, "account_pool_accounts")
	require.NoError(t, err)
	require.True(t, removed)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(body), "accounts")
	require.Contains(t, string(body), "enabled")
	require.Contains(t, string(body), "strategy")
	state, err = config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
}

func TestPremiumStatusCacheTTLIsNoLongerAConfigSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Effective("premium_status_cache_ttl")
	require.EqualError(t, err, `unknown config key "premium_status_cache_ttl"`)

	require.NoError(t, os.WriteFile(path, []byte("[premium]\nstatus_cache_ttl = \"3h\"\n"), 0o600))
	state, err = config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
}

func TestDownloadSettingsExposeDirectoryTemplateAndRequestInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[download]\ndirectory_template = \"{author}/{date}\"\n[network]\nrequest_interval = \"2s\"\n"), 0o600))
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "{author}/{date}", runtime.DirectoryTemplate)
	require.Equal(t, 2*time.Second, runtime.RequestInterval)

	t.Setenv("PIXIV_REQUEST_INTERVAL", "3s")
	runtime, err = state.Runtime()
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, runtime.RequestInterval)
	state, err = config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err = state.Runtime()
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, runtime.RequestInterval)
}

func TestRequestIntervalRejectsNegativeDuration(t *testing.T) {
	_, _, err := config.ParseSettingInput("request_interval", "-1s")
	require.EqualError(t, err, "request_interval must not be negative")

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[network]\nrequest_interval = \"-1s\"\n"), 0o600))
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.EqualError(t, err, "request_interval must not be negative")
}

func TestRuntimePreservesReverseSearchNetworkAndFlareSolverrSeparately(t *testing.T) {
	withoutProxyEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[network]
https_proxy = "http://global-proxy"

[reverse_search.network]
proxy_url = ""
user_agent = "custom-reverse-search-agent"

[reverse_search.flaresolverr]
url = "http://reverse-solver.example"
proxy_url = "socks5://reverse-solver-proxy.example"

[fanbox.network]
proxy_url = "http://fanbox-proxy.example"
user_agent = "custom-fanbox-agent"

[fanbox.flaresolverr]
url = "http://fanbox-solver.example"
proxy_url = "socks5://fanbox-solver-proxy.example"
`), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)

	require.Equal(t, "http://global-proxy", runtime.HTTPSProxy)
	require.True(t, runtime.ReverseSearchNetwork.ProxyURL.Present)
	require.Empty(t, runtime.ReverseSearchNetwork.ProxyURL.Value)
	require.True(t, runtime.ReverseSearchNetwork.UserAgent.Present)
	require.Equal(t, "custom-reverse-search-agent", runtime.ReverseSearchNetwork.UserAgent.Value)
	require.NotNil(t, runtime.ReverseSearchFlareSolverr)
	require.Equal(t, "http://reverse-solver.example", runtime.ReverseSearchFlareSolverr.URL)
	require.Equal(t, "socks5://reverse-solver-proxy.example", runtime.ReverseSearchFlareSolverr.ProxyURL)

	require.Equal(t, "http://fanbox-proxy.example", runtime.FanboxNetwork.ProxyURL.Value)
	require.Equal(t, "custom-fanbox-agent", runtime.FanboxNetwork.UserAgent.Value)
	require.NotNil(t, runtime.FanboxFlareSolverr)
	require.Equal(t, "http://fanbox-solver.example", runtime.FanboxFlareSolverr.URL)
	require.Equal(t, "socks5://fanbox-solver-proxy.example", runtime.FanboxFlareSolverr.ProxyURL)
}

func TestRuntimeRejectsMalformedReverseSearchAdvancedValues(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "network proxy must be a string",
			body:    "[reverse_search.network]\nproxy_url = 42\n",
			wantErr: "reverse_search.network.proxy_url must be a string",
		},
		{
			name:    "network user agent must be a string",
			body:    "[reverse_search.network]\nuser_agent = 42\n",
			wantErr: "reverse_search.network.user_agent must be a string",
		},
		{
			name:    "solver proxy must be a string",
			body:    "[reverse_search.flaresolverr]\nurl = 'http://solver.example'\nproxy_url = 42\n",
			wantErr: "reverse_search.flaresolverr.proxy_url must be a string",
		},
		{
			name:    "solver URL is required",
			body:    "[reverse_search.flaresolverr]\nproxy_url = 'socks5://solver-proxy.example'\n",
			wantErr: "reverse_search.flaresolverr.url must be set when reverse_search.flaresolverr is configured",
		},
		{
			name:    "solver URL cannot be blank",
			body:    "[reverse_search.flaresolverr]\nurl = '   '\n",
			wantErr: "reverse_search.flaresolverr.url must be set when reverse_search.flaresolverr is configured",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(test.body), 0o600))
			state, err := config.LoadSnapshotAt(path)
			require.NoError(t, err)
			_, err = state.Runtime()
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestRuntimePreservesServiceNetworkPresenceAndValues(t *testing.T) {
	withoutProxyEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[network]
https_proxy = "http://global-proxy"

[fanbox.network]
proxy_url = ""
user_agent = "custom-fanbox-agent"

[fanbox.flaresolverr]
url = "http://solver.example"
proxy_url = "socks5://solver-proxy.example"
`), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)

	require.Equal(t, "http://global-proxy", runtime.HTTPSProxy)
	require.False(t, runtime.PixivNetwork.ProxyURL.Present)
	require.True(t, runtime.FanboxNetwork.ProxyURL.Present)
	require.Empty(t, runtime.FanboxNetwork.ProxyURL.Value)
	require.True(t, runtime.FanboxNetwork.UserAgent.Present)
	require.Equal(t, "custom-fanbox-agent", runtime.FanboxNetwork.UserAgent.Value)
	require.NotNil(t, runtime.FanboxFlareSolverr)
	require.Equal(t, "http://solver.example", runtime.FanboxFlareSolverr.URL)
	require.Equal(t, "socks5://solver-proxy.example", runtime.FanboxFlareSolverr.ProxyURL)
}

func TestRuntimeLeavesOptionalServiceNetworkTablesAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://global-proxy\"\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)

	require.False(t, runtime.PixivNetwork.ProxyURL.Present)
	require.False(t, runtime.FanboxNetwork.ProxyURL.Present)
	require.False(t, runtime.FanboxNetwork.UserAgent.Present)
	require.False(t, runtime.ReverseSearchNetwork.ProxyURL.Present)
	require.False(t, runtime.ReverseSearchNetwork.UserAgent.Present)
	require.Nil(t, runtime.FanboxFlareSolverr)
	require.Nil(t, runtime.ReverseSearchFlareSolverr)
}

func TestRuntimeRejectsMalformedAdvancedNetworkValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[fanbox.network]\nproxy_url = 42\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.EqualError(t, err, "fanbox.network.proxy_url must be a string")
}

// withoutProxyEnvironment removes any proxy variables exported by the host
// shell so the assertions reflect the configuration file alone. The existing
// environment-over-file precedence contract is unchanged; this only isolates
// the test from the machine running it.
func withoutProxyEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"https_proxy", "HTTPS_PROXY"} {
		value, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		t.Setenv(name, value)
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestSetAndUnsetConfigValuePreserveDocumentAndApplyPlatformPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pixiv")
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	original := "# keep this comment\n[download]\npath = './old'\n\n[custom]\nkey = 'keep'\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	value, err := parser.ParseValue("'./new'")
	require.NoError(t, err)

	require.NoError(t, config.SetConfigValue(path, "download_path", value))
	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	effective, err := state.Effective("download_path")
	require.NoError(t, err)
	assert.Equal(t, "./new", effective.Value)
	assertConfigPersistence(t, path)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "# keep this comment")
	assert.Contains(t, string(body), "[custom]\nkey = 'keep'")

	removed, err := config.UnsetConfigValue(path, "download_path")
	require.NoError(t, err)
	assert.True(t, removed)
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "[download]")
	assert.Contains(t, string(body), "[custom]\nkey = 'keep'")
	assertConfigPersistence(t, path)
}

func assertConfigPersistence(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Windows mode bits 不代表 DACL；首次创建继承父目录 ACL，替换保留既有 target ACL。
	} else {
		assert.Equal(t, os.FileMode(paths.PrivateFileMode), info.Mode().Perm())
		assert.Equal(t, os.FileMode(paths.PrivateDirMode), parent.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestRuntimeConfigSurfaceExcludesRefreshToken(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[auth]\nrefresh_token = \"must-not-enter-runtime\"\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)

	encoded, err := json.Marshal(runtimeConfig)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "must-not-enter-runtime", "TOML refresh token entered the runtime DTO")
	var surface map[string]any
	require.NoError(t, json.Unmarshal(encoded, &surface))
	for key := range surface {
		require.NotEqual(t, "refreshtoken", normalizedSensitiveKey(key), "runtime DTO exposes a refresh token field")
	}
	for _, alias := range config.ValidSettingAliases() {
		require.NotEqual(t, "refreshtoken", normalizedSensitiveKey(alias), "configuration aliases expose a refresh token setting")
	}
	_, ok := config.SettingSpecByAlias("refresh_token")
	require.False(t, ok, "refresh token must not be writable through the TOML setting surface")
}

func normalizedSensitiveKey(key string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
}

func TestRuntimeReadsLoggingConfigAndEnvironmentOverrides(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[logging]\nlevel = \"debug\"\nformat = \"json\"\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "debug", runtimeConfig.LogLevel)
	require.Equal(t, "json", runtimeConfig.LogFormat)

	t.Setenv("PIXIV_LOG_LEVEL", "info")
	t.Setenv("PIXIV_LOG_FORMAT", "text")
	state, err = config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err = state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "info", runtimeConfig.LogLevel)
	require.Equal(t, "text", runtimeConfig.LogFormat)
}

func TestLoggingSettingsRejectUnsupportedValues(t *testing.T) {
	for _, test := range []struct {
		alias string
		key   string
		raw   string
		want  string
	}{
		{alias: "log_level", key: "level", raw: "trace", want: "log_level must be one of: info, debug"},
		{alias: "log_format", key: "format", raw: "yaml", want: "log_format must be one of: text, json"},
	} {
		t.Run(test.alias, func(t *testing.T) {
			_, _, err := config.ParseSettingInput(test.alias, test.raw)
			require.EqualError(t, err, test.want)

			path := t.TempDir() + "/config.toml"
			require.NoError(t, os.WriteFile(path, []byte("[logging]\n"+test.key+" = \""+test.raw+"\"\n"), 0o600))
			state, err := config.LoadSnapshotAt(path)
			require.NoError(t, err)
			_, err = state.Runtime()
			require.EqualError(t, err, test.want)
		})
	}
}

// 历史 relay 的 shared-secret 字段曾属于私有配置。升级后保留文件内容不报错，
// 但运行时不得读取它们或恢复任何旧转发行为。
func TestRuntimeSilentlyIgnoresLegacySharedSecretRelayConfiguration(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[login]\nrelay_public_url = \"http://relay.example\"\nrelay_listen_addr = \"127.0.0.1:8080\"\nrelay_secret = \"obsolete-secret\"\nrelay_target_url = \"http://old-client.example\"\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "http://relay.example", runtimeConfig.LoginRelayPublicURL)
	require.Equal(t, "127.0.0.1:8080", runtimeConfig.LoginRelayListenAddr)
	_, hasSecret := config.SettingSpecByAlias("login_relay_secret")
	_, hasTarget := config.SettingSpecByAlias("login_relay_target_url")
	require.False(t, hasSecret)
	require.False(t, hasTarget)
}
