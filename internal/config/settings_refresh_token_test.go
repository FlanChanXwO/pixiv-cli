package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigSurfaceExcludesRefreshToken(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[auth]\nrefresh_token = \"must-not-enter-runtime\"\n"), 0o600))

	state, err := LoadSettingsStateAt(path)
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
	for _, alias := range ValidSettingAliases() {
		require.NotEqual(t, "refreshtoken", normalizedSensitiveKey(alias), "configuration aliases expose a refresh token setting")
	}
	_, ok := SettingSpecByAlias("refresh_token")
	require.False(t, ok, "refresh token must not be writable through the TOML setting surface")
}

func normalizedSensitiveKey(key string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
}

func TestRefreshTokenFromEnvRejectsCookie(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "refresh_token=secret")
	_, err := RefreshTokenFromEnv()
	require.ErrorContains(t, err, "cookie input is not supported; provide a Pixiv App API refresh token")
}

func TestRefreshTokenFromEnvAcceptsOpaqueToken(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "opaque=value")
	token, err := RefreshTokenFromEnv()
	require.NoError(t, err)
	require.Equal(t, "opaque=value", token)
}
