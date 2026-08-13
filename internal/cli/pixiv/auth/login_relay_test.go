package auth_test

import (
	"testing"

	auth "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredRelayServerOptionsIgnoreLegacyClientRelaySettings(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	require.NoError(t, filesecret.WritePrivateFile(configPath, []byte("[login]\nrelay_secret = \"legacy-secret\"\nrelay_target_url = \"http://127.0.0.1:1\"\n"), localstate.PrivateFileMode))

	settings, err := config.LoadSnapshot()
	require.NoError(t, err)
	runtime, err := settings.Runtime()
	require.NoError(t, err)

	opts, enabled, err := auth.ConfiguredRelayServerOptions(changedFlags{}, auth.AccountLoginOptions{}, runtime)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Empty(t, opts)
}

func TestConfiguredRelayServerOptionsRequirePublicURLAndListener(t *testing.T) {
	_, enabled, err := auth.ConfiguredRelayServerOptions(changedFlags{}, auth.AccountLoginOptions{}, config.RuntimeConfig{LoginRelayPublicURL: "https://relay.example"})
	require.EqualError(t, err, "remote login relay requires login_relay_public_url and login_relay_listen_addr")
	assert.False(t, enabled)
}

type changedFlags map[string]bool

func (f changedFlags) Changed(name string) bool { return f[name] }
