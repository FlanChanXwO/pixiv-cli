//go:build darwin

package loginhelper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/require"
)

func TestEnsurePixivURLHandlerAppDoesNotFollowLegacyTemporarySourceSymlink(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	canaryPath := filepath.Join(tempDir, "canary")
	legacySourcePath := filepath.Join(tempDir, "pixiv-cli-url-handler.swift")
	require.NoError(t, os.WriteFile(canaryPath, []byte("do not overwrite"), constants.PrivateFileMode))
	require.NoError(t, os.Symlink(canaryPath, legacySourcePath))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ensurePixivURLHandlerApp(ctx, filepath.Join(tempDir, "PixivCLIURLHandler.app"))
	require.Error(t, err)

	content, readErr := os.ReadFile(canaryPath)
	require.NoError(t, readErr)
	require.Equal(t, "do not overwrite", string(content))
}

func TestPixivURLHandlerAppPathUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := pixivURLHandlerAppPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "url-handler", "PixivCLIURLHandler.app"), path)
}

func TestDisablePersistentRetainsManifestWhenNoPreviousMacOSHandlerExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, saveHandlerManifest(handlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv"}))

	originalQuery := queryDarwinURLSchemeHandler
	originalSet := setDarwinURLSchemeHandler
	queryDarwinURLSchemeHandler = func(context.Context, string) (string, error) { return pixivURLHandlerBundleID, nil }
	setDarwinURLSchemeHandler = func(context.Context, string, string) error {
		t.Fatal("no prior handler must not be silently replaced")
		return nil
	}
	t.Cleanup(func() {
		queryDarwinURLSchemeHandler = originalQuery
		setDarwinURLSchemeHandler = originalSet
	})

	err := DisablePersistent(context.Background())
	require.EqualError(t, err, "cannot safely restore a previous macOS Pixiv URL handler")
	_, exists, loadErr := loadHandlerManifest()
	require.NoError(t, loadErr)
	require.True(t, exists)
}

func TestEnsurePixivURLHandlerAppCompilesPrivateRandomSourceAndCleansIt(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	legacySourcePath := filepath.Join(tempDir, "pixiv-cli-url-handler.swift")

	var sourcePath string
	compile := func(_ context.Context, actualSourcePath, executablePath string) ([]byte, error) {
		sourcePath = actualSourcePath
		require.NotEqual(t, legacySourcePath, actualSourcePath)
		require.Equal(t, tempDir, filepath.Dir(filepath.Dir(actualSourcePath)))
		require.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(actualSourcePath)), "pixiv-cli-url-handler-"))

		dirInfo, err := os.Stat(filepath.Dir(actualSourcePath))
		require.NoError(t, err)
		require.True(t, dirInfo.IsDir())
		require.Equal(t, os.FileMode(constants.PrivateDirMode), dirInfo.Mode().Perm())

		sourceInfo, err := os.Lstat(actualSourcePath)
		require.NoError(t, err)
		require.True(t, sourceInfo.Mode().IsRegular())
		require.Equal(t, os.FileMode(constants.PrivateFileMode), sourceInfo.Mode().Perm())
		content, err := os.ReadFile(actualSourcePath)
		require.NoError(t, err)
		require.Equal(t, pixivURLHandlerSwiftSource, string(content))

		require.NoError(t, os.WriteFile(executablePath, []byte("compiled"), constants.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, ensurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile))
	require.NotEmpty(t, sourcePath)
	version, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "source-version"))
	require.NoError(t, err)
	require.Equal(t, pixivURLHandlerSourceVersion+"\n", string(version))
	_, err = os.Stat(filepath.Dir(sourcePath))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnsurePixivURLHandlerAppRebuildsHelperWithoutCurrentSourceVersion(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), constants.PrivateDirMode))
	require.NoError(t, os.WriteFile(executablePath, []byte("old helper"), constants.PrivateFileMode))
	require.NoError(t, os.WriteFile(infoPath, []byte("old metadata"), constants.PrivateFileMode))

	compiled := false
	compile := func(_ context.Context, _ string, executable string) ([]byte, error) {
		compiled = true
		require.NoError(t, os.WriteFile(executable, []byte("new helper"), constants.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, ensurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile))
	require.True(t, compiled)
	version, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "source-version"))
	require.NoError(t, err)
	require.Equal(t, pixivURLHandlerSourceVersion+"\n", string(version))
}

func TestPixivURLHandlerStartsHiddenCallbackProcess(t *testing.T) {
	require.Contains(t, pixivURLHandlerSwiftSource, "process.arguments = [\"auth\", \"_callback\", callbackURL]")
	require.Contains(t, pixivURLHandlerSwiftSource, "handler-manifest.json")
	require.NotContains(t, pixivURLHandlerSwiftSource, "URLSession.shared.dataTask")
}

func TestPixivURLHandlerReadsPrivateManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	manifest, err := handlerManifestPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "url-handler", handlerManifestFilename), manifest)
	require.Contains(t, pixivURLHandlerSwiftSource, "NSHomeDirectory()")
	require.Contains(t, pixivURLHandlerSwiftSource, constants.AppDataDirName+"/url-handler/"+handlerManifestFilename)
	require.NotContains(t, pixivURLHandlerSwiftSource, "applicationSupportDirectory")
}

func TestPixivURLHandlerSwiftSourceCompiles(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "url-handler.swift")
	executablePath := filepath.Join(tempDir, "PixivCLIURLHandler")
	require.NoError(t, os.WriteFile(sourcePath, []byte(pixivURLHandlerSwiftSource), constants.PrivateFileMode))

	out, err := exec.Command("swiftc", sourcePath, "-o", executablePath).CombinedOutput()
	require.NoErrorf(t, err, "compile embedded macOS URL helper: %s", strings.TrimSpace(string(out)))
}

func TestEnsurePixivURLHandlerAppPreservesCompilerOutputAndCleansSourceOnFailure(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	compilerErr := errors.New("compiler failed")

	var sourceDir string
	compile := func(_ context.Context, sourcePath, _ string) ([]byte, error) {
		sourceDir = filepath.Dir(sourcePath)
		return []byte("specific swift compiler output\n"), compilerErr
	}

	err := ensurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile)
	require.ErrorIs(t, err, compilerErr)
	require.ErrorContains(t, err, "specific swift compiler output")
	require.NotEmpty(t, sourceDir)
	_, statErr := os.Stat(sourceDir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
