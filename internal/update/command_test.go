package update_test

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/stretchr/testify/require"
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
