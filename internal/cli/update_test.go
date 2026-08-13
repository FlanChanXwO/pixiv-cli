package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunUpdateCheckJSONUsesExplicitProxyAndKeepsStdoutPure(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	checker := &cliReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}}
	coordinatorConstructed := false
	restore := stubUpdateCommand(t, config.RuntimeConfig{HTTPSProxy: "http://config-proxy"}, func(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
		coordinatorConstructed = true
		assert.Equal(t, "http://flag-proxy", proxy)
		return newCLIUpdateCoordinator(checker)
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "update", "--check", "--json", "--prerelease", "--proxy", "http://flag-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Empty(t, stderr.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, map[string]any{
		"source":            "release",
		"current_version":   "v0.1.0",
		"latest_version":    "v0.2.0",
		"latest_prerelease": false,
		"update_available":  true,
	}, result)
	assert.True(t, coordinatorConstructed, "CLI must retain an injectable update coordinator seam")
	assert.Equal(t, []update.ReleaseCheckOptions{{IncludePrerelease: true, Automatic: false}}, checker.options)
}

func TestRunUpdateRejectsJSONWithoutCheckBeforeLoadingDependencies(t *testing.T) {
	useTempPaths(t)
	oldLoad := loadCLIRuntimeConfig
	oldNew := newUpdateCommandCoordinator
	oldResources := newCLIRunResources
	oldCleanup := cleanupPendingWindowsUpdate
	resourceCalls := 0
	cleanupCalls := 0
	loadCLIRuntimeConfig = func() (config.RuntimeConfig, error) {
		t.Fatal("--json without --check must fail before reading configuration")
		return config.RuntimeConfig{}, nil
	}
	newUpdateCommandCoordinator = func(string, io.Writer, io.Writer) (*update.UpdateCoordinator, error) {
		t.Fatal("--json without --check must fail before creating source/checker/runner")
		return nil, nil
	}
	newCLIRunResources = func() (*runResources, error) {
		resourceCalls++
		return &runResources{}, nil
	}
	cleanupPendingWindowsUpdate = func() error {
		cleanupCalls++
		return nil
	}
	t.Cleanup(func() {
		loadCLIRuntimeConfig = oldLoad
		newUpdateCommandCoordinator = oldNew
		newCLIRunResources = oldResources
		cleanupPendingWindowsUpdate = oldCleanup
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "update", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "--json is only supported with --check")
	assert.Zero(t, resourceCalls)
	assert.Zero(t, cleanupCalls)
}

func TestRunUpdateUsesConfiguredProxyAndDoesNotInheritOutputJSON(t *testing.T) {
	useTempPaths(t)
	useReleaseBuildInfo(t, "v0.1.0")
	restore := stubUpdateCommand(t, config.RuntimeConfig{HTTPSProxy: "http://config-proxy", OutputJSON: true}, func(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
		assert.Equal(t, "http://config-proxy", proxy)
		return newCLIUpdateCoordinator(&cliReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.1.0"}}})
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "update", "--check"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "source: release\ncurrent version: v0.1.0\nlatest version: v0.1.0\nupdate available: no\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunUpdateRejectsMalformedExplicitProxyWithoutLeakingSensitiveComponents(t *testing.T) {
	useTempPaths(t)
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "update", "--check", "--proxy", proxy}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid proxy configuration")
	for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
		assert.NotContains(t, stderr.String(), secret)
	}
}

func stubUpdateCommand(t *testing.T, runtimeConfig config.RuntimeConfig, coordinator func(string, io.Writer, io.Writer) (*update.UpdateCoordinator, error)) func() {
	t.Helper()
	oldLoad := loadCLIRuntimeConfig
	oldNew := newUpdateCommandCoordinator
	loadCLIRuntimeConfig = func() (config.RuntimeConfig, error) { return runtimeConfig, nil }
	newUpdateCommandCoordinator = coordinator
	return func() {
		loadCLIRuntimeConfig = oldLoad
		newUpdateCommandCoordinator = oldNew
	}
}

func newCLIUpdateCoordinator(checker update.ReleaseChecker) (*update.UpdateCoordinator, error) {
	return update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: update.SourceDetectorFunc(func(buildinfo.Info) (update.InstallSource, error) {
			return update.InstallSourceRelease, nil
		}),
		ReleaseChecker: checker,
	})
}

func useReleaseBuildInfo(t *testing.T, version string) {
	t.Helper()
	oldVersion := buildinfo.Version
	buildinfo.Version = version
	t.Cleanup(func() { buildinfo.Version = oldVersion })
}

type cliReleaseChecker struct {
	result  update.ReleaseCheckResult
	options []update.ReleaseCheckOptions
}

func (f *cliReleaseChecker) Check(_ context.Context, options update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
	f.options = append(f.options, options)
	return f.result, nil
}
