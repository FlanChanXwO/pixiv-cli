package settings_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetAndUnsetConfigValuePreserveDocumentAndApplyPlatformPermissions checks
// sparse write-back keeps unrelated TOML content and private file modes.
func TestSetAndUnsetConfigValuePreserveDocumentAndApplyPlatformPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pixiv")
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	original := "# keep this comment\n[download]\npath = './old'\n\n[custom]\nkey = 'keep'\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	restore := paths.SetConfigFilePathForTest(path)
	defer restore()
	store := config.DefaultStore()
	_, err := store.Set(config.KeyDownloadPath, "./new")
	require.NoError(t, err)
	state, err := currentSnapshot(path)
	require.NoError(t, err)
	effective, err := state.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	assert.Equal(t, "./new", effective.Value)
	assertConfigPersistence(t, path)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "# keep this comment")
	assert.Contains(t, string(body), "[custom]\nkey = 'keep'")

	_, err = store.Unset(config.KeyDownloadPath)
	require.NoError(t, err)
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "[download]")
	assert.Contains(t, string(body), "[custom]\nkey = 'keep'")
	assertConfigPersistence(t, path)
}

func assertConfigPersistence(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Windows mode bits do not describe the DACL: creation inherits the
		// parent ACL and replacement keeps the existing target ACL.
	} else {
		assert.Equal(t, os.FileMode(paths.PrivateFileMode), info.Mode().Perm())
		assert.Equal(t, os.FileMode(paths.PrivateDirMode), parent.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
