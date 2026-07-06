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
	Name       string
	Code       string
	Verifier   string
	OAuthBase  string
	UseProfile bool
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
	name := strings.TrimSpace(req.Name)
	if err := auth.ValidateAccountName(name); err != nil {
		return AccountResult{}, err
	}
	cfg, err := s.runtime()
	if err != nil {
		return AccountResult{}, err
	}
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
	store, err := s.authStore()
	if err != nil {
		return AccountResult{}, err
	}
	store.Upsert(auth.Account{Name: name, RefreshToken: token.RefreshToken, UserID: token.UserID})
	if req.UseProfile || store.DefaultAccount == "" {
		store.DefaultAccount = name
	}
	if err := s.Auth.Save(store); err != nil {
		return AccountResult{}, err
	}
	return accountResult(name, store.DefaultAccount == name, token.UserID, true), nil
}

func (s LoginService) authStore() (auth.AuthStore, error) {
	if s.Auth == nil {
		return auth.AuthStore{}, errors.New("auth repository is not configured")
	}
	return s.Auth.Load()
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
