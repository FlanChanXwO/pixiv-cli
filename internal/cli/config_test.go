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
	assert.Contains(t, string(body), `filename_template = "{author} - {title}_{id}"`)

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

func TestConfigPathCreatesBaselineConfigWithoutAdvancedSettings(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "[download]")
	assert.Contains(t, string(body), `path = "./downloads"`)
	assert.Contains(t, string(body), "[web]")
	assert.Contains(t, string(body), "[output]")
	assert.NotContains(t, string(body), "premium")
	assert.NotContains(t, string(body), "logging")
	assert.NotContains(t, string(body), "timeout")
}

func TestConfigInitializationNeverOverwritesExistingFile(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	const original = "# keep me\n[custom]\nvalue = \"preserved\"\n"
	require.NoError(t, config.WritePrivateFile(configPath, []byte(original)))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(body))
}

func TestCommandGroupHelpDoesNotInitializeConfig(t *testing.T) {
	for _, args := range [][]string{{"pixiv", "auth"}, {"pixiv", "config"}} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			clearConfigEnv(t)
			_, configPath := useTempPaths(t)

			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			_, err := os.Stat(configPath)
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
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

func TestConfigOnlyManagesApprovedKeys(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)

	for _, args := range [][]string{
		{"pixiv", "config", "get", "log_level"},
		{"pixiv", "config", "set", "log_level", "debug"},
		{"pixiv", "config", "get", "web_fallback_enabled"},
		{"pixiv", "config", "set", "login_timeout", "30s"},
		{"pixiv", "config", "set", "premium_status_cache_ttl", "3h"},
		{"pixiv", "config", "set", "login_relay_secret", "secret"},
		{"pixiv", "config", "unset", "update_check_enabled"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(args, strings.NewReader(""), &stdout, &stderr)
		assert.NotZero(t, code, strings.Join(args, " "))
		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "valid keys: download_path, filename_template, https_proxy")
	}
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

func TestConfigSetHTTPSProxyDoesNotRevealEnvironmentOverride(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	t.Setenv("HTTPS_PROXY", "https://proxy-user:proxy-password@proxy-host/private?token=proxy-query")

	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"pixiv", "config", "set", "https_proxy", "http://written-proxy"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "https_proxy updated\n", stdout.String())
	assert.Equal(t, "note: https_proxy is currently overridden by environment; effective value remains controlled by environment\n", stderr.String())
	for _, secret := range []string{"proxy-user", "proxy-password", "proxy-host", "/private", "proxy-query"} {
		assert.NotContains(t, stdout.String(), secret)
		assert.NotContains(t, stderr.String(), secret)
	}

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), `https_proxy = "http://written-proxy"`)
}

func TestConfigUnsetHTTPSProxyDoesNotRevealEnvironmentOverride(t *testing.T) {
	clearConfigEnv(t)
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("HTTPS_PROXY", "https://upper-user:upper-password@upper-host/upper-private?token=upper-query")
	t.Setenv("https_proxy", "https://lower-user:lower-password@lower-host/lower-private?token=lower-query")

	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"pixiv", "config", "unset", "https_proxy"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "https_proxy removed\n", stdout.String())
	assert.Equal(t, "note: https_proxy is currently overridden by environment; effective value remains controlled by environment\n", stderr.String())
	for _, secret := range []string{
		"upper-user", "upper-password", "upper-host", "upper-private", "upper-query",
		"lower-user", "lower-password", "lower-host", "lower-private", "lower-query",
	} {
		assert.NotContains(t, stdout.String(), secret)
		assert.NotContains(t, stderr.String(), secret)
	}

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "file-proxy")
}

func TestConfigSetNonSensitiveOverrideStillReportsEffectiveValue(t *testing.T) {
	clearConfigEnv(t)
	useTempPaths(t)
	t.Setenv("DOWNLOAD_PATH", "/tmp/environment-downloads")

	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"pixiv", "config", "set", "download_path", "/tmp/config-downloads"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "download_path updated\n", stdout.String())
	assert.Equal(t, "note: download_path is currently overridden by environment and effective value remains \"/tmp/environment-downloads\"\n", stderr.String())
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

	for _, name := range []string{"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "PIXIV_LOG_LEVEL", "https_proxy", "HTTPS_PROXY"} {
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
