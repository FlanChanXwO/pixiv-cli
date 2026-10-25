package database_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/stretchr/testify/require"
)

// 样本由 v1.2.1 初始化，账号 42 和 refresh token 均为合成数据。
func TestOpenDatabaseCreatedByV121(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "v1.2.1.db"))
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(database.DatabasePath(directory), body, 0o600))
	db, err := database.Open(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	account, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, "fixture-artist", account.Username)
	require.Equal(t, "fixture-refresh-token", string(account.RefreshTokenCopy()))
}

func TestDatabaseUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are enforced on Unix platforms")
	}
	directory := filepath.Join(t.TempDir(), "private")
	require.NoError(t, os.Mkdir(directory, 0o755))
	db, err := database.Open(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	file, err := os.Stat(database.DatabasePath(directory))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), file.Mode().Perm())
	parent, err := os.Stat(directory)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), parent.Mode().Perm())
}
