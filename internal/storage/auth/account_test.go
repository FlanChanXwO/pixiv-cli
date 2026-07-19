package auth

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthStoreReadHookIsScopedAndRestored(t *testing.T) {
	interceptedPath := filepath.Join(t.TempDir(), "intercepted-auth.json")
	otherPath := filepath.Join(t.TempDir(), "other-auth.json")
	require.NoError(t, os.WriteFile(otherPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"synthetic-other-token"}]}`), 0o600))
	restore := SetReadAuthStoreFileForTest(interceptedPath, func(path string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	})
	t.Cleanup(restore)

	_, err := LoadAuthStore(interceptedPath)
	require.ErrorIs(t, err, fs.ErrPermission)
	store, err := LoadAuthStore(otherPath)
	require.NoError(t, err)
	assert.Equal(t, int64(7), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)

	restore()
	store, err = LoadAuthStore(interceptedPath)
	require.NoError(t, err)
	assert.Empty(t, store.Accounts)
}

func TestLoadSaveAuthStorePreservesDataAndAppliesPlatformPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	store := AuthStore{
		DefaultUserID: 123,
		Accounts: []Account{
			{UserID: 123, Username: "alice", RefreshToken: "secret"},
			{UserID: 456, Username: "bob", RefreshToken: "other-secret"},
		},
	}

	require.NoError(t, SaveAuthStore(path, store))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Windows mode bits 不能证明 DACL 私有性；这里只验证数据与原子替换行为。
	} else {
		assert.Equal(t, os.FileMode(DefaultAuthFileMode), info.Mode().Perm())
	}

	loaded, err := LoadAuthStore(path)
	require.NoError(t, err)
	assert.Equal(t, store, loaded)
	matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".pixiv-private-*"))
	require.NoError(t, globErr)
	assert.Empty(t, matches)
	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Windows 首次创建继承父目录 ACL，本任务不把 mode bits 当作 ACL 断言。
	} else {
		assert.Equal(t, os.FileMode(0o700), parent.Mode().Perm())
	}
}

func TestLoadAuthStoreDropsMissingDefaultUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"default_user_id":999,"accounts":[{"user_id":123,"refresh_token":"secret"}]}`), 0o600))

	loaded, err := LoadAuthStore(path)
	require.NoError(t, err)
	assert.Zero(t, loaded.DefaultUserID)
}

func TestLoadAuthStoreRejectsLegacyAccountNameSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"default_account":"main","accounts":[{"name":"main","refresh_token":"secret"}]}`), 0o600))

	_, err := LoadAuthStore(path)
	require.ErrorContains(t, err, "legacy")
	assert.ErrorContains(t, err, "pixiv auth import/login")
	assert.NotContains(t, err.Error(), "pixiv auth add")
}

func TestLoadAuthStoreRejectsMixedLegacyNameSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"default_account":"main","default_user_id":123,"accounts":[{"name":"main","user_id":123,"refresh_token":"secret"}]}`), 0o600))

	_, err := LoadAuthStore(path)
	require.ErrorContains(t, err, "legacy")
}

func TestLoadAuthStoreRejectsLegacyKeysEvenWhenNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"default_account":null,"default_user_id":123,"accounts":[{"name":null,"user_id":123,"refresh_token":"secret"}]}`), 0o600))

	_, err := LoadAuthStore(path)
	require.ErrorContains(t, err, "legacy")
}

func TestLoadAuthStoreMissingUserIDUsesImportRecoveryGuidance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"accounts":[{"refresh_token":"secret"}]}`), 0o600))

	_, err := LoadAuthStore(path)
	require.ErrorContains(t, err, "user_id is required")
	assert.ErrorContains(t, err, "pixiv auth import/login")
	assert.NotContains(t, err.Error(), "pixiv auth add")
}

func TestAuthStoreRemovePromotesFirstRemainingUserID(t *testing.T) {
	store := AuthStore{
		DefaultUserID: 123,
		Accounts: []Account{
			{UserID: 123, RefreshToken: "secret"},
			{UserID: 456, RefreshToken: "other-secret"},
		},
	}

	assert.True(t, store.Remove(123))
	assert.Equal(t, int64(456), store.DefaultUserID)
	assert.Equal(t, []int64{456}, store.UserIDs())
}

func TestSaveAuthStoreRejectsDuplicateUserIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := AuthStore{
		DefaultUserID: 123,
		Accounts: []Account{
			{UserID: 123, RefreshToken: "secret"},
			{UserID: 123, RefreshToken: "other-secret"},
		},
	}

	err := SaveAuthStore(path, store)
	require.ErrorContains(t, err, "unique")
}

func TestSaveAuthStoreRejectsMissingDefaultUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := AuthStore{
		DefaultUserID: 999,
		Accounts:      []Account{{UserID: 123, RefreshToken: "secret"}},
	}

	err := SaveAuthStore(path, store)
	require.ErrorContains(t, err, "default_user_id")
}

func TestSaveAuthStoreRejectsAccountsWithoutDefaultUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := AuthStore{
		Accounts: []Account{{UserID: 123, RefreshToken: "secret"}},
	}

	err := SaveAuthStore(path, store)
	require.ErrorContains(t, err, "default_user_id")
}
