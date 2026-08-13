package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationServicesReportMissingDependencies(t *testing.T) {
	_, err := (config.Store{}).Path()
	require.ErrorContains(t, err, "config file store is not configured")

	_, err = accountpixiv.AccountService{}.Check(context.Background(), 123)
	require.ErrorContains(t, err, "pixiv account service is not configured")
}

func TestAccountServiceUsesAuthDBAccountStore(t *testing.T) {
	callOrder := make([]string, 0, 2)
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]accountpixiv.AccountSummary, error) {
		callOrder = append(callOrder, "list")
		return []accountpixiv.AccountSummary{{UserID: 123, Username: "alice", Default: true}}, nil
	}
	store.importAccount = func(_ context.Context, token string, setDefault bool) (accountpixiv.AccountSummary, error) {
		callOrder = append(callOrder, "import")
		if token != "main/token" {
			t.Fatalf("ImportAccount token=%q", token)
		}
		return accountpixiv.AccountSummary{UserID: 456, Username: "bob"}, nil
	}
	service := newAccountServiceForTest(store)

	result, err := service.Import(context.Background(), accountpixiv.AccountImportRequest{TokenInput: "  main/token  "})
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
	store.listAccounts = func(context.Context) ([]accountpixiv.AccountSummary, error) {
		return []accountpixiv.AccountSummary{{UserID: 456, Username: "before"}}, nil
	}
	store.importAccount = func(context.Context, string, bool) (accountpixiv.AccountSummary, error) {
		return accountpixiv.AccountSummary{UserID: 456, Username: "after", Default: true}, nil
	}

	result, err := newAccountServiceForTest(store).Import(context.Background(), accountpixiv.AccountImportRequest{TokenInput: "opaque-token"})
	require.NoError(t, err)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"user_id":456,"username":"after","status":"updated"}`, string(body))
}

func TestAccountServiceExportUsesLocalTokens(t *testing.T) {
	store := &fakeAccountStore{}
	store.accountsWithTokens = func(context.Context) ([]accountpixiv.AccountWithToken, error) {
		return []accountpixiv.AccountWithToken{{UserID: 456, Username: "alice", Default: true, RefreshToken: "opaque-exported-token"}}, nil
	}

	result, err := accountpixiv.AccountService{Pixiv: store}.Export(accountpixiv.AccountExportRequest{UserID: 456})
	require.NoError(t, err)
	assert.Equal(t, "opaque-exported-token", result.RefreshToken)
	assert.Equal(t, 1, result.AccountCount)
}

func TestAccountServiceExportAllEncodesBundle(t *testing.T) {
	store := &fakeAccountStore{}
	store.accountsWithTokens = func(context.Context) ([]accountpixiv.AccountWithToken, error) {
		return []accountpixiv.AccountWithToken{
			{UserID: 456, Username: "alice", Default: true, RefreshToken: "token-a"},
			{UserID: 789, Username: "bob", RefreshToken: "token-b"},
		}, nil
	}

	result, err := accountpixiv.AccountService{Pixiv: store}.Export(accountpixiv.AccountExportRequest{All: true})
	require.NoError(t, err)
	assert.Equal(t, 2, result.AccountCount)
	assert.JSONEq(t, `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":456,"accounts":[{"user_id":456,"username":"alice","refresh_token":"token-a"},{"user_id":789,"username":"bob","refresh_token":"token-b"}]}`, string(result.Bundle))
}

func TestAccountServiceImportBundleRestoresOffline(t *testing.T) {
	var restored []accountpixiv.AccountSummary
	var tokens []string
	store := &fakeAccountStore{}
	store.restoreAccount = func(_ context.Context, account accountpixiv.AccountSummary, token string, setDefault bool) error {
		restored = append(restored, account)
		tokens = append(tokens, token)
		return nil
	}
	const body = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":321,"accounts":[{"user_id":654,"username":"new","refresh_token":"new-secret"},{"user_id":321,"username":"updated","refresh_token":"updated-secret"}]}`

	result, err := accountpixiv.AccountService{Pixiv: store}.ImportBundle([]byte(body))
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
	store.checkAccount = func(_ context.Context, userID int64) (accountpixiv.AccountSummary, error) {
		assert.Equal(t, int64(123), userID)
		return accountpixiv.AccountSummary{UserID: 123, Username: "alice", Default: true}, nil
	}
	result, err := newAccountServiceForTest(store).CheckWithRequest(context.Background(), accountpixiv.AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, accountpixiv.AccountResult{UserID: 123, Username: "alice", Default: true, HasToken: true}, result)
}

func TestAccountServiceCheckUsesStoredAccount(t *testing.T) {
	store := &fakeAccountStore{}
	store.currentUser = func(context.Context) (*accountpixiv.AccountSummary, error) {
		return &accountpixiv.AccountSummary{UserID: 123}, nil
	}
	store.checkAccount = func(_ context.Context, userID int64) (accountpixiv.AccountSummary, error) {
		assert.Equal(t, int64(123), userID)
		return accountpixiv.AccountSummary{UserID: 123, Username: "stored"}, nil
	}
	store.checkRefreshToken = func(_ context.Context, token string) (accountpixiv.AccountSummary, error) {
		t.Fatalf("CheckRefreshToken(%q) must not be called for a stored-account check", token)
		return accountpixiv.AccountSummary{}, nil
	}
	service := newAccountServiceForTest(store)

	result, err := service.CheckWithRequest(context.Background(), accountpixiv.AccountCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, accountpixiv.AccountResult{UserID: 123, Username: "stored", HasToken: true}, result)
}

func TestAccountServiceCheckExplicitUIDDoesNotUseEnvironmentToken(t *testing.T) {
	store := &fakeAccountStore{}
	store.checkAccount = func(_ context.Context, userID int64) (accountpixiv.AccountSummary, error) {
		assert.Equal(t, int64(123), userID)
		return accountpixiv.AccountSummary{UserID: 123, Username: "stored", Default: true}, nil
	}
	store.checkRefreshToken = func(_ context.Context, token string) (accountpixiv.AccountSummary, error) {
		t.Fatalf("CheckRefreshToken(%q) was called for an explicit UID", token)
		return accountpixiv.AccountSummary{}, nil
	}
	service := newAccountServiceForTest(store)

	result, err := service.CheckWithRequest(context.Background(), accountpixiv.AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, accountpixiv.AccountResult{UserID: 123, Username: "stored", Default: true, HasToken: true}, result)
}

func TestAccountServiceRefreshUpdatesPremiumStatus(t *testing.T) {
	premium := false
	store := &fakeAccountStore{}
	store.refreshAccount = func(_ context.Context, userID int64) (accountpixiv.AccountSummary, error) {
		assert.Equal(t, int64(123), userID)
		return accountpixiv.AccountSummary{UserID: 123, Username: "refreshed-name", Default: true, Premium: &premium}, nil
	}

	result, err := accountpixiv.AccountService{Pixiv: store}.RefreshWithRequest(context.Background(), accountpixiv.AccountRefreshRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, accountpixiv.AccountResult{UserID: 123, Username: "refreshed-name", Default: true, HasToken: true, PremiumStatus: &premium}, result)
}

func TestAccountServiceImportPassesCookieShapedInputToOAuthBoundary(t *testing.T) {
	var got string
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]accountpixiv.AccountSummary, error) { return nil, nil }
	store.importAccount = func(_ context.Context, token string, _ bool) (accountpixiv.AccountSummary, error) {
		got = token
		return accountpixiv.AccountSummary{UserID: 123, Username: "oauth-boundary"}, nil
	}

	_, err := (accountpixiv.AccountService{Pixiv: store}).Import(context.Background(), accountpixiv.AccountImportRequest{TokenInput: "refresh_token=secret"})
	require.NoError(t, err)
	assert.Equal(t, "refresh_token=secret", got)
}

func TestAccountServiceRemoveReturnsRemovedAccountAndUpdatedDefault(t *testing.T) {
	listCalls := 0
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]accountpixiv.AccountSummary, error) {
		listCalls++
		if listCalls == 1 {
			return []accountpixiv.AccountSummary{
				{UserID: 123, Username: "alice", Default: true},
				{UserID: 456, Username: "bob"},
			}, nil
		}
		return []accountpixiv.AccountSummary{
			{UserID: 456, Username: "bob", Default: true},
		}, nil
	}
	store.removeAccount = func(_ context.Context, userID int64) error {
		assert.Equal(t, int64(123), userID)
		return nil
	}

	removed, defaultUserID, err := newAccountServiceForTest(store).Remove(123)
	require.NoError(t, err)
	assert.Equal(t, accountpixiv.AccountResult{UserID: 123, Username: "alice", HasToken: true}, removed)
	assert.Equal(t, int64(456), defaultUserID)
	assert.Equal(t, 2, listCalls)
}

func TestAccountServiceRemoveReportsNotFoundWithoutMutation(t *testing.T) {
	store := &fakeAccountStore{}
	store.listAccounts = func(context.Context) ([]accountpixiv.AccountSummary, error) {
		return []accountpixiv.AccountSummary{{UserID: 123, Username: "alice", Default: true}}, nil
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
	_, err := (accountpixiv.AccountService{Pixiv: nil}).Use(123)
	require.ErrorContains(t, err, "pixiv account service is not configured")
}

// fakeAccountStore 实现 accountpixiv.AccountStore；只覆写本组测试经过的方法。
type fakeAccountStore struct {
	importAccount      func(context.Context, string, bool) (accountpixiv.AccountSummary, error)
	listAccounts       func(context.Context) ([]accountpixiv.AccountSummary, error)
	useAccount         func(context.Context, int64) error
	removeAccount      func(context.Context, int64) error
	checkAccount       func(context.Context, int64) (accountpixiv.AccountSummary, error)
	checkRefreshToken  func(context.Context, string) (accountpixiv.AccountSummary, error)
	exportRefreshToken func(context.Context, int64) (string, error)
	refreshAccount     func(context.Context, int64) (accountpixiv.AccountSummary, error)
	currentUser        func(context.Context) (*accountpixiv.AccountSummary, error)
	restoreAccount     func(context.Context, accountpixiv.AccountSummary, string, bool) error
	accountsWithTokens func(context.Context) ([]accountpixiv.AccountWithToken, error)
}

func (f *fakeAccountStore) ImportAccount(ctx context.Context, token string, setDefault bool) (accountpixiv.AccountSummary, error) {
	if f.importAccount != nil {
		return f.importAccount(ctx, token, setDefault)
	}
	return accountpixiv.AccountSummary{}, errors.New("unexpected import")
}

func (f *fakeAccountStore) ListAccounts(ctx context.Context) ([]accountpixiv.AccountSummary, error) {
	if f.listAccounts != nil {
		return f.listAccounts(ctx)
	}
	return []accountpixiv.AccountSummary{}, nil
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

func (f *fakeAccountStore) CheckAccount(ctx context.Context, userID int64) (accountpixiv.AccountSummary, error) {
	if f.checkAccount != nil {
		return f.checkAccount(ctx, userID)
	}
	return accountpixiv.AccountSummary{}, errors.New("unexpected check")
}

func (f *fakeAccountStore) CheckRefreshToken(ctx context.Context, token string) (accountpixiv.AccountSummary, error) {
	if f.checkRefreshToken != nil {
		return f.checkRefreshToken(ctx, token)
	}
	return accountpixiv.AccountSummary{}, errors.New("unexpected refresh token check")
}

func (f *fakeAccountStore) ExportRefreshToken(ctx context.Context, userID int64) (string, error) {
	if f.exportRefreshToken != nil {
		return f.exportRefreshToken(ctx, userID)
	}
	return "", errors.New("unexpected export")
}

func (f *fakeAccountStore) RefreshAccount(ctx context.Context, userID int64) (accountpixiv.AccountSummary, error) {
	if f.refreshAccount != nil {
		return f.refreshAccount(ctx, userID)
	}
	return accountpixiv.AccountSummary{}, errors.New("unexpected refresh")
}

func (f *fakeAccountStore) CurrentUser(ctx context.Context) (*accountpixiv.AccountSummary, error) {
	if f.currentUser != nil {
		return f.currentUser(ctx)
	}
	return &accountpixiv.AccountSummary{}, nil
}

func (f *fakeAccountStore) RestoreAccount(ctx context.Context, account accountpixiv.AccountSummary, token string, setDefault bool) error {
	if f.restoreAccount != nil {
		return f.restoreAccount(ctx, account, token, setDefault)
	}
	return nil
}

func (f *fakeAccountStore) AccountsWithTokens(ctx context.Context) ([]accountpixiv.AccountWithToken, error) {
	if f.accountsWithTokens != nil {
		return f.accountsWithTokens(ctx)
	}
	return []accountpixiv.AccountWithToken{}, nil
}

func newAccountServiceForTest(store accountpixiv.AccountStore) accountpixiv.AccountService {
	return accountpixiv.AccountService{Pixiv: store}
}
