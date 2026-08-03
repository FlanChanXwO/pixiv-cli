package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
)

// AccountStore 是 AccountService 依赖的 authdb-backed 账号能力，由
// pixivapp.Service 实现；接口让测试可以注入内存替身而不触发 OAuth 网络调用。
type AccountStore interface {
	ImportAccount(context.Context, string, bool) (pixivapp.Account, error)
	ListAccounts(context.Context) ([]pixivapp.Account, error)
	UseAccount(context.Context, int64) error
	RemoveAccount(context.Context, int64) error
	CheckAccount(context.Context, int64) (pixivapp.Account, error)
	CheckRefreshToken(context.Context, string) (pixivapp.Account, error)
	ExportRefreshToken(context.Context, int64) (string, error)
	RefreshAccount(context.Context, int64) (pixivapp.Account, error)
	CurrentUser(context.Context) (*pixivapp.Account, error)
	RestoreAccount(context.Context, pixivapp.Account, string, bool) error
	AccountsWithTokens(context.Context) ([]pixivapp.AccountWithToken, error)
}

// AccountService 只适配 authdb-backed 的本地账号 API。账号文件、token rotation 和
// OAuth 身份验证均由 pixivapp.Service 维护，应用层不再复制另一份 auth store 调用链。
type AccountService struct {
	Pixiv               AccountStore
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

// AuthExportBundle 是版本化的本地账号导出 bundle，只用于显式 auth export/import。
// 它保留既有 schema/version 常量，使旧 bundle 文件仍可被当前版本解码。
type AuthExportBundle struct {
	Schema        string                    `json:"schema"`
	Version       int                       `json:"version"`
	DefaultUserID int64                     `json:"default_user_id,omitempty"`
	Accounts      []AuthExportSecretAccount `json:"accounts"`
}

type AuthExportSecretAccount struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	RefreshToken string `json:"refresh_token"`
}

const (
	authExportBundleSchema  = "pixiv-cli.auth-export"
	authExportBundleVersion = 1
)

func (s AccountService) pixiv() (AccountStore, error) {
	if s.Pixiv == nil {
		return nil, errors.New("pixiv account service is not configured")
	}
	return s.Pixiv, nil
}

func (s AccountService) Import(ctx context.Context, request AccountImportRequest) (AccountImportResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountImportResult{}, err
	}
	validatedToken, err := credentials.ValidateRefreshTokenInput(request.TokenInput)
	if err != nil {
		return AccountImportResult{}, err
	}
	if validatedToken == "" {
		return AccountImportResult{}, errors.New("refresh token cannot be empty")
	}
	// 先读取本地非 secret 账号列表；OAuth 返回的 UID 仍是身份权威值，
	// 列表只用于区分本次写入是 added 还是 updated。
	accountsBeforeImport, err := service.ListAccounts(ctx)
	if err != nil {
		return AccountImportResult{}, err
	}
	account, err := service.ImportAccount(ctx, validatedToken, false)
	if err != nil {
		return AccountImportResult{}, err
	}
	status := AccountImportStatusAdded
	for _, existing := range accountsBeforeImport {
		if existing.UserID == account.UserID {
			status = AccountImportStatusUpdated
			break
		}
	}
	return accountImportResult(account, status), nil
}

// ImportBundle 解码并离线恢复 bundle；身份验证和 transport 均不参与此路径。
func (s AccountService) ImportBundle(body []byte) (AccountBundleImportResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountBundleImportResult{}, err
	}
	bundle, err := decodeAuthExportBundle(body)
	if err != nil {
		return AccountBundleImportResult{}, err
	}
	ctx := context.Background()
	result := AccountBundleImportResult{
		Accounts:      make([]AccountImportResult, 0, len(bundle.Accounts)),
		DefaultUserID: bundle.DefaultUserID,
	}
	for _, account := range bundle.Accounts {
		restoredAccount := pixivapp.Account{UserID: account.UserID, Username: account.Username}
		if err := service.RestoreAccount(ctx, restoredAccount, account.RefreshToken, account.UserID == bundle.DefaultUserID); err != nil {
			return AccountBundleImportResult{}, err
		}
		result.Accounts = append(result.Accounts, accountImportResult(restoredAccount, AccountImportStatusAdded))
	}
	if result.DefaultUserID == 0 && len(result.Accounts) > 0 {
		// 没有显式默认的 bundle 恢复后，首个入库账号成为默认账号。
		result.DefaultUserID = result.Accounts[0].UserID
	}
	return result, nil
}

func (s AccountService) List() (AccountListResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountListResult{}, err
	}
	accounts, err := service.ListAccounts(context.Background())
	if err != nil {
		return AccountListResult{}, err
	}
	out := AccountListResult{Accounts: make([]AccountResult, 0, len(accounts))}
	for _, account := range accounts {
		out.Accounts = append(out.Accounts, accountResultFromPixiv(account))
		if account.Default {
			out.DefaultUserID = account.UserID
		}
	}
	return out, nil
}

// Export 只读取本地认证快照；它不应用环境 token 或运行时 UID 覆写。
func (s AccountService) Export(request AccountExportRequest) (AccountExportResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountExportResult{}, err
	}
	ctx := context.Background()
	accounts, err := service.AccountsWithTokens(ctx)
	if err != nil {
		return AccountExportResult{}, err
	}
	if !request.All && request.UserID == 0 {
		// 默认账号的 raw token。
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return AccountExportResult{}, err
		}
		request.UserID = current.UserID
	}
	selected := accounts
	if !request.All {
		selected = nil
		for _, account := range accounts {
			if account.UserID == request.UserID {
				selected = []pixivapp.AccountWithToken{account}
				break
			}
		}
		if selected == nil {
			return AccountExportResult{}, fmt.Errorf("account uid %d not found", request.UserID)
		}
	}
	bundle := AuthExportBundle{Schema: authExportBundleSchema, Version: authExportBundleVersion, Accounts: make([]AuthExportSecretAccount, 0, len(selected))}
	for _, account := range selected {
		if account.Default {
			bundle.DefaultUserID = account.UserID
		}
		bundle.Accounts = append(bundle.Accounts, AuthExportSecretAccount{UserID: account.UserID, Username: account.Username, RefreshToken: account.RefreshToken})
	}
	body, err := encodeAuthExportBundle(&bundle)
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
	service, err := s.pixiv()
	if err != nil {
		return AccountResult{}, 0, err
	}
	ctx := context.Background()
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
	if err := service.RemoveAccount(ctx, userID); err != nil {
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
	service, err := s.pixiv()
	if err != nil {
		return 0, err
	}
	if err := service.UseAccount(context.Background(), userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s AccountService) Check(ctx context.Context, userID int64) (AccountResult, error) {
	return s.CheckWithRequest(ctx, AccountCheckRequest{UserID: userID})
}

func (s AccountService) CheckWithRequest(ctx context.Context, request AccountCheckRequest) (AccountResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountResult{}, err
	}
	if request.UserID == 0 {
		if token, err := s.refreshTokenFromEnv(); err != nil {
			return AccountResult{}, err
		} else if token != "" {
			account, err := service.CheckRefreshToken(ctx, token)
			if err != nil {
				return AccountResult{}, err
			}
			return accountResultFromPixiv(account), nil
		}
	}
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return AccountResult{}, err
		}
		request.UserID = current.UserID
	}
	account, err := service.CheckAccount(ctx, request.UserID)
	if err != nil {
		return AccountResult{}, err
	}
	return accountResultFromPixiv(account), nil
}

// RefreshWithRequest 先建立一次稳定的已认证操作快照，刷新 access token 和可能
// 轮换的 refresh token；再强制读取个人 profile，将 Premium 资格与检查时间写回 authdb。
func (s AccountService) RefreshWithRequest(ctx context.Context, request AccountRefreshRequest) (AccountResult, error) {
	service, err := s.pixiv()
	if err != nil {
		return AccountResult{}, err
	}
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return AccountResult{}, err
		}
		request.UserID = current.UserID
	}
	account, err := service.RefreshAccount(ctx, request.UserID)
	if err != nil {
		return AccountResult{}, err
	}
	return accountResultFromPixiv(account), nil
}

func accountResultFromPixiv(account pixivapp.Account) AccountResult {
	return AccountResult{
		UserID:        account.UserID,
		Username:      account.Username,
		Default:       account.Default,
		HasToken:      true,
		PremiumStatus: account.Premium,
	}
}

func accountImportResult(account pixivapp.Account, status string) AccountImportResult {
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

func encodeAuthExportBundle(bundle *AuthExportBundle) ([]byte, error) {
	if bundle == nil || len(bundle.Accounts) == 0 {
		return nil, errors.New("auth export bundle has no accounts")
	}
	return json.Marshal(bundle)
}

func decodeAuthExportBundle(body []byte) (*AuthExportBundle, error) {
	if !json.Valid(body) {
		return nil, errors.New("invalid auth export bundle JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var bundle AuthExportBundle
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("invalid auth export bundle: %w", err)
	}
	if bundle.Schema != authExportBundleSchema || bundle.Version != authExportBundleVersion {
		return nil, errors.New("unsupported auth export bundle schema or version")
	}
	if len(bundle.Accounts) == 0 {
		return nil, errors.New("auth export bundle has no accounts")
	}
	for _, account := range bundle.Accounts {
		if account.UserID <= 0 || account.RefreshToken == "" {
			return nil, errors.New("auth export bundle contains an invalid account")
		}
	}
	return &bundle, nil
}
