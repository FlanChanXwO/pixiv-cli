package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshTokenFromEnvRejectsCookie(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "refresh_token=secret")
	_, err := RefreshTokenFromEnv()
	require.ErrorContains(t, err, "cookie input is not supported; provide a Pixiv App API refresh token")
}

func TestRefreshTokenFromEnvAcceptsOpaqueToken(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "opaque=value")
	token, err := RefreshTokenFromEnv()
	require.NoError(t, err)
	require.Equal(t, "opaque=value", token)
}
