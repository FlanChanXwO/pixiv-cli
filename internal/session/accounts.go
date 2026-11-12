package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
)

// Accounts 将默认账号配置与 Pixiv 账号存储接到命令和会话，保持输出摘要契约。
type Accounts struct {
	store    *account.Store
	defaults config.Store
}

// AccountSummary 汇总 Pixiv 账号的公开摘要。
type AccountSummary struct {
	UserID           int64
	Username         string
	Default          bool
	Premium          *bool
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
	PoolStatusKnown  bool
}

// ImportAccount 导入一个 refresh token 并在线验证；连接选项由调用方组装，
// 服务不读取配置或推断网络策略。
func (s *Accounts) ImportAccount(ctx context.Context, refreshToken string, setDefault bool, options pixiv.Options) (AccountSummary, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return AccountSummary{}, errors.New("pixiv refresh token is required")
	}
	saved, err := s.store.ImportPixiv(ctx, refreshToken, options)
	if err != nil {
		return AccountSummary{}, err
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return AccountSummary{}, fmt.Errorf("read pixiv default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.defaults.SetPixivDefaultUserID(saved.UserID); err != nil {
			return AccountSummary{}, fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	return s.summary(ctx, saved.UserID)
}

// ListAccounts 返回按显示顺序排列的 Pixiv 账号。
func (s *Accounts) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.store.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	defaultID, explicit, err := s.defaults.ReadPixivDefaultUserID()
	if err != nil {
		return nil, err
	}
	if !explicit && len(accounts) > 0 {
		defaultID = accounts[0].UserID
	}
	out := make([]AccountSummary, 0, len(accounts))
	for _, saved := range accounts {
		out = append(out, accountSummary(saved, saved.UserID == defaultID, saved.PoolLastSelected))
	}
	return out, nil
}

// SetPoolSchedulable 原子更新一组已存在的本地账号；未知 UID 会使整批失败。
func (s *Accounts) SetPoolSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	return s.store.SetPixivSchedulable(ctx, userIDs, schedulable)
}

// SetAllPoolSchedulable 原子更新当前全部本地账号。
func (s *Accounts) SetAllPoolSchedulable(ctx context.Context, schedulable bool) error {
	return s.store.SetAllPixivSchedulable(ctx, schedulable)
}

// RemoveAccount 删除 Pixiv 账号；若它是显式默认账号，先清除配置以恢复自动选择。
func (s *Accounts) RemoveAccount(ctx context.Context, userID int64) error {
	defaultID, explicit, err := s.defaults.ReadPixivDefaultUserID()
	if err != nil {
		return fmt.Errorf("read pixiv default account: %w", err)
	}
	if explicit && defaultID == userID {
		if err := s.defaults.ClearPixivDefaultUserID(); err != nil {
			return fmt.Errorf("clear pixiv default account: %w", err)
		}
	}
	if err := s.store.RemovePixiv(ctx, userID); err != nil {
		if explicit && defaultID == userID {
			return errors.Join(err, s.defaults.SetPixivDefaultUserID(defaultID))
		}
		return err
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Accounts) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.store.GetPixiv(ctx, userID); err != nil {
		return err
	}
	return s.defaults.SetPixivDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Accounts) UseAuto() error {
	return s.defaults.ClearPixivDefaultUserID()
}

// CurrentUser 返回当前选中账号的身份摘要。
func (s *Accounts) CurrentUser(ctx context.Context) (*AccountSummary, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.store.GetPixiv(ctx, userID)
	if err != nil {
		return nil, err
	}
	summary := accountSummary(account, true, account.PoolLastSelected)
	return &summary, nil
}

// CheckAccount 在线校验指定账号的 refresh token，并持久化 OAuth 返回的 rotation。
func (s *Accounts) CheckAccount(ctx context.Context, userID int64, options pixiv.Options) (AccountSummary, error) {
	client, err := s.store.OpenAccountClient(ctx, userID, options)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	return AccountSummary{UserID: client.UserID(), Username: client.Username()}, nil
}

// RefreshAccount 刷新指定账号的凭据并强制读取当前 profile，把会员资格快照写回
// 账号文件，最后返回账号摘要。
func (s *Accounts) RefreshAccount(ctx context.Context, userID int64, options pixiv.Options) (AccountSummary, error) {
	client, err := s.store.OpenAccountClient(ctx, userID, options)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	detail, err := client.CurrentUser(ctx, pixiv.CurrentUserRequest{})
	if err != nil {
		return AccountSummary{}, err
	}
	premium := detail.Profile.IsPremium
	if err := s.setPremiumStatus(ctx, userID, &premium); err != nil {
		return AccountSummary{}, err
	}
	return s.summary(ctx, userID)
}

func (s *Accounts) completeLogin(ctx context.Context, saved account.Pixiv, setDefault, preserveDefault bool) (AccountSummary, error) {
	if preserveDefault {
		summary, err := s.summary(ctx, saved.UserID)
		if err != nil {
			// 持久化已成功；新调用方仍需知道账号已保存，不能宣称回滚。
			return accountSummary(saved, false, false), err
		}
		return summary, nil
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return accountSummary(saved, false, false), fmt.Errorf("read pixiv default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.defaults.SetPixivDefaultUserID(saved.UserID); err != nil {
			return accountSummary(saved, false, false), fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	summary, err := s.summary(ctx, saved.UserID)
	if err != nil {
		return accountSummary(saved, false, false), err
	}
	return summary, nil
}

// AccountsWithTokens 返回全部账号及其 refresh token，仅用于显式 auth export。
func (s *Accounts) AccountsWithTokens(ctx context.Context) ([]AccountWithToken, error) {
	accounts, err := s.store.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountWithToken, 0, len(accounts))
	for _, account := range accounts {
		isDefault, err := s.isDefault(ctx, account.UserID)
		if err != nil {
			return nil, err
		}
		out = append(out, AccountWithToken{UserID: account.UserID, Username: account.Username, Default: isDefault, Credential: account.Credential})
	}
	return out, nil
}

// RestoreAccountInput 描述一次 bundle 恢复的单账号输入。
type RestoreAccountInput struct {
	Account         AccountSummary
	RefreshToken    string
	IsBundleDefault bool
}

// RestoreAccountsResult 汇总一次原子 bundle 恢复的结果与最终选中的默认账号。
type RestoreAccountsResult struct {
	Accounts         []RestoreAccountOutcome
	ResultingDefault int64
}

// RestoreAccountOutcome 描述单账号恢复结果。IsReplacement 区分新增与替换。
type RestoreAccountOutcome struct {
	Account       AccountSummary
	IsReplacement bool
}

// RestoreAccounts 原子合并一个 bundle：先读取恢复前 default 与已有 UID 集合，
// 再用一次批量事务写入全部凭据，最后仅在本地无 default 时采用 bundle default。
// 任一凭据写入失败时事务整体回滚，default 配置保持不变。
func (s *Accounts) RestoreAccounts(ctx context.Context, inputs []RestoreAccountInput) (RestoreAccountsResult, error) {
	if len(inputs) == 0 {
		return RestoreAccountsResult{}, errors.New("pixiv restore bundle has no accounts")
	}
	stored := make([]account.Pixiv, 0, len(inputs))
	for _, input := range inputs {
		refreshToken := strings.TrimSpace(input.RefreshToken)
		if input.Account.UserID <= 0 || refreshToken == "" {
			return RestoreAccountsResult{}, errors.New("pixiv refresh token is required")
		}
		account := account.Pixiv{UserID: input.Account.UserID, Username: input.Account.Username, Credential: credential.New([]byte(refreshToken))}
		account.PremiumStatus = input.Account.Premium
		stored = append(stored, account)
	}
	preExistingDefault, hasDefault, err := s.defaults.ReadPixivDefaultUserID()
	if err != nil {
		return RestoreAccountsResult{}, fmt.Errorf("read pixiv default account: %w", err)
	}
	existingUIDs := make(map[int64]struct{})
	if before, err := s.store.ListPixiv(ctx); err != nil {
		return RestoreAccountsResult{}, fmt.Errorf("read pixiv accounts: %w", err)
	} else {
		for _, account := range before {
			existingUIDs[account.UserID] = struct{}{}
		}
	}
	if err := s.store.SavePixivCredentials(ctx, stored); err != nil {
		return RestoreAccountsResult{}, fmt.Errorf("restore pixiv accounts: %w", err)
	}
	resultingDefault := preExistingDefault
	if !hasDefault {
		for _, input := range inputs {
			if input.IsBundleDefault {
				resultingDefault = input.Account.UserID
				break
			}
		}
		if resultingDefault == 0 {
			resultingDefault = inputs[0].Account.UserID
		}
		if err := s.defaults.SetPixivDefaultUserID(resultingDefault); err != nil {
			return RestoreAccountsResult{}, fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	outcomes := make([]RestoreAccountOutcome, 0, len(inputs))
	for _, input := range inputs {
		_, replaced := existingUIDs[input.Account.UserID]
		outcomes = append(outcomes, RestoreAccountOutcome{Account: input.Account, IsReplacement: replaced})
	}
	return RestoreAccountsResult{Accounts: outcomes, ResultingDefault: resultingDefault}, nil
}

func (s *Accounts) setPremiumStatus(ctx context.Context, userID int64, premium *bool) error {
	account, err := s.store.GetPixiv(ctx, userID)
	if err != nil {
		return err
	}
	checkedAt := time.Now().UTC()
	account.PremiumStatus = premium
	account.PremiumCheckedAt = &checkedAt
	return s.store.UpdatePixivMetadata(ctx, account.UserID, account.Username, account.PremiumStatus, account.PremiumCheckedAt)
}

func (s *Accounts) summary(ctx context.Context, userID int64) (AccountSummary, error) {
	account, err := s.store.GetPixiv(ctx, userID)
	if err != nil {
		return AccountSummary{}, err
	}
	isDefault, err := s.isDefault(ctx, account.UserID)
	if err != nil {
		return AccountSummary{}, err
	}
	return accountSummary(account, isDefault, account.PoolLastSelected), nil
}

func accountSummary(saved account.Pixiv, isDefault, lastSelected bool) AccountSummary {
	now := time.Now().UTC().Unix()
	var frozenUntil *int64
	if saved.Pool.FrozenUntil != nil {
		value := saved.Pool.FrozenUntil.Unix()
		frozenUntil = &value
	}
	if frozenUntil != nil && *frozenUntil <= now {
		frozenUntil = nil
	}
	return AccountSummary{
		UserID:           saved.UserID,
		Username:         saved.Username,
		Default:          isDefault,
		Premium:          saved.PremiumStatus,
		Schedulable:      saved.Pool.Enabled,
		PoolFrozenUntil:  frozenUntil,
		PoolLastSelected: lastSelected,
		Eligible:         saved.Pool.Enabled && frozenUntil == nil,
		PoolStatusKnown:  true,
	}
}

func (s *Accounts) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := s.defaultUserID(ctx); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.store.GetPixiv(ctx, userID); err != nil {
			if errors.Is(err, account.ErrPixivNotFound) {
				return 0, fmt.Errorf("configured default pixiv account %d is missing", userID)
			}
			return 0, err
		}
		return userID, nil
	}
	return 0, sdk.NewError("pixiv", "auth", sdk.Unauthorized, sdk.WithDetail("no pixiv account is authenticated"))
}

// defaultUserID 优先返回显式配置，否则按文件顺序选择首个账号。
func (s *Accounts) defaultUserID(ctx context.Context) (int64, bool, error) {
	if userID, ok, err := s.defaults.ReadPixivDefaultUserID(); err != nil {
		return 0, false, err
	} else if ok {
		return userID, true, nil
	}
	accounts, err := s.store.ListPixiv(ctx)
	if err != nil {
		return 0, false, err
	}
	if len(accounts) == 0 {
		return 0, false, nil
	}
	return accounts[0].UserID, true, nil
}

func (s *Accounts) hasDefault() (bool, error) {
	_, ok, err := s.defaults.ReadPixivDefaultUserID()
	return ok, err
}

func (s *Accounts) isDefault(ctx context.Context, userID int64) (bool, error) {
	defaultID, ok, err := s.defaultUserID(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return defaultID == userID, nil
}
