package files

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserConfigFileBuildsPathUnderNamedAppDir(t *testing.T) {
	path, err := UserConfigFile(constants.AppConfigDirName, "auth.json")
	require.NoError(t, err)

	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, constants.AppConfigDirName, "auth.json"), path)
}

func TestWritePrivateFileCreatesPrivateParentAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "config.toml")

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPrivateDirMode(t, parent.Mode().Perm())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPrivateFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "body", string(body))
}

func TestWritePrivateFileResetsExistingFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode))
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, WritePrivateFile(path, []byte("new"), constants.PrivateFileMode))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPrivateFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
}

func TestWritePrivateFileResetsExistingParentDirectoryMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPrivateDirMode(t, parent.Mode().Perm())
}

func assertPrivateFileMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Windows 通过 ACL 管理访问控制，os.FileMode 不保留 Unix 的 0600 位。
		assert.Equal(t, os.FileMode(0o666), actual)
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateFileMode), actual)
}

func assertPrivateDirMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Windows 通过 ACL 管理访问控制，os.FileMode 不保留 Unix 的 0700 位。
		assert.Equal(t, os.FileMode(0o777), actual)
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateDirMode), actual)
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
