package application

import (
	"context"
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
	client := &fakeAccountSDKClient{accounts: sdk.AccountsResult{DefaultUserID: 123, Accounts: []sdk.Account{{UserID: 123, Username: "alice", Default: true, HasToken: true}}}}
	client.importAccount = func(_ context.Context, token string) (*sdk.Account, error) {
		if token != "main/token" {
			t.Fatalf("ImportAccount token=%q", token)
		}
		return &sdk.Account{UserID: 456, Username: "bob", HasToken: true}, nil
	}
	service := newAccountServiceForTest(client)

	result, err := service.Add(context.Background(), AccountAddRequest{TokenInput: "refresh_token=main%2Ftoken"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 456, Username: "bob", HasToken: true}, result)
	list, err := service.List()
	require.NoError(t, err)
	assert.Equal(t, int64(123), list.DefaultUserID)
	require.Len(t, list.Accounts, 1)
	assert.Equal(t, int64(123), list.Accounts[0].UserID)
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
	service.RefreshTokenFromEnv = func() string { return "environment-token" }

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
	service.RefreshTokenFromEnv = func() string { return "environment-token" }

	result, err := service.CheckWithRequest(context.Background(), AccountCheckRequest{UserID: 123})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "stored", Default: true, HasToken: true}, result)
}

// fakeAccountSDKClient 嵌入完整公开 facade，只覆写本组测试经过的账号方法；这样
// 测试从 application 到公开 SDK 边界观察行为，不再模拟 legacy Source。
type fakeAccountSDKClient struct {
	SDKClient
	accounts          sdk.AccountsResult
	importAccount     func(context.Context, string) (*sdk.Account, error)
	checkAccount      func(context.Context, int64) (*sdk.Account, error)
	checkRefreshToken func(context.Context, string) (*sdk.Account, error)
}

func (f *fakeAccountSDKClient) ImportAccount(ctx context.Context, token string) (*sdk.Account, error) {
	if f.importAccount != nil {
		return f.importAccount(ctx, token)
	}
	return nil, errors.New("unexpected import")
}
func (f *fakeAccountSDKClient) ListAccounts() (*sdk.AccountsResult, error) { return &f.accounts, nil }
func (*fakeAccountSDKClient) SelectAccount(int64) error                    { return nil }
func (*fakeAccountSDKClient) RemoveAccount(int64) error                    { return nil }
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
