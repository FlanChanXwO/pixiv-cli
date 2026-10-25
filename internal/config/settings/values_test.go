package settings_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReverseSearchProviderAcceptsOnlySupportedValues(t *testing.T) {
	for _, provider := range []string{"saucenao", "ascii2d-color", "ascii2d-bovw", "all"} {
		t.Run(provider, func(t *testing.T) {
			value, _, err := config.ParseSettingInput(config.KeyReverseSearchProvider, provider)
			require.NoError(t, err)
			require.Equal(t, provider, value.Value)
		})
	}

	_, _, err := config.ParseSettingInput(config.KeyReverseSearchProvider, "unknown")
	require.EqualError(t, err, "reverse_search_provider must be one of: saucenao, ascii2d-color, ascii2d-bovw, all")
}

// TestRequestIntervalRejectsNegativeDuration covers both input paths: the CLI
// parser and a value read from the file.
func TestRequestIntervalRejectsNegativeDuration(t *testing.T) {
	_, _, err := config.ParseSettingInput(config.KeyRequestInterval, "-1s")
	require.EqualError(t, err, "request_interval must not be negative")

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[network]\nrequest_interval = \"-1s\"\n"), 0o600))
	state, err := currentSnapshot(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.EqualError(t, err, "request_interval must not be negative")
}

func TestParseSettingInputNormalizesExactlyAsBefore(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		value, _, err := config.ParseSettingInput(config.KeyDownloadPath, "  ./out  ")
		require.NoError(t, err)
		assert.Equal(t, "./out", value.Value)
		assert.Equal(t, "cli", value.Source)
	})

	t.Run("bool accepts only parseable values", func(t *testing.T) {
		value, _, err := config.ParseSettingInput(config.KeyOutputJSON, "true")
		require.NoError(t, err)
		assert.Equal(t, true, value.Value)
		_, _, err = config.ParseSettingInput(config.KeyOutputJSON, "yes")
		require.Error(t, err)
	})

	t.Run("duration is normalized to its canonical string", func(t *testing.T) {
		value, _, err := config.ParseSettingInput(config.KeyRequestInterval, "90s")
		require.NoError(t, err)
		assert.Equal(t, 90*time.Second, value.Value)
		assert.Equal(t, "1m30s", value.Text)
	})

	t.Run("log level and format reject unsupported values", func(t *testing.T) {
		_, _, err := config.ParseSettingInput(config.KeyLogLevel, "trace")
		require.EqualError(t, err, "log_level must be one of: info, debug, warn, error")
		_, _, err = config.ParseSettingInput(config.KeyLogFormat, "yaml")
		require.EqualError(t, err, "log_format must be one of: text, json")
	})

	t.Run("unknown key is rejected", func(t *testing.T) {
		_, _, err := config.ParseSettingInput(config.Key("not_a_key"), "x")
		require.EqualError(t, err, `unknown config key "not_a_key"`)
	})
}

// TestRetiredAliasesAreUnreachable keeps deleted keys out of the writable surface.
func TestRetiredAliasesAreUnreachable(t *testing.T) {
	for _, retired := range []string{"web_fallback_enabled", "account_pool_accounts", "mcp_gallery_cache_ttl", "mcp_gallery_concurrency", "premium_status_cache_ttl"} {
		_, ok := config.SettingSpecByAlias(config.Key(retired))
		require.Falsef(t, ok, "retired setting %q must not be reachable", retired)
	}
}
