package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSaveAuthStoreKeepsPrivatePermissionsAndDefaultAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	store := AuthStore{
		DefaultAccount: "main",
		Accounts: []Account{
			{Name: "main", RefreshToken: "secret", UserID: 123},
			{Name: "other", RefreshToken: "other-secret"},
		},
	}

	require.NoError(t, SaveAuthStore(path, store))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(DefaultAuthFileMode), info.Mode().Perm())

	loaded, err := LoadAuthStore(path)
	require.NoError(t, err)
	assert.Equal(t, store.DefaultAccount, loaded.DefaultAccount)
	assert.Equal(t, []string{"main", "other"}, loaded.Names())
}

func TestLoadAuthStoreDropsMissingDefaultAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"default_account":"ghost","accounts":[{"name":"main","refresh_token":"secret"}]}`), 0o600))

	loaded, err := LoadAuthStore(path)
	require.NoError(t, err)
	assert.Empty(t, loaded.DefaultAccount)
}

func TestAuthStoreRemovePromotesFirstRemainingAccount(t *testing.T) {
	store := AuthStore{
		DefaultAccount: "main",
		Accounts: []Account{
			{Name: "main", RefreshToken: "secret"},
			{Name: "other", RefreshToken: "other-secret"},
		},
	}

	assert.True(t, store.Remove("main"))
	assert.Equal(t, "other", store.DefaultAccount)
	assert.Equal(t, []string{"other"}, store.Names())
}
