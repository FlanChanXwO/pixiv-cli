package update_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
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
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}},
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

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

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{APIBaseURL: server.URL})
	require.NoError(t, err)
	result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	require.NoError(t, err)
	require.NotNil(t, result.Release)

	cachePath := filepath.Join(home, paths.AppDataDirName, "cache", release.CacheFilename)
	cacheBytes, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Contains(t, string(cacheBytes), `"schema_version":2`)
}
func TestAutomaticUpdateCheckerReportsStableReleaseNotice(t *testing.T) {
	checker := &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}}
	automatic, err := update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	notice, err := automatic.Check(context.Background(), update.AutomaticUpdateRequest{
		BuildInfo: buildinfo.Info{Version: "v0.1.0"},
	})

	require.NoError(t, err)
	require.NotNil(t, notice)
	assert.Equal(t, update.InstallSourceRelease, notice.Source)
	assert.Equal(t, "v0.1.0", notice.CurrentVersion)
	assert.Equal(t, "v0.2.0", notice.LatestVersion)
	assert.Equal(t, "pixiv update", notice.UpdateCommand)
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestAutomaticUpdateCheckerDoesNotPromptAfterThrottleOrWithoutNewRelease(t *testing.T) {
	tests := []struct {
		name    string
		result  update.ReleaseCheckResult
		version string
	}{
		{name: "throttled", result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}, Throttled: true}, version: "v0.1.0"},
		{name: "no release", result: update.ReleaseCheckResult{}, version: "v0.1.0"},
		{name: "same version", result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.1.0"}}, version: "v0.1.0"},
		{name: "older version", result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.1.0"}}, version: "v0.2.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := &fakeReleaseChecker{result: test.result}
			detected := false
			automatic, err := update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
				SourceDetector: update.SourceDetectorFunc(func(buildinfo.Info) (update.InstallSource, error) {
					detected = true
					return update.InstallSourceRelease, nil
				}),
				ReleaseChecker: checker,
			})
			require.NoError(t, err)

			notice, err := automatic.Check(context.Background(), update.AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: test.version}})

			require.NoError(t, err)
			assert.Nil(t, notice)
			assert.False(t, detected, "no prompt must avoid unnecessary source detection")
			assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: true}}, checker.options)
		})
	}
}

func TestAutomaticUpdateCheckerReturnsSourceSpecificCommands(t *testing.T) {
	tests := []struct {
		name   string
		source update.InstallSource
		want   string
	}{
		{name: "homebrew stable", source: update.InstallSourceHomebrewStable, want: "brew upgrade FlanChanXwO/tap/pixiv-cli"},
		{name: "homebrew beta returns to stable", source: update.InstallSourceHomebrewBeta, want: "pixiv update"},
		{name: "go install", source: update.InstallSourceGoInstall, want: "go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@v0.2.0"},
		{name: "release", source: update.InstallSourceRelease, want: "pixiv update"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			automatic, err := update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{
					Release: &update.Release{TagName: "v0.2.0"},
				}},
			})
			require.NoError(t, err)

			notice, err := automatic.Check(context.Background(), update.AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

			require.NoError(t, err)
			require.NotNil(t, notice)
			assert.Equal(t, test.want, notice.UpdateCommand)
		})
	}
}

func TestAutomaticUpdateCheckerDevelopmentBuildAvoidsAllDependencies(t *testing.T) {
	automatic, err := update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
		SourceDetector: update.SourceDetectorFunc(func(buildinfo.Info) (update.InstallSource, error) {
			t.Fatal("development build must not detect an installation source")
			return "", nil
		}),
		ReleaseChecker: automaticReleaseCheckerFunc(func(context.Context, update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
			t.Fatal("development build must not query GitHub Releases")
			return update.ReleaseCheckResult{}, nil
		}),
	})
	require.NoError(t, err)

	notice, err := automatic.Check(context.Background(), update.AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: "dev"}})

	require.NoError(t, err)
	assert.Nil(t, notice)
}

func TestAutomaticUpdateCheckerPreservesFailures(t *testing.T) {
	failure := errors.New("upstream failure")
	tests := []struct {
		name      string
		buildInfo buildinfo.Info
		checker   update.ReleaseChecker
		detector  update.SourceDetector
	}{
		{
			name:      "invalid current version",
			buildInfo: buildinfo.Info{Version: "not-a-version"},
			checker: automaticReleaseCheckerFunc(func(context.Context, update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
				t.Fatal("invalid local version must not query releases")
				return update.ReleaseCheckResult{}, nil
			}),
			detector: fakeSourceDetector{source: update.InstallSourceRelease},
		},
		{
			name:      "release checker",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker: automaticReleaseCheckerFunc(func(context.Context, update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
				return update.ReleaseCheckResult{}, failure
			}),
			detector: fakeSourceDetector{source: update.InstallSourceRelease},
		},
		{
			name:      "invalid selected tag",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "bad-tag"}}},
			detector:  fakeSourceDetector{source: update.InstallSourceRelease},
		},
		{
			name:      "source detector",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}},
			detector: update.SourceDetectorFunc(func(buildinfo.Info) (update.InstallSource, error) {
				return "", failure
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			automatic, err := update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = automatic.Check(context.Background(), update.AutomaticUpdateRequest{BuildInfo: test.buildInfo})

			require.Error(t, err)
			if test.name == "release checker" || test.name == "source detector" {
				assert.ErrorIs(t, err, failure)
			}
		})
	}
}

type automaticReleaseCheckerFunc func(context.Context, update.ReleaseCheckOptions) (update.ReleaseCheckResult, error)

func (f automaticReleaseCheckerFunc) Check(ctx context.Context, options update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
	return f(ctx, options)
}
func TestUpdateCoordinatorGoInstallUsesExactReleaseTag(t *testing.T) {
	checker := &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0", Version: "0.2.0"}}}
	runner := &fakeCommandRunner{}
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceGoInstall},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, []update.Command{{Name: "go", Args: []string{"install", "github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@v0.2.0"}}}, runner.commands)
	assert.Equal(t, []update.ReleaseCheckOptions{{Automatic: false}}, checker.options)
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
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: update.InstallSourceRelease},
				ReleaseChecker:   &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: test.candidate}}},
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: test.current}})

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
		source     update.InstallSource
		prerelease bool
		current    string
		candidate  string
		want       []update.Command
	}{
		{
			name:      "stable upgrade",
			source:    update.InstallSourceHomebrewStable,
			current:   "v0.2.0",
			candidate: "v0.2.0",
			want:      []update.Command{{Name: "brew", Args: []string{"upgrade", stable}}},
		},
		{
			name:       "stable switches to beta even when selected prerelease is older",
			source:     update.InstallSourceHomebrewStable,
			prerelease: true,
			current:    "v2.0.0",
			candidate:  "v1.0.0-beta.1",
			want: []update.Command{
				{Name: "brew", Args: []string{"uninstall", stable}},
				{Name: "brew", Args: []string{"install", beta}},
			},
		},
		{
			name:       "beta upgrade",
			source:     update.InstallSourceHomebrewBeta,
			prerelease: true,
			current:    "v0.2.0-beta.1",
			candidate:  "v0.2.0-beta.1",
			want:       []update.Command{{Name: "brew", Args: []string{"upgrade", beta}}},
		},
		{
			name:      "beta switches to stable even when stable is older",
			source:    update.InstallSourceHomebrewBeta,
			current:   "v2.0.0-beta.1",
			candidate: "v1.0.0",
			want: []update.Command{
				{Name: "brew", Args: []string{"uninstall", beta}},
				{Name: "brew", Args: []string{"install", stable}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: test.candidate}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), update.UpdateRequest{
				BuildInfo:         buildinfo.Info{Version: test.current},
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
		source         update.InstallSource
		prerelease     bool
		runnerErrors   []error
		wantSubstrings []string
	}{
		{
			name:           "stable to beta restores stable after beta install fails",
			source:         update.InstallSourceHomebrewStable,
			prerelease:     true,
			runnerErrors:   []error{nil, errors.New("install beta failed"), nil},
			wantSubstrings: []string{"install beta failed", "rollback", "succeeded", stable},
		},
		{
			name:           "beta to stable reports rollback failure",
			source:         update.InstallSourceHomebrewBeta,
			prerelease:     false,
			runnerErrors:   []error{nil, errors.New("install stable failed"), errors.New("restore beta failed")},
			wantSubstrings: []string{"install stable failed", "rollback", "failed", "restore beta failed", beta},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{errors: test.runnerErrors}
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0-beta.1"}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), update.UpdateRequest{
				BuildInfo:         buildinfo.Info{Version: "v0.1.0"},
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
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceDevelopment},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "dev"}})

	require.ErrorContains(t, err, "development builds cannot update")
	assert.Empty(t, checker.options)
	assert.Empty(t, runner.commands)
}

func TestUpdateCoordinatorCheckNeverRunsCommandOrInstaller(t *testing.T) {
	tests := []struct {
		name   string
		source update.InstallSource
	}{
		{name: "release", source: update.InstallSourceRelease},
		{name: "homebrew", source: update.InstallSourceHomebrewStable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			installer := &fakeReleaseInstaller{}
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: test.source},
				ReleaseChecker:   &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.2.0"}}},
				CommandRunner:    runner,
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), update.UpdateRequest{
				BuildInfo: buildinfo.Info{Version: "v0.1.0"},
				Check:     true,
			})

			require.NoError(t, err)
			assert.True(t, result.UpdateAvailable)
			assert.Empty(t, runner.commands)
			assert.Empty(t, installer.releases)
		})
	}
}

func TestUpdateCoordinatorCheckReportsNoReleaseAsNilLatestVersion(t *testing.T) {
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{},
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), update.UpdateRequest{
		BuildInfo: buildinfo.Info{Version: "v0.1.0"},
		Check:     true,
	})

	require.NoError(t, err)
	assert.Nil(t, result.LatestVersion)
	assert.False(t, result.LatestPrerelease)
	assert.False(t, result.UpdateAvailable)
}

func TestUpdateCoordinatorCheckReportsHomebrewChannelSwitchWhenSemVerIsOlder(t *testing.T) {
	tests := []struct {
		name       string
		source     update.InstallSource
		prerelease bool
		current    string
		candidate  string
	}{
		{name: "stable to beta", source: update.InstallSourceHomebrewStable, prerelease: true, current: "v2.0.0", candidate: "v1.0.0-beta.1"},
		{name: "beta to stable", source: update.InstallSourceHomebrewBeta, current: "v2.0.0-beta.1", candidate: "v1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: test.candidate}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), update.UpdateRequest{
				BuildInfo:         buildinfo.Info{Version: test.current},
				Check:             true,
				IncludePrerelease: test.prerelease,
			})

			require.NoError(t, err)
			assert.True(t, result.UpdateAvailable)
			assert.Empty(t, runner.commands)
		})
	}
}

func TestUpdateCoordinatorRejectsInvalidFormalVersionBeforeReleaseCheck(t *testing.T) {
	checker := &fakeReleaseChecker{}
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "0.1.0"}})

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
			detector:      fakeSourceDetector{source: update.InstallSourceRelease},
			checker:       &fakeReleaseChecker{err: errors.New("GitHub unavailable")},
			wantError:     "check available releases: GitHub unavailable",
			wantCheckCall: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

			require.ErrorContains(t, err, test.wantError)
			assert.Equal(t, test.wantCheckCall, len(test.checker.options) == 1)
		})
	}
}

func TestUpdateCoordinatorGoInstallSkipsRunnerWithoutStrictUpdate(t *testing.T) {
	runner := &fakeCommandRunner{}
	coordinator, err := update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: update.InstallSourceGoInstall},
		ReleaseChecker: &fakeReleaseChecker{result: update.ReleaseCheckResult{Release: &update.Release{TagName: "v0.1.0"}}},
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), update.UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.NoError(t, err)
	assert.False(t, result.UpdateAvailable)
	assert.Empty(t, runner.commands)
}

type fakeSourceDetector struct {
	source update.InstallSource
	err    error
}

func (f fakeSourceDetector) Detect(buildinfo.Info) (update.InstallSource, error) {
	return f.source, f.err
}

type fakeReleaseChecker struct {
	result  update.ReleaseCheckResult
	err     error
	options []update.ReleaseCheckOptions
}

func (f *fakeReleaseChecker) Check(_ context.Context, options update.ReleaseCheckOptions) (update.ReleaseCheckResult, error) {
	f.options = append(f.options, options)
	return f.result, f.err
}

type fakeCommandRunner struct {
	commands []update.Command
	errors   []error
}

type fakeReleaseInstaller struct {
	releases []update.Release
	err      error
}

func (f *fakeReleaseInstaller) Install(_ context.Context, release update.Release) error {
	f.releases = append(f.releases, release)
	return f.err
}

func (f *fakeCommandRunner) Run(_ context.Context, command update.Command) error {
	f.commands = append(f.commands, command)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}
