package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAutomaticUpdateNoticeKeepsJSONStdoutPure(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	checker := &cliAutomaticReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}}
	constructed := false
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{UpdateCheckEnabled: true}, func(proxy string) (*update.AutomaticUpdateChecker, error) {
		constructed = true
		assert.Empty(t, proxy)
		return newTestAutomaticUpdateChecker(checker, update.InstallSourceRelease)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Empty(t, result.Accounts)
	assert.Contains(t, stderr.String(), "update available: v0.1.0 -> v0.2.0\nrun: pixiv update\n")
	assert.True(t, constructed)
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestRunAutomaticUpdateRunsAfterConfigLeafCommand(t *testing.T) {
	_, configPath := useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{UpdateCheckEnabled: true}, func(string) (*update.AutomaticUpdateChecker, error) {
		return newTestAutomaticUpdateChecker(&cliAutomaticReleaseChecker{
			result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}},
		}, update.InstallSourceHomebrewStable)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, configPath+"\n", stdout.String())
	assert.Equal(t, "update available: v0.1.0 -> v0.2.0\nrun: brew upgrade FlanChanXwO/tap/pixiv-cli\n", stderr.String())
}

func TestRunAutomaticUpdateCanBeDisabledWithoutConstructingChecker(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{UpdateCheckEnabled: false}, func(string) (*update.AutomaticUpdateChecker, error) {
		t.Fatal("opt-out must not construct an automatic update checker")
		return nil, nil
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.NotContains(t, stderr.String(), "update available:")
	assert.NotContains(t, stderr.String(), "warning:")
}

func TestRunAutomaticUpdateFailureOnlyWritesStderrWarning(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	failure := errors.New("release API unavailable")
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{UpdateCheckEnabled: true}, func(string) (*update.AutomaticUpdateChecker, error) {
		return nil, failure
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Contains(t, stderr.String(), "warning: create automatic update checker: release API unavailable\n")
}

func TestRunAutomaticCheckFailureOnlyWritesStderrWarning(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	failure := errors.New("GitHub Releases unavailable")
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{UpdateCheckEnabled: true}, func(string) (*update.AutomaticUpdateChecker, error) {
		return newTestAutomaticUpdateChecker(&cliAutomaticReleaseChecker{err: failure}, update.InstallSourceRelease)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Contains(t, stderr.String(), "warning: check for updates: check available releases: GitHub Releases unavailable\n")
}

func TestRunSkipsAutomaticUpdateForExcludedCommands(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restoreAutomatic := automaticUpdateMustNotRun(t)
	t.Cleanup(restoreAutomatic)

	t.Run("root help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Contains(t, stdout.String(), "Usage:")
	})
	t.Run("explicit help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "search", "--help"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Contains(t, stdout.String(), "Usage:")
	})
	t.Run("version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "version", "--json"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		var info buildinfo.Info
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &info))
	})
	t.Run("root version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "--version"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Equal(t, "pixiv v0.1.0\n", stdout.String())
	})
	t.Run("MCP", func(t *testing.T) {
		oldMCP := runMCPServer
		runMCPServer = func(context.Context, io.Writer, *string) error { return nil }
		t.Cleanup(func() { runMCPServer = oldMCP })

		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Empty(t, stdout.String())
	})
	t.Run("update", func(t *testing.T) {
		restoreUpdate := stubUpdateCommand(t, config.RuntimeConfig{}, func(string, io.Writer, io.Writer) (*update.UpdateCoordinator, error) {
			return newCLIUpdateCoordinator(&cliReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.1.0"}}})
		})
		t.Cleanup(restoreUpdate)

		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "update", "--check"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Contains(t, stdout.String(), "current version: v0.1.0")
	})
}

func TestRunDevelopmentBuildSkipsAutomaticUpdate(t *testing.T) {
	useTempPaths(t)
	restoreAutomatic := automaticUpdateMustNotRun(t)
	t.Cleanup(restoreAutomatic)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.NotContains(t, stderr.String(), "update available:")
	assert.NotContains(t, stderr.String(), "warning:")
}

func TestRunReleaseBuildSkipsAutomaticUpdateForAuthExport(t *testing.T) {
	authPath, _ := useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restoreAutomatic := automaticUpdateMustNotRun(t)
	t.Cleanup(restoreAutomatic)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []auth.Account{{UserID: 7, RefreshToken: "release-token"}},
	}))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "release-token\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunReleaseBuildSkipsAutomaticUpdateForOfflineAuthBundleImport(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restoreAutomatic := automaticUpdateMustNotRun(t)
	t.Cleanup(restoreAutomatic)
	const bundle = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"accounts":[{"user_id":7,"username":"","refresh_token":"offline-import-secret"}]}`

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "--file", "-"}, strings.NewReader(bundle), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:7\ndefault uid: 7\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunAutomaticUpdateUsesCurrentCommandProxyOverride(t *testing.T) {
	useTempPaths(t)
	// 命令完成日志是 info 级诊断；测试应显式声明其可见性，不能依赖默认日志级别。
	t.Setenv("PIXIV_LOG_LEVEL", "info")
	useReleaseBuildInfo(t, "v0.1.0")
	setTestSDKCommandClient(t, proxySDKClient())
	checker := &cliAutomaticReleaseChecker{}
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{HTTPSProxy: "http://config-proxy", UpdateCheckEnabled: true}, func(proxy string) (*update.AutomaticUpdateChecker, error) {
		assert.Equal(t, "http://command-proxy", proxy)
		return newTestAutomaticUpdateChecker(checker, update.InstallSourceRelease)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--json", "--proxy", "http://command-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Contains(t, stderr.String(), "pixiv operation")
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestRunAutomaticUpdateUsesCurrentCommandNoProxyOverride(t *testing.T) {
	useTempPaths(t)
	// 命令完成日志是 info 级诊断；测试应显式声明其可见性，不能依赖默认日志级别。
	t.Setenv("PIXIV_LOG_LEVEL", "info")
	useReleaseBuildInfo(t, "v0.1.0")
	setTestSDKCommandClient(t, proxySDKClient())
	checker := &cliAutomaticReleaseChecker{}
	restore := stubAutomaticUpdateCheck(t, config.RuntimeConfig{HTTPSProxy: "http://config-proxy", UpdateCheckEnabled: true}, func(proxy string) (*update.AutomaticUpdateChecker, error) {
		assert.Empty(t, proxy)
		return newTestAutomaticUpdateChecker(checker, update.InstallSourceRelease)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--json", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Contains(t, stderr.String(), "pixiv operation")
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestRunAutomaticUpdateMalformedRuntimeProxyWritesSafeWarningWithoutNetwork(t *testing.T) {
	_, configPath := useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = "+fmt.Sprintf("%q", proxy)+"\n[update]\ncheck_enabled = true\n")))
	// 环境代理优先于文件配置；显式移除它们，确保 canary 来自临时 runtime config。
	for _, name := range []string{"https_proxy", "HTTPS_PROXY"} {
		value, existed := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if existed {
				require.NoError(t, os.Setenv(name, value))
				return
			}
			require.NoError(t, os.Unsetenv(name))
		})
	}

	oldLoad := loadAutomaticUpdateRuntimeConfig
	oldNew := newCLIAutomaticUpdateChecker
	loadAutomaticUpdateRuntimeConfig = bootstrap.LoadRuntimeConfig
	constructorCalls := 0
	newCLIAutomaticUpdateChecker = func(gotProxy string) (*update.AutomaticUpdateChecker, error) {
		constructorCalls++
		require.Equal(t, proxy, gotProxy)
		return bootstrap.NewAutomaticUpdateChecker(gotProxy)
	}
	t.Cleanup(func() {
		loadAutomaticUpdateRuntimeConfig = oldLoad
		newCLIAutomaticUpdateChecker = oldNew
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, configPath+"\n", stdout.String())
	assert.Equal(t, 1, constructorCalls)
	for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
		assert.NotContains(t, stderr.String(), secret)
	}
	assert.Contains(t, stderr.String(), "warning: create automatic update checker")
	assert.Contains(t, stderr.String(), "invalid proxy configuration")
}

func stubAutomaticUpdateCheck(t *testing.T, runtimeConfig config.RuntimeConfig, constructor func(string) (*update.AutomaticUpdateChecker, error)) func() {
	t.Helper()
	oldLoad := loadAutomaticUpdateRuntimeConfig
	oldNew := newCLIAutomaticUpdateChecker
	loadAutomaticUpdateRuntimeConfig = func() (config.RuntimeConfig, error) { return runtimeConfig, nil }
	newCLIAutomaticUpdateChecker = constructor
	return func() {
		loadAutomaticUpdateRuntimeConfig = oldLoad
		newCLIAutomaticUpdateChecker = oldNew
	}
}

func automaticUpdateMustNotRun(t *testing.T) func() {
	t.Helper()
	oldLoad := loadAutomaticUpdateRuntimeConfig
	oldNew := newCLIAutomaticUpdateChecker
	loadAutomaticUpdateRuntimeConfig = func() (config.RuntimeConfig, error) {
		t.Fatal("excluded command must not load automatic update configuration")
		return config.RuntimeConfig{}, nil
	}
	newCLIAutomaticUpdateChecker = func(string) (*update.AutomaticUpdateChecker, error) {
		t.Fatal("excluded command must not construct an automatic update checker")
		return nil, nil
	}
	return func() {
		loadAutomaticUpdateRuntimeConfig = oldLoad
		newCLIAutomaticUpdateChecker = oldNew
	}
}

func newTestAutomaticUpdateChecker(checker update.ReleaseChecker, source update.InstallSource) (*update.AutomaticUpdateChecker, error) {
	return update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
		SourceDetector: update.SourceDetectorFunc(func(buildinfo.Info) (update.InstallSource, error) {
			return source, nil
		}),
		ReleaseChecker: checker,
	})
}

type cliAutomaticReleaseChecker struct {
	result  update.ReleaseCheckResult
	options []update.ReleaseCheckOptions
	err     error
}

func (f *cliAutomaticReleaseChecker) Check(_ context.Context, options update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
	f.options = append(f.options, options)
	return f.result, f.err
}
