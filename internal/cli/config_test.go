package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPathGetSetUnset(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, configPath+"\n", stdout.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "config", "set", "download_path", "/tmp/pixiv"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "[download]")
	assert.Contains(t, string(body), `path = "/tmp/pixiv"`)
	assert.NotContains(t, string(body), "filename_template")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "config", "get", "download_path"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "/tmp/pixiv\n", stdout.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "config", "unset", "download_path"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "config", "get", "download_path"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, config.DefaultDownloadPath+"\n", stdout.String())
}

func TestConfigMissingValueReturnsPlaceholder(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "get", "https_proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Equal(t, configMissingPlaceholder+"\n", stdout.String())
	assert.Contains(t, stderr.String(), "unset")
}

func TestConfigStrongTypesAndSparseWrite(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "set", "output_json", "true"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	code = Run([]string{"pixiv", "config", "set", "login_timeout", "30s"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	code = Run([]string{"pixiv", "config", "set", "web_fallback_enabled", "false"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(config.DefaultConfigFileMode), info.Mode().Perm())

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "[output]")
	assert.Contains(t, string(body), "json = true")
	assert.Contains(t, string(body), "[login]")
	assert.Contains(t, string(body), `timeout = "30s"`)
	assert.Contains(t, string(body), "[web]")
	assert.Contains(t, string(body), "fallback_enabled = false")
	assert.NotContains(t, string(body), "download")
}

func TestConfigWebFallbackDefaultEnabled(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)

	settings, err := config.LoadSettingsState()
	require.NoError(t, err)
	cfg, err := settings.Runtime()
	require.NoError(t, err)
	assert.True(t, cfg.WebFallbackEnabled)
}

func TestConfigUpdateCheckDefaultEnabled(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)

	settings, err := config.LoadSettingsState()
	require.NoError(t, err)
	runtimeConfig, err := settings.Runtime()
	require.NoError(t, err)
	assert.True(t, runtimeConfig.UpdateCheckEnabled)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "get", "update_check_enabled"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "true\n", stdout.String())
}

func TestConfigUpdateCheckCanBeExplicitlyDisabled(t *testing.T) {
	t.Run("config file", func(t *testing.T) {
		clearConfigEnv(t)
		_, configPath := useTempPaths(t)
		require.NoError(t, config.WritePrivateFile(configPath, []byte("[update]\ncheck_enabled = false\n")))

		settings, err := config.LoadSettingsState()
		require.NoError(t, err)
		runtimeConfig, err := settings.Runtime()
		require.NoError(t, err)
		assert.False(t, runtimeConfig.UpdateCheckEnabled)
	})

	t.Run("config set", func(t *testing.T) {
		clearConfigEnv(t)
		_, configPath := useTempPaths(t)

		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "config", "set", "update_check_enabled", "false"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())

		body, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(body), "[update]")
		assert.Contains(t, string(body), "check_enabled = false")
	})
}

func TestConfigGetUsesEnvironmentOverrideAndSetWarns(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("HTTPS_PROXY", "http://env-proxy")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "get", "https_proxy"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "http://env-proxy\n", stdout.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "config", "set", "https_proxy", "http://written-proxy"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stderr.String(), "overridden by environment")

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), `https_proxy = "http://written-proxy"`)
}

func TestConfigLowercaseProxyBeatsUppercase(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)
	t.Setenv("HTTPS_PROXY", "http://upper")
	t.Setenv("https_proxy", "http://lower")

	settings, err := config.LoadSettingsState()
	require.NoError(t, err)
	cfg, err := settings.Runtime()
	require.NoError(t, err)
	assert.Equal(t, "http://lower", cfg.HTTPSProxy)
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY"} {
		oldValue, hadValue := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if hadValue {
				require.NoError(t, os.Setenv(name, oldValue))
				return
			}
			require.NoError(t, os.Unsetenv(name))
		})
	}
}
