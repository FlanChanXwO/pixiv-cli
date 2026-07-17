package bootstrap

import (
	"errors"
	"io"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/stretchr/testify/require"
)

func TestNewUpdateCoordinatorBuildsDedicatedUpdater(t *testing.T) {
	coordinator, err := NewUpdateCoordinator("", io.Discard, io.Discard)

	require.NoError(t, err)
	require.NotNil(t, coordinator)
}

func TestNewUpdateCoordinatorRejectsInvalidProxy(t *testing.T) {
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	coordinator, err := NewUpdateCoordinator(proxy, io.Discard, io.Discard)

	require.Nil(t, coordinator)
	require.ErrorIs(t, err, uri.ErrInvalidProxy)
	require.ErrorContains(t, err, "parse update proxy URL")
	assertInvalidProxyChainIsSafe(t, err)
}

func TestNewAutomaticUpdateCheckerBuildsDedicatedChecker(t *testing.T) {
	checker, err := NewAutomaticUpdateChecker("")

	require.NoError(t, err)
	require.NotNil(t, checker)
}

func TestNewAutomaticUpdateCheckerRejectsInvalidProxy(t *testing.T) {
	proxy := "socks5://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret?proxy-query-secret=value"

	checker, err := NewAutomaticUpdateChecker(proxy)

	require.Nil(t, checker)
	require.ErrorIs(t, err, uri.ErrInvalidProxy)
	require.ErrorContains(t, err, "absolute HTTP(S) URL")
	assertInvalidProxyChainIsSafe(t, err)
}

func assertInvalidProxyChainIsSafe(t *testing.T, err error) {
	t.Helper()
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
			require.NotContains(t, current.Error(), secret)
		}
	}
}
