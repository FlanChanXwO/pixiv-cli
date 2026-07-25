package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPremiumStatusCacheTTLDefaultsToOneDayAndAcceptsConfigOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, 24*time.Hour, runtime.PremiumStatusCacheTTL)

	require.NoError(t, os.WriteFile(path, []byte("[premium]\nstatus_cache_ttl = \"3h\"\n"), 0o600))
	state, err = LoadSettingsStateAt(path)
	require.NoError(t, err)
	value, err := state.Effective("premium_status_cache_ttl")
	require.NoError(t, err)
	require.Equal(t, 3*time.Hour, value.Value)
	runtime, err = state.Runtime()
	require.NoError(t, err)
	require.Equal(t, 3*time.Hour, runtime.PremiumStatusCacheTTL)

	require.NoError(t, os.WriteFile(path, []byte("[premium]\nstatus_cache_ttl = \"-1h\"\n"), 0o600))
	state, err = LoadSettingsStateAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.EqualError(t, err, "premium_status_cache_ttl must be greater than or equal to zero")
}
