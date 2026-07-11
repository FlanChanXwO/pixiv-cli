package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateCoordinatorGoInstallUsesExactReleaseTag(t *testing.T) {
	checker := &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0", Version: "0.2.0"}}}
	runner := &fakeCommandRunner{}
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceGoInstall},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, []Command{{Name: "go", Args: []string{"install", "github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@v0.2.0"}}}, runner.commands)
	assert.Equal(t, []ReleaseCheckOptions{{Automatic: false}}, checker.options)
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
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: InstallSourceRelease},
				ReleaseChecker:   &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: test.candidate}}},
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: test.current}})

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
		source     InstallSource
		prerelease bool
		current    string
		candidate  string
		want       []Command
	}{
		{
			name:      "stable upgrade",
			source:    InstallSourceHomebrewStable,
			current:   "v0.2.0",
			candidate: "v0.2.0",
			want:      []Command{{Name: "brew", Args: []string{"upgrade", stable}}},
		},
		{
			name:       "stable switches to beta even when selected prerelease is older",
			source:     InstallSourceHomebrewStable,
			prerelease: true,
			current:    "v2.0.0",
			candidate:  "v1.0.0-beta.1",
			want: []Command{
				{Name: "brew", Args: []string{"uninstall", stable}},
				{Name: "brew", Args: []string{"install", beta}},
			},
		},
		{
			name:       "beta upgrade",
			source:     InstallSourceHomebrewBeta,
			prerelease: true,
			current:    "v0.2.0-beta.1",
			candidate:  "v0.2.0-beta.1",
			want:       []Command{{Name: "brew", Args: []string{"upgrade", beta}}},
		},
		{
			name:      "beta switches to stable even when stable is older",
			source:    InstallSourceHomebrewBeta,
			current:   "v2.0.0-beta.1",
			candidate: "v1.0.0",
			want: []Command{
				{Name: "brew", Args: []string{"uninstall", beta}},
				{Name: "brew", Args: []string{"install", stable}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: test.candidate}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), UpdateRequest{
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
		source         InstallSource
		prerelease     bool
		runnerErrors   []error
		wantSubstrings []string
	}{
		{
			name:           "stable to beta restores stable after beta install fails",
			source:         InstallSourceHomebrewStable,
			prerelease:     true,
			runnerErrors:   []error{nil, errors.New("install beta failed"), nil},
			wantSubstrings: []string{"install beta failed", "rollback", "succeeded", stable},
		},
		{
			name:           "beta to stable reports rollback failure",
			source:         InstallSourceHomebrewBeta,
			prerelease:     false,
			runnerErrors:   []error{nil, errors.New("install stable failed"), errors.New("restore beta failed")},
			wantSubstrings: []string{"install stable failed", "rollback", "failed", "restore beta failed", beta},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{errors: test.runnerErrors}
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0-beta.1"}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), UpdateRequest{
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
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceDevelopment},
		ReleaseChecker: checker,
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "dev"}})

	require.ErrorContains(t, err, "development builds cannot update")
	assert.Empty(t, checker.options)
	assert.Empty(t, runner.commands)
}

func TestUpdateCoordinatorCheckNeverRunsCommandOrInstaller(t *testing.T) {
	tests := []struct {
		name   string
		source InstallSource
	}{
		{name: "release", source: InstallSourceRelease},
		{name: "homebrew", source: InstallSourceHomebrewStable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			installer := &fakeReleaseInstaller{}
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector:   fakeSourceDetector{source: test.source},
				ReleaseChecker:   &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}},
				CommandRunner:    runner,
				ReleaseInstaller: installer,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), UpdateRequest{
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
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{},
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), UpdateRequest{
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
		source     InstallSource
		prerelease bool
		current    string
		candidate  string
	}{
		{name: "stable to beta", source: InstallSourceHomebrewStable, prerelease: true, current: "v2.0.0", candidate: "v1.0.0-beta.1"},
		{name: "beta to stable", source: InstallSourceHomebrewBeta, current: "v2.0.0-beta.1", candidate: "v1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeCommandRunner{}
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: test.candidate}}},
				CommandRunner:  runner,
			})
			require.NoError(t, err)

			result, err := coordinator.Execute(context.Background(), UpdateRequest{
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
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "0.1.0"}})

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
			detector:      fakeSourceDetector{source: InstallSourceRelease},
			checker:       &fakeReleaseChecker{err: errors.New("GitHub unavailable")},
			wantError:     "check available releases: GitHub unavailable",
			wantCheckCall: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

			require.ErrorContains(t, err, test.wantError)
			assert.Equal(t, test.wantCheckCall, len(test.checker.options) == 1)
		})
	}
}

func TestUpdateCoordinatorGoInstallSkipsRunnerWithoutStrictUpdate(t *testing.T) {
	runner := &fakeCommandRunner{}
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceGoInstall},
		ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.1.0"}}},
		CommandRunner:  runner,
	})
	require.NoError(t, err)

	result, err := coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.NoError(t, err)
	assert.False(t, result.UpdateAvailable)
	assert.Empty(t, runner.commands)
}

type fakeSourceDetector struct {
	source InstallSource
	err    error
}

func (f fakeSourceDetector) Detect(buildinfo.Info) (InstallSource, error) {
	return f.source, f.err
}

type fakeReleaseChecker struct {
	result  ReleaseCheckResult
	err     error
	options []ReleaseCheckOptions
}

func (f *fakeReleaseChecker) Check(_ context.Context, options ReleaseCheckOptions) (ReleaseCheckResult, error) {
	f.options = append(f.options, options)
	return f.result, f.err
}

type fakeCommandRunner struct {
	commands []Command
	errors   []error
}

type fakeReleaseInstaller struct {
	releases []Release
	err      error
}

func (f *fakeReleaseInstaller) Install(_ context.Context, release Release) error {
	f.releases = append(f.releases, release)
	return f.err
}

func (f *fakeCommandRunner) Run(_ context.Context, command Command) error {
	f.commands = append(f.commands, command)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}
