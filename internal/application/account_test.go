package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationServicesReportMissingDependencies(t *testing.T) {
	_, err := ConfigService{}.Path()
	require.ErrorContains(t, err, "config store is not configured")

	_, err = AccountService{}.Check(context.Background(), 123)
	require.ErrorContains(t, err, "pixiv account service is not configured")
}

func TestAccountServiceUsesAuthDBAccountStore(t *testing.T) {
	callOrder := make([]string, 0, 2)
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]pixivapp.Account, error) {
		callOrder = append(callOrder, "list")
		return []pixivapp.Account{{UserID: 123, Username: "alice", Default: true}}, nil
	}
	store.importAccount = func(_ context.Context, token string, setDefault bool) (pixivapp.Account, error) {
		callOrder = append(callOrder, "import")
		if token != "main/token" {
			t.Fatalf("ImportAccount token=%q", token)
		}
		return pixivapp.Account{UserID: 456, Username: "bob"}, nil
	}
	service := newAccountServiceForTest(store)

	result, err := service.Import(context.Background(), AccountImportRequest{TokenInput: "  main/token  "})
	require.NoError(t, err)
	assert.Equal(t, []string{"list", "import"}, callOrder)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"user_id":456,"username":"bob","status":"added"}`, string(body))
	list, err := service.List()
	require.NoError(t, err)
	assert.Equal(t, int64(123), list.DefaultUserID)
	require.Len(t, list.Accounts, 1)
	assert.Equal(t, int64(123), list.Accounts[0].UserID)
}

func TestAccountServiceImportReportsUpdatedByAuthoritativeReturnedUID(t *testing.T) {
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]pixivapp.Account, error) {
		return []pixivapp.Account{{UserID: 456, Username: "before"}}, nil
	}
	store.importAccount = func(context.Context, string, bool) (pixivapp.Account, error) {
		return pixivapp.Account{UserID: 456, Username: "after", Default: true}, nil
	}

	result, err := newAccountServiceForTest(store).Import(context.Background(), AccountImportRequest{TokenInput: "opaque-token"})
	require.NoError(t, err)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"user_id":456,"username":"after","status":"updated"}`, string(body))
}

func TestAccountServiceExportUsesLocalTokens(t *testing.T) {
	store := &fakeAccountStore{}
	store.accountsWithTokens = func(context.Context) ([]pixivapp.AccountWithToken, error) {
		return []pixivapp.AccountWithToken{{UserID: 456, Username: "alice", Default: true, RefreshToken: "opaque-exported-token"}}, nil
	}

	result, err := AccountService{Pixiv: store}.Export(AccountExportRequest{UserID: 456})
	require.NoError(t, err)
	assert.Equal(t, "opaque-exported-token", result.RefreshToken)
	assert.Equal(t, 1, result.AccountCount)
}

func TestAccountServiceExportAllEncodesBundle(t *testing.T) {
	store := &fakeAccountStore{}
	store.accountsWithTokens = func(context.Context) ([]pixivapp.AccountWithToken, error) {
		return []pixivapp.AccountWithToken{
			{UserID: 456, Username: "alice", Default: true, RefreshToken: "token-a"},
			{UserID: 789, Username: "bob", RefreshToken: "token-b"},
		}, nil
	}

	result, err := AccountService{Pixiv: store}.Export(AccountExportRequest{All: true})
	require.NoError(t, err)
	assert.Equal(t, 2, result.AccountCount)
	assert.JSONEq(t, `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":456,"accounts":[{"user_id":456,"username":"alice","refresh_token":"token-a"},{"user_id":789,"username":"bob","refresh_token":"token-b"}]}`, string(result.Bundle))
}

func TestAccountServiceImportBundleRestoresOffline(t *testing.T) {
	var restored []pixivapp.Account
	var tokens []string
	store := &fakeAccountStore{}
	store.restoreAccount = func(_ context.Context, account pixivapp.Account, token string, setDefault bool) error {
		restored = append(restored, account)
		tokens = append(tokens, token)
		return nil
	}
	const body = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":321,"accounts":[{"user_id":654,"username":"new","refresh_token":"new-secret"},{"user_id":321,"username":"updated","refresh_token":"updated-secret"}]}`

	result, err := AccountService{Pixiv: store}.ImportBundle([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, int64(321), result.DefaultUserID)
	require.Len(t, restored, 2)
	assert.Equal(t, []string{"new-secret", "updated-secret"}, tokens)
	resultBody, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"accounts":[{"user_id":654,"username":"new","status":"added"},{"user_id":321,"username":"updated","status":"added"}],"default_user_id":321}`, string(resultBody))
}

func TestAccountServiceCheckUsesAccountStore(t *testing.T) {
	store := &fakeAccountStore{}
	store.checkAccount = func(_ context.Context, userID int64) (pixivapp.Account, error) {
		assert.Equal(t, int64(123), userID)
		return pixivapp.Account{UserID: 123, Username: "alice", Default: true}, nil
	}
	result, err := newAccountServiceForTest(store).CheckWithRequest(context.Background(), AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "alice", Default: true, HasToken: true}, result)
}

func TestAccountServiceCheckPrefersEnvironmentTokenWithoutSelectingStoredAccount(t *testing.T) {
	store := &fakeAccountStore{}
	store.checkAccount = func(_ context.Context, userID int64) (pixivapp.Account, error) {
		t.Fatalf("stored CheckAccount(%d) was called while an environment token exists", userID)
		return pixivapp.Account{}, nil
	}
	store.checkRefreshToken = func(_ context.Context, token string) (pixivapp.Account, error) {
		assert.Equal(t, "environment-token", token)
		return pixivapp.Account{UserID: 456, Username: "environment"}, nil
	}
	service := newAccountServiceForTest(store)
	service.RefreshTokenFromEnv = func() (string, error) { return "environment-token", nil }

	result, err := service.CheckWithRequest(context.Background(), AccountCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 456, Username: "environment", HasToken: true}, result)
}

func TestAccountServiceCheckExplicitUIDDoesNotUseEnvironmentToken(t *testing.T) {
	store := &fakeAccountStore{}
	store.checkAccount = func(_ context.Context, userID int64) (pixivapp.Account, error) {
		assert.Equal(t, int64(123), userID)
		return pixivapp.Account{UserID: 123, Username: "stored", Default: true}, nil
	}
	store.checkRefreshToken = func(_ context.Context, token string) (pixivapp.Account, error) {
		t.Fatalf("CheckRefreshToken(%q) was called for an explicit UID", token)
		return pixivapp.Account{}, nil
	}
	service := newAccountServiceForTest(store)
	service.RefreshTokenFromEnv = func() (string, error) { return "environment-token", nil }

	result, err := service.CheckWithRequest(context.Background(), AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "stored", Default: true, HasToken: true}, result)
}

func TestAccountServiceRefreshUpdatesPremiumStatus(t *testing.T) {
	premium := false
	store := &fakeAccountStore{}
	store.refreshAccount = func(_ context.Context, userID int64) (pixivapp.Account, error) {
		assert.Equal(t, int64(123), userID)
		return pixivapp.Account{UserID: 123, Username: "refreshed-name", Default: true, Premium: &premium}, nil
	}

	result, err := AccountService{Pixiv: store}.RefreshWithRequest(context.Background(), AccountRefreshRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "refreshed-name", Default: true, HasToken: true, PremiumStatus: &premium}, result)
}

func TestAccountServiceImportRejectsCookieBeforeOpeningStore(t *testing.T) {
	service := AccountService{Pixiv: &fakeAccountStore{}}

	_, err := service.Import(context.Background(), AccountImportRequest{TokenInput: "refresh_token=secret"})
	require.ErrorContains(t, err, "cookie input is not supported")
}

func TestAccountServiceRemoveReturnsRemovedAccountAndUpdatedDefault(t *testing.T) {
	listCalls := 0
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]pixivapp.Account, error) {
		listCalls++
		if listCalls == 1 {
			return []pixivapp.Account{
				{UserID: 123, Username: "alice", Default: true},
				{UserID: 456, Username: "bob"},
			}, nil
		}
		return []pixivapp.Account{
			{UserID: 456, Username: "bob", Default: true},
		}, nil
	}
	store.removeAccount = func(_ context.Context, userID int64) error {
		assert.Equal(t, int64(123), userID)
		return nil
	}

	removed, defaultUserID, err := newAccountServiceForTest(store).Remove(123)
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "alice", HasToken: true}, removed)
	assert.Equal(t, int64(456), defaultUserID)
	assert.Equal(t, 2, listCalls)
}

func TestAccountServiceRemoveReportsNotFoundWithoutMutation(t *testing.T) {
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]pixivapp.Account, error) {
		return []pixivapp.Account{{UserID: 123, Username: "alice", Default: true}}, nil
	}
	store.removeAccount = func(context.Context, int64) error {
		t.Fatal("RemoveAccount was called for an unknown uid")
		return nil
	}

	_, _, err := newAccountServiceForTest(store).Remove(999)
	require.EqualError(t, err, "account uid 999 not found")
}

func TestAccountServiceUseSelectsRequestedAccount(t *testing.T) {
	store := &fakeAccountStore{}
	store.useAccount = func(_ context.Context, userID int64) error {
		assert.Equal(t, int64(456), userID)
		return nil
	}

	userID, err := newAccountServiceForTest(store).Use(456)
	require.NoError(t, err)
	assert.Equal(t, int64(456), userID)
}

func TestAccountServicePropagatesMissingStoreError(t *testing.T) {
	_, err := (AccountService{Pixiv: nil}).Use(123)
	require.ErrorContains(t, err, "pixiv account service is not configured")
}

// fakeAccountStore 实现 AccountStore；只覆写本组测试经过的方法。
type fakeAccountStore struct {
	importAccount      func(context.Context, string, bool) (pixivapp.Account, error)
	listAccounts       func(context.Context) ([]pixivapp.Account, error)
	useAccount         func(context.Context, int64) error
	removeAccount      func(context.Context, int64) error
	checkAccount       func(context.Context, int64) (pixivapp.Account, error)
	checkRefreshToken  func(context.Context, string) (pixivapp.Account, error)
	exportRefreshToken func(context.Context, int64) (string, error)
	refreshAccount     func(context.Context, int64) (pixivapp.Account, error)
	currentUser        func(context.Context) (*pixivapp.Account, error)
	restoreAccount     func(context.Context, pixivapp.Account, string, bool) error
	accountsWithTokens func(context.Context) ([]pixivapp.AccountWithToken, error)
}

func (f *fakeAccountStore) ImportAccount(ctx context.Context, token string, setDefault bool) (pixivapp.Account, error) {
	if f.importAccount != nil {
		return f.importAccount(ctx, token, setDefault)
	}
	return pixivapp.Account{}, errors.New("unexpected import")
}

func (f *fakeAccountStore) ListAccounts(ctx context.Context) ([]pixivapp.Account, error) {
	if f.listAccounts != nil {
		return f.listAccounts(ctx)
	}
	return []pixivapp.Account{}, nil
}

func (f *fakeAccountStore) UseAccount(ctx context.Context, userID int64) error {
	if f.useAccount != nil {
		return f.useAccount(ctx, userID)
	}
	return nil
}

func (f *fakeAccountStore) RemoveAccount(ctx context.Context, userID int64) error {
	if f.removeAccount != nil {
		return f.removeAccount(ctx, userID)
	}
	return nil
}

func (f *fakeAccountStore) CheckAccount(ctx context.Context, userID int64) (pixivapp.Account, error) {
	if f.checkAccount != nil {
		return f.checkAccount(ctx, userID)
	}
	return pixivapp.Account{}, errors.New("unexpected check")
}

func (f *fakeAccountStore) CheckRefreshToken(ctx context.Context, token string) (pixivapp.Account, error) {
	if f.checkRefreshToken != nil {
		return f.checkRefreshToken(ctx, token)
	}
	return pixivapp.Account{}, errors.New("unexpected refresh token check")
}

func (f *fakeAccountStore) ExportRefreshToken(ctx context.Context, userID int64) (string, error) {
	if f.exportRefreshToken != nil {
		return f.exportRefreshToken(ctx, userID)
	}
	return "", errors.New("unexpected export")
}

func (f *fakeAccountStore) RefreshAccount(ctx context.Context, userID int64) (pixivapp.Account, error) {
	if f.refreshAccount != nil {
		return f.refreshAccount(ctx, userID)
	}
	return pixivapp.Account{}, errors.New("unexpected refresh")
}

func (f *fakeAccountStore) CurrentUser(ctx context.Context) (*pixivapp.Account, error) {
	if f.currentUser != nil {
		return f.currentUser(ctx)
	}
	return &pixivapp.Account{}, nil
}

func (f *fakeAccountStore) RestoreAccount(ctx context.Context, account pixivapp.Account, token string, setDefault bool) error {
	if f.restoreAccount != nil {
		return f.restoreAccount(ctx, account, token, setDefault)
	}
	return nil
}

func (f *fakeAccountStore) AccountsWithTokens(ctx context.Context) ([]pixivapp.AccountWithToken, error) {
	if f.accountsWithTokens != nil {
		return f.accountsWithTokens(ctx)
	}
	return []pixivapp.AccountWithToken{}, nil
}

func newAccountServiceForTest(store AccountStore) AccountService {
	return AccountService{Pixiv: store}
}
