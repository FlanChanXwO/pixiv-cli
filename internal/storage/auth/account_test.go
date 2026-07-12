package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileKeepsOldAuthWhenReplacementFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), DefaultAuthFileMode))
	err := writePrivateFile(path, []byte("new"), func(string, string) error { return errors.New("replace failed") })
	require.Error(t, err)
	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".auth-*"))
	require.NoError(t, globErr)
	assert.Empty(t, matches)
}

func TestLoadSaveAuthStoreKeepsPrivatePermissionsAndDefaultUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
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
	assert.Equal(t, os.FileMode(DefaultAuthFileMode), info.Mode().Perm())

	loaded, err := LoadAuthStore(path)
	require.NoError(t, err)
	assert.Equal(t, store.DefaultUserID, loaded.DefaultUserID)
	assert.Equal(t, []int64{123, 456}, loaded.UserIDs())
	assert.Equal(t, "alice", loaded.Accounts[0].Username)
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
