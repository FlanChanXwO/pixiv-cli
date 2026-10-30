package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// accountOut 定义所有 auth 账号结果的 JSON 形状；省略字段视为未知，不当作 false。
type accountOut struct {
	UserID          int64      `json:"user_id,omitempty"`
	Username        string     `json:"username,omitempty"`
	Default         bool       `json:"default"`
	PremiumStatus   *bool      `json:"premium_status,omitempty"`
	Schedulable     *bool      `json:"schedulable,omitempty"`
	PoolFrozenUntil *time.Time `json:"pool_frozen_until,omitempty"`
}

type accountListOut struct {
	Accounts []accountOut `json:"accounts"`
}

// accountImportOut 只包含 user_id、username 和 status；导入结果不会回显 token，
// 也不带默认账号标记。
type accountImportOut struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type accountBundleImportOut struct {
	Accounts      []accountImportOut `json:"accounts"`
	defaultUserID int64
}

type accountPoolStatusOut struct {
	Enabled             bool         `json:"enabled"`
	Strategy            string       `json:"strategy"`
	EarliestFrozenUntil *time.Time   `json:"earliest_frozen_until,omitempty"`
	Accounts            []accountOut `json:"accounts"`
}

// AccountService 把账号领域服务接到 auth 命令上。
// 账号领域服务不读取 CLI 配置；代理和运行时开关只在这里转换为 SDK options。
type AccountService struct {
	Pixiv       accountPort
	LoadRuntime func() (settings.RuntimeConfig, error)
}

// accountPort 定义 auth adapter 消费的账号业务端口，领域服务保持独立于 CLI DTO。
type accountPort interface {
	ImportAccount(context.Context, string, bool, pixiv.Options) (account.AccountSummary, error)
	ListAccounts(context.Context) ([]account.AccountSummary, error)
	UseAccount(context.Context, int64) error
	RemoveAccount(context.Context, int64) error
	CheckAccount(context.Context, int64, pixiv.Options) (account.AccountSummary, error)
	RefreshAccount(context.Context, int64, pixiv.Options) (account.AccountSummary, error)
	CurrentUser(context.Context) (*account.AccountSummary, error)
	RestoreAccounts(context.Context, []account.RestoreAccountInput) (account.RestoreAccountsResult, error)
	AccountsWithTokens(context.Context) ([]account.AccountWithToken, error)
	SetPoolSchedulable(context.Context, []int64, bool) error
	SetAllPoolSchedulable(context.Context, bool) error
}

type accountImportRequest struct {
	TokenInput         string
	HTTPSProxyOverride *string
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

func (s AccountService) sdkOptions(ctx context.Context, proxyOverride *string) (pixiv.Options, error) {
	proxy := proxyOverride
	if proxy == nil {
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
	client, err := httpclient.HTTPClient(*proxy)
	if err != nil {
		return pixiv.Options{}, err
	}
	return pixiv.Options{HTTPClient: client, Logger: logging.From(ctx)}, nil
}

func pixivOptionsForProxy(proxyOverride *string) (pixiv.LoginOptions, error) {
	if proxyOverride == nil {
		return pixiv.LoginOptions{}, nil
	}
	client, err := httpclient.HTTPClient(*proxyOverride)
	if err != nil {
		return pixiv.LoginOptions{}, err
	}
	return pixiv.LoginOptions{HTTPClient: client}, nil
}

func (s AccountService) importAccount(ctx context.Context, request accountImportRequest) (accountImportOut, error) {
	service := s.Pixiv
	validatedToken := strings.TrimSpace(request.TokenInput)
	if validatedToken == "" {
		return accountImportOut{}, errors.New("refresh token cannot be empty")
	}
	before, err := service.ListAccounts(ctx)
	if err != nil {
		return accountImportOut{}, err
	}
	options, err := s.sdkOptions(ctx, request.HTTPSProxyOverride)
	if err != nil {
		return accountImportOut{}, err
	}
	account, err := service.ImportAccount(ctx, validatedToken, false, options)
	if err != nil {
		return accountImportOut{}, err
	}
	status := accountImportStatusAdded
	for _, existing := range before {
		if existing.UserID == account.UserID {
			status = accountImportStatusUpdated
			break
		}
	}
	return accountImportOutFrom(account, status), nil
}

func (s AccountService) importBundle(ctx context.Context, body []byte) (accountBundleImportOut, error) {
	service := s.Pixiv
	bundle, err := decodeAuthExportBundle(body)
	if err != nil {
		return accountBundleImportOut{}, err
	}
	inputs := make([]account.RestoreAccountInput, 0, len(bundle.Accounts))
	for _, entry := range bundle.Accounts {
		summary := account.AccountSummary{UserID: entry.UserID, Username: entry.Username}
		inputs = append(inputs, account.RestoreAccountInput{
			Account:         summary,
			RefreshToken:    entry.RefreshToken,
			IsBundleDefault: entry.UserID == bundle.DefaultUserID,
		})
	}
	restore, err := service.RestoreAccounts(ctx, inputs)
	if err != nil {
		return accountBundleImportOut{}, err
	}
	result := accountBundleImportOut{
		Accounts:      make([]accountImportOut, 0, len(restore.Accounts)),
		defaultUserID: restore.ResultingDefault,
	}
	for _, outcome := range restore.Accounts {
		status := accountImportStatusAdded
		if outcome.IsReplacement {
			status = accountImportStatusUpdated
		}
		result.Accounts = append(result.Accounts, accountImportOutFrom(outcome.Account, status))
	}
	return result, nil
}

func (s AccountService) listWithContext(ctx context.Context) (accountListOut, error) {
	service := s.Pixiv
	accounts, err := service.ListAccounts(ctx)
	if err != nil {
		return accountListOut{}, err
	}
	result := accountListOut{Accounts: make([]accountOut, 0, len(accounts))}
	for _, account := range accounts {
		result.Accounts = append(result.Accounts, accountFromPixiv(account))
	}
	return result, nil
}

func (s AccountService) poolStatus(ctx context.Context) (accountPoolStatusOut, error) {
	service := s.Pixiv
	runtime, err := s.LoadRuntime()
	if err != nil {
		return accountPoolStatusOut{}, err
	}
	accounts, err := service.ListAccounts(ctx)
	if err != nil {
		return accountPoolStatusOut{}, err
	}
	result := accountPoolStatusOut{Enabled: runtime.AccountPool.Enabled, Strategy: string(runtime.AccountPool.Strategy), Accounts: make([]accountOut, 0, len(accounts))}
	for _, account := range accounts {
		converted := accountFromPixiv(account)
		result.Accounts = append(result.Accounts, converted)
		if converted.PoolFrozenUntil != nil && (result.EarliestFrozenUntil == nil || converted.PoolFrozenUntil.Before(*result.EarliestFrozenUntil)) {
			value := *converted.PoolFrozenUntil
			result.EarliestFrozenUntil = &value
		}
	}
	return result, nil
}

func (s AccountService) setPool(ctx context.Context, userIDs []int64, schedulable, all bool) error {
	service := s.Pixiv
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

func (s AccountService) export(request accountExportRequest) (accountExportResult, error) {
	service := s.Pixiv
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
		for _, entry := range accounts {
			if entry.UserID == request.UserID {
				selected = []account.AccountWithToken{entry}
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

func (s AccountService) remove(userID int64) (accountOut, int64, error) {
	service := s.Pixiv
	list, err := s.listWithContext(context.Background())
	if err != nil {
		return accountOut{}, 0, err
	}
	var removed accountOut
	found := false
	for _, account := range list.Accounts {
		if account.UserID == userID {
			removed, found = account, true
			break
		}
	}
	if !found {
		return accountOut{}, 0, fmt.Errorf("account uid %d not found", userID)
	}
	if err := service.RemoveAccount(context.Background(), userID); err != nil {
		return accountOut{}, 0, err
	}
	updated, err := s.listWithContext(context.Background())
	if err != nil {
		return accountOut{}, 0, err
	}
	removed.Default = false
	return removed, updated.defaultUserID(), nil
}

func (s AccountService) use(userID int64) (int64, error) {
	service := s.Pixiv
	if err := service.UseAccount(context.Background(), userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s AccountService) checkWithRequest(ctx context.Context, request accountCheckRequest) (accountOut, error) {
	service := s.Pixiv
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return accountOut{}, err
		}
		request.UserID = current.UserID
	}
	options, err := s.sdkOptions(ctx, request.HTTPSProxyOverride)
	if err != nil {
		return accountOut{}, err
	}
	account, err := service.CheckAccount(ctx, request.UserID, options)
	if err != nil {
		return accountOut{}, err
	}
	return accountFromPixiv(account), nil
}

func (s AccountService) refreshWithRequest(ctx context.Context, request accountRefreshRequest) (accountOut, error) {
	service := s.Pixiv
	if request.UserID == 0 {
		current, err := service.CurrentUser(ctx)
		if err != nil {
			return accountOut{}, err
		}
		request.UserID = current.UserID
	}
	options, err := s.sdkOptions(ctx, request.HTTPSProxyOverride)
	if err != nil {
		return accountOut{}, err
	}
	account, err := service.RefreshAccount(ctx, request.UserID, options)
	if err != nil {
		return accountOut{}, err
	}
	return accountFromPixiv(account), nil
}

// AccountJSON 返回与 `pixiv auth list --json` 单账号项相同的公开摘要；不含凭据。
func AccountJSON(account account.AccountSummary) any { return accountFromPixiv(account) }

func accountFromPixiv(account account.AccountSummary) accountOut {
	result := accountOut{UserID: account.UserID, Username: account.Username, Default: account.Default, PremiumStatus: account.Premium}
	if account.PoolStatusKnown {
		schedulable := account.Schedulable
		result.Schedulable = &schedulable
		if account.PoolFrozenUntil != nil {
			frozen := time.Unix(*account.PoolFrozenUntil, 0).UTC()
			result.PoolFrozenUntil = &frozen
		}
	}
	return result
}

func accountImportOutFrom(account account.AccountSummary, status string) accountImportOut {
	return accountImportOut{UserID: account.UserID, Username: account.Username, Status: status}
}

func parseAuthUID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("uid cannot be empty")
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("invalid uid: expected a positive integer")
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
	if err := rejectDuplicateObjectKeys(body); err != nil {
		return nil, fmt.Errorf("invalid auth export bundle: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	var bundle authExportBundle
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("invalid auth export bundle: %w", err)
	}
	if dec := json.NewDecoder(bytes.NewReader(body)); true {
		dec.UseNumber()
		_ = dec.Decode(new(json.RawMessage))
		if dec.More() {
			return nil, errors.New("auth export bundle has trailing JSON")
		}
	}
	if bundle.Schema != authExportBundleSchema || bundle.Version != authExportBundleVersion {
		return nil, errors.New("unsupported auth export bundle schema or version")
	}
	if len(bundle.Accounts) == 0 {
		return nil, errors.New("auth export bundle has no accounts")
	}
	seen := make(map[int64]struct{}, len(bundle.Accounts))
	for _, account := range bundle.Accounts {
		if account.UserID <= 0 || account.RefreshToken == "" {
			return nil, errors.New("auth export bundle contains an invalid account")
		}
		if _, duplicate := seen[account.UserID]; duplicate {
			return nil, fmt.Errorf("auth export bundle contains duplicate account %d", account.UserID)
		}
		seen[account.UserID] = struct{}{}
	}
	if bundle.DefaultUserID != 0 {
		if _, ok := seen[bundle.DefaultUserID]; !ok {
			return nil, errors.New("auth export bundle default does not name an included account")
		}
	}
	return &bundle, nil
}

// rejectDuplicateObjectKeys 流式扫描 JSON，拒绝同一 object 内重复出现的 key。
// strict versioned codec 要求 canonical JSON：重复 key 会被编码器视为歧义输入。
func rejectDuplicateObjectKeys(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	// frame.expectKey 为 true 表示下一个 string token 是 object key；否则是值。
	type frame struct {
		seen      map[string]struct{}
		expectKey bool
	}
	var stack []frame
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(stack) == 0 {
			if delim, isDelim := token.(json.Delim); isDelim && delim == '{' {
				stack = append(stack, frame{seen: make(map[string]struct{}), expectKey: true})
				continue
			}
			continue
		}
		top := &stack[len(stack)-1]
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, frame{seen: make(map[string]struct{}), expectKey: true})
			case '[':
				stack = append(stack, frame{seen: nil, expectKey: false})
			case '}', ']':
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					parent := &stack[len(stack)-1]
					parent.expectKey = true
				}
			}
		case string:
			if top.expectKey {
				if _, duplicate := top.seen[value]; duplicate {
					return fmt.Errorf("auth export bundle JSON has duplicate object key %q", value)
				}
				top.seen[value] = struct{}{}
				top.expectKey = false
			} else {
				top.expectKey = true
			}
		default:
			top.expectKey = true
		}
	}
	return nil
}

func (list accountListOut) defaultUserID() int64 {
	for _, account := range list.Accounts {
		if account.Default {
			return account.UserID
		}
	}
	return 0
}
