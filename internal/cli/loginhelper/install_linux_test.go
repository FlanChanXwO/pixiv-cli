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

func TestLinuxDesktopFilePathResolvesDesktopIDWithoutTraversal(t *testing.T) {
	home := t.TempDir()
	dataHome := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", filepath.Join(home, "system-data"))

	applicationsDir := filepath.Join(dataHome, "applications")
	desktopID := "com.example.Pixiv.desktop"
	want := filepath.Join(applicationsDir, desktopID)
	require.NoError(t, os.MkdirAll(applicationsDir, 0o700))
	require.NoError(t, os.WriteFile(want, []byte("[Desktop Entry]\nType=Application\n"), 0o600))

	actual, err := linuxDesktopFilePath(desktopID)
	require.NoError(t, err)
	require.Equal(t, want, actual)
	_, err = linuxDesktopFilePath("../outside.desktop")
	require.Error(t, err)
}

func TestLinuxDelegateToPreviousLaunchesResolvedDesktopFile(t *testing.T) {
	home := t.TempDir()
	dataHome := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataHome)
	desktopID := "com.example.Pixiv.desktop"
	desktopPath := filepath.Join(dataHome, "applications", desktopID)
	require.NoError(t, os.MkdirAll(filepath.Dir(desktopPath), 0o700))
	require.NoError(t, os.WriteFile(desktopPath, []byte("[Desktop Entry]\nType=Application\n"), 0o600))
	require.NoError(t, saveHandlerManifest(handlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv", PreviousHandler: desktopID}))

	originalGio := runLinuxGioLaunch
	originalFind := findLinuxCommand
	var gotDesktop, gotURL string
	runLinuxGioLaunch = func(_ context.Context, path, rawURL string) error {
		gotDesktop = path
		gotURL = rawURL
		return nil
	}
	findLinuxCommand = func(name string) (string, error) {
		require.Equal(t, "gio", name)
		return "/usr/bin/gio", nil
	}
	t.Cleanup(func() {
		runLinuxGioLaunch = originalGio
		findLinuxCommand = originalFind
	})

	const rawURL = "pixiv://unrelated/path?code=one-time-code"
	require.NoError(t, DelegateToPrevious(context.Background(), rawURL))
	require.Equal(t, desktopPath, gotDesktop)
	require.Equal(t, rawURL, gotURL)
}

func TestLinuxPersistentHandlerRestoresCompleteMimeStateWhenNoPreviousHandlerExists(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	dataHome := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	configPath := filepath.Join(configHome, "mimeapps.list")
	original := []byte("[Default Applications]\ntext/plain=editor.desktop;\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	require.NoError(t, os.WriteFile(configPath, original, 0o600))

	originalExecutable := linuxExecutablePath
	originalXDG := runLinuxXDGMIme
	originalFind := findLinuxCommand
	originalQuery := queryLinuxDefaultHandler
	linuxExecutablePath = func() (string, error) { return "/tmp/pixiv", nil }
	findLinuxCommand = func(name string) (string, error) {
		require.True(t, name == "xdg-mime" || name == "gio")
		return "/usr/bin/" + name, nil
	}
	current := ""
	queryLinuxDefaultHandler = func(context.Context) (string, error) { return current, nil }
	runLinuxXDGMIme = func(_ context.Context, args ...string) error {
		require.Equal(t, []string{"default", linuxURLHandlerDesktopFile, linuxPixivURLScheme}, args)
		current = linuxURLHandlerDesktopFile
		return os.WriteFile(configPath, []byte("[Default Applications]\nx-scheme-handler/pixiv="+linuxURLHandlerDesktopFile+";\n"), 0o600)
	}
	t.Cleanup(func() {
		linuxExecutablePath = originalExecutable
		runLinuxXDGMIme = originalXDG
		findLinuxCommand = originalFind
		queryLinuxDefaultHandler = originalQuery
	})

	require.NoError(t, EnsurePersistent(context.Background()))
	manifest, exists, err := loadHandlerManifest()
	require.NoError(t, err)
	require.True(t, exists)
	require.Empty(t, manifest.PreviousHandler)
	require.NotEmpty(t, manifest.LinuxMIMESnapshots)

	applicationsDir, err := linuxApplicationsDir()
	require.NoError(t, err)
	desktopPath := filepath.Join(applicationsDir, linuxURLHandlerDesktopFile)
	_, err = os.Stat(desktopPath)
	require.NoError(t, err)

	require.NoError(t, DisablePersistent(context.Background()))
	restored, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	_, err = os.Stat(desktopPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, exists, err = loadHandlerManifest()
	require.NoError(t, err)
	require.False(t, exists)
}
