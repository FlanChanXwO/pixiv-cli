package bootstrap

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUpdateCoordinatorBuildsDedicatedUpdater(t *testing.T) {
	coordinator, err := NewUpdateCoordinator("", io.Discard, io.Discard)

	require.NoError(t, err)
	require.NotNil(t, coordinator)
}

func TestNewUpdateCoordinatorRejectsInvalidProxy(t *testing.T) {
	_, err := NewUpdateCoordinator("socks5://proxy.example:1080", io.Discard, io.Discard)

	require.ErrorContains(t, err, "absolute HTTP(S) URL")
}

func TestNewAutomaticUpdateCheckerBuildsDedicatedChecker(t *testing.T) {
	checker, err := NewAutomaticUpdateChecker("")

	require.NoError(t, err)
	require.NotNil(t, checker)
}

func TestNewAutomaticUpdateCheckerRejectsInvalidProxy(t *testing.T) {
	_, err := NewAutomaticUpdateChecker("socks5://proxy.example:1080")

	require.ErrorContains(t, err, "absolute HTTP(S) URL")
}
