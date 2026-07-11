package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
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
	assert.Equal(t, "update available: v0.1.0 -> v0.2.0\nrun: pixiv update\n", stderr.String())
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
	assert.Empty(t, stderr.String())
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
	assert.Equal(t, "warning: create automatic update checker: release API unavailable\n", stderr.String())
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
	assert.Equal(t, "warning: check for updates: check available releases: GitHub Releases unavailable\n", stderr.String())
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
	assert.Empty(t, stderr.String())
}

func TestRunAutomaticUpdateUsesCurrentCommandProxyOverride(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	setTestCLIClientFactory(t, func(clientConfig) (cliPixivClient, error) {
		return proxyFlagPixivClient{}, nil
	})
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
	assert.Empty(t, stderr.String())
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestRunAutomaticUpdateUsesCurrentCommandNoProxyOverride(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	setTestCLIClientFactory(t, func(clientConfig) (cliPixivClient, error) {
		return proxyFlagPixivClient{}, nil
	})
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
	assert.Empty(t, stderr.String())
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
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
