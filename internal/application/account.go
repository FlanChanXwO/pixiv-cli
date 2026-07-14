package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// AccountService 只适配 public SDK 的本地账号 API。账号文件、token rotation 和
// OAuth 身份验证均由 SDK 维护，应用层不再复制另一份 auth store 调用链。
type AccountService struct {
	SDK                 SDKService
	RefreshTokenFromEnv func() string
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

func (s AccountService) Add(ctx context.Context, request AccountAddRequest) (AccountResult, error) {
	token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(request.TokenInput)
	if token == "" {
		if parsedCookie {
			return AccountResult{}, errors.New("cookie does not contain refresh_token")
		}
		return AccountResult{}, errors.New("refresh token cannot be empty")
	}
	client, err := s.SDK.Client(SDKClientRequest{HTTPSProxyOverride: request.HTTPSProxyOverride})
	if err != nil {
		return AccountResult{}, err
	}
	account, err := client.ImportAccount(ctx, token)
	if err != nil {
		return AccountResult{}, err
	}
	return sdkAccountResult(*account), nil
}

func (s AccountService) List() (AccountListResult, error) {
	client, err := s.SDK.Client(SDKClientRequest{})
	if err != nil {
		return AccountListResult{}, err
	}
	accounts, err := client.ListAccounts()
	if err != nil {
		return AccountListResult{}, err
	}
	out := AccountListResult{DefaultUserID: accounts.DefaultUserID, Accounts: make([]AccountResult, len(accounts.Accounts))}
	for index, account := range accounts.Accounts {
		out.Accounts[index] = sdkAccountResult(account)
	}
	return out, nil
}

func (s AccountService) Remove(userID int64) (AccountResult, int64, error) {
	list, err := s.List()
	if err != nil {
		return AccountResult{}, 0, err
	}
	var removed AccountResult
	found := false
	for _, account := range list.Accounts {
		if account.UserID == userID {
			removed, found = account, true
			break
		}
	}
	if !found {
		return AccountResult{}, 0, fmt.Errorf("account uid %d not found", userID)
	}
	client, err := s.SDK.Client(SDKClientRequest{})
	if err != nil {
		return AccountResult{}, 0, err
	}
	if err := client.RemoveAccount(userID); err != nil {
		return AccountResult{}, 0, err
	}
	updated, err := s.List()
	if err != nil {
		return AccountResult{}, 0, err
	}
	removed.Default = false
	return removed, updated.DefaultUserID, nil
}

func (s AccountService) Use(userID int64) (int64, error) {
	client, err := s.SDK.Client(SDKClientRequest{})
	if err != nil {
		return 0, err
	}
	if err := client.SelectAccount(userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s AccountService) Check(ctx context.Context, userID int64) (AccountResult, error) {
	return s.CheckWithRequest(ctx, AccountCheckRequest{UserID: userID})
}

func (s AccountService) CheckWithRequest(ctx context.Context, request AccountCheckRequest) (AccountResult, error) {
	client, err := s.SDK.Client(SDKClientRequest{UserID: request.UserID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	if err != nil {
		return AccountResult{}, err
	}
	if request.UserID == 0 {
		if token := s.refreshTokenFromEnv(); token != "" {
			account, err := client.CheckRefreshToken(ctx, token)
			if err != nil {
				return AccountResult{}, err
			}
			return sdkAccountResult(*account), nil
		}
	}
	account, err := client.CheckAccount(ctx, request.UserID)
	if err != nil {
		return AccountResult{}, err
	}
	return sdkAccountResult(*account), nil
}

func sdkAccountResult(account sdk.Account) AccountResult {
	return AccountResult{UserID: account.UserID, Username: account.Username, Default: account.Default, HasToken: account.HasToken}
}

func (s AccountService) refreshTokenFromEnv() string {
	if s.RefreshTokenFromEnv == nil {
		return ""
	}
	return strings.TrimSpace(s.RefreshTokenFromEnv())
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
