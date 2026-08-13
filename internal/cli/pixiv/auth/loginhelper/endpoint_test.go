package loginhelper_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginhelper"
	"github.com/stretchr/testify/require"
)

func TestCallbackRelayURLUsesPrivateLoopbackEndpointAndFragment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	wantPath, err := loginhelper.CallbackEndpointPath()
	require.NoError(t, err)
	path, err := loginhelper.WriteCallbackEndpoint("http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })
	require.Equal(t, wantPath, path)

	relay, err := loginhelper.CallbackRelayURL("pixiv://account/login?code=one-time-code")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:41871/callback#pixiv://account/login?code=one-time-code", relay)
	require.NotContains(t, strings.TrimSpace(string(mustReadFile(t, path))), "one-time-code")
}

func TestCallbackRelayURLRejectsNonLoopbackAndInactiveEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	path, err := loginhelper.CallbackEndpointPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("http://example.invalid/callback\n"), 0o600))

	_, err = loginhelper.CallbackRelayURL("pixiv://account/login?code=one-time-code")
	require.EqualError(t, err, "Pixiv login callback endpoint is invalid")

	require.NoError(t, os.Remove(path))
	_, err = loginhelper.CallbackRelayURL("pixiv://account/login?code=one-time-code")
	require.EqualError(t, err, "Pixiv login callback is no longer active")

	_, err = loginhelper.CallbackRelayURL("https://example.invalid/callback?code=one-time-code")
	require.EqualError(t, err, "invalid Pixiv callback URL")
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}
