//go:build linux

package loginhelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinuxInstallRegistersHandlerAndRestoresUserMimeState(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	dataHome := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	configPath := filepath.Join(configHome, "mimeapps.list")
	original := []byte("[Default Applications]\nx-scheme-handler/pixiv=other.desktop;\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	require.NoError(t, os.WriteFile(configPath, original, 0o600))

	oldExecutable := linuxExecutablePath
	oldXDGMIme := runLinuxXDGMIme
	t.Cleanup(func() {
		linuxExecutablePath = oldExecutable
		runLinuxXDGMIme = oldXDGMIme
	})
	linuxExecutablePath = func() (string, error) { return "/tmp/pixiv cli", nil }
	var calls [][]string
	runLinuxXDGMIme = func(_ context.Context, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		require.NoError(t, os.WriteFile(configPath, []byte("[Default Applications]\nx-scheme-handler/pixiv="+linuxURLHandlerDesktopFile+";\n"), 0o600))
		return nil
	}

	cleanup, err := Install(context.Background(), "http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, []string{"default", linuxURLHandlerDesktopFile, linuxPixivURLScheme}, calls[0])

	applicationsDir, err := linuxApplicationsDir()
	require.NoError(t, err)
	desktopPath := filepath.Join(applicationsDir, linuxURLHandlerDesktopFile)
	desktop, err := os.ReadFile(desktopPath)
	require.NoError(t, err)
	require.Contains(t, string(desktop), `Exec="/tmp/pixiv cli" auth _callback %u`)
	require.Contains(t, string(desktop), "MimeType=x-scheme-handler/pixiv;")

	endpoint, err := callbackEndpointPath()
	require.NoError(t, err)
	endpointBody, err := os.ReadFile(endpoint)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:41871/callback\n", string(endpointBody))

	cleanup()
	restored, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	_, err = os.Stat(desktopPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(endpoint)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLinuxInstallRemovesNewMimeStateWhenNoPreviousHandlerExists(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	oldExecutable := linuxExecutablePath
	oldXDGMIme := runLinuxXDGMIme
	t.Cleanup(func() {
		linuxExecutablePath = oldExecutable
		runLinuxXDGMIme = oldXDGMIme
	})
	linuxExecutablePath = func() (string, error) { return "/tmp/pixiv", nil }
	configPath := filepath.Join(configHome, "mimeapps.list")
	runLinuxXDGMIme = func(_ context.Context, _ ...string) error {
		require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
		return os.WriteFile(configPath, []byte("[Default Applications]\nx-scheme-handler/pixiv="+linuxURLHandlerDesktopFile+";\n"), 0o600)
	}

	cleanup, err := Install(context.Background(), "http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	cleanup()
	_, err = os.Stat(configPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLinuxInstallFailureRemovesEndpointAndDesktopEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	oldExecutable := linuxExecutablePath
	oldXDGMIme := runLinuxXDGMIme
	t.Cleanup(func() {
		linuxExecutablePath = oldExecutable
		runLinuxXDGMIme = oldXDGMIme
	})
	linuxExecutablePath = func() (string, error) { return "/tmp/pixiv", nil }
	runLinuxXDGMIme = func(context.Context, ...string) error { return errors.New("xdg-mime unavailable") }

	cleanup, err := Install(context.Background(), "http://127.0.0.1:41871/callback")
	require.Nil(t, cleanup)
	require.EqualError(t, err, "xdg-mime unavailable")
	endpoint, err := callbackEndpointPath()
	require.NoError(t, err)
	_, err = os.Stat(endpoint)
	require.ErrorIs(t, err, os.ErrNotExist)
	applicationsDir, err := linuxApplicationsDir()
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(applicationsDir, linuxURLHandlerDesktopFile))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLinuxDesktopExecArgumentCannotCreateExtraArguments(t *testing.T) {
	entry := linuxDesktopEntry(`/tmp/pixiv "quoted" $value`)
	require.True(t, strings.Contains(entry, `Exec="/tmp/pixiv \"quoted\" \$value" auth _callback %u`), entry)
}
