package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

type OAuthToken struct {
	RefreshToken string
	UserID       int64
	Username     string
}

type OAuthExchanger interface {
	ExchangeAuthorizationCode(context.Context, string, string) (OAuthToken, error)
}

type OAuthClientFactory func(config.RuntimeConfig, string) (OAuthExchanger, error)

type LoginService struct {
	Auth        AuthRepository
	LoadRuntime func() (config.RuntimeConfig, error)
	NewOAuth    OAuthClientFactory
}

type LoginStart struct {
	Verifier  string
	Challenge string
	State     string
}

type LoginCompleteRequest struct {
	Code               string
	Verifier           string
	OAuthBase          string
	UseAfterLogin      bool
	HTTPSProxyOverride *string
}

func (s LoginService) Start() (LoginStart, error) {
	verifier, challenge, err := GeneratePKCEPair()
	if err != nil {
		return LoginStart{}, err
	}
	state, err := RandomURLToken(32)
	if err != nil {
		return LoginStart{}, err
	}
	return LoginStart{
		Verifier:  verifier,
		Challenge: challenge,
		State:     state,
	}, nil
}

func (s LoginService) Complete(ctx context.Context, req LoginCompleteRequest) (AccountResult, error) {
	cfg, err := s.runtime()
	if err != nil {
		return AccountResult{}, err
	}
	applyHTTPSProxyOverride(&cfg, req.HTTPSProxyOverride)
	oauthBase := req.OAuthBase
	if strings.TrimSpace(oauthBase) == "" {
		oauthBase = pixiv.DefaultOAuthBase
	}
	if s.NewOAuth == nil {
		return AccountResult{}, errors.New("oauth client factory is not configured")
	}
	client, err := s.NewOAuth(cfg, oauthBase)
	if err != nil {
		return AccountResult{}, err
	}
	token, err := client.ExchangeAuthorizationCode(ctx, req.Code, req.Verifier)
	if err != nil {
		return AccountResult{}, err
	}
	if token.UserID == 0 {
		return AccountResult{}, errors.New("authorization_code response did not include user_id")
	}
	store, err := s.authStoreForRecreate()
	if err != nil {
		return AccountResult{}, err
	}
	acct := auth.Account{UserID: token.UserID, Username: strings.TrimSpace(token.Username), RefreshToken: token.RefreshToken}
	store.Upsert(acct)
	if req.UseAfterLogin || store.DefaultUserID == 0 {
		store.DefaultUserID = token.UserID
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, err
	}
	return accountResult(acct, store.DefaultUserID == token.UserID), nil
}

func (s LoginService) authStore() (auth.AuthStore, error) {
	if s.Auth == nil {
		return auth.AuthStore{}, errors.New("auth repository is not configured")
	}
	return s.Auth.Load()
}

func (s LoginService) authStoreForRecreate() (auth.AuthStore, error) {
	store, err := s.authStore()
	if auth.IsLegacySchemaError(err) {
		return auth.AuthStore{Accounts: []auth.Account{}}, nil
	}
	return store, err
}

func (s LoginService) runtime() (config.RuntimeConfig, error) {
	if s.LoadRuntime == nil {
		return config.RuntimeConfig{}, errors.New("runtime config loader is not configured")
	}
	return s.LoadRuntime()
}

func GeneratePKCEPair() (verifier, challenge string, err error) {
	verifier, err = RandomURLToken(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func RandomURLToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}
