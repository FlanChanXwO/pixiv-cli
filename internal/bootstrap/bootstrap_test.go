package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestLoadRuntimeConfigUsesDefaultsWhenFileIsMissing(t *testing.T) {
	clearRuntimeEnvironment(t)
	path := filepath.Join(t.TempDir(), "missing", "config.toml")
	t.Cleanup(config.SetFilePathForTest(path))

	got, err := LoadRuntimeConfig()

	require.NoError(t, err)
	require.Equal(t, config.DefaultDownloadPath, got.DownloadPath)
	require.Equal(t, config.DefaultFilenameTemplate, got.FilenameTemplate)
	require.Empty(t, got.HTTPSProxy)
	require.False(t, got.OutputJSON)
	require.True(t, got.WebFallbackEnabled)
}

func TestLoadRuntimeConfigUsesEnvironmentBeforeFile(t *testing.T) {
	clearRuntimeEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(path))
	require.NoError(t, config.WritePrivateFile(path, []byte(`
[download]
path = "/file/path"
filename_template = "file-template"

[network]
https_proxy = "http://file-proxy"

[output]
json = true
`)))
	t.Setenv("DOWNLOAD_PATH", "/environment/path")
	t.Setenv("HTTPS_PROXY", "http://environment-proxy")

	got, err := LoadRuntimeConfig()

	require.NoError(t, err)
	require.Equal(t, "/environment/path", got.DownloadPath)
	require.Equal(t, "file-template", got.FilenameTemplate)
	require.Equal(t, "http://environment-proxy", got.HTTPSProxy)
	require.True(t, got.OutputJSON)
}

func TestLoadRuntimeConfigReportsMalformedFile(t *testing.T) {
	clearRuntimeEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(path))
	require.NoError(t, config.WritePrivateFile(path, []byte("[download\npath = broken")))

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
}

func TestApplyRuntimeProxyOverridePreservesValueForNilOverride(t *testing.T) {
	cfg := config.RuntimeConfig{HTTPSProxy: "http://configured-proxy"}

	applyRuntimeProxyOverride(&cfg, nil)

	require.Equal(t, "http://configured-proxy", cfg.HTTPSProxy)
}

func TestApplyRuntimeProxyOverrideClearsValueForEmptyOverride(t *testing.T) {
	cfg := config.RuntimeConfig{HTTPSProxy: "http://configured-proxy"}
	override := ""

	applyRuntimeProxyOverride(&cfg, &override)

	require.Empty(t, cfg.HTTPSProxy)
}

func TestApplyRuntimeProxyOverrideReplacesValueForNonemptyOverride(t *testing.T) {
	cfg := config.RuntimeConfig{HTTPSProxy: "http://configured-proxy"}
	override := "http://flag-proxy"

	applyRuntimeProxyOverride(&cfg, &override)

	require.Equal(t, "http://flag-proxy", cfg.HTTPSProxy)
}

func TestNewServicesConfiguresDownloadFactory(t *testing.T) {
	services := NewServices()

	require.NotNil(t, services.Download.NewManager)
	manager, err := services.Download.NewManager(bootstrapDownloadClientStub{}, t.TempDir(), "{id}")

	require.NoError(t, err)
	require.NotNil(t, manager)
}

type bootstrapDownloadClientStub struct{}

func (bootstrapDownloadClientStub) IllustDetail(context.Context, int64) (*sdk.IllustDetail, error) {
	return nil, nil
}

func (bootstrapDownloadClientStub) UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error) {
	return nil, nil
}

func (bootstrapDownloadClientStub) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (bootstrapDownloadClientStub) DownloadResource(context.Context, sdk.ResourceRef, string) (sdk.ResourceDownloadResult, error) {
	return sdk.ResourceDownloadResult{}, nil
}

func clearRuntimeEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"DOWNLOAD_PATH",
		"FILENAME_TEMPLATE",
		"https_proxy",
		"HTTPS_PROXY",
	} {
		value, ok := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		if ok {
			t.Cleanup(func() { require.NoError(t, os.Setenv(name, value)) })
		} else {
			t.Cleanup(func() { require.NoError(t, os.Unsetenv(name)) })
		}
	}
}
