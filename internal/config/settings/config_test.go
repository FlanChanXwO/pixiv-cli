package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedAliases is the complete ValidSettingAliases set without tombstones.
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
	"mcp_gallery_animation_temp_capacity_mib",
	"mcp_gallery_cache_capacity_mib",
	"output_json",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
	"update_check_enabled",
}

// expectedCLIManagedAliases is the set exposed by pixiv config get/set/unset.
var expectedCLIManagedAliases = []string{
	"account_pool_enabled",
	"account_pool_strategy",
	"directory_template",
	"download_path",
	"filename_template",
	"https_proxy",
	"log_format",
	"log_level",
	"mcp_gallery_animation_temp_capacity_mib",
	"mcp_gallery_cache_capacity_mib",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
}

// expectedRemovedAliases stay resolvable for config unset but are not valid keys.
var expectedRemovedAliases = []string{"account_pool_accounts", "mcp_gallery_cache_ttl", "mcp_gallery_concurrency", "web_fallback_enabled"}

// allResolvableAliases includes tombstones for SettingSpecByAlias lookups.
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
	"mcp_gallery_animation_temp_capacity_mib",
	"mcp_gallery_cache_capacity_mib",
	"mcp_gallery_cache_ttl",
	"mcp_gallery_concurrency",
	"output_json",
	"request_interval",
	"reverse_search_pixiv_only",
	"reverse_search_provider",
	"saucenao_api_key",
	"update_check_enabled",
	"web_fallback_enabled",
}

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

// TestOnlySauceNAOKeyIsSensitiveAndStaysOutOfPublicOutput covers sensitivity
// itself; the baseline exclusion is in defaults_test.go and mutation results in
// store_test.go.
func TestOnlySauceNAOKeyIsSensitiveAndStaysOutOfPublicOutput(t *testing.T) {
	for _, alias := range config.ValidSettingAliases() {
		wantSensitive := alias == "saucenao_api_key"
		assert.Equalf(t, wantSensitive, config.IsSensitiveSetting(alias), "sensitivity of %q changed", alias)
	}

	assert.Equal(t, "<redacted>", config.PublicSettingText("saucenao_api_key", "must-not-leak"))
	assert.Equal(t, "", config.PublicSettingText("saucenao_api_key", ""))
	assert.Equal(t, "info", config.PublicSettingText("log_level", "info"))
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

func TestSettingSpecByAliasTableIsDetached(t *testing.T) {
	spec, ok := config.SettingSpecByAlias("download_path")
	if !ok || len(spec.Table) == 0 {
		t.Fatal("download_path must have a declared table")
	}
	spec.Table[0] = "mutated_table"

	again, ok := config.SettingSpecByAlias("download_path")
	if !ok || !reflect.DeepEqual(again.Table, []string{"download"}) {
		t.Errorf("returned Table mutation changed a later lookup: %v", again.Table)
	}

	_, value, err := config.ParseSettingInput("download_path", "./isolated")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.SetConfigValue(path, "download_path", value); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(body)), "[download]\npath = \"./isolated\""; got != want {
		t.Errorf("returned Table mutation changed a later config write: got %q, want %q", got, want)
	}
}
