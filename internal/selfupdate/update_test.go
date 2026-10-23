package selfupdate_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"

	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/release"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCoordinatorDefaultReleaseInstallerReportsMissingTrustedKey(t *testing.T) {
	coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0"}}},
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: "v0.1.0"}})

	require.ErrorContains(t, err, "trusted release signing key is not configured")
}

func TestNewGitHubReleaseClientDefaultCacheUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	client, err := selfupdate.NewGitHubReleaseClient(selfupdate.ReleaseClientOptions{APIBaseURL: server.URL})
	require.NoError(t, err)
	result, err := client.Check(context.Background(), selfupdate.ReleaseCheckOptions{})
	require.NoError(t, err)
	require.NotNil(t, result.Release)

	cachePath := filepath.Join(home, paths.AppDataDirName, "cache", release.CacheFilename)
	cacheBytes, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Contains(t, string(cacheBytes), `"schema_version":2`)
}

func TestAutomaticUpdateCheckerReportsStableReleaseNotice(t *testing.T) {
	checker := &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0"}}}
	automatic, err := selfupdate.NewAutomaticUpdateChecker(selfupdate.AutomaticUpdateCheckerOptions{
		SourceDetector: fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	notice, err := automatic.Check(context.Background(), selfupdate.AutomaticUpdateRequest{
		BuildInfo: version.Info{Version: "v0.1.0"},
	})

	require.NoError(t, err)
	require.NotNil(t, notice)
	assert.Equal(t, selfupdate.InstallSourceRelease, notice.Source)
	assert.Equal(t, "v0.1.0", notice.CurrentVersion)
	assert.Equal(t, "v0.2.0", notice.LatestVersion)
	assert.Equal(t, "pixiv update", notice.UpdateCommand)
	assert.Equal(t, []selfupdate.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestAutomaticUpdateCheckerPreservesFailures(t *testing.T) {
	failure := errors.New("upstream failure")
	tests := []struct {
		name      string
		buildInfo version.Info
		checker   selfupdate.ReleaseChecker
		detector  selfupdate.SourceDetector
	}{
		{
			name:      "invalid current version",
			buildInfo: version.Info{Version: "not-a-version"},
			checker: automaticReleaseCheckerFunc(func(context.Context, selfupdate.ReleaseCheckOptions) (selfupdate.ReleaseCheckResult, error) {
				t.Fatal("invalid local version must not query releases")
				return selfupdate.ReleaseCheckResult{}, nil
			}),
			detector: fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		},
		{
			name:      "release checker",
			buildInfo: version.Info{Version: "v0.1.0"},
			checker: automaticReleaseCheckerFunc(func(context.Context, selfupdate.ReleaseCheckOptions) (selfupdate.ReleaseCheckResult, error) {
				return selfupdate.ReleaseCheckResult{}, failure
			}),
			detector: fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		},
		{
			name:      "invalid selected tag",
			buildInfo: version.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "bad-tag"}}},
			detector:  fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		},
		{
			name:      "source detector",
			buildInfo: version.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0"}}},
			detector: selfupdate.SourceDetectorFunc(func(version.Info) (selfupdate.InstallSource, error) {
				return "", failure
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			automatic, err := selfupdate.NewAutomaticUpdateChecker(selfupdate.AutomaticUpdateCheckerOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = automatic.Check(context.Background(), selfupdate.AutomaticUpdateRequest{BuildInfo: test.buildInfo})

			require.Error(t, err)
			if test.name == "release checker" || test.name == "source detector" {
				assert.ErrorIs(t, err, failure)
			}
		})
	}
}

type automaticReleaseCheckerFunc func(context.Context, selfupdate.ReleaseCheckOptions) (selfupdate.ReleaseCheckResult, error)

func (f automaticReleaseCheckerFunc) Check(ctx context.Context, options selfupdate.ReleaseCheckOptions) (selfupdate.ReleaseCheckResult, error) {
	return f(ctx, options)
}

func TestUpdateCoordinatorGoInstallUsesExactReleaseTag(t *testing.T) {
	checker := &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0", Version: "0.2.0"}}}
	runner := &fakeCommandRunner{}
	coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: selfupdate.InstallSourceGoInstall},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: "v0.1.0"}})

	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, []selfupdate.Command{{Name: "go", Args: []string{"install", "github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@v0.2.0"}}}, runner.commands)
	assert.Equal(t, []selfupdate.ReleaseCheckOptions{{Automatic: false}}, checker.options)
}

func TestUpdateCoordinatorReleaseUsesInstallerOnlyForStrictlyNewerVersion(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		candidate     string
		wantInstalled bool
	}{
		{name: "newer release", current: "v0.1.0", candidate: "v0.2.0", wantInstalled: true},
		{name: "same release", current: "v0.2.0", candidate: "v0.2.0", wantInstalled: false},
		{name: "older release", current: "v0.3.0", candidate: "v0.2.0", wantInstalled: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installer := &fakeReleaseInstaller{}
			coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: selfupdate.InstallSourceRelease},
				ReleaseChecker:   &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: test.candidate}}},
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: test.current}})

			require.NoError(t, err)
			assert.Equal(t, test.wantInstalled, len(installer.releases) == 1)
			assert.Equal(t, test.wantInstalled, result.UpdateAvailable)
		})
	}
}

func TestUpdateCoordinatorHomebrewRunsChannelCommands(t *testing.T) {
	stable := "FlanChanXwO/tap/pixiv-cli"
	beta := "FlanChanXwO/tap/pixiv-cli-beta"
	tests := []struct {
		name       string
		source     selfupdate.InstallSource
		prerelease bool
		current    string
		candidate  string
		want       []selfupdate.Command
	}{
		{
			name:      "stable upgrade",
			source:    selfupdate.InstallSourceHomebrewStable,
			current:   "v0.2.0",
			candidate: "v0.2.0",
			want:      []selfupdate.Command{{Name: "brew", Args: []string{"upgrade", stable}}},
		},
		{
			name:       "stable switches to beta even when selected prerelease is older",
			source:     selfupdate.InstallSourceHomebrewStable,
			prerelease: true,
			current:    "v2.0.0",
			candidate:  "v1.0.0-beta.1",
			want: []selfupdate.Command{
				{Name: "brew", Args: []string{"uninstall", stable}},
				{Name: "brew", Args: []string{"install", beta}},
			},
		},
		{
			name:       "beta upgrade",
			source:     selfupdate.InstallSourceHomebrewBeta,
			prerelease: true,
			current:    "v0.2.0-beta.1",
			candidate:  "v0.2.0-beta.1",
			want:       []selfupdate.Command{{Name: "brew", Args: []string{"upgrade", beta}}},
		},
		{
			name:      "beta switches to stable even when stable is older",
			source:    selfupdate.InstallSourceHomebrewBeta,
			current:   "v2.0.0-beta.1",
			candidate: "v1.0.0",
			want: []selfupdate.Command{
				{Name: "brew", Args: []string{"uninstall", beta}},
				{Name: "brew", Args: []string{"install", stable}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: test.candidate}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), selfupdate.UpdateRequest{
				BuildInfo:         version.Info{Version: test.current},
				IncludePrerelease: test.prerelease,
			})

			require.NoError(t, err)
			assert.Equal(t, test.want, runner.commands)
			if strings.Contains(test.name, "older") {
				assert.True(t, result.UpdateAvailable, "an explicit Homebrew channel switch is actionable despite SemVer ordering")
			}
		})
	}
}

func TestUpdateCoordinatorHomebrewSwitchRollsBackAndReportsBothOutcomes(t *testing.T) {
	stable := "FlanChanXwO/tap/pixiv-cli"
	beta := "FlanChanXwO/tap/pixiv-cli-beta"
	tests := []struct {
		name           string
		source         selfupdate.InstallSource
		prerelease     bool
		runnerErrors   []error
		wantSubstrings []string
	}{
		{
			name:           "stable to beta restores stable after beta install fails",
			source:         selfupdate.InstallSourceHomebrewStable,
			prerelease:     true,
			runnerErrors:   []error{nil, errors.New("install beta failed"), nil},
			wantSubstrings: []string{"install beta failed", "rollback", "succeeded", stable},
		},
		{
			name:           "beta to stable reports rollback failure",
			source:         selfupdate.InstallSourceHomebrewBeta,
			prerelease:     false,
			runnerErrors:   []error{nil, errors.New("install stable failed"), errors.New("restore beta failed")},
			wantSubstrings: []string{"install stable failed", "rollback", "failed", "restore beta failed", beta},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{errors: test.runnerErrors}
			coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0-beta.1"}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), selfupdate.UpdateRequest{
				BuildInfo:         version.Info{Version: "v0.1.0"},
				IncludePrerelease: test.prerelease,
			})

			require.Error(t, err)
			for _, want := range test.wantSubstrings {
				assert.ErrorContains(t, err, want)
			}
		})
	}
}

func TestUpdateCoordinatorDevelopmentBuildRejectsBeforeNetworkOrCommands(t *testing.T) {
	checker := &fakeReleaseChecker{}
	runner := &fakeCommandRunner{}
	coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: selfupdate.InstallSourceDevelopment},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: "dev"}})

	require.ErrorContains(t, err, "development builds cannot update")
	assert.Empty(t, checker.options)
	assert.Empty(t, runner.commands)
}

func TestUpdateCoordinatorCheckNeverRunsCommandOrInstaller(t *testing.T) {
	tests := []struct {
		name   string
		source selfupdate.InstallSource
	}{
		{name: "release", source: selfupdate.InstallSourceRelease},
		{name: "homebrew", source: selfupdate.InstallSourceHomebrewStable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			installer := &fakeReleaseInstaller{}
			coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: test.source},
				ReleaseChecker:   &fakeReleaseChecker{result: selfupdate.ReleaseCheckResult{Release: &selfupdate.Release{TagName: "v0.2.0"}}},
				CommandRunner:    runner,
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), selfupdate.UpdateRequest{
				BuildInfo: version.Info{Version: "v0.1.0"},
				Check:     true,
			})

			require.NoError(t, err)
			assert.True(t, result.UpdateAvailable)
			assert.Empty(t, runner.commands)
			assert.Empty(t, installer.releases)
		})
	}
}

func TestUpdateCoordinatorRejectsInvalidFormalVersionBeforeReleaseCheck(t *testing.T) {
	checker := &fakeReleaseChecker{}
	coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: selfupdate.InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: "0.1.0"}})

	require.ErrorContains(t, err, `parse current build version "0.1.0"`)
	assert.Empty(t, checker.options)
}

func TestUpdateCoordinatorPreservesDetectorAndCheckerFailures(t *testing.T) {
	tests := []struct {
		name          string
		detector      fakeSourceDetector
		checker       *fakeReleaseChecker
		wantError     string
		wantCheckCall bool
	}{
		{
			name:      "source detector",
			detector:  fakeSourceDetector{err: errors.New("source unavailable")},
			checker:   &fakeReleaseChecker{},
			wantError: "detect installation source: source unavailable",
		},
		{
			name:          "release checker",
			detector:      fakeSourceDetector{source: selfupdate.InstallSourceRelease},
			checker:       &fakeReleaseChecker{err: errors.New("GitHub unavailable")},
			wantError:     "check available releases: GitHub unavailable",
			wantCheckCall: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator, err := selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), selfupdate.UpdateRequest{BuildInfo: version.Info{Version: "v0.1.0"}})

			require.ErrorContains(t, err, test.wantError)
			assert.Equal(t, test.wantCheckCall, len(test.checker.options) == 1)
		})
	}
}

type fakeSourceDetector struct {
	source selfupdate.InstallSource
	err    error
}

func (f fakeSourceDetector) Detect(version.Info) (selfupdate.InstallSource, error) {
	return f.source, f.err
}

type fakeReleaseChecker struct {
	result  selfupdate.ReleaseCheckResult
	err     error
	options []selfupdate.ReleaseCheckOptions
}

func (f *fakeReleaseChecker) Check(_ context.Context, options selfupdate.ReleaseCheckOptions) (selfupdate.ReleaseCheckResult, error) {
	f.options = append(f.options, options)
	return f.result, f.err
}

type fakeCommandRunner struct {
	commands []selfupdate.Command
	errors   []error
}

type fakeReleaseInstaller struct {
	releases []selfupdate.Release
	err      error
}

func (f *fakeReleaseInstaller) Install(_ context.Context, release selfupdate.Release) error {
	f.releases = append(f.releases, release)
	return f.err
}

func (f *fakeCommandRunner) Run(_ context.Context, command selfupdate.Command) error {
	f.commands = append(f.commands, command)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}
