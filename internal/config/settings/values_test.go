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
			value, _, err := config.ParseSettingInput("reverse_search_provider", provider)
			require.NoError(t, err)
			require.Equal(t, provider, value.Value)
		})
	}

	_, _, err := config.ParseSettingInput("reverse_search_provider", "unknown")
	require.EqualError(t, err, "reverse_search_provider must be one of: saucenao, ascii2d-color, ascii2d-bovw, all")
}

// TestRequestIntervalRejectsNegativeDuration covers both input paths: the CLI
// parser and a value read from the file.
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

	t.Run("unknown key is rejected", func(t *testing.T) {
		_, _, err := config.ParseSettingInput("not_a_key", "x")
		require.EqualError(t, err, `unknown config key "not_a_key"`)
	})
}

// TestRemovedAliasRejectsWrites covers the parser contract: tombstones stay
// resolvable for config unset but every write names the cleanup command.
func TestRemovedAliasRejectsWrites(t *testing.T) {
	for _, removed := range expectedRemovedAliases {
		spec, ok := config.SettingSpecByAlias(removed)
		require.True(t, ok, "removed alias %q must stay resolvable for config unset", removed)
		require.True(t, spec.Removed)

		_, _, err := config.ParseSettingInput(removed, "true")
		require.ErrorIs(t, err, config.ErrRemovedSetting)
		require.ErrorContains(t, err, "pixiv config unset "+removed)
	}
	_, ok := config.SettingSpecByAlias("premium_status_cache_ttl")
	require.False(t, ok, "retired setting premium_status_cache_ttl must not be reachable")
}
