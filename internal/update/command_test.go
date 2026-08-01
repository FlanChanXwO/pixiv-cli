package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/testutil/socks5test"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReleaseHTTPClientUsesOnlyConfiguredProxyWithoutFixedTimeout(t *testing.T) {
	for _, configuredProxy := range []string{"http://proxy.example:7890", "https://proxy.example:7890"} {
		t.Run(configuredProxy, func(t *testing.T) {
			client, err := NewReleaseHTTPClient(configuredProxy)
			require.NoError(t, err)
			assert.Zero(t, client.Timeout)

			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok)
			request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases", nil)
			require.NoError(t, err)
			proxy, err := transport.Proxy(request)
			require.NoError(t, err)
			require.NotNil(t, proxy)
			assert.Equal(t, configuredProxy, proxy.String())
		})
	}
}

func TestNewReleaseHTTPClientEmptyProxyDisablesEnvironmentFallback(t *testing.T) {
	client, err := NewReleaseHTTPClient("")
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy)
}

func TestNewReleaseHTTPClientAcceptsSOCKSProxy(t *testing.T) {
	proxy := "socks5://proxy-user-secret:proxy-pass-secret@proxy-host-secret.example:1080/proxy-path-secret?proxy-query-secret=value"

	client, err := NewReleaseHTTPClient(proxy)

	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewReleaseHTTPClientRoutesRequestsThroughSOCKS5(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "release through socks")
	}))
	t.Cleanup(target.Close)
	proxy, err := socks5test.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = proxy.Close() })

	client, err := NewReleaseHTTPClient(proxy.URL("socks5h"))
	require.NoError(t, err)
	response, err := client.Get(target.URL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "release through socks", string(body))
}

func TestNewReleaseHTTPClientRejectsMalformedProxyWithoutLeakingSensitiveComponents(t *testing.T) {
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	client, err := NewReleaseHTTPClient(proxy)

	require.Nil(t, client)
	require.ErrorIs(t, err, uri.ErrInvalidProxy)
	require.ErrorContains(t, err, "parse update proxy URL")
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
			require.NotContains(t, current.Error(), secret)
		}
	}
}

func TestUpdateCoordinatorDefaultReleaseInstallerReportsMissingTrustedKey(t *testing.T) {
	coordinator, err := NewUpdateCoordinator(UpdateCoordinatorOptions{
		SourceDetector: fakeSourceDetector{source: InstallSourceRelease},
		ReleaseChecker: &fakeReleaseChecker{result: ReleaseCheckResult{Release: &Release{TagName: "v0.2.0"}}},
	})
	require.NoError(t, err)

	_, err = coordinator.Execute(context.Background(), UpdateRequest{BuildInfo: buildinfo.Info{Version: "v0.1.0"}})

	require.ErrorContains(t, err, "trusted release signing key is not configured")
}
