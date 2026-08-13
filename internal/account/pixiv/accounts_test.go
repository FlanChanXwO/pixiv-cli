package pixiv_test

import (
	"context"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (*accountpixiv.Service, *database.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
	restore := localstate.SetConfigFilePathForTest(filepath.Join(appDataDir, "config.toml"))
	t.Cleanup(restore)
	db, err := database.Open(appDataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return accountpixiv.NewService(db, testPixivDefaults{}), db
}

func TestRemoveAccountRejectsExplicitDefaultBeforeDatabaseMutation(t *testing.T) {
	service, db := newTestService(t)
	require.NoError(t, db.SavePixivCredential(context.Background(), accountpixiv.New(42, "first", []byte("token"))))
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
			err := accountpixiv.VerifyAccountIdentity(tc.selected, tc.authenticated)
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
	second := accountpixiv.New(42, "second", []byte("token-2"))
	second.SortOrder = 2
	first := accountpixiv.New(7, "first", []byte("token-1"))
	first.SortOrder = 1
	require.NoError(t, db.SavePixivCredential(context.Background(), second))
	require.NoError(t, db.SavePixivCredential(context.Background(), first))

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
	require.NoError(t, db.SavePixivCredentials(ctx, []accountpixiv.Account{
		accountpixiv.New(42, "first", []byte("token-42")),
		accountpixiv.New(43, "second", []byte("token-43")),
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
	restore := localstate.SetConfigFilePathForTest(configPath)
	defer restore()
	require.NoError(t, db.SavePixivCredential(context.Background(), accountpixiv.New(42, "first", []byte("token"))))

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	_, getErr := db.GetPixiv(context.Background(), 42)
	require.NoError(t, getErr)
}
