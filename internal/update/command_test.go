package update

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/stretchr/testify/require"
)

func TestUpdateCoordinatorDefaultReleaseInstallerReportsMissingTrustedKey(t *testing.T) {
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}},
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.ErrorContains(t, err, "trusted release signing key is not configured")
}
