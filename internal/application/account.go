package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// AccountService 只适配 public SDK 的本地账号 API。账号文件、token rotation 和
// OAuth 身份验证均由 SDK 维护，应用层不再复制另一份 auth store 调用链。
type AccountService struct {
	SDK                 SDKService
	RefreshTokenFromEnv func() (string, error)
}

type AccountImportRequest struct {
	TokenInput         string
	HTTPSProxyOverride *string
}

type AccountBundleImportResult struct {
	Accounts      []AccountImportResult `json:"accounts"`
	DefaultUserID int64                 `json:"default_user_id"`
}

const (
	AccountImportStatusAdded   = "added"
	AccountImportStatusUpdated = "updated"
)

// AccountImportResult 是 direct import 与 bundle restore 共用的安全报告 DTO。
// 它刻意不携带 token、default 或 has_token，避免导入报告复用账号列表契约。
type AccountImportResult struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type AccountCheckRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// AccountRefreshRequest 指定要刷新凭据与账号状态的已保存账号；UID 为零时使用默认账号。
// OpenOperation 负责刷新 OAuth access token 及可能轮换的 refresh token，随后强制更新 profile。
type AccountRefreshRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

type AccountExportRequest struct {
	UserID int64
	All    bool
}

type AccountExportResult struct {
	RefreshToken string
	Bundle       []byte
	AccountCount int
}

type AccountResult struct {
	UserID                 int64      `json:"user_id,omitempty"`
	Username               string     `json:"username,omitempty"`
	Default                bool       `json:"default"`
	HasToken               bool       `json:"has_token"`
	PremiumStatus          *bool      `json:"premium_status,omitempty"`
	PremiumStatusCheckedAt *time.Time `json:"premium_status_checked_at,omitempty"`
	Warning                string     `json:"warning,omitempty"`
}

type AccountListResult struct {
	DefaultUserID int64
	Accounts      []AccountResult
}

func (s AccountService) Import(ctx context.Context, request AccountImportRequest) (AccountImportResult, error) {
	validatedToken, err := credentials.ValidateRefreshTokenInput(request.TokenInput)
	if err != nil {
		return AccountImportResult{}, err
	}
	if validatedToken == "" {
		return AccountImportResult{}, errors.New("refresh token cannot be empty")
	}
	client, err := s.SDK.Client(SDKClientRequest{HTTPSProxyOverride: request.HTTPSProxyOverride})
	if err != nil {
		return AccountImportResult{}, err
	}
	// 先读取 public SDK 的非 secret snapshot；OAuth 返回的 UID 仍是身份权威值，
	// snapshot 只用于区分本次写入是 added 还是 updated。
	accountsBeforeImport, err := client.ListAccounts()
	if err != nil {
		return AccountImportResult{}, err
	}
	account, err := client.ImportAccount(ctx, request.TokenInput)
	if err != nil {
		return AccountImportResult{}, err
	}
	status := AccountImportStatusAdded
	for _, existing := range accountsBeforeImport.Accounts {
		if existing.UserID == account.UserID {
			status = AccountImportStatusUpdated
			break
		}
	}
	return accountImportResult(*account, status), nil
}

// ImportBundle 解码并离线恢复 bundle；身份验证和 transport 均不参与此路径。
func (s AccountService) ImportBundle(body []byte) (AccountBundleImportResult, error) {
	bundle, err := sdk.DecodeAuthExportBundle(body)
	if err != nil {
		return AccountBundleImportResult{}, err
	}
	client, err := s.SDK.AuthBundleClient(SDKClientRequest{})
	if err != nil {
		return AccountBundleImportResult{}, err
	}
	restored, err := client.RestoreAuthBundle(bundle)
	if err != nil {
		return AccountBundleImportResult{}, err
	}
	result := AccountBundleImportResult{
		Accounts:      make([]AccountImportResult, 0, len(bundle.Accounts)),
		DefaultUserID: restored.DefaultUserID,
	}
	restoredByUserID := make(map[int64]AccountImportResult, len(restored.Added)+len(restored.Updated))
	for _, account := range restored.Added {
		restoredByUserID[account.UserID] = accountImportResult(account, AccountImportStatusAdded)
	}
	for _, account := range restored.Updated {
		restoredByUserID[account.UserID] = accountImportResult(account, AccountImportStatusUpdated)
	}
	// SDK 的 restore outcome 按状态分组；按照已验证 bundle 的账号顺序重新组装报告。
	for _, imported := range bundle.Accounts {
		account, ok := restoredByUserID[imported.UserID]
		if !ok {
			return AccountBundleImportResult{}, errors.New("auth restore result omitted an imported account")
		}
		result.Accounts = append(result.Accounts, account)
	}
	return result, nil
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

// Export 只通过 public SDK 读取本地认证快照；它不应用环境 token 或运行时 UID 覆写。
func (s AccountService) Export(request AccountExportRequest) (AccountExportResult, error) {
	client, err := s.SDK.AuthBundleClient(SDKClientRequest{})
	if err != nil {
		return AccountExportResult{}, err
	}
	bundle, err := client.ExportAuthBundle(sdk.AuthExportSelection{UserID: request.UserID, All: request.All})
	if err != nil {
		return AccountExportResult{}, err
	}
	body, err := sdk.EncodeAuthExportBundle(bundle)
	if err != nil {
		return AccountExportResult{}, err
	}
	result := AccountExportResult{Bundle: body, AccountCount: len(bundle.Accounts)}
	if !request.All && len(bundle.Accounts) == 1 {
		result.RefreshToken = bundle.Accounts[0].RefreshToken
	}
	return result, nil
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
		if token, err := s.refreshTokenFromEnv(); err != nil {
			return AccountResult{}, err
		} else if token != "" {
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

// RefreshWithRequest 先建立一次稳定的已认证 SDK 操作快照，以刷新 access token 和可能
// 轮换的 refresh token；再强制读取个人 profile，将 Premium 资格与检查时间写回 auth store。
func (s AccountService) RefreshWithRequest(ctx context.Context, request AccountRefreshRequest) (AccountResult, error) {
	client, err := s.SDK.OpenOperation(ctx, SDKClientRequest{UserID: request.UserID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	if err != nil {
		return AccountResult{}, err
	}
	userID, err := client.CurrentUserID(ctx)
	if err != nil {
		return AccountResult{}, err
	}
	refresher, ok := client.(interface {
		RefreshPremiumStatus(context.Context) (*sdk.PremiumStatus, error)
	})
	if !ok {
		return AccountResult{}, errors.New("pixiv sdk premium status refresher is not configured")
	}
	status, err := refresher.RefreshPremiumStatus(ctx)
	if err != nil {
		return AccountResult{}, err
	}
	list, err := s.List()
	if err != nil {
		return AccountResult{}, err
	}
	for _, account := range list.Accounts {
		if account.UserID != userID {
			continue
		}
		account.PremiumStatus = boolPointer(status.IsPremium)
		checkedAt := status.CheckedAt
		account.PremiumStatusCheckedAt = &checkedAt
		return account, nil
	}
	return AccountResult{}, fmt.Errorf("refreshed account uid %d was not found in local account store", userID)
}

func sdkAccountResult(account sdk.Account) AccountResult {
	var premium *bool
	var premiumCheckedAt *time.Time
	if account.PremiumStatus != nil {
		value := *account.PremiumStatus
		premium = &value
	}
	if account.PremiumStatusCheckedAt != nil {
		value := *account.PremiumStatusCheckedAt
		premiumCheckedAt = &value
	}
	return AccountResult{
		UserID: account.UserID, Username: account.Username, Default: account.Default, HasToken: account.HasToken,
		PremiumStatus: premium, PremiumStatusCheckedAt: premiumCheckedAt,
	}
}

func boolPointer(value bool) *bool { return &value }

func accountImportResult(account sdk.Account, status string) AccountImportResult {
	return AccountImportResult{UserID: account.UserID, Username: account.Username, Status: status}
}

func (s AccountService) refreshTokenFromEnv() (string, error) {
	if s.RefreshTokenFromEnv == nil {
		return "", nil
	}
	token, err := s.RefreshTokenFromEnv()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
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
