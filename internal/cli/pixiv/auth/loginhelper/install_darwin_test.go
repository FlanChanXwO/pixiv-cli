//go:build darwin

package loginhelper_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/require"
)

func TestEnsurePixivURLHandlerAppDoesNotFollowLegacyTemporarySourceSymlink(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	canaryPath := filepath.Join(tempDir, "canary")
	legacySourcePath := filepath.Join(tempDir, "pixiv-cli-url-handler.swift")
	require.NoError(t, os.WriteFile(canaryPath, []byte("do not overwrite"), localstate.PrivateFileMode))
	require.NoError(t, os.Symlink(canaryPath, legacySourcePath))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := loginhelper.EnsurePixivURLHandlerApp(ctx, filepath.Join(tempDir, "PixivCLIURLHandler.app"))
	require.Error(t, err)

	content, readErr := os.ReadFile(canaryPath)
	require.NoError(t, readErr)
	require.Equal(t, "do not overwrite", string(content))
}

func TestPixivURLHandlerAppPathUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := loginhelper.PixivURLHandlerAppPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, localstate.AppDataDirName, "url-handler", "PixivCLIURLHandler.app"), path)
}

func TestDisablePersistentRetainsManifestWhenNoPreviousMacOSHandlerExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, loginhelper.SaveHandlerManifest(loginhelper.HandlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv"}))

	t.Cleanup(loginhelper.SetQueryDarwinURLSchemeHandler(func(context.Context, string) (string, error) { return loginhelper.PixivURLHandlerBundleID, nil }))
	t.Cleanup(loginhelper.SetSetDarwinURLSchemeHandler(func(context.Context, string, string) error {
		t.Fatal("no prior handler must not be silently replaced")
		return nil
	}))

	err := loginhelper.DisablePersistent(context.Background())
	require.EqualError(t, err, "cannot safely restore a previous macOS Pixiv URL handler")
	_, exists, loadErr := loginhelper.LoadHandlerManifest()
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
		require.Equal(t, os.FileMode(localstate.PrivateDirMode), dirInfo.Mode().Perm())

		sourceInfo, err := os.Lstat(actualSourcePath)
		require.NoError(t, err)
		require.True(t, sourceInfo.Mode().IsRegular())
		require.Equal(t, os.FileMode(localstate.PrivateFileMode), sourceInfo.Mode().Perm())
		content, err := os.ReadFile(actualSourcePath)
		require.NoError(t, err)
		require.Equal(t, loginhelper.PixivURLHandlerSwiftSource, string(content))

		require.NoError(t, os.WriteFile(executablePath, []byte("compiled"), localstate.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, loginhelper.EnsurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile))
	require.NotEmpty(t, sourcePath)
	version, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "source-version"))
	require.NoError(t, err)
	require.Equal(t, loginhelper.PixivURLHandlerSourceVersion+"\n", string(version))
	_, err = os.Stat(filepath.Dir(sourcePath))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnsurePixivURLHandlerAppRebuildsHelperWithoutCurrentSourceVersion(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), localstate.PrivateDirMode))
	require.NoError(t, os.WriteFile(executablePath, []byte("old helper"), localstate.PrivateFileMode))
	require.NoError(t, os.WriteFile(infoPath, []byte("old metadata"), localstate.PrivateFileMode))

	compiled := false
	compile := func(_ context.Context, _ string, executable string) ([]byte, error) {
		compiled = true
		require.NoError(t, os.WriteFile(executable, []byte("new helper"), localstate.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, loginhelper.EnsurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile))
	require.True(t, compiled)
	version, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "source-version"))
	require.NoError(t, err)
	require.Equal(t, loginhelper.PixivURLHandlerSourceVersion+"\n", string(version))
}

func TestPixivURLHandlerStartsHiddenCallbackProcess(t *testing.T) {
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, "process.arguments = [\"auth\", \"_callback\", callbackURL]")
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, "handler-manifest.json")
	require.NotContains(t, loginhelper.PixivURLHandlerSwiftSource, "URLSession.shared.dataTask")
}

func TestPixivURLHandlerReadsPrivateManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	manifest, err := loginhelper.HandlerManifestPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, localstate.AppDataDirName, "url-handler", loginhelper.HandlerManifestFilename), manifest)
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, "NSHomeDirectory()")
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, localstate.AppDataDirName+"/url-handler/"+loginhelper.HandlerManifestFilename)
	require.NotContains(t, loginhelper.PixivURLHandlerSwiftSource, "applicationSupportDirectory")
}

func TestPixivURLHandlerStoresLaunchManifestInItsOwnBundle(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	manifest := loginhelper.HandlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv"}

	require.NoError(t, loginhelper.SaveHandlerBundleManifest(appPath, manifest))

	body, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", loginhelper.HandlerManifestFilename))
	require.NoError(t, err)
	var got loginhelper.HandlerManifest
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, manifest, got)
	info, err := os.Stat(filepath.Join(appPath, "Contents", "Resources", loginhelper.HandlerManifestFilename))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(localstate.PrivateFileMode), info.Mode().Perm())

	// LaunchServices 启动 helper 时不会继承调用 CLI 的 HOME；它必须优先
	// 从自己被注册的 bundle 获取本次启动目标，才可支持隔离环境。
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, "Bundle.main.resourceURL")
}

func TestPixivURLHandlerStartsCallbackWithManifestHome(t *testing.T) {
	// LaunchServices 从图形会话启动 helper，不能假定它继承启动 pixiv
	// 命令时覆写的 HOME；callback 子进程必须使用 manifest 记录的目录。
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, `object["home_directory"]`)
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, `environment["HOME"] = homeDirectory`)
	require.Contains(t, loginhelper.PixivURLHandlerSwiftSource, `environment["USERPROFILE"] = homeDirectory`)
}

func TestPixivURLHandlerSwiftSourceCompiles(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "url-handler.swift")
	executablePath := filepath.Join(tempDir, "PixivCLIURLHandler")
	require.NoError(t, os.WriteFile(sourcePath, []byte(loginhelper.PixivURLHandlerSwiftSource), localstate.PrivateFileMode))

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

	err := loginhelper.EnsurePixivURLHandlerAppWithCompiler(context.Background(), appPath, compile)
	require.ErrorIs(t, err, compilerErr)
	require.ErrorContains(t, err, "specific swift compiler output")
	require.NotEmpty(t, sourceDir)
	_, statErr := os.Stat(sourceDir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
