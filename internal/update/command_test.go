package update

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReleaseHTTPClientUsesOnlyConfiguredProxyWithoutFixedTimeout(t *testing.T) {
	client, err := NewReleaseHTTPClient("http://proxy.example:7890")
	require.NoError(t, err)
	assert.Zero(t, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases", nil)
	require.NoError(t, err)
	proxy, err := transport.Proxy(request)
	require.NoError(t, err)
	require.NotNil(t, proxy)
	assert.Equal(t, "http://proxy.example:7890", proxy.String())
}

func TestNewReleaseHTTPClientRejectsNonHTTPProxy(t *testing.T) {
	_, err := NewReleaseHTTPClient("socks5://proxy.example:1080")
	require.ErrorContains(t, err, "absolute HTTP(S) URL")
}

func TestUpdateCoordinatorDefaultReleaseInstallerReportsUnavailable(t *testing.T) {
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}},
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.ErrorContains(t, err, "release self-update is not available yet")
}
