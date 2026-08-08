package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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

func TestRuntimeOptionsMergeExplicitOverrides(t *testing.T) {
	configuredProxy := "http://configured-proxy"
	flagProxy := "http://flag-proxy"
	interval := time.Second
	base := pixivapp.SDKClientRequest{HTTPSProxyOverride: &configuredProxy, RequestIntervalOverride: &interval}
	merged := mergeSDKClientRequest(base, pixivapp.SDKClientRequest{HTTPSProxyOverride: &flagProxy})
	require.Equal(t, flagProxy, *merged.HTTPSProxyOverride)
	require.Equal(t, interval, *merged.RequestIntervalOverride)
	merged = mergeSDKClientRequest(base, pixivapp.SDKClientRequest{})
	require.Equal(t, configuredProxy, *merged.HTTPSProxyOverride)
}

func TestNewRuntimeExplicitCloseAfterSDKConstructionFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	invalidProxy := "http://proxy-host.invalid/%zz"
	runtime, err := NewRuntime(RuntimeOptions{ProxyOverride: &invalidProxy})
	require.NoError(t, err)
	_, err = runtime.SDK.Client(pixivapp.SDKClientRequest{})
	require.Error(t, err)
	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())
}

func TestNewRuntimeConfiguresDownloadFactory(t *testing.T) {
	// Runtime 拥有本次测试打开的鉴权数据库；隔离 home 防止测试写宿主目录。
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	runtime, err := NewRuntime(RuntimeOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })

	require.NotNil(t, runtime.Download.NewManager)
	manager, err := runtime.Download.NewManager(bootstrapDownloadClientStub{}, t.TempDir(), "{id}")

	require.NoError(t, err)
	require.NotNil(t, manager)
}

func TestNewRuntimeMigratesLegacyAccountPoolAllowlistIntoAuthDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, filesystem.AppDataDirName)
	require.NoError(t, os.MkdirAll(appDataDir, 0o700))
	db, err := authdb.Open(appDataDir)
	require.NoError(t, err)
	require.NoError(t, db.SavePixivCredentials(context.Background(), []authdb.PixivAccount{
		{UserID: 42, Username: "enabled", RefreshToken: []byte("token-42")},
		{UserID: 43, Username: "disabled", RefreshToken: []byte("token-43")},
	}))
	require.NoError(t, db.Close())

	configPath := filepath.Join(appDataDir, "config.toml")
	restore := config.SetFilePathForTest(configPath)
	defer restore()
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[account_pool]\nenabled=true\nstrategy='random'\naccounts=[42]\n[download]\npath='keep-me'\n")))

	runtime, err := NewRuntime(RuntimeOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	status, err := runtime.Account.PoolStatus(context.Background())
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.Equal(t, config.AccountPoolStrategyRandom, status.Strategy)
	require.Len(t, status.Accounts, 2)
	require.True(t, *status.Accounts[0].Schedulable)
	require.False(t, *status.Accounts[1].Schedulable)
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NotContains(t, string(body), "accounts")
	require.Contains(t, string(body), "enabled")
	require.Contains(t, string(body), "strategy")
	require.Contains(t, string(body), "keep-me")
}

func TestNewRuntimeRetriesLegacyAccountPoolConfigRemovalAfterDBCommit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, filesystem.AppDataDirName)
	require.NoError(t, os.MkdirAll(appDataDir, 0o700))
	db, err := authdb.Open(appDataDir)
	require.NoError(t, err)
	require.NoError(t, db.SavePixivCredentials(context.Background(), []authdb.PixivAccount{
		{UserID: 42, RefreshToken: []byte("token-42")},
		{UserID: 43, RefreshToken: []byte("token-43")},
	}))
	require.NoError(t, db.Close())

	configPath := filepath.Join(appDataDir, "config.toml")
	restore := config.SetFilePathForTest(configPath)
	defer restore()
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[account_pool]\nenabled=true\naccounts=[42]\n")))
	originalRemove := removeLegacyAccountPoolConfig
	removeLegacyAccountPoolConfig = func(string) error { return errors.New("account pool config remove failed") }
	runtime, err := NewRuntime(RuntimeOptions{})
	require.ErrorContains(t, err, "account pool config remove failed")
	// 迁移失败时 NewRuntime 仍关闭已打开的 DB，不返回半初始化 Runtime。
	require.Nil(t, runtime)
	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(body), "accounts")

	removeLegacyAccountPoolConfig = originalRemove
	runtime, err = NewRuntime(RuntimeOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	status, err := runtime.Account.PoolStatus(context.Background())
	require.NoError(t, err)
	require.True(t, *status.Accounts[0].Schedulable)
	require.False(t, *status.Accounts[1].Schedulable)
	body, err = os.ReadFile(configPath)
	require.NoError(t, err)
	require.NotContains(t, string(body), "accounts")
}

type bootstrapDownloadClientStub struct{}

func (bootstrapDownloadClientStub) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	return pixiv.Artwork{}, nil
}

func (bootstrapDownloadClientStub) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	return pixiv.UgoiraMetadata{}, nil
}

func (bootstrapDownloadClientStub) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (bootstrapDownloadClientStub) SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error) {
	return sdk.SavedResource{}, nil
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
