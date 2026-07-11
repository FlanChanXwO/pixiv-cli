package update

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomaticUpdateCheckerReportsStableReleaseNotice(t *testing.T) {
	checker := &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}}
	automatic, err := NewAutomaticUpdateChecker(AutomaticUpdateCheckerOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: checker,
	})
	require.NoError(t, err)

	notice, err := automatic.Check(context.Background(), AutomaticUpdateRequest{
		BuildInfo: buildinfo.Info{Version: "v0.1.0"},
	})

	require.NoError(t, err)
	require.NotNil(t, notice)
	assert.Equal(t, InstallSourceRelease, notice.Source)
	assert.Equal(t, "v0.1.0", notice.CurrentVersion)
	assert.Equal(t, "v0.2.0", notice.LatestVersion)
	assert.Equal(t, "pixiv update", notice.UpdateCommand)
	assert.Equal(t, []ReleaseCheckOptions{{Automatic: true}}, checker.options)
}

func TestAutomaticUpdateCheckerDoesNotPromptAfterThrottleOrWithoutNewRelease(t *testing.T) {
	tests := []struct {
		name    string
		result  ReleaseCheckResult
		version string
	}{
		{name: "throttled", result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}, Throttled: true}, version: "v0.1.0"},
		{name: "no release", result: ReleaseCheckResult{}, version: "v0.1.0"},
		{name: "same version", result: ReleaseCheckResult{Release: &Release{TagName: "v0.1.0"}}, version: "v0.1.0"},
		{name: "older version", result: ReleaseCheckResult{Release: &Release{TagName: "v0.1.0"}}, version: "v0.2.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := &fakeReleaseChecker{result: test.result}
			detected := false
			automatic, err := NewAutomaticUpdateChecker(AutomaticUpdateCheckerOptions{
				SourceDetector: SourceDetectorFunc(func(buildinfo.Info) (InstallSource, error) {
					detected = true
					return InstallSourceRelease, nil
				}),
				ReleaseChecker: checker,
			})
			require.NoError(t, err)

			notice, err := automatic.Check(context.Background(), AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: test.version}})

			require.NoError(t, err)
			assert.Nil(t, notice)
			assert.False(t, detected, "no prompt must avoid unnecessary source detection")
			assert.Equal(t, []ReleaseCheckOptions{{Automatic: true}}, checker.options)
		})
	}
}

func TestAutomaticUpdateCheckerReturnsSourceSpecificCommands(t *testing.T) {
	tests := []struct {
		name   string
		source InstallSource
		want   string
	}{
		{name: "homebrew stable", source: InstallSourceHomebrewStable, want: "brew upgrade FlanChanXwO/tap/pixiv-cli"},
		{name: "homebrew beta returns to stable", source: InstallSourceHomebrewBeta, want: "pixiv update"},
		{name: "go install", source: InstallSourceGoInstall, want: "go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@v0.2.0"},
		{name: "release", source: InstallSourceRelease, want: "pixiv update"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			automatic, err := NewAutomaticUpdateChecker(AutomaticUpdateCheckerOptions{
				SourceDetector: fakeSourceDetector{source: test.source},
				ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{
					Release: &Release{TagName: "v0.2.0"},
				}},
			})
			require.NoError(t, err)

			notice, err := automatic.Check(context.Background(), AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

			require.NoError(t, err)
			require.NotNil(t, notice)
			assert.Equal(t, test.want, notice.UpdateCommand)
		})
	}
}

func TestAutomaticUpdateCheckerDevelopmentBuildAvoidsAllDependencies(t *testing.T) {
	automatic, err := NewAutomaticUpdateChecker(AutomaticUpdateCheckerOptions{
		SourceDetector: SourceDetectorFunc(func(buildinfo.Info) (InstallSource, error) {
			t.Fatal("development build must not detect an installation source")
			return "", nil
		}),
		ReleaseChecker: automaticReleaseCheckerFunc(func(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error) {
			t.Fatal("development build must not query GitHub Releases")
			return ReleaseCheckResult{}, nil
		}),
	})
	require.NoError(t, err)

	notice, err := automatic.Check(context.Background(), AutomaticUpdateRequest{BuildInfo: buildinfo.Info{Version: "dev"}})

	require.NoError(t, err)
	assert.Nil(t, notice)
}

func TestAutomaticUpdateCheckerPreservesFailures(t *testing.T) {
	failure := errors.New("upstream failure")
	tests := []struct {
		name      string
		buildInfo buildinfo.Info
		checker   ReleaseChecker
		detector  SourceDetector
	}{
		{
			name:      "invalid current version",
			buildInfo: buildinfo.Info{Version: "not-a-version"},
			checker: automaticReleaseCheckerFunc(func(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error) {
				t.Fatal("invalid local version must not query releases")
				return ReleaseCheckResult{}, nil
			}),
			detector: fakeSourceDetector{source: InstallSourceRelease},
		},
		{
			name:      "release checker",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker: automaticReleaseCheckerFunc(func(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error) {
				return ReleaseCheckResult{}, failure
			}),
			detector: fakeSourceDetector{source: InstallSourceRelease},
		},
		{
			name:      "invalid selected tag",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "bad-tag"}}},
			detector:  fakeSourceDetector{source: InstallSourceRelease},
		},
		{
			name:      "source detector",
			buildInfo: buildinfo.Info{Version: "v0.1.0"},
			checker:   &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}},
			detector: SourceDetectorFunc(func(buildinfo.Info) (InstallSource, error) {
				return "", failure
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			automatic, err := NewAutomaticUpdateChecker(AutomaticUpdateCheckerOptions{
				SourceDetector: test.detector,
				ReleaseChecker: test.checker,
			})
			require.NoError(t, err)

			_, err = automatic.Check(context.Background(), AutomaticUpdateRequest{BuildInfo: test.buildInfo})

			require.Error(t, err)
			if test.name == "release checker" || test.name == "source detector" {
				assert.ErrorIs(t, err, failure)
			}
		})
	}
}

type automaticReleaseCheckerFunc func(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error)

func (f automaticReleaseCheckerFunc) Check(ctx context.Context, options ReleaseCheckOptions) (ReleaseCheckResult, error) {
	return f(ctx, options)
}
