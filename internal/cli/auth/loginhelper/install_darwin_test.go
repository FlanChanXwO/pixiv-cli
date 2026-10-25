//go:build darwin

package loginhelper

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/stretchr/testify/require"
)

func TestEnsurePixivURLHandlerAppDoesNotFollowLegacyTemporarySourceSymlink(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	canaryPath := filepath.Join(tempDir, "canary")
	legacySourcePath := filepath.Join(tempDir, "pixiv-cli-url-handler.swift")
	require.NoError(t, os.WriteFile(canaryPath, []byte("do not overwrite"), paths.PrivateFileMode))
	require.NoError(t, os.Symlink(canaryPath, legacySourcePath))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := newURLHandlerInstaller().ensureApp(ctx, filepath.Join(tempDir, "PixivCLIURLHandler.app"))
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
	require.Equal(t, filepath.Join(home, paths.AppDataDirName, "url-handler", "PixivCLIURLHandler.app"), path)
}

func TestDelegateToPreviousUsesRecordedBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, saveHandlerManifest(handlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv", PreviousHandler: "com.example.pixiv"}))

	var openedBundle, openedURL string
	installer := newURLHandlerInstaller()
	installer.openPrevious = func(_ context.Context, bundleID, rawURL string) error {
		openedBundle, openedURL = bundleID, rawURL
		return nil
	}
	const rawURL = "pixiv://unrelated/path?code=one-time-code"
	require.NoError(t, installer.delegateToPrevious(context.Background(), rawURL))
	require.Equal(t, "com.example.pixiv", openedBundle)
	require.Equal(t, rawURL, openedURL)
}

func TestEnsurePixivURLHandlerAppCompilesPrivateRandomSourceAndCleansIt(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	legacySourcePath := filepath.Join(tempDir, "pixiv-cli-url-handler.swift")

	var sourcePath string
	installer := newURLHandlerInstaller()
	installer.compile = func(_ context.Context, actualSourcePath, executablePath string) ([]byte, error) {
		sourcePath = actualSourcePath
		require.NotEqual(t, legacySourcePath, actualSourcePath)
		require.Equal(t, tempDir, filepath.Dir(filepath.Dir(actualSourcePath)))
		require.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(actualSourcePath)), "pixiv-cli-url-handler-"))

		dirInfo, err := os.Stat(filepath.Dir(actualSourcePath))
		require.NoError(t, err)
		require.True(t, dirInfo.IsDir())
		require.Equal(t, os.FileMode(paths.PrivateDirMode), dirInfo.Mode().Perm())

		sourceInfo, err := os.Lstat(actualSourcePath)
		require.NoError(t, err)
		require.True(t, sourceInfo.Mode().IsRegular())
		require.Equal(t, os.FileMode(paths.PrivateFileMode), sourceInfo.Mode().Perm())
		content, err := os.ReadFile(actualSourcePath)
		require.NoError(t, err)
		require.Equal(t, pixivURLHandlerSwiftSource, string(content))

		require.NoError(t, os.WriteFile(executablePath, []byte("compiled"), paths.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, installer.ensureApp(context.Background(), appPath))
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
	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), paths.PrivateDirMode))
	require.NoError(t, os.WriteFile(executablePath, []byte("old helper"), paths.PrivateFileMode))
	require.NoError(t, os.WriteFile(infoPath, []byte("old metadata"), paths.PrivateFileMode))

	compiled := false
	installer := newURLHandlerInstaller()
	installer.compile = func(_ context.Context, _ string, executable string) ([]byte, error) {
		compiled = true
		require.NoError(t, os.WriteFile(executable, []byte("new helper"), paths.PrivateFileMode))
		return nil, nil
	}

	require.NoError(t, installer.ensureApp(context.Background(), appPath))
	require.True(t, compiled)
	version, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "source-version"))
	require.NoError(t, err)
	require.Equal(t, pixivURLHandlerSourceVersion+"\n", string(version))
}

func TestPixivURLHandlerReadsPrivateManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	manifest, err := handlerManifestPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, paths.AppDataDirName, "url-handler", handlerManifestFilename), manifest)
	require.Contains(t, pixivURLHandlerSwiftSource, "Bundle.main.resourceURL")
	require.NotContains(t, pixivURLHandlerSwiftSource, "NSHomeDirectory()")
	require.NotContains(t, pixivURLHandlerSwiftSource, paths.AppDataDirName+"/url-handler/"+handlerManifestFilename)
}

func TestPixivURLHandlerStoresLaunchManifestInItsOwnBundle(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	manifest := handlerManifest{Version: 1, ExecutablePath: "/tmp/pixiv"}

	require.NoError(t, saveHandlerBundleManifest(appPath, manifest))

	body, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", handlerManifestFilename))
	require.NoError(t, err)
	var got handlerManifest
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, manifest, got)
	info, err := os.Stat(filepath.Join(appPath, "Contents", "Resources", handlerManifestFilename))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(paths.PrivateFileMode), info.Mode().Perm())

	// LaunchServices 启动 helper 时不会继承调用 CLI 的 HOME；它必须优先
	// 从自己被注册的 bundle 获取本次启动目标，才可支持隔离环境。
	require.Contains(t, pixivURLHandlerSwiftSource, "Bundle.main.resourceURL")
}

func TestPixivURLHandlerStartsCallbackWithManifestHome(t *testing.T) {
	// LaunchServices 从图形会话启动 helper，不能假定它继承启动 pixiv
	// 命令时覆写的 HOME；callback 子进程必须使用 manifest 记录的目录。
	require.Contains(t, pixivURLHandlerSwiftSource, `object["home_directory"]`)
	require.Contains(t, pixivURLHandlerSwiftSource, `environment["HOME"] = homeDirectory`)
	require.Contains(t, pixivURLHandlerSwiftSource, `environment["USERPROFILE"] = homeDirectory`)
}

func TestPixivURLHandlerSwiftSourceCompiles(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "url-handler.swift")
	executablePath := filepath.Join(tempDir, "PixivCLIURLHandler")
	require.NoError(t, os.WriteFile(sourcePath, []byte(pixivURLHandlerSwiftSource), paths.PrivateFileMode))

	out, err := exec.Command("swiftc", sourcePath, "-o", executablePath).CombinedOutput()
	require.NoErrorf(t, err, "compile embedded macOS URL helper: %s", strings.TrimSpace(string(out)))
}

func TestEnsurePixivURLHandlerAppPreservesCompilerOutputAndCleansSourceOnFailure(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	appPath := filepath.Join(t.TempDir(), "PixivCLIURLHandler.app")
	compilerErr := errors.New("compiler failed")

	var sourceDir string
	installer := newURLHandlerInstaller()
	installer.compile = func(_ context.Context, sourcePath, _ string) ([]byte, error) {
		sourceDir = filepath.Dir(sourcePath)
		return []byte("specific swift compiler output\n"), compilerErr
	}

	err := installer.ensureApp(context.Background(), appPath)
	require.ErrorIs(t, err, compilerErr)
	require.ErrorContains(t, err, "specific swift compiler output")
	require.NotEmpty(t, sourceDir)
	_, statErr := os.Stat(sourceDir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
