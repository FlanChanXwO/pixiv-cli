package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileKeepsOldFileWhenWriteFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.write = func(*os.File, []byte) (int, error) {
		return 0, errors.New("write failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "write failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileKeepsOldFileWhenFileSyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.syncFile = func(*os.File) error {
		return errors.New("file sync failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "file sync failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileKeepsOldFileWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(string, string) (privateFileReplaceOutcome, error) {
		return privateFileReplaceOutcome{}, errors.New("replace failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "replace failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileRejectsPartialWriteWithoutReplacingOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.write = func(file *os.File, body []byte) (int, error) {
		return file.Write(body[:len(body)-1])
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, io.ErrShortWrite)

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestUserConfigFileBuildsPathUnderNamedAppDir(t *testing.T) {
	path, err := UserConfigFile(constants.AppConfigDirName, "auth.json")
	require.NoError(t, err)

	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, constants.AppConfigDirName, "auth.json"), path)
}

func TestWritePrivateFileCreatesParentAndFileWithPlatformPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "config.toml")

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPlatformDirMode(t, parent.Mode().Perm())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPlatformFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "body", string(body))
	assertNoPrivateFileTemporaries(t, filepath.Dir(path))
}

func TestWritePrivateFileAppliesPlatformModeToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode))
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, WritePrivateFile(path, []byte("new"), constants.PrivateFileMode))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPlatformFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	assertNoPrivateFileTemporaries(t, filepath.Dir(path))
}

func TestWritePrivateFileAppliesPlatformModeToExistingParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPlatformDirMode(t, parent.Mode().Perm())
}

func assertPlatformFileMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Go 的 Windows mode bits 不能证明 DACL 私有性；本任务不声称主动收紧 DACL。
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateFileMode), actual)
}

func assertPlatformDirMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Mkdir mode 在 Windows 被忽略，不能把 0777 观测值当作 ACL 保证。
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateDirMode), actual)
}

func assertNoPrivateFileTemporaries(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func assertNoPrivateFileArtifacts(t *testing.T, dir string) {
	t.Helper()
	assertNoPrivateFileTemporaries(t, dir)
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-private-*.recovery"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestReplaceFileReplacesTargetAndRemovesSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

	require.NoError(t, ReplaceFile(source, target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(source + ".recovery")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReplaceFileCreatesMissingTargetAndRemovesSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o644))

	require.NoError(t, ReplaceFile(source, target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)
}
