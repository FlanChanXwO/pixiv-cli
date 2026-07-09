package application

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRefreshTokenPriority(t *testing.T) {
	store := auth.AuthStore{
		DefaultUserID: 111,
		Accounts: []auth.Account{
			{UserID: 111, RefreshToken: "main-token"},
			{UserID: 222, RefreshToken: "other-token"},
		},
	}

	token, err := ResolveRefreshToken(store, 0, "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "env-token", token)

	token, err = ResolveRefreshToken(store, 222, "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "other-token", token)

	token, err = ResolveRefreshToken(store, 222, "flag-token", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "flag-token", token)
}

func TestClientResolverDoesNotReadEnvironmentWithoutInjectedProvider(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "env-token")
	repo := &memoryAuthRepository{}
	resolver := ClientResolver{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewClient: func(cfg config.RuntimeConfig) (ClientBundle, error) {
			assert.Empty(t, cfg.RefreshToken)
			return ClientBundle{}, assert.AnError
		},
	}

	_, err := resolver.Resolve(context.Background(), ClientRequest{})
	require.ErrorIs(t, err, assert.AnError)
}

func TestApplicationServicesReportMissingDependencies(t *testing.T) {
	_, err := ConfigService{}.Path()
	require.ErrorContains(t, err, "config store is not configured")

	_, err = ClientResolver{
		Auth:        &memoryAuthRepository{},
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
	}.Resolve(context.Background(), ClientRequest{})
	require.ErrorContains(t, err, "pixiv client factory is not configured")

	_, err = AccountService{
		Auth:        &memoryAuthRepository{store: auth.AuthStore{Accounts: []auth.Account{{UserID: 123, RefreshToken: "token"}}}},
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
	}.Check(context.Background(), 123)
	require.ErrorContains(t, err, "pixiv client factory is not configured")

	_, err = LoginService{
		Auth:        &memoryAuthRepository{},
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
	}.Complete(context.Background(), LoginCompleteRequest{Code: "code", Verifier: "verifier"})
	require.ErrorContains(t, err, "oauth client factory is not configured")
}

func TestAccountServiceAddListUsesAuthenticatedUID(t *testing.T) {
	repo := &memoryAuthRepository{}
	service := AccountService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewClient: func(cfg config.RuntimeConfig) (AuthenticatedPixivClient, error) {
			return &fakeAuthenticatedPixivClient{userID: 123, username: "alice"}, nil
		},
	}

	first, err := service.Add(context.Background(), AccountAddRequest{TokenInput: "foo=bar; refresh_token=main%2Ftoken"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 123, Username: "alice", Default: true, HasToken: true}, first)

	service.NewClient = func(cfg config.RuntimeConfig) (AuthenticatedPixivClient, error) {
		return &fakeAuthenticatedPixivClient{userID: 456, username: "bob"}, nil
	}
	second, err := service.Add(context.Background(), AccountAddRequest{TokenInput: "other-token"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 456, Username: "bob", Default: false, HasToken: true}, second)

	list, err := service.List()
	require.NoError(t, err)
	assert.Equal(t, int64(123), list.DefaultUserID)
	require.Len(t, list.Accounts, 2)
	assert.Equal(t, int64(123), list.Accounts[0].UserID)
	assert.Equal(t, "alice", list.Accounts[0].Username)
	assert.Equal(t, int64(456), list.Accounts[1].UserID)
	assert.Equal(t, "bob", list.Accounts[1].Username)
	assert.True(t, list.Accounts[0].HasToken)
	assert.True(t, list.Accounts[1].HasToken)

	store, err := repo.Load()
	require.NoError(t, err)
	require.Len(t, store.Accounts, 2)
	assert.Equal(t, "main/token", store.Accounts[0].RefreshToken)
	assert.Equal(t, "other-token", store.Accounts[1].RefreshToken)
}

func TestAccountServiceAddWarnsWhenUsernameLookupFails(t *testing.T) {
	repo := &memoryAuthRepository{}
	service := AccountService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewClient: func(cfg config.RuntimeConfig) (AuthenticatedPixivClient, error) {
			return &fakeAuthenticatedPixivClient{
				userID:    999,
				detailErr: errors.New("detail unavailable"),
			}, nil
		},
	}

	result, err := service.Add(context.Background(), AccountAddRequest{TokenInput: "refresh-token"})
	require.NoError(t, err)
	assert.Equal(t, int64(999), result.UserID)
	assert.Empty(t, result.Username)
	assert.Contains(t, result.Warning, "detail unavailable")

	store, err := repo.Load()
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(999), store.Accounts[0].UserID)
	assert.Empty(t, store.Accounts[0].Username)
}

func TestAccountServiceAddRecreatesLegacyAuthStore(t *testing.T) {
	repo := &legacyThenMemoryAuthRepository{}
	service := AccountService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewClient: func(cfg config.RuntimeConfig) (AuthenticatedPixivClient, error) {
			return &fakeAuthenticatedPixivClient{userID: 101, username: "new-user"}, nil
		},
	}

	result, err := service.Add(context.Background(), AccountAddRequest{TokenInput: "new-refresh"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 101, Username: "new-user", Default: true, HasToken: true}, result)

	store, err := repo.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(101), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "new-refresh", store.Accounts[0].RefreshToken)
}

func TestAccountServiceCheckRejectsMismatchedUID(t *testing.T) {
	repo := &memoryAuthRepository{store: auth.AuthStore{
		DefaultUserID: 123,
		Accounts:      []auth.Account{{UserID: 123, RefreshToken: "wrong-token"}},
	}}
	service := AccountService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewClient: func(cfg config.RuntimeConfig) (AuthenticatedPixivClient, error) {
			return &fakeAuthenticatedPixivClient{userID: 456, username: "other-user"}, nil
		},
	}

	_, err := service.Check(context.Background(), 123)
	require.ErrorContains(t, err, "returned uid 456")

	store, err := repo.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(123), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(123), store.Accounts[0].UserID)
	assert.Empty(t, store.Accounts[0].Username)
}

func TestLoginServiceCompleteStoresOAuthUID(t *testing.T) {
	repo := &memoryAuthRepository{}
	service := LoginService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewOAuth: func(config.RuntimeConfig, string) (OAuthExchanger, error) {
			return fakeOAuthExchanger{
				token: OAuthToken{RefreshToken: "login-refresh", UserID: 789, Username: "carol"},
			}, nil
		},
	}

	result, err := service.Complete(context.Background(), LoginCompleteRequest{
		Code:          "code",
		Verifier:      "verifier",
		UseAfterLogin: true,
	})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 789, Username: "carol", Default: true, HasToken: true}, result)

	store, err := repo.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(789), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(789), store.Accounts[0].UserID)
	assert.Equal(t, "carol", store.Accounts[0].Username)
	assert.Equal(t, "login-refresh", store.Accounts[0].RefreshToken)
}

func TestLoginServiceCompleteRecreatesLegacyAuthStore(t *testing.T) {
	repo := &legacyThenMemoryAuthRepository{}
	service := LoginService{
		Auth:        repo,
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
		NewOAuth: func(config.RuntimeConfig, string) (OAuthExchanger, error) {
			return fakeOAuthExchanger{token: OAuthToken{RefreshToken: "login-refresh", UserID: 202, Username: "login-user"}}, nil
		},
	}

	result, err := service.Complete(context.Background(), LoginCompleteRequest{Code: "code", Verifier: "verifier"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 202, Username: "login-user", Default: true, HasToken: true}, result)

	store, err := repo.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(202), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
}

type fakeOAuthExchanger struct {
	token OAuthToken
}

func (e fakeOAuthExchanger) ExchangeAuthorizationCode(context.Context, string, string) (OAuthToken, error) {
	return e.token, nil
}

type fakeAuthenticatedPixivClient struct {
	userID    int64
	username  string
	detailErr error
}

func (c *fakeAuthenticatedPixivClient) Refresh(context.Context) error { return nil }
func (c *fakeAuthenticatedPixivClient) RefreshTokenValue() string     { return "" }
func (c *fakeAuthenticatedPixivClient) UserID() int64                 { return c.userID }
func (c *fakeAuthenticatedPixivClient) UserName() string              { return c.username }
func (c *fakeAuthenticatedPixivClient) UserDetail(context.Context, int64) (*pixiv.User, error) {
	if c.detailErr != nil {
		return nil, c.detailErr
	}
	return &pixiv.User{ID: c.userID, Name: c.username}, nil
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
