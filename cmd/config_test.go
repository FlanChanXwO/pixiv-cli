package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPathGetSetUnset(t *testing.T) {
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
	assert.Equal(t, defaultDownloadPath+"\n", stdout.String())
}

func TestConfigMissingValueReturnsPlaceholder(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "get", "https_proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Equal(t, configMissingPlaceholder+"\n", stdout.String())
	assert.Contains(t, stderr.String(), "unset")
}

func TestConfigStrongTypesAndSparseWrite(t *testing.T) {
	_, configPath := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "set", "output_json", "true"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	code = Run([]string{"pixiv", "config", "set", "login_timeout", "30s"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(defaultConfigFileMode), info.Mode().Perm())

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "[output]")
	assert.Contains(t, string(body), "json = true")
	assert.Contains(t, string(body), "[login]")
	assert.Contains(t, string(body), `timeout = "30s"`)
	assert.NotContains(t, string(body), "download")
}

func TestConfigGetUsesEnvironmentOverrideAndSetWarns(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, writePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
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
	useTempPaths(t)
	t.Setenv("HTTPS_PROXY", "http://upper")
	t.Setenv("https_proxy", "http://lower")

	settings, err := loadSettingsState()
	require.NoError(t, err)
	cfg, err := settings.runtime()
	require.NoError(t, err)
	assert.Equal(t, "http://lower", cfg.HTTPSProxy)
}
