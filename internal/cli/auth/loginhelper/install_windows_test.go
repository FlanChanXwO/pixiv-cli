//go:build windows

package loginhelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func TestWindowsURLHandlerCommandQuotesExecutableAndCallback(t *testing.T) {
	command := windowsURLHandlerCommand(`C:\Users\A Name\pixiv.exe`)
	require.Equal(t, `"C:\Users\A Name\pixiv.exe" auth _callback "%1"`, command)

	command = windowsURLHandlerCommand(`C:\path\"quoted"\pixiv.exe`)
	require.Contains(t, command, `\"quoted\"`)
	require.Contains(t, command, `auth _callback "%1"`)
}

func TestWindowsDelegateToPreviousUsesPrivateProgID(t *testing.T) {
	useWindowsTemporaryConfig(t)
	require.NoError(t, saveHandlerManifest(handlerManifest{Version: 1, ExecutablePath: `C:\pixiv.exe`, PreviousHandler: previousWindowsURLHandlerProgID}))

	var className, callback string
	installer := newURLHandlerInstaller()
	installer.openPrevious = func(_ context.Context, class, rawURL string) error {
		className = class
		callback = rawURL
		return nil
	}

	const rawURL = "pixiv://unrelated/path?value=one-time-code"
	require.NoError(t, installer.delegateToPrevious(context.Background(), rawURL))
	require.Equal(t, previousWindowsURLHandlerProgID, className)
	require.Equal(t, rawURL, callback)
}

func TestWindowsInstallRestoresExistingCurrentUserProtocolTree(t *testing.T) {
	useWindowsTemporaryConfig(t)
	keyPath := windowsTestRegistryKey(t)
	seedWindowsRegistryTree(t, keyPath)

	installer := newURLHandlerInstaller()
	installer.urlHandlerKey = keyPath
	installer.executable = func() (string, error) { return `C:\Program Files\pixiv\pixiv.exe`, nil }
	t.Cleanup(func() { deleteWindowsRegistryTree(t, keyPath) })

	cleanup, err := installer.install(context.Background(), "http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	installed, err := registry.OpenKey(registry.CURRENT_USER, windowsRegistrySubpath(keyPath), registry.READ)
	require.NoError(t, err)
	commandKey, err := registry.OpenKey(installed, `shell\open\command`, registry.READ)
	require.NoError(t, err)
	command, _, err := commandKey.GetStringValue("")
	require.NoError(t, err)
	require.Contains(t, command, `auth _callback "%1"`)
	require.NoError(t, commandKey.Close())
	require.NoError(t, installed.Close())

	cleanup()
	restored, err := registry.OpenKey(registry.CURRENT_USER, windowsRegistrySubpath(keyPath), registry.READ)
	require.NoError(t, err)
	defer restored.Close()
	name, _, err := restored.GetStringValue("")
	require.NoError(t, err)
	require.Equal(t, "previous protocol", name)
	child, err := registry.OpenKey(restored, "previous", registry.READ)
	require.NoError(t, err)
	defer child.Close()
	value, _, err := child.GetStringValue("marker")
	require.NoError(t, err)
	require.Equal(t, "keep", value)
}

func TestWindowsInstallRemovesNewProtocolTreeWhenNoneExisted(t *testing.T) {
	useWindowsTemporaryConfig(t)
	keyPath := windowsTestRegistryKey(t)
	deleteWindowsRegistryTree(t, keyPath)

	installer := newURLHandlerInstaller()
	installer.urlHandlerKey = keyPath
	installer.executable = func() (string, error) { return `C:\pixiv.exe`, nil }
	t.Cleanup(func() { deleteWindowsRegistryTree(t, keyPath) })

	cleanup, err := installer.install(context.Background(), "http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	cleanup()
	_, err = registry.OpenKey(registry.CURRENT_USER, windowsRegistrySubpath(keyPath), registry.READ)
	require.ErrorIs(t, err, registry.ErrNotExist)
}

func TestWindowsInstallFailureRemovesPrivateCallbackEndpoint(t *testing.T) {
	useWindowsTemporaryConfig(t)
	installer := newURLHandlerInstaller()
	installer.urlHandlerKey = windowsTestRegistryKey(t)
	installer.executable = func() (string, error) { return `C:\pixiv.exe`, nil }
	installer.runRegistry = func(context.Context, ...string) error { return errors.New("registry unavailable") }

	cleanup, err := installer.install(context.Background(), "http://127.0.0.1:41871/callback")
	require.Nil(t, cleanup)
	require.EqualError(t, err, "registry unavailable")
	endpoint, err := callbackEndpointPath()
	require.NoError(t, err)
	_, err = os.Stat(endpoint)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func useWindowsTemporaryConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

func windowsTestRegistryKey(t *testing.T) string {
	t.Helper()
	return `HKCU\Software\Classes\pixiv-cli-test-` + fmt.Sprintf("%d", os.Getpid()) + `-` + filepath.Base(t.TempDir())
}

func windowsRegistrySubpath(fullKey string) string {
	const prefix = `HKCU\`
	return fullKey[len(prefix):]
}

func seedWindowsRegistryTree(t *testing.T, keyPath string) {
	t.Helper()
	deleteWindowsRegistryTree(t, keyPath)
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsRegistrySubpath(keyPath), registry.ALL_ACCESS)
	require.NoError(t, err)
	require.NoError(t, key.SetStringValue("", "previous protocol"))
	child, _, err := registry.CreateKey(key, "previous", registry.ALL_ACCESS)
	require.NoError(t, err)
	require.NoError(t, child.SetStringValue("marker", "keep"))
	require.NoError(t, child.Close())
	require.NoError(t, key.Close())
}

func deleteWindowsRegistryTree(t *testing.T, keyPath string) {
	t.Helper()
	_ = execWindowsRegistryDelete(keyPath)
}

func execWindowsRegistryDelete(keyPath string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, "reg.exe", "delete", keyPath, "/f")
	return command.Run()
}
