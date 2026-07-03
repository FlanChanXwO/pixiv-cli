package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/utils"
)

type AuthRepository interface {
	Load() (auth.AuthStore, error)
	Save(auth.AuthStore) error
}

type AuthenticatedPixivClient interface {
	Refresh(context.Context) error
	UserID() int64
}

type AuthenticatedPixivFactory func(config.RuntimeConfig) (AuthenticatedPixivClient, error)

type AccountService struct {
	Auth            AuthRepository
	LoadRuntime     func() (config.RuntimeConfig, error)
	RefreshTokenEnv func() string
	NewClient       AuthenticatedPixivFactory
}

type AccountAddRequest struct {
	Name       string
	TokenInput string
}

type AccountResult struct {
	Name     string
	Default  bool
	UserID   int64
	HasToken bool
}

type AccountListResult struct {
	DefaultAccount string
	Accounts       []AccountResult
}

func (s AccountService) Add(req AccountAddRequest) (AccountResult, error) {
	name := strings.TrimSpace(req.Name)
	if err := auth.ValidateAccountName(name); err != nil {
		return AccountResult{}, err
	}
	token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(req.TokenInput)
	if token == "" {
		if parsedCookie {
			return AccountResult{}, errors.New("cookie does not contain refresh_token")
		}
		return AccountResult{}, errors.New("refresh token cannot be empty")
	}
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, err
	}
	userID := int64(0)
	if _, acct, ok := store.Get(name); ok {
		userID = acct.UserID
	}
	store.Upsert(auth.Account{Name: name, RefreshToken: token, UserID: userID})
	if store.DefaultAccount == "" {
		store.DefaultAccount = name
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, err
	}
	return accountResult(name, store.DefaultAccount == name, userID, true), nil
}

func (s AccountService) List() (AccountListResult, error) {
	store, err := s.authStore()
	if err != nil {
		return AccountListResult{}, err
	}
	out := AccountListResult{DefaultAccount: store.DefaultAccount}
	for _, acct := range store.Accounts {
		out.Accounts = append(out.Accounts, accountResult(acct.Name, acct.Name == store.DefaultAccount, acct.UserID, acct.RefreshToken != ""))
	}
	return out, nil
}

func (s AccountService) Remove(name string) (AccountResult, string, error) {
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, "", err
	}
	name = strings.TrimSpace(name)
	if !store.Remove(name) {
		return AccountResult{}, "", fmt.Errorf("account %q not found", name)
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, "", err
	}
	return accountResult(name, false, 0, false), store.DefaultAccount, nil
}

func (s AccountService) Use(name string) (string, error) {
	store, err := s.authStore()
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if !store.Has(name) {
		return "", fmt.Errorf("account %q not found", name)
	}
	store.DefaultAccount = name
	if err := s.Auth.Save(store); err != nil {
		return "", err
	}
	return name, nil
}

func (s AccountService) Check(ctx context.Context, name string) (AccountResult, error) {
	cfg, err := s.runtime()
	if err != nil {
		return AccountResult{}, err
	}
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, err
	}
	selectedName := ""
	if strings.TrimSpace(name) != "" {
		selectedName = strings.TrimSpace(name)
		_, acct, ok := auth.SelectAuthAccount(store, selectedName)
		if !ok {
			return AccountResult{}, fmt.Errorf("account %q not found", selectedName)
		}
		cfg.RefreshToken = acct.RefreshToken
	} else if token := s.refreshTokenFromEnv(); token != "" {
		cfg.RefreshToken = token
	} else if chosen, acct, ok := auth.SelectAuthAccount(store, ""); ok {
		selectedName = chosen
		cfg.RefreshToken = acct.RefreshToken
	}
	if cfg.RefreshToken == "" {
		return AccountResult{}, errors.New("no account or PIXIV_REFRESH_TOKEN to check")
	}
	if s.NewClient == nil {
		return AccountResult{}, errors.New("pixiv client factory is not configured")
	}
	client, err := s.NewClient(cfg)
	if err != nil {
		return AccountResult{}, err
	}
	if err := client.Refresh(ctx); err != nil {
		return AccountResult{}, err
	}
	userID := client.UserID()
	if selectedName != "" {
		if idx, acct, ok := store.Get(selectedName); ok {
			acct.UserID = userID
			store.Accounts[idx] = acct
			if err := s.Auth.Save(store); err != nil {
				return AccountResult{}, err
			}
		}
	}
	return accountResult(selectedName, selectedName != "" && selectedName == store.DefaultAccount, userID, true), nil
}

func (s AccountService) authStore() (auth.AuthStore, error) {
	if s.Auth == nil {
		return auth.AuthStore{}, errors.New("auth repository is not configured")
	}
	return s.Auth.Load()
}

func (s AccountService) runtime() (config.RuntimeConfig, error) {
	if s.LoadRuntime == nil {
		return config.RuntimeConfig{}, errors.New("runtime config loader is not configured")
	}
	return s.LoadRuntime()
}

func (s AccountService) refreshTokenFromEnv() string {
	if s.RefreshTokenEnv != nil {
		return s.RefreshTokenEnv()
	}
	return ""
}

func accountResult(name string, isDefault bool, userID int64, hasToken bool) AccountResult {
	return AccountResult{Name: name, Default: isDefault, UserID: userID, HasToken: hasToken}
}

func ValidateAccountName(name string) error {
	return auth.ValidateAccountName(name)
}

func ResolveRefreshToken(store auth.AuthStore, requestedProfile, requestedToken string, refreshTokenEnv func() string) (string, error) {
	if strings.TrimSpace(requestedToken) != "" {
		token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(requestedToken)
		if token == "" {
			if parsedCookie {
				return "", errors.New("refresh-token cookie does not contain refresh_token")
			}
			return "", errors.New("refresh-token cannot be empty")
		}
		return token, nil
	}
	if profile := strings.TrimSpace(requestedProfile); profile != "" {
		_, acct, ok := auth.SelectAuthAccount(store, profile)
		if !ok {
			return "", fmt.Errorf("account %q not found", profile)
		}
		return acct.RefreshToken, nil
	}
	if refreshTokenEnv != nil {
		if token := refreshTokenEnv(); token != "" {
			return token, nil
		}
	}
	if _, acct, ok := auth.SelectAuthAccount(store, ""); ok {
		return acct.RefreshToken, nil
	}
	return "", nil
}
