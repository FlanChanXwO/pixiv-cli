package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
)

type AuthRepository interface {
	Load() (auth.AuthStore, error)
	Save(auth.AuthStore) error
}

type AuthenticatedPixivClient interface {
	Refresh(context.Context) error
	RefreshTokenValue() string
	UserID() int64
	UserName() string
	UserDetail(context.Context, int64) (*pixiv.User, error)
}

type AuthenticatedPixivFactory func(config.RuntimeConfig) (AuthenticatedPixivClient, error)

type AccountService struct {
	Auth            AuthRepository
	LoadRuntime     func() (config.RuntimeConfig, error)
	RefreshTokenEnv func() string
	NewClient       AuthenticatedPixivFactory
}

type AccountAddRequest struct {
	TokenInput         string
	HTTPSProxyOverride *string
}

type AccountCheckRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

type AccountResult struct {
	UserID   int64  `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Default  bool   `json:"default"`
	HasToken bool   `json:"has_token"`
	Warning  string `json:"warning,omitempty"`
}

type AccountListResult struct {
	DefaultUserID int64
	Accounts      []AccountResult
}

func (s AccountService) Add(ctx context.Context, req AccountAddRequest) (AccountResult, error) {
	token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(req.TokenInput)
	if token == "" {
		if parsedCookie {
			return AccountResult{}, errors.New("cookie does not contain refresh_token")
		}
		return AccountResult{}, errors.New("refresh token cannot be empty")
	}
	store, err := s.authStoreForRecreate()
	if err != nil {
		return AccountResult{}, err
	}
	cfg, err := s.runtime()
	if err != nil {
		return AccountResult{}, err
	}
	applyHTTPSProxyOverride(&cfg, req.HTTPSProxyOverride)
	cfg.RefreshToken = token
	acct, warning, err := s.identityAccount(ctx, cfg)
	if err != nil {
		return AccountResult{}, err
	}
	store.Upsert(acct)
	if store.DefaultUserID == 0 {
		store.DefaultUserID = acct.UserID
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, err
	}
	result := accountResult(acct, store.DefaultUserID == acct.UserID)
	result.Warning = warning
	return result, nil
}

func (s AccountService) List() (AccountListResult, error) {
	store, err := s.authStore()
	if err != nil {
		return AccountListResult{}, err
	}
	out := AccountListResult{DefaultUserID: store.DefaultUserID}
	for _, acct := range store.Accounts {
		out.Accounts = append(out.Accounts, accountResult(acct, acct.UserID == store.DefaultUserID))
	}
	return out, nil
}

func (s AccountService) Remove(userID int64) (AccountResult, int64, error) {
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, 0, err
	}
	_, acct, ok := store.Get(userID)
	if !ok {
		return AccountResult{}, 0, fmt.Errorf("account uid %d not found", userID)
	}
	if !store.Remove(userID) {
		return AccountResult{}, 0, fmt.Errorf("account uid %d not found", userID)
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, 0, err
	}
	return accountResult(acct, false), store.DefaultUserID, nil
}

func (s AccountService) Use(userID int64) (int64, error) {
	store, err := s.authStore()
	if err != nil {
		return 0, err
	}
	if !store.Has(userID) {
		return 0, fmt.Errorf("account uid %d not found", userID)
	}
	store.DefaultUserID = userID
	if err := s.Auth.Save(store); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s AccountService) Check(ctx context.Context, userID int64) (AccountResult, error) {
	return s.CheckWithRequest(ctx, AccountCheckRequest{UserID: userID})
}

func (s AccountService) CheckWithRequest(ctx context.Context, req AccountCheckRequest) (AccountResult, error) {
	cfg, err := s.runtime()
	if err != nil {
		return AccountResult{}, err
	}
	applyHTTPSProxyOverride(&cfg, req.HTTPSProxyOverride)
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, err
	}
	selectedUserID := int64(0)
	if req.UserID != 0 {
		selectedUserID = req.UserID
		_, acct, ok := auth.SelectAuthAccount(store, selectedUserID)
		if !ok {
			return AccountResult{}, fmt.Errorf("account uid %d not found", selectedUserID)
		}
		cfg.RefreshToken = acct.RefreshToken
	} else if token := s.refreshTokenFromEnv(); token != "" {
		cfg.RefreshToken = token
	} else if chosen, acct, ok := auth.SelectAuthAccount(store, 0); ok {
		selectedUserID = chosen
		cfg.RefreshToken = acct.RefreshToken
	}
	if cfg.RefreshToken == "" {
		return AccountResult{}, errors.New("no account or PIXIV_REFRESH_TOKEN to check")
	}
	acct, warning, err := s.identityAccount(ctx, cfg)
	if err != nil {
		return AccountResult{}, err
	}
	if selectedUserID != 0 && acct.UserID != selectedUserID {
		return AccountResult{}, fmt.Errorf("account uid %d token returned uid %d", selectedUserID, acct.UserID)
	}
	if selectedUserID != 0 {
		if idx, old, ok := store.Get(selectedUserID); ok {
			old.RefreshToken = acct.RefreshToken
			old.UserID = acct.UserID
			old.Username = acct.Username
			store.Accounts[idx] = old
			if err := s.Auth.Save(store); err != nil {
				return AccountResult{}, err
			}
		}
	}
	result := accountResult(acct, selectedUserID != 0 && selectedUserID == store.DefaultUserID)
	result.Warning = warning
	return result, nil
}

func (s AccountService) authStore() (auth.AuthStore, error) {
	if s.Auth == nil {
		return auth.AuthStore{}, errors.New("auth repository is not configured")
	}
	return s.Auth.Load()
}

func (s AccountService) authStoreForRecreate() (auth.AuthStore, error) {
	store, err := s.authStore()
	if auth.IsLegacySchemaError(err) {
		return auth.AuthStore{Accounts: []auth.Account{}}, nil
	}
	return store, err
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

func (s AccountService) identityAccount(ctx context.Context, cfg config.RuntimeConfig) (auth.Account, string, error) {
	if s.NewClient == nil {
		return auth.Account{}, "", errors.New("pixiv client factory is not configured")
	}
	client, err := s.NewClient(cfg)
	if err != nil {
		return auth.Account{}, "", err
	}
	if err := client.Refresh(ctx); err != nil {
		return auth.Account{}, "", err
	}
	userID := client.UserID()
	if userID == 0 {
		return auth.Account{}, "", errors.New("token refresh response did not include user_id")
	}
	refreshToken := client.RefreshTokenValue()
	if strings.TrimSpace(refreshToken) == "" {
		refreshToken = cfg.RefreshToken
	}
	username := strings.TrimSpace(client.UserName())
	warning := ""
	if username == "" {
		user, err := client.UserDetail(ctx, userID)
		if err != nil {
			warning = fmt.Sprintf("username lookup failed: %v", err)
		} else if user != nil {
			username = strings.TrimSpace(user.Name)
		}
	}
	return auth.Account{UserID: userID, Username: username, RefreshToken: refreshToken}, warning, nil
}

func accountResult(acct auth.Account, isDefault bool) AccountResult {
	return AccountResult{
		UserID:   acct.UserID,
		Username: acct.Username,
		Default:  isDefault,
		HasToken: acct.RefreshToken != "",
	}
}

func ParseUID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("uid cannot be empty")
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid uid %q", raw)
	}
	return userID, nil
}

func ResolveRefreshToken(store auth.AuthStore, requestedUserID int64, requestedToken string, refreshTokenEnv func() string) (string, error) {
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
	if requestedUserID != 0 {
		_, acct, ok := auth.SelectAuthAccount(store, requestedUserID)
		if !ok {
			return "", fmt.Errorf("account uid %d not found", requestedUserID)
		}
		return acct.RefreshToken, nil
	}
	if refreshTokenEnv != nil {
		if token := refreshTokenEnv(); token != "" {
			return token, nil
		}
	}
	if _, acct, ok := auth.SelectAuthAccount(store, 0); ok {
		return acct.RefreshToken, nil
	}
	return "", nil
}
