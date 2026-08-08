package auth

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredRelayServerOptionsIgnoreLegacyClientRelaySettings(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[login]\nrelay_secret = \"legacy-secret\"\nrelay_target_url = \"http://127.0.0.1:1\"\n")))

	settings, err := config.LoadSettingsState()
	require.NoError(t, err)
	runtime, err := settings.Runtime()
	require.NoError(t, err)

	opts, enabled, err := configuredRelayServerOptions(changedFlags{}, accountLoginOptions{}, runtime)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Empty(t, opts)
}

func TestConfiguredRelayServerOptionsRequirePublicURLAndListener(t *testing.T) {
	_, enabled, err := configuredRelayServerOptions(changedFlags{}, accountLoginOptions{}, config.RuntimeConfig{LoginRelayPublicURL: "https://relay.example"})
	require.EqualError(t, err, "remote login relay requires login_relay_public_url and login_relay_listen_addr")
	assert.False(t, enabled)
}

type changedFlags map[string]bool

func (f changedFlags) Changed(name string) bool { return f[name] }
