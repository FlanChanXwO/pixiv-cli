package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultSurface records each alias's Runtime projection at defaults; the
// Effective source semantics are covered by TestDefaultDeclarationDistinguishesFalseZeroAndEmpty.
var defaultSurface = map[string]any{
	"download_path":             "./downloads",
	"filename_template":         "{author} - {title}_{id}",
	"directory_template":        "",
	"output_json":               false,
	"login_open_browser":        true,
	"login_use_after_login":     false,
	"update_check_enabled":      true,
	"log_level":                 "info",
	"log_format":                "text",
	"https_proxy":               "",
	"request_interval":          time.Duration(0),
	"reverse_search_provider":   "saucenao",
	"reverse_search_pixiv_only": true,
	"saucenao_api_key":          "",
	"login_relay_public_url":    "",
	"login_relay_listen_addr":   "",
	"login_relay_tls_cert_file": "",
	"login_relay_tls_key_file":  "",
}

func TestRuntimeDefaultSurfaceIsStable(t *testing.T) {
	clearSettingEnvironment(t)
	state, err := currentSnapshot(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	runtime, err := state.Runtime()
	require.NoError(t, err)

	assert.Equal(t, defaultSurface["download_path"], runtime.DownloadPath)
	assert.Equal(t, defaultSurface["filename_template"], runtime.FilenameTemplate)
	assert.Equal(t, defaultSurface["directory_template"], runtime.DirectoryTemplate)
	assert.Equal(t, defaultSurface["https_proxy"], runtime.HTTPSProxy)
	assert.Equal(t, defaultSurface["log_level"], string(runtime.LogLevel))
	assert.Equal(t, defaultSurface["log_format"], string(runtime.LogFormat))
	assert.Equal(t, defaultSurface["output_json"], runtime.OutputJSON)
	assert.Equal(t, defaultSurface["login_open_browser"], runtime.LoginOpenBrowser)
	assert.Equal(t, defaultSurface["login_use_after_login"], runtime.LoginUseAfterLogin)
	assert.Equal(t, defaultSurface["update_check_enabled"], runtime.UpdateCheckEnabled)
	assert.Equal(t, defaultSurface["reverse_search_provider"], runtime.ReverseSearchProvider)
	assert.Equal(t, defaultSurface["reverse_search_pixiv_only"], runtime.ReverseSearchPixivOnly)
	assert.Equal(t, defaultSurface["saucenao_api_key"], runtime.SauceNAOAPIKey)
	assert.Equal(t, defaultSurface["request_interval"], runtime.RequestInterval)
	assert.Equal(t, defaultSurface["login_relay_public_url"], runtime.LoginRelayPublicURL)
	assert.Equal(t, defaultSurface["login_relay_listen_addr"], runtime.LoginRelayListenAddr)
	assert.Equal(t, defaultSurface["login_relay_tls_cert_file"], runtime.LoginRelayTLSCertFile)
	assert.Equal(t, defaultSurface["login_relay_tls_key_file"], runtime.LoginRelayTLSKeyFile)
	assert.False(t, runtime.AccountPool.Enabled, "account pool must stay disabled by default")
	assert.Equal(t, config.AccountPoolStrategyRoundRobin, runtime.AccountPool.Strategy)
}

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
		{name: "enabled must be a boolean", body: "[account_pool]\nenabled = 'yes'\n", wantErr: "account_pool.enabled must be a boolean"},
		{name: "strategy must be a string", body: "[account_pool]\nstrategy = 123\n", wantErr: "account_pool.strategy must be one of: round_robin, random"},
		{name: "unknown strategy", body: "[account_pool]\nstrategy = 'weighted'\n", wantErr: "account_pool.strategy must be one of: round_robin, random"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if test.body != "" {
				require.NoError(t, os.WriteFile(path, []byte(test.body), 0o600))
			}
			state, err := currentSnapshot(path)
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

func TestSauceNAOAPIKeyEnvironmentOverridesFileInSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[reverse_search]\nsaucenao_api_key = 'file-key'\n"), 0o600))
	t.Setenv("SAUCENAO_API_KEY", "environment-key")

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	t.Setenv("SAUCENAO_API_KEY", "changed-after-snapshot")

	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "environment-key", runtimeConfig.SauceNAOAPIKey)
}

func TestDownloadSettingsExposeDirectoryTemplateAndRequestInterval(t *testing.T) {
	clearSettingEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[download]\ndirectory_template = \"{author}/{date}\"\n[network]\nrequest_interval = \"2s\"\n"), 0o600))
	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "{author}/{date}", runtime.DirectoryTemplate)
	require.Equal(t, 2*time.Second, runtime.RequestInterval)

}

func TestRuntimeReadsLoggingConfigFromFile(t *testing.T) {
	clearSettingEnvironment(t)
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[logging]\nlevel = \"debug\"\nformat = \"json\"\n"), 0o600))

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, config.LogLevelDebug, runtimeConfig.LogLevel)
	require.Equal(t, config.LogFormatJSON, runtimeConfig.LogFormat)

}

// TestRuntimePreservesReverseSearchNetworkAndFlareSolverrSeparately covers the
// private advanced groups end to end.
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

`), 0o600))

	state, err := currentSnapshot(path)
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
}

// TestPixivNetworkRejectsUserAgentWhileFanboxKeepsIt covers the structural
// guard: only the narrow Pixiv type omits UserAgent.
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

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)

	assert.True(t, runtime.PixivNetwork.ProxyURL.Present)
	assert.Equal(t, "http://pixiv-proxy", runtime.PixivNetwork.ProxyURL.Value)
	// A user_agent under pixiv must be ignored, not held as a new capability:
	// the serialized runtime is the observable surface for that guarantee.
	encoded, err := json.Marshal(runtime)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "must-not-be-accepted")

	assert.True(t, runtime.FanboxNetwork.UserAgent.Present)
	assert.Equal(t, "fanbox-agent", runtime.FanboxNetwork.UserAgent.Value)
	assert.True(t, runtime.ReverseSearchNetwork.UserAgent.Present)
	assert.Equal(t, "reverse-search-agent", runtime.ReverseSearchNetwork.UserAgent.Value)
}

// TestPixivNetworkDistinguishesAbsentFromExplicitEmpty keeps "absent inherits
// the global proxy" distinct from "explicit empty means direct".
func TestPixivNetworkDistinguishesAbsentFromExplicitEmpty(t *testing.T) {
	withoutProxyEnvironment(t)

	t.Run("absent means inherit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://global\"\n"), 0o600))
		state, err := currentSnapshot(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.False(t, runtime.PixivNetwork.ProxyURL.Present, "absent pixiv proxy must stay unset so the global proxy applies")
	})

	t.Run("explicit empty means direct", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://global\"\n\n[pixiv.network]\nproxy_url = \"\"\n"), 0o600))
		state, err := currentSnapshot(path)
		require.NoError(t, err)
		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.True(t, runtime.PixivNetwork.ProxyURL.Present, "an explicit empty pixiv proxy means direct access, not inheritance")
		assert.Empty(t, runtime.PixivNetwork.ProxyURL.Value)
	})
}

// TestRuntimeRejectsMalformedAdvancedValues covers strict typing and solver
// enablement for the private groups.
func TestRuntimeRejectsMalformedAdvancedValues(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "pixiv proxy must be a string", body: "[pixiv.network]\nproxy_url = 42\n", wantErr: "pixiv.network.proxy_url must be a string"},
		{name: "pixiv proxy rejects a bool", body: "[pixiv.network]\nproxy_url = true\n", wantErr: "pixiv.network.proxy_url must be a string"},
		{name: "pixiv proxy rejects a nested table", body: "[pixiv.network.proxy_url]\nvalue = \"x\"\n", wantErr: "pixiv.network.proxy_url must be a string"},
		{name: "reverse network proxy must be a string", body: "[reverse_search.network]\nproxy_url = 42\n", wantErr: "reverse_search.network.proxy_url must be a string"},
		{name: "reverse network user agent must be a string", body: "[reverse_search.network]\nuser_agent = 42\n", wantErr: "reverse_search.network.user_agent must be a string"},
		{name: "solver proxy must be a string", body: "[reverse_search.flaresolverr]\nurl = 'http://solver.example'\nproxy_url = 42\n", wantErr: "reverse_search.flaresolverr.proxy_url must be a string"},
		{name: "solver URL is required", body: "[reverse_search.flaresolverr]\nproxy_url = 'socks5://solver-proxy.example'\n", wantErr: "reverse_search.flaresolverr.url must be set when reverse_search.flaresolverr is configured"},
		{name: "solver URL cannot be blank", body: "[reverse_search.flaresolverr]\nurl = '   '\n", wantErr: "reverse_search.flaresolverr.url must be set when reverse_search.flaresolverr is configured"},
		{name: "fanbox network proxy must be a string", body: "[fanbox.network]\nproxy_url = 42\n", wantErr: "fanbox.network.proxy_url must be a string"},
		{name: "fanbox solver URL is required", body: "[fanbox.flaresolverr]\nproxy_url = \"socks5://solver-proxy.example\"\n", wantErr: "fanbox.flaresolverr.url must be set when fanbox.flaresolverr is configured"},
		{name: "fanbox blank solver URL", body: "[fanbox.flaresolverr]\nurl = \"   \"\n", wantErr: "fanbox.flaresolverr.url must be set when fanbox.flaresolverr is configured"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(test.body), 0o600))
			state, err := currentSnapshot(path)
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

	state, err := currentSnapshot(path)
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

	state, err := currentSnapshot(path)
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

// TestLoggingSettingsRejectUnsupportedValues checks both the CLI parser and the
// file path use the same domain errors.
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
			_, _, err := config.ParseSettingInput(config.Key(test.alias), test.raw)
			require.EqualError(t, err, test.want)

			path := t.TempDir() + "/config.toml"
			require.NoError(t, os.WriteFile(path, []byte("[logging]\n"+test.key+" = \""+test.raw+"\"\n"), 0o600))
			state, err := currentSnapshot(path)
			require.NoError(t, err)
			_, err = state.Runtime()
			require.EqualError(t, err, test.want)
		})
	}
}

func TestGalleryCacheSettings(t *testing.T) {
	for _, raw := range []string{"true", "1.5", "'256'", "-1", "8796093022208"} {
		p := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(p, []byte("[mcp]\ngallery_cache_capacity_mib = "+raw+"\n"), 0o600))
		s, err := currentSnapshot(p)
		require.NoError(t, err)
		_, err = s.Runtime()
		require.Error(t, err, raw)
	}
	// Zero disables the cache and is accepted.
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(p, []byte("[mcp]\ngallery_cache_capacity_mib = 0\n"), 0o600))
	s, err := currentSnapshot(p)
	require.NoError(t, err)
	_, err = s.Runtime()
	require.NoError(t, err)

	for alias, want := range map[string]string{"mcp_gallery_animation_temp_capacity_mib": "1024", "mcp_gallery_cache_capacity_mib": "256"} {
		value, _, err := config.ParseSettingInput(config.Key(alias), want)
		require.NoError(t, err)
		require.Equal(t, want, value.Text)
	}
}

func TestRuntimeGallerySettingsBindDeclaredValues(t *testing.T) {
	for _, tc := range []struct {
		name, toml     string
		capacity, temp int64
	}{
		{"defaults", "", 256, 1024},
		{"configured", "[mcp]\ngallery_cache_capacity_mib=0\ngallery_animation_temp_capacity_mib=512\n", 0, 512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tc.toml), 0o600))
			snapshot, err := currentSnapshot(path)
			require.NoError(t, err)
			cfg, err := snapshot.Runtime()
			require.NoError(t, err)
			require.Equal(t, tc.capacity, cfg.MCPGalleryCacheCapacityMiB)
			require.Equal(t, tc.temp, cfg.MCPGalleryAnimationTempCapacityMiB)
		})
	}
}

// TestRuntimeSilentlyIgnoresLegacySharedSecretRelayConfiguration keeps old
// files loadable without restoring forwarding fields.
func TestRuntimeSilentlyIgnoresLegacySharedSecretRelayConfiguration(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[login]\nrelay_public_url = \"http://relay.example\"\nrelay_listen_addr = \"127.0.0.1:8080\"\nrelay_secret = \"obsolete-secret\"\nrelay_target_url = \"http://old-client.example\"\n"), 0o600))

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "http://relay.example", runtimeConfig.LoginRelayPublicURL)
	require.Equal(t, "127.0.0.1:8080", runtimeConfig.LoginRelayListenAddr)
	_, hasSecret := config.SettingSpecByAlias(config.Key("login_relay_secret"))
	_, hasTarget := config.SettingSpecByAlias(config.Key("login_relay_target_url"))
	require.False(t, hasSecret)
	require.False(t, hasTarget)
}

// withoutProxyEnvironment isolates the file-only view from host proxy exports.
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

// TestUnknownSectionsInExistingFilesDoNotBlockStartup covers upgrade
// compatibility: keys this version does not declare are ignored rather than
// rejected, including retired and MCP endpoint settings.
func TestUnknownSectionsInExistingFilesDoNotBlockStartup(t *testing.T) {
	for _, alias := range []config.Key{"mcp_listen_addr", "mcp_base_url", "premium_status_cache_ttl"} {
		_, ok := config.SettingSpecByAlias(alias)
		require.Falsef(t, ok, "%q must not be a setting", alias)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[mcp]
listen_addr = "0.0.0.0:8123"
base_url = "http://old.example"

[premium]
status_cache_ttl = "3h"
`), 0o600))
	state, err := currentSnapshot(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
}

// TestDefaultDeclarationDistinguishesFalseZeroAndEmpty runs through Effective:
// false and 0s are declared defaults, not "no default"; aliases without a
// default stay unset rather than borrowing zero.
func TestDefaultDeclarationDistinguishesFalseZeroAndEmpty(t *testing.T) {
	clearSettingEnvironment(t)
	state, err := currentSnapshot(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	for _, test := range []struct {
		alias string
		want  any
		text  string
	}{
		{alias: "output_json", want: false, text: "false"},
		{alias: "login_use_after_login", want: false, text: "false"},
		{alias: "account_pool_enabled", want: false, text: "false"},
		{alias: "request_interval", want: time.Duration(0), text: "0s"},
	} {
		t.Run(test.alias, func(t *testing.T) {
			value, err := state.Effective(config.Key(test.alias))
			require.NoError(t, err)
			assert.True(t, value.HasValue, "explicit false/0s default must be declared, not inferred from a non-zero check")
			assert.Equal(t, "default", value.Source)
			assert.Equal(t, test.want, value.Value)
			assert.Equal(t, test.text, value.Text)
		})
	}

	for _, alias := range []string{
		"directory_template", "https_proxy", "saucenao_api_key",
		"login_relay_public_url", "login_relay_listen_addr",
		"login_relay_tls_cert_file", "login_relay_tls_key_file",
	} {
		t.Run(alias, func(t *testing.T) {
			value, err := state.Effective(config.Key(alias))
			require.NoError(t, err)
			assert.False(t, value.HasValue, "an alias without a declared default must stay unset")
			assert.Nil(t, value.Value, "an absent default must not expose a zero value as if it were configured")
			assert.Equal(t, "unset", value.Source)
		})
	}
}

// authoritativeEnvNames is the published env binding list: names in priority
// order. It is independent of the field tags so a broken declaration cannot
// silently redefine the expectation.
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

// envProbeSentinels selects values that are valid for pass-through strings and
// invalid for typed settings, so "env was read" is observable either way.
var envProbeSentinels = map[string]string{
	"DOWNLOAD_PATH":          "./__env__/downloads",
	"FILENAME_TEMPLATE":      "__env__{id}",
	"DIRECTORY_TEMPLATE":     "__env__/{author}",
	"PIXIV_REQUEST_INTERVAL": "__env__not-a-duration",
	"PIXIV_LOG_LEVEL":        "__env__not-a-level",
	"PIXIV_LOG_FORMAT":       "__env__not-a-format",
	"SAUCENAO_API_KEY":       "__env__api-key",
}

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

// clearSettingEnvironment removes configuration environment variables so an
// assertion reflects the file alone; t.Setenv restores the host values.
func clearSettingEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range envControlledVariables {
		if value, present := os.LookupEnv(name); present {
			t.Setenv(name, value)
		}
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestDeclaredEnvironmentBindingsAreExactlyTheDocumentedSet(t *testing.T) {
	clearSettingEnvironment(t)
	for name, sentinel := range envProbeSentinels {
		t.Setenv(name, sentinel)
	}

	state, err := currentSnapshot(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	for _, alias := range config.ValidSettingAliases() {
		if _, declared := authoritativeEnvNames[string(alias)]; declared {
			continue
		}
		value, err := state.Effective(alias)
		require.NoErrorf(t, err, "alias %q must not read any environment variable", alias)
		assert.NotEqualf(t, "env", value.Source, "alias %q must not read any environment variable", alias)
	}

	// Valid sentinels are delivered verbatim with an env source.
	for alias, want := range map[string]any{
		"download_path":      envProbeSentinels["DOWNLOAD_PATH"],
		"filename_template":  envProbeSentinels["FILENAME_TEMPLATE"],
		"directory_template": envProbeSentinels["DIRECTORY_TEMPLATE"],
	} {
		value, err := state.Effective(config.Key(alias))
		require.NoErrorf(t, err, "Effective(%q)", alias)
		assert.Equalf(t, want, value.Value, "alias %q did not deliver its declared environment value", alias)
		assert.Equal(t, "env", value.Source)
		assert.True(t, value.HasValue)
	}

	// Invalid sentinels must fail with the alias's own validation error.
	for alias, wantErr := range map[string]string{
		"log_level":        "log_level must be one of: info, debug",
		"log_format":       "log_format must be one of: text, json",
		"request_interval": `request_interval: time: invalid duration "__env__not-a-duration"`,
	} {
		_, err := state.Effective(config.Key(alias))
		require.EqualErrorf(t, err, wantErr, "alias %q must take its value from the declared environment variable", alias)
	}

	_, err = state.Runtime()
	require.Error(t, err, "Runtime must reject the sentinel environment values rather than falling back to defaults")
}

// TestEnvironmentPrecedenceKeepsDeclaredOrder pins the https_proxy order:
// lowercase wins, and an explicitly empty lowercase value stays selected.
func TestEnvironmentPrecedenceKeepsDeclaredOrder(t *testing.T) {
	t.Run("lowercase proxy wins over uppercase", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n"), 0o600))
		// Windows environment names are case-insensitive; write the uppercase
		// name first so it does not overwrite the lowercase value under test.
		t.Setenv("HTTPS_PROXY", "http://upper-proxy")
		t.Setenv("https_proxy", "http://lower-proxy")

		state, err := currentSnapshot(path)
		require.NoError(t, err)
		value, err := state.Effective(config.KeyHTTPSProxy)
		require.NoError(t, err)
		assert.Equal(t, "http://lower-proxy", value.Value)
		assert.Equal(t, "env", value.Source)

		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.Equal(t, "http://lower-proxy", runtime.HTTPSProxy)
	})

	t.Run("empty lowercase proxy wins over uppercase", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		t.Setenv("HTTPS_PROXY", "http://upper-proxy")
		t.Setenv("https_proxy", "")

		state, err := currentSnapshot(path)
		require.NoError(t, err)
		value, err := state.Effective(config.KeyHTTPSProxy)
		require.NoError(t, err)
		assert.Equal(t, "env", value.Source, "an explicitly present lowercase proxy must be selected, not skipped")
		assert.False(t, value.HasValue, "an empty value is present but not configured")
		assert.Nil(t, value.Value, "an empty proxy must not resolve to the uppercase fallback")

		runtime, err := state.Runtime()
		require.NoError(t, err)
		assert.Equal(t, "", runtime.HTTPSProxy, "the uppercase variable must not leak into runtime")
	})
}

func TestEnvironmentEmptyValueStillCountsAsConfigured(t *testing.T) {
	t.Run("empty env override is not treated as absent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[download]\npath = \"./from-file\"\n"), 0o600))
		t.Setenv("DOWNLOAD_PATH", "")

		state, err := currentSnapshot(path)
		require.NoError(t, err)
		value, err := state.Effective(config.KeyDownloadPath)
		require.NoError(t, err)
		assert.Equal(t, "", value.Value)
		assert.Equal(t, "env", value.Source, "an explicitly empty env value must win over the file")
	})

	t.Run("explicit empty file string is preserved", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[download]\ndirectory_template = \"\"\n"), 0o600))

		state, err := currentSnapshot(path)
		require.NoError(t, err)
		value, err := state.Effective(config.KeyDirectoryTemplate)
		require.NoError(t, err)
		// directory_template has no default, so an empty file string is unset.
		assert.False(t, value.HasValue)
		assert.Equal(t, "file", value.Source)
	})
}

// TestRuntimePreservesValidationOrder keeps the historical first-error order:
// field declaration order, then the web tombstone, then the pool's own checks.
func TestRuntimePreservesValidationOrder(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"interval before logging", "[network]\nrequest_interval='bad'\n[logging]\nlevel='bad'", `request_interval: time: invalid duration "bad"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			state, err := currentSnapshot(path)
			require.NoError(t, err)
			_, err = state.Runtime()
			require.EqualError(t, err, tc.want)
		})
	}
}

// TestRetiredKeysInExistingFilesAreIgnored keeps old config files loading:
// quiet deleted keys are unknown to this version, not startup failures.
func TestRetiredKeysInExistingFilesAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[web]\nfallback_enabled = true\n\n[account_pool]\naccounts = \"1,2\"\n\n[mcp]\ngallery_cache_ttl = \"3m\"\ngallery_concurrency = 2\n\n[download]\npath = \"./kept\"\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	assert.Equal(t, "./kept", runtime.DownloadPath)
	assert.False(t, runtime.AccountPool.Enabled)
}
