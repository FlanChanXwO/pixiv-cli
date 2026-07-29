package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPremiumStatusCacheTTLIsNoLongerAConfigSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	_, err = state.Effective("premium_status_cache_ttl")
	require.EqualError(t, err, `unknown config key "premium_status_cache_ttl"`)

	require.NoError(t, os.WriteFile(path, []byte("[premium]\nstatus_cache_ttl = \"3h\"\n"), 0o600))
	state, err = LoadSettingsStateAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
}
