package application

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRefreshTokenPriority(t *testing.T) {
	store := auth.AuthStore{
		DefaultAccount: "main",
		Accounts: []auth.Account{
			{Name: "main", RefreshToken: "main-token"},
			{Name: "other", RefreshToken: "other-token"},
		},
	}

	token, err := ResolveRefreshToken(store, "", "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "env-token", token)

	token, err = ResolveRefreshToken(store, "other", "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "other-token", token)

	token, err = ResolveRefreshToken(store, "other", "flag-token", func() string { return "env-token" })
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
		Auth:        &memoryAuthRepository{store: auth.AuthStore{Accounts: []auth.Account{{Name: "main", RefreshToken: "token"}}}},
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
	}.Check(context.Background(), "main")
	require.ErrorContains(t, err, "pixiv client factory is not configured")

	_, err = LoginService{
		Auth:        &memoryAuthRepository{},
		LoadRuntime: func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil },
	}.Complete(context.Background(), LoginCompleteRequest{Name: "main", Code: "code", Verifier: "verifier"})
	require.ErrorContains(t, err, "oauth client factory is not configured")
}

func TestAccountServiceAddListPreservesTokensPrivately(t *testing.T) {
	repo := &memoryAuthRepository{}
	service := AccountService{Auth: repo}

	first, err := service.Add(AccountAddRequest{Name: "main", TokenInput: "foo=bar; refresh_token=main%2Ftoken"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{Name: "main", Default: true, HasToken: true}, first)

	second, err := service.Add(AccountAddRequest{Name: "other", TokenInput: "other-token"})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{Name: "other", Default: false, HasToken: true}, second)

	list, err := service.List()
	require.NoError(t, err)
	assert.Equal(t, "main", list.DefaultAccount)
	require.Len(t, list.Accounts, 2)
	assert.Equal(t, "main", list.Accounts[0].Name)
	assert.Equal(t, "other", list.Accounts[1].Name)
	assert.True(t, list.Accounts[0].HasToken)
	assert.True(t, list.Accounts[1].HasToken)

	store, err := repo.Load()
	require.NoError(t, err)
	require.Len(t, store.Accounts, 2)
	assert.Equal(t, "main/token", store.Accounts[0].RefreshToken)
	assert.Equal(t, "other-token", store.Accounts[1].RefreshToken)
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
