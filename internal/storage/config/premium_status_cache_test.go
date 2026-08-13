package config_test

import (
	"os"
	"path/filepath"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/stretchr/testify/require"
)

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
