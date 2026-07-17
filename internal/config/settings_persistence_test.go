package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/creachadair/tomledit/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndUnsetConfigValuePreserveDocumentAndApplyPlatformPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pixiv")
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	original := "# keep this comment\n[download]\npath = './old'\n\n[custom]\nkey = 'keep'\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	value, err := parser.ParseValue("'./new'")
	require.NoError(t, err)

	require.NoError(t, SetConfigValue(path, "download_path", value))
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	effective, err := state.Effective("download_path")
	require.NoError(t, err)
	assert.Equal(t, "./new", effective.Value)
	assertConfigPersistence(t, path)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "# keep this comment")
	assert.Contains(t, string(body), "[custom]\nkey = 'keep'")

	removed, err := UnsetConfigValue(path, "download_path")
	require.NoError(t, err)
	assert.True(t, removed)
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
		// Windows mode bits 不代表 DACL；首次创建继承父目录 ACL，替换保留既有 target ACL。
	} else {
		assert.Equal(t, os.FileMode(DefaultConfigFileMode), info.Mode().Perm())
		assert.Equal(t, os.FileMode(0o700), parent.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
