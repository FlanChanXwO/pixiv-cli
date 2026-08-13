package config_test

import (
	"os"
	"path/filepath"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/stretchr/testify/require"
)

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
	require.Nil(t, runtime.FanboxFlareSolverr)
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
