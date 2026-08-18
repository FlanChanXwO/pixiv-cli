package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/network"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// AccountService 是 auth command 对本地账号领域服务的薄适配器。
// 账号领域服务不读取 CLI 配置；代理和运行时开关只在这里转换为 SDK options。
type AccountService struct {
	Pixiv       AccountPort
	LoadRuntime func() (config.RuntimeConfig, error)
}

// AccountPort 是 auth adapter 需要的最小业务端口。它保持在 CLI adapter 中，
// 使 auth 测试可以注入无网络替身，同时不把 CLI DTO 反向放进 services。
type AccountPort interface {
	ImportAccountWith(context.Context, string, bool, pixiv.Options) (pixivaccount.AccountSummary, error)
	ListAccounts(context.Context) ([]pixivaccount.AccountSummary, error)
	UseAccount(context.Context, int64) error
	RemoveAccount(context.Context, int64) error
	CheckAccountWith(context.Context, int64, pixiv.Options) (pixivaccount.AccountSummary, error)
	RefreshAccountWith(context.Context, int64, pixiv.Options) (pixivaccount.AccountSummary, error)
	CurrentUser(context.Context) (*pixivaccount.AccountSummary, error)
	RestoreAccount(context.Context, pixivaccount.AccountSummary, string, bool) error
	AccountsWithTokens(context.Context) ([]pixivaccount.AccountWithToken, error)
	SetPoolSchedulable(context.Context, []int64, bool) error
	SetAllPoolSchedulable(context.Context, bool) error
}

type accountImportRequest struct {
	TokenInput         string
	HTTPSProxyOverride *string
}

type accountImportResult struct {
	UserID   int64
	Username string
	Status   string
}

type accountBundleImportResult struct {
	Accounts      []accountImportResult
	DefaultUserID int64
}

type accountCheckRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

type accountRefreshRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

type accountExportRequest struct {
	UserID int64
	All    bool
}

type accountExportResult struct {
	RefreshToken string
	Bundle       []byte
	AccountCount int
}

type accountPoolStatusResult struct {
	Enabled             bool
	Strategy            config.AccountPoolStrategy
	EarliestFrozenUntil *time.Time
	Accounts            []accountResult
}

type accountResult struct {
	UserID                 int64
	Username               string
	Default                bool
	HasToken               bool
	PremiumStatus          *bool
	PremiumStatusCheckedAt *time.Time
	Schedulable            *bool
	Eligible               *bool
	PoolFrozenUntil        *time.Time
	Warning                string
}

type accountListResult struct {
	DefaultUserID int64
	Accounts      []accountResult
}

type authExportBundle struct {
	Schema        string                    `json:"schema"`
	Version       int                       `json:"version"`
	DefaultUserID int64                     `json:"default_user_id,omitempty"`
	Accounts      []authExportSecretAccount `json:"accounts"`
}

type authExportSecretAccount struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	RefreshToken string `json:"refresh_token"`
}

const (
	accountImportStatusAdded   = "added"
	accountImportStatusUpdated = "updated"
	authExportBundleSchema     = "pixiv-cli.auth-export"
	authExportBundleVersion    = 1
)

func (s AccountService) pixivService() (AccountPort, error) {
	if s.Pixiv == nil {
		return nil, errors.New("pixiv account service is not configured")
	}
	return s.Pixiv, nil
}

func (s AccountService) sdkOptions(proxyOverride *string) (pixiv.Options, error) {
	proxy := proxyOverride
	if proxy == nil {
		if s.LoadRuntime == nil {
			return pixiv.Options{}, errors.New("pixiv auth runtime loader is not configured")
		}
		runtime, err := s.LoadRuntime()
		if err != nil {
			return pixiv.Options{}, err
		}
		if runtime.PixivNetwork.ProxyURL.Present {
			value := runtime.PixivNetwork.ProxyURL.Value
			proxy = &value
		} else {
			value := runtime.HTTPSProxy
			proxy = &value
		}
	}
	if proxy == nil {
		return pixiv.Options{}, nil
	}
	client, err := network.HTTPClient(*proxy)
	if err != nil {
		return pixiv.Options{}, err
	}
	return pixiv.Options{HTTPClient: client}, nil
}

func pixivOptionsForProxy(proxyOverride *string) (pixiv.LoginOptions, error) {
	if proxyOverride == nil {
		return pixiv.LoginOptions{}, nil
	}
	client, err := network.HTTPClient(*proxyOverride)
	if err != nil {
		return pixiv.LoginOptions{}, err
	}
	return pixiv.LoginOptions{HTTPClient: client}, nil
}

func (s AccountService) Import(ctx context.Context, request accountImportRequest) (accountImportResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountImportResult{}, err
	}
	validatedToken := strings.TrimSpace(request.TokenInput)
	if validatedToken == "" {
		return accountImportResult{}, errors.New("refresh token cannot be empty")
	}
	before, err := service.ListAccounts(ctx)
	if err != nil {
		return accountImportResult{}, err
	}
	options, err := s.sdkOptions(request.HTTPSProxyOverride)
	if err != nil {
		return accountImportResult{}, err
	}
	account, err := service.ImportAccountWith(ctx, validatedToken, false, options)
	if err != nil {
		return accountImportResult{}, err
	}
	status := accountImportStatusAdded
	for _, existing := range before {
		if existing.UserID == account.UserID {
			status = accountImportStatusUpdated
			break
		}
	}
	return accountImportResultFrom(account, status), nil
}

func (s AccountService) ImportBundle(body []byte) (accountBundleImportResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountBundleImportResult{}, err
	}
	bundle, err := decodeAuthExportBundle(body)
	if err != nil {
		return accountBundleImportResult{}, err
	}
	result := accountBundleImportResult{
		Accounts:      make([]accountImportResult, 0, len(bundle.Accounts)),
		DefaultUserID: bundle.DefaultUserID,
	}
	for _, account := range bundle.Accounts {
		summary := pixivaccount.AccountSummary{UserID: account.UserID, Username: account.Username}
		if err := service.RestoreAccount(context.Background(), summary, account.RefreshToken, account.UserID == bundle.DefaultUserID); err != nil {
			return accountBundleImportResult{}, err
		}
		result.Accounts = append(result.Accounts, accountImportResultFrom(summary, accountImportStatusAdded))
	}
	if result.DefaultUserID == 0 && len(result.Accounts) > 0 {
		result.DefaultUserID = result.Accounts[0].UserID
	}
	return result, nil
}

func (s AccountService) List() (accountListResult, error) {
	return s.ListWithContext(context.Background())
}

func (s AccountService) ListWithContext(ctx context.Context) (accountListResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountListResult{}, err
	}
	accounts, err := service.ListAccounts(ctx)
	if err != nil {
		return accountListResult{}, err
	}
	result := accountListResult{Accounts: make([]accountResult, 0, len(accounts))}
	for _, account := range accounts {
		result.Accounts = append(result.Accounts, accountResultFromPixiv(account))
		if account.Default {
			result.DefaultUserID = account.UserID
		}
	}
	return result, nil
}

func (s AccountService) PoolStatus(ctx context.Context) (accountPoolStatusResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountPoolStatusResult{}, err
	}
	if s.LoadRuntime == nil {
		return accountPoolStatusResult{}, errors.New("account pool runtime loader is not configured")
	}
	runtime, err := s.LoadRuntime()
	if err != nil {
		return accountPoolStatusResult{}, err
	}
	accounts, err := service.ListAccounts(ctx)
	if err != nil {
		return accountPoolStatusResult{}, err
	}
	result := accountPoolStatusResult{Enabled: runtime.AccountPool.Enabled, Strategy: runtime.AccountPool.Strategy, Accounts: make([]accountResult, 0, len(accounts))}
	for _, account := range accounts {
		converted := accountResultFromPixiv(account)
		result.Accounts = append(result.Accounts, converted)
		if converted.PoolFrozenUntil != nil && (result.EarliestFrozenUntil == nil || converted.PoolFrozenUntil.Before(*result.EarliestFrozenUntil)) {
			value := *converted.PoolFrozenUntil
			result.EarliestFrozenUntil = &value
		}
	}
	return result, nil
}

func (s AccountService) SetPool(ctx context.Context, userIDs []int64, schedulable, all bool) error {
	service, err := s.pixivService()
	if err != nil {
		return err
	}
	if all {
		if len(userIDs) != 0 {
			return errors.New("--all cannot be combined with UIDs")
		}
		return service.SetAllPoolSchedulable(ctx, schedulable)
	}
	if len(userIDs) == 0 {
		return errors.New("at least one UID is required unless --all is used")
	}
	return service.SetPoolSchedulable(ctx, userIDs, schedulable)
}

func (s AccountService) Export(request accountExportRequest) (accountExportResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountExportResult{}, err
	}
	accounts, err := service.AccountsWithTokens(context.Background())
	if err != nil {
		return accountExportResult{}, err
	}
	if !request.All && request.UserID == 0 {
		current, err := service.CurrentUser(context.Background())
		if err != nil {
			return accountExportResult{}, err
		}
		request.UserID = current.UserID
	}
	selected := accounts
	if !request.All {
		selected = nil
		for _, account := range accounts {
			if account.UserID == request.UserID {
				selected = []pixivaccount.AccountWithToken{account}
				break
			}
		}
		if selected == nil {
			return accountExportResult{}, fmt.Errorf("account uid %d not found", request.UserID)
		}
	}
	bundle := authExportBundle{Schema: authExportBundleSchema, Version: authExportBundleVersion, Accounts: make([]authExportSecretAccount, 0, len(selected))}
	for _, account := range selected {
		if account.Default {
			bundle.DefaultUserID = account.UserID
		}
		bundle.Accounts = append(bundle.Accounts, authExportSecretAccount{UserID: account.UserID, Username: account.Username, RefreshToken: account.RefreshToken()})
	}
	body, err := encodeAuthExportBundle(&bundle)
	if err != nil {
		return accountExportResult{}, err
	}
	result := accountExportResult{Bundle: body, AccountCount: len(bundle.Accounts)}
	if !request.All && len(bundle.Accounts) == 1 {
		result.RefreshToken = bundle.Accounts[0].RefreshToken
	}
	return result, nil
}

func (s AccountService) Remove(userID int64) (accountResult, int64, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountResult{}, 0, err
	}
	list, err := s.List()
	if err != nil {
		return accountResult{}, 0, err
	}
	var removed accountResult
	found := false
	for _, account := range list.Accounts {
		if account.UserID == userID {
			removed, found = account, true
			break
		}
	}
	if !found {
		return accountResult{}, 0, fmt.Errorf("account uid %d not found", userID)
	}
	if err := service.RemoveAccount(context.Background(), userID); err != nil {
		return accountResult{}, 0, err
	}
	updated, err := s.List()
	if err != nil {
		return accountResult{}, 0, err
	}
	removed.Default = false
	return removed, updated.DefaultUserID, nil
}

func (s AccountService) Use(userID int64) (int64, error) {
	service, err := s.pixivService()
	if err != nil {
		return 0, err
	}
	if err := service.UseAccount(context.Background(), userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s AccountService) CheckWithRequest(ctx context.Context, request accountCheckRequest) (accountResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountResult{}, err
	}
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return accountResult{}, err
		}
		request.UserID = current.UserID
	}
	options, err := s.sdkOptions(request.HTTPSProxyOverride)
	if err != nil {
		return accountResult{}, err
	}
	account, err := service.CheckAccountWith(ctx, request.UserID, options)
	if err != nil {
		return accountResult{}, err
	}
	return accountResultFromPixiv(account), nil
}

func (s AccountService) RefreshWithRequest(ctx context.Context, request accountRefreshRequest) (accountResult, error) {
	service, err := s.pixivService()
	if err != nil {
		return accountResult{}, err
	}
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return accountResult{}, err
		}
		request.UserID = current.UserID
	}
	options, err := s.sdkOptions(request.HTTPSProxyOverride)
	if err != nil {
		return accountResult{}, err
	}
	account, err := service.RefreshAccountWith(ctx, request.UserID, options)
	if err != nil {
		return accountResult{}, err
	}
	return accountResultFromPixiv(account), nil
}

func accountResultFromPixiv(account pixivaccount.AccountSummary) accountResult {
	result := accountResult{UserID: account.UserID, Username: account.Username, Default: account.Default, HasToken: true, PremiumStatus: account.Premium}
	if account.PoolStatusKnown {
		schedulable, eligible := account.Schedulable, account.Eligible
		result.Schedulable, result.Eligible = &schedulable, &eligible
		if account.PoolFrozenUntil != nil {
			frozen := time.Unix(*account.PoolFrozenUntil, 0).UTC()
			result.PoolFrozenUntil = &frozen
		}
	}
	return result
}

func accountImportResultFrom(account pixivaccount.AccountSummary, status string) accountImportResult {
	return accountImportResult{UserID: account.UserID, Username: account.Username, Status: status}
}

func parseAuthUID(raw string) (int64, error) {
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

func encodeAuthExportBundle(bundle *authExportBundle) ([]byte, error) {
	if bundle == nil || len(bundle.Accounts) == 0 {
		return nil, errors.New("auth export bundle has no accounts")
	}
	return json.Marshal(bundle)
}

func decodeAuthExportBundle(body []byte) (*authExportBundle, error) {
	if !json.Valid(body) {
		return nil, errors.New("invalid auth export bundle JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var bundle authExportBundle
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
