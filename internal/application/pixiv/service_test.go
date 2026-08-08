package pixiv

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (*Service, *authdb.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, filesystem.AppDataDirName)
	restore := config.SetFilePathForTest(filepath.Join(appDataDir, "config.toml"))
	t.Cleanup(restore)
	db, err := authdb.Open(appDataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return New(testPixivRepository{db: db}, testPixivDefaults{}), db
}

func TestRemoveAccountRejectsExplicitDefaultBeforeDatabaseMutation(t *testing.T) {
	service, db := newTestService(t)
	require.NoError(t, db.SavePixivCredential(context.Background(), authdb.PixivAccount{
		UserID: 42, Username: "first", RefreshToken: []byte("token"),
	}))
	require.NoError(t, config.SetPixivDefaultUserID(42))

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth use --auto")
	_, getErr := db.GetPixiv(context.Background(), 42)
	require.NoError(t, getErr)
	userID, ok, readErr := config.ReadPixivDefaultUserID()
	require.NoError(t, readErr)
	require.True(t, ok)
	require.Equal(t, int64(42), userID)
}

func TestVerifyPixivAccountIdentity(t *testing.T) {
	tests := []struct {
		name          string
		selected      int64
		authenticated int64
		wantError     bool
	}{
		{name: "matching identities", selected: 42, authenticated: 42},
		{name: "different identities", selected: 42, authenticated: 43, wantError: true},
		{name: "missing selected identity", authenticated: 42, wantError: true},
		{name: "missing authenticated identity", selected: 42, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyPixivAccountIdentity(tc.selected, tc.authenticated)
			if tc.wantError {
				require.Error(t, err)
				require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
				require.NotContains(t, err.Error(), "42")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPixivAutoDefaultMarksSmallestSortOrder(t *testing.T) {
	service, db := newTestService(t)
	require.NoError(t, db.SavePixivCredential(context.Background(), authdb.PixivAccount{
		UserID: 42, SortOrder: 2, Username: "second", RefreshToken: []byte("token-2"),
	}))
	require.NoError(t, db.SavePixivCredential(context.Background(), authdb.PixivAccount{
		UserID: 7, SortOrder: 1, Username: "first", RefreshToken: []byte("token-1"),
	}))

	accounts, err := service.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.True(t, accounts[0].Default)
	require.False(t, accounts[1].Default)
	require.Equal(t, int64(7), accounts[0].UserID)
}

func TestPixivPoolManagementPersistsNonSecretStatus(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	require.NoError(t, db.SavePixivCredentials(ctx, []authdb.PixivAccount{
		{UserID: 42, Username: "first", RefreshToken: []byte("token-42")},
		{UserID: 43, Username: "second", RefreshToken: []byte("token-43")},
	}))

	require.NoError(t, service.SetPoolSchedulable(ctx, []int64{42}, false))
	accounts, err := service.ListAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.False(t, accounts[0].Schedulable)
	require.False(t, accounts[0].Eligible)
	require.True(t, accounts[1].Schedulable)
	require.True(t, accounts[1].Eligible)

	require.NoError(t, service.SetAllPoolSchedulable(ctx, true))
	status, err := service.PoolStatus(ctx)
	require.NoError(t, err)
	require.Len(t, status.Accounts, 2)
	for _, account := range status.Accounts {
		require.True(t, account.Schedulable)
		require.True(t, account.Eligible)
	}
}

func TestPixivDefaultConfigReadErrorIsReturnedBeforeRemoval(t *testing.T) {
	service, db := newTestService(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.Mkdir(configPath, 0o700))
	restore := config.SetFilePathForTest(configPath)
	defer restore()
	require.NoError(t, db.SavePixivCredential(context.Background(), authdb.PixivAccount{
		UserID: 42, Username: "first", RefreshToken: []byte("token"),
	}))

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	_, getErr := db.GetPixiv(context.Background(), 42)
	require.NoError(t, getErr)
}
