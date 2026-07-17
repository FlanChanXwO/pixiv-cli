//go:build darwin

package loginhelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
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
	_, err := os.Stat(filepath.Dir(sourcePath))
	require.ErrorIs(t, err, os.ErrNotExist)
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
