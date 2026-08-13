package config_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/stretchr/testify/require"
)

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

func TestRuntimeIgnoresLegacyLoggingConfiguration(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[logging]\nlevel = \"loud\"\n"), 0o600))
	t.Setenv("PIXIV_LOG_LEVEL", "debug")

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, config.DefaultDownloadPath, runtimeConfig.DownloadPath)
	_, ok := config.SettingSpecByAlias("log_level")
	require.False(t, ok)
}

// 历史 relay 的 shared-secret 字段曾属于私有配置。升级后保留文件内容不报错，
// 但运行时不得读取它们或恢复任何旧转发行为。
func TestRuntimeSilentlyIgnoresLegacySharedSecretRelayConfiguration(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte("[login]\nrelay_public_url = \"http://relay.example\"\nrelay_listen_addr = \"127.0.0.1:8080\"\nrelay_secret = \"obsolete-secret\"\nrelay_target_url = \"http://old-client.example\"\n"), 0o600))

	state, err := config.LoadSnapshotAt(path)
	require.NoError(t, err)
	runtimeConfig, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "http://relay.example", runtimeConfig.LoginRelayPublicURL)
	require.Equal(t, "127.0.0.1:8080", runtimeConfig.LoginRelayListenAddr)
	_, hasSecret := config.SettingSpecByAlias("login_relay_secret")
	_, hasTarget := config.SettingSpecByAlias("login_relay_target_url")
	require.False(t, hasSecret)
	require.False(t, hasTarget)
}
