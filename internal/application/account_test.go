package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationServicesReportMissingDependencies(t *testing.T) {
	_, err := ConfigService{}.Path()
	require.ErrorContains(t, err, "config store is not configured")

	_, err = AccountService{}.Check(context.Background(), 123)
	require.ErrorContains(t, err, "pixiv sdk client factory is not configured")

}

func TestAccountServiceUsesPublicSDKAccountStore(t *testing.T) {
	callOrder := make([]string, 0, 2)
	client := &fakeAccountSDKClient{accounts: sdk.AccountsResult{DefaultUserID: 123, Accounts: []sdk.Account{{UserID: 123, Username: "alice", Default: true, HasToken: true}}}}
	client.listAccounts = func() (*sdk.AccountsResult, error) {
		callOrder = append(callOrder, "list")
		return &client.accounts, nil
	}
	client.importAccount = func(_ context.Context, token string) (*sdk.Account, error) {
		callOrder = append(callOrder, "import")
		if token != "  main/token  " {
			t.Fatalf("ImportAccount token=%q", token)
		}
		return &sdk.Account{UserID: 456, Username: "bob", HasToken: true}, nil
	}
	service := newAccountServiceForTest(client)

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
	client := &fakeAccountSDKClient{accounts: sdk.AccountsResult{Accounts: []sdk.Account{{UserID: 456, Username: "before", HasToken: true}}}}
	client.importAccount = func(context.Context, string) (*sdk.Account, error) {
		return &sdk.Account{UserID: 456, Username: "after", Default: true, HasToken: true}, nil
	}

	result, err := newAccountServiceForTest(client).Import(context.Background(), AccountImportRequest{TokenInput: "opaque-token"})
	require.NoError(t, err)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"user_id":456,"username":"after","status":"updated"}`, string(body))
}

func TestAccountServiceExportUsesPublicSDKLocalBundle(t *testing.T) {
	client := &fakeAccountSDKClient{}
	client.exportAuthBundle = func(selection sdk.AuthExportSelection) (*sdk.AuthExportBundle, error) {
		assert.Equal(t, sdk.AuthExportSelection{UserID: 456}, selection)
		return &sdk.AuthExportBundle{
			Schema:        sdk.AuthExportBundleSchema,
			Version:       sdk.AuthExportBundleVersion,
			DefaultUserID: 456,
			Accounts:      []sdk.AuthExportSecretAccount{{UserID: 456, RefreshToken: "opaque-exported-token"}},
		}, nil
	}
	var gotRequest SDKClientRequest
	service := AccountService{SDK: SDKService{NewClient: func(request SDKClientRequest) (SDKClient, error) {
		gotRequest = request
		return client, nil
	}}}

	result, err := service.Export(AccountExportRequest{UserID: 456})
	require.NoError(t, err)
	assert.Equal(t, "opaque-exported-token", result.RefreshToken)
	assert.Equal(t, 1, result.AccountCount)
	assert.JSONEq(t, `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":456,"accounts":[{"user_id":456,"username":"","refresh_token":"opaque-exported-token"}]}`, string(result.Bundle))
	assert.Equal(t, SDKClientRequest{}, gotRequest)
}

func TestAccountServiceImportBundleUsesPublicSDKOfflineRestore(t *testing.T) {
	client := &fakeAccountSDKClient{}
	client.restoreAuthBundle = func(bundle *sdk.AuthExportBundle) (*sdk.AuthRestoreResult, error) {
		require.Len(t, bundle.Accounts, 2)
		assert.Equal(t, "new-secret", bundle.Accounts[0].RefreshToken)
		assert.Equal(t, "updated-secret", bundle.Accounts[1].RefreshToken)
		return &sdk.AuthRestoreResult{
			DefaultUserID: 321,
			Added:         []sdk.Account{{UserID: 654, Username: "new", HasToken: true}},
			Updated:       []sdk.Account{{UserID: 321, Username: "updated", Default: true, HasToken: true}},
		}, nil
	}
	var gotRequest SDKClientRequest
	service := AccountService{SDK: SDKService{NewClient: func(request SDKClientRequest) (SDKClient, error) {
		gotRequest = request
		return client, nil
	}}}
	const body = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":321,"accounts":[{"user_id":654,"username":"new","refresh_token":"new-secret"},{"user_id":321,"username":"updated","refresh_token":"updated-secret"}]}`

	result, err := service.ImportBundle([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, SDKClientRequest{}, gotRequest)
	assert.Equal(t, int64(321), result.DefaultUserID)
	resultBody, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"accounts":[{"user_id":654,"username":"new","status":"added"},{"user_id":321,"username":"updated","status":"updated"}],"default_user_id":321}`, string(resultBody))
}

func TestAccountServiceCheckUsesPublicSDKRequest(t *testing.T) {
	client := &fakeAccountSDKClient{}
	client.checkAccount = func(_ context.Context, userID int64) (*sdk.Account, error) {
		assert.Equal(t, int64(123), userID)
		return &sdk.Account{UserID: 123, Username: "alice", Default: true, HasToken: true}, nil
	}
	result, err := newAccountServiceForTest(client).CheckWithRequest(context.Background(), AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "alice", Default: true, HasToken: true}, result)
}

func TestAccountServiceCheckPrefersEnvironmentTokenWithoutSelectingStoredAccount(t *testing.T) {
	client := &fakeAccountSDKClient{}
	client.checkAccount = func(_ context.Context, userID int64) (*sdk.Account, error) {
		t.Fatalf("stored CheckAccount(%d) was called while an environment token exists", userID)
		return nil, nil
	}
	client.checkRefreshToken = func(_ context.Context, token string) (*sdk.Account, error) {
		assert.Equal(t, "environment-token", token)
		return &sdk.Account{UserID: 456, Username: "environment", HasToken: true}, nil
	}
	service := newAccountServiceForTest(client)
	service.RefreshTokenFromEnv = func() (string, error) { return "environment-token", nil }

	result, err := service.CheckWithRequest(context.Background(), AccountCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 456, Username: "environment", HasToken: true}, result)
}

func TestAccountServiceCheckExplicitUIDDoesNotUseEnvironmentToken(t *testing.T) {
	client := &fakeAccountSDKClient{}
	client.checkAccount = func(_ context.Context, userID int64) (*sdk.Account, error) {
		assert.Equal(t, int64(123), userID)
		return &sdk.Account{UserID: 123, Username: "stored", Default: true, HasToken: true}, nil
	}
	client.checkRefreshToken = func(_ context.Context, token string) (*sdk.Account, error) {
		t.Fatalf("CheckRefreshToken(%q) was called for an explicit UID", token)
		return nil, nil
	}
	service := newAccountServiceForTest(client)
	service.RefreshTokenFromEnv = func() (string, error) { return "environment-token", nil }

	result, err := service.CheckWithRequest(context.Background(), AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "stored", Default: true, HasToken: true}, result)
}

func TestAccountServiceImportRejectsCookieBeforeOpeningSDK(t *testing.T) {
	service := AccountService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		t.Fatal("SDK client was opened for a cookie input")
		return nil, nil
	}}}

	_, err := service.Import(context.Background(), AccountImportRequest{TokenInput: "refresh_token=secret"})
	require.ErrorContains(t, err, "cookie input is not supported")
}

func TestAccountServiceRemoveReturnsRemovedAccountAndUpdatedDefault(t *testing.T) {
	listCalls := 0
	client := &fakeAccountSDKClient{}
	client.listAccounts = func() (*sdk.AccountsResult, error) {
		listCalls++
		if listCalls == 1 {
			return &sdk.AccountsResult{DefaultUserID: 123, Accounts: []sdk.Account{
				{UserID: 123, Username: "alice", Default: true, HasToken: true},
				{UserID: 456, Username: "bob", HasToken: true},
			}}, nil
		}
		return &sdk.AccountsResult{DefaultUserID: 456, Accounts: []sdk.Account{
			{UserID: 456, Username: "bob", Default: true, HasToken: true},
		}}, nil
	}
	client.removeAccount = func(userID int64) error {
		assert.Equal(t, int64(123), userID)
		return nil
	}

	removed, defaultUserID, err := newAccountServiceForTest(client).Remove(123)
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "alice", HasToken: true}, removed)
	assert.Equal(t, int64(456), defaultUserID)
	assert.Equal(t, 2, listCalls)
}

func TestAccountServiceRemoveReportsNotFoundWithoutMutation(t *testing.T) {
	client := &fakeAccountSDKClient{accounts: sdk.AccountsResult{
		DefaultUserID: 123,
		Accounts:      []sdk.Account{{UserID: 123, Username: "alice", Default: true, HasToken: true}},
	}}
	client.removeAccount = func(int64) error {
		t.Fatal("RemoveAccount was called for an unknown uid")
		return nil
	}

	_, _, err := newAccountServiceForTest(client).Remove(999)
	require.EqualError(t, err, "account uid 999 not found")
}

func TestAccountServiceRemovePropagatesDependencyErrors(t *testing.T) {
	t.Run("initial client", func(t *testing.T) {
		wantErr := errors.New("open list client")
		service := AccountService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
			return nil, wantErr
		}}}

		_, _, err := service.Remove(123)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("initial list", func(t *testing.T) {
		wantErr := errors.New("list accounts")
		client := &fakeAccountSDKClient{listAccounts: func() (*sdk.AccountsResult, error) {
			return nil, wantErr
		}}

		_, _, err := newAccountServiceForTest(client).Remove(123)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("remove client", func(t *testing.T) {
		wantErr := errors.New("open remove client")
		client := &fakeAccountSDKClient{accounts: sdk.AccountsResult{
			Accounts: []sdk.Account{{UserID: 123, HasToken: true}},
		}}
		calls := 0
		service := AccountService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
			calls++
			if calls == 2 {
				return nil, wantErr
			}
			return client, nil
		}}}

		_, _, err := service.Remove(123)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("remove account", func(t *testing.T) {
		wantErr := errors.New("remove account")
		client := &fakeAccountSDKClient{
			accounts: sdk.AccountsResult{Accounts: []sdk.Account{{UserID: 123, HasToken: true}}},
			removeAccount: func(int64) error {
				return wantErr
			},
		}

		_, _, err := newAccountServiceForTest(client).Remove(123)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("updated list", func(t *testing.T) {
		wantErr := errors.New("updated list accounts")
		listCalls := 0
		client := &fakeAccountSDKClient{listAccounts: func() (*sdk.AccountsResult, error) {
			listCalls++
			if listCalls == 2 {
				return nil, wantErr
			}
			return &sdk.AccountsResult{Accounts: []sdk.Account{{UserID: 123, HasToken: true}}}, nil
		}}

		_, _, err := newAccountServiceForTest(client).Remove(123)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestAccountServiceUseSelectsRequestedAccount(t *testing.T) {
	client := &fakeAccountSDKClient{selectAccount: func(userID int64) error {
		assert.Equal(t, int64(456), userID)
		return nil
	}}

	userID, err := newAccountServiceForTest(client).Use(456)
	require.NoError(t, err)
	assert.Equal(t, int64(456), userID)
}

func TestAccountServiceUsePropagatesDependencyErrors(t *testing.T) {
	clientErr := errors.New("open select client")
	service := AccountService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		return nil, clientErr
	}}}
	_, err := service.Use(123)
	require.ErrorIs(t, err, clientErr)

	selectErr := errors.New("select account")
	client := &fakeAccountSDKClient{selectAccount: func(int64) error { return selectErr }}
	_, err = newAccountServiceForTest(client).Use(123)
	require.ErrorIs(t, err, selectErr)
}

// fakeAccountSDKClient 嵌入完整公开 facade，只覆写本组测试经过的账号方法；这样
// 测试从 application 到公开 SDK 边界观察行为，不再模拟 legacy Source。
type fakeAccountSDKClient struct {
	SDKClient
	accounts          sdk.AccountsResult
	importAccount     func(context.Context, string) (*sdk.Account, error)
	listAccounts      func() (*sdk.AccountsResult, error)
	selectAccount     func(int64) error
	removeAccount     func(int64) error
	checkAccount      func(context.Context, int64) (*sdk.Account, error)
	checkRefreshToken func(context.Context, string) (*sdk.Account, error)
	exportAuthBundle  func(sdk.AuthExportSelection) (*sdk.AuthExportBundle, error)
	restoreAuthBundle func(*sdk.AuthExportBundle) (*sdk.AuthRestoreResult, error)
}

func (f *fakeAccountSDKClient) ExportAuthBundle(selection sdk.AuthExportSelection) (*sdk.AuthExportBundle, error) {
	if f.exportAuthBundle != nil {
		return f.exportAuthBundle(selection)
	}
	return nil, errors.New("unexpected auth bundle export")
}

func (f *fakeAccountSDKClient) RestoreAuthBundle(bundle *sdk.AuthExportBundle) (*sdk.AuthRestoreResult, error) {
	if f.restoreAuthBundle != nil {
		return f.restoreAuthBundle(bundle)
	}
	return nil, errors.New("unexpected auth bundle restore")
}

func (f *fakeAccountSDKClient) ImportAccount(ctx context.Context, token string) (*sdk.Account, error) {
	if f.importAccount != nil {
		return f.importAccount(ctx, token)
	}
	return nil, errors.New("unexpected import")
}
func (f *fakeAccountSDKClient) ListAccounts() (*sdk.AccountsResult, error) {
	if f.listAccounts != nil {
		return f.listAccounts()
	}
	return &f.accounts, nil
}
func (f *fakeAccountSDKClient) SelectAccount(userID int64) error {
	if f.selectAccount != nil {
		return f.selectAccount(userID)
	}
	return nil
}
func (f *fakeAccountSDKClient) RemoveAccount(userID int64) error {
	if f.removeAccount != nil {
		return f.removeAccount(userID)
	}
	return nil
}
func (f *fakeAccountSDKClient) CheckAccount(ctx context.Context, userID int64) (*sdk.Account, error) {
	if f.checkAccount != nil {
		return f.checkAccount(ctx, userID)
	}
	return nil, errors.New("unexpected check")
}

func (f *fakeAccountSDKClient) CheckRefreshToken(ctx context.Context, token string) (*sdk.Account, error) {
	if f.checkRefreshToken != nil {
		return f.checkRefreshToken(ctx, token)
	}
	return nil, errors.New("unexpected refresh token check")
}

func newAccountServiceForTest(client SDKClient) AccountService {
	return AccountService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) { return client, nil }}}
}

type memoryAuthRepository struct {
	store auth.AuthStore
}

func (r *memoryAuthRepository) Load() (auth.AuthStore, error) {
	if r.store.Accounts == nil {
		r.store.Accounts = []auth.Account{}
	}
	return r.store, nil
}

func (r *memoryAuthRepository) Save(store auth.AuthStore) error {
	r.store = store
	return nil
}

type legacyThenMemoryAuthRepository struct {
	store auth.AuthStore
	saved bool
}

func (r *legacyThenMemoryAuthRepository) Load() (auth.AuthStore, error) {
	if !r.saved {
		return auth.AuthStore{}, auth.LegacySchemaError{Field: "default_account"}
	}
	return r.store, nil
}

func (r *legacyThenMemoryAuthRepository) Save(store auth.AuthStore) error {
	r.store = store
	r.saved = true
	return nil
}
