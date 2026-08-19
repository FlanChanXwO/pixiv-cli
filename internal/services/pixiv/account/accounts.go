// Package pixiv 编排 Pixiv 账号选择、refresh token 轮换持久化与公开 SDK 调用。
package pixiv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Service 是 Pixiv 新应用层服务：账号选择 + rotation 持久化 + 公开 SDK。
type Service struct {
	repo     Repository
	defaults DefaultStore
}

// New 构造 Pixiv 应用服务；repository/defaults 均由 composition root 注入。
func NewService(repo Repository, defaults DefaultStore) *Service {
	return &Service{repo: repo, defaults: defaults}
}

// AccountSummary 是 Pixiv 账号的公开摘要。
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

// OpenClient 选择默认账号，执行一次 refresh rotation，持久化 rotated
// credentials，并返回持有新 access token 的 Client。rotation 持久化失败时
// 不发起任何内容请求。
func (s *Service) OpenClient(ctx context.Context) (*pixiv.Client, error) {
	return s.OpenClientWith(ctx, pixiv.Options{})
}

// OpenClientWith 是 OpenClient 的显式连接选项变体。
func (s *Service) OpenClientWith(ctx context.Context, options pixiv.Options) (*pixiv.Client, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	return s.openAccount(ctx, userID, options)
}

// OpenAccountClient 打开指定账号（不改变默认），执行一次 refresh rotation 并
// 持久化 rotated credentials。
func (s *Service) OpenAccountClient(ctx context.Context, userID int64) (*pixiv.Client, error) {
	return s.OpenAccountClientWith(ctx, userID, pixiv.Options{})
}

// OpenAccountClientWith 是 OpenAccountClient 的显式连接选项变体。
func (s *Service) OpenAccountClientWith(ctx context.Context, userID int64, options pixiv.Options) (*pixiv.Client, error) {
	return s.openAccount(ctx, userID, options)
}

func (s *Service) openAccount(ctx context.Context, userID int64, options pixiv.Options) (*pixiv.Client, error) {
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select pixiv account: %w", err)
	}
	client, credentials, err := pixiv.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		return nil, err
	}
	if err := RotateCredential(ctx, s.repo, userID, credentials.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("persist rotated pixiv credentials: %w", err)
	}
	return client, nil
}

// ImportAccount 保留旧账号服务的默认连接行为；验证失败不产生记录。
func (s *Service) ImportAccount(ctx context.Context, refreshToken string, setDefault bool) (AccountSummary, error) {
	return s.ImportAccountWith(ctx, refreshToken, setDefault, pixiv.Options{})
}

// ImportAccountWith 导入一个 refresh token 并在线验证；连接选项由调用方组装，
// 服务不读取配置或推断网络策略。
func (s *Service) ImportAccountWith(ctx context.Context, refreshToken string, setDefault bool, options pixiv.Options) (AccountSummary, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return AccountSummary{}, errors.New("pixiv refresh token is required")
	}
	client, credentials, err := pixiv.OpenWith(ctx, refreshToken, options)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	account := New(credentials.UserID, credentials.Username, []byte(credentials.RefreshToken()))
	account.CredentialRevision = 1
	if err := s.repo.SavePixivCredential(ctx, account); err != nil {
		return AccountSummary{}, fmt.Errorf("save pixiv account: %w", err)
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return AccountSummary{}, fmt.Errorf("read pixiv default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.setDefaultUserID(credentials.UserID); err != nil {
			return AccountSummary{}, fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	return s.summary(ctx, credentials.UserID)
}

// ListAccounts 返回按 sort_order 排列的 Pixiv 账号。
func (s *Service) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	poolStatus, err := s.repo.ListPixivPoolStatus(ctx, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	accounts, err := s.repo.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountSummary, 0, len(accounts))
	for _, account := range accounts {
		isDefault, err := s.isDefault(ctx, account.UserID)
		if err != nil {
			return nil, err
		}
		poolAccount := poolStatusByUserID(poolStatus, account.UserID)
		out = append(out, AccountSummary{
			UserID:           account.UserID,
			Username:         account.Username,
			Default:          isDefault,
			Premium:          account.PremiumStatus,
			Schedulable:      poolAccount.Schedulable,
			PoolFrozenUntil:  poolAccount.PoolFrozenUntil,
			PoolLastSelected: poolAccount.PoolLastSelected,
			Eligible:         poolAccount.Eligible,
			PoolStatusKnown:  true,
		})
	}
	return out, nil
}

func poolStatusByUserID(status PoolStatus, userID int64) PoolCandidate {
	for _, account := range status.Accounts {
		if account.UserID == userID {
			return account
		}
	}
	return PoolCandidate{UserID: userID}
}

// SetPoolSchedulable 原子更新一组已存在的本地账号；未知 UID 会使整批失败。
func (s *Service) SetPoolSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	return s.repo.SetPixivSchedulable(ctx, userIDs, schedulable)
}

// SetAllPoolSchedulable 原子更新当前全部本地账号。
func (s *Service) SetAllPoolSchedulable(ctx context.Context, schedulable bool) error {
	return s.repo.SetAllPixivSchedulable(ctx, schedulable)
}

// PoolStatus 返回一个不含 credential 的账号池快照。
func (s *Service) PoolStatus(ctx context.Context) (PoolStatus, error) {
	return s.repo.ListPixivPoolStatus(ctx, time.Now().UTC().Unix())
}

// RemoveAccount 删除一个 Pixiv 账号。当被删账号是显式 default 时，先清除
// 显式 default 再删除凭据，使首个剩余账号成为新的隐式 default；删除失败时
// 显式 default 不被改变，保持与文档「删除 default 后自动选择首个剩余账号」一致。
func (s *Service) RemoveAccount(ctx context.Context, userID int64) error {
	defaultID, explicit, err := s.readDefaultUserID()
	if err != nil {
		return fmt.Errorf("read pixiv default account: %w", err)
	}
	if explicit && defaultID == userID {
		if err := s.clearDefaultUserID(); err != nil {
			return fmt.Errorf("clear pixiv default account: %w", err)
		}
	}
	if err := s.repo.RemovePixiv(ctx, userID); err != nil {
		return err
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Service) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.repo.GetPixiv(ctx, userID); err != nil {
		return err
	}
	return s.setDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Service) UseAuto() error {
	return s.clearDefaultUserID()
}

// ExportRefreshToken 返回选中账号的 raw refresh token，只允许显式 auth export
// 写入 stdout。
func (s *Service) ExportRefreshToken(ctx context.Context, userID int64) (string, error) {
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return "", err
	}
	return string(account.RefreshTokenCopy()), nil
}

// CurrentUser 返回当前选中账号的身份摘要。
func (s *Service) CurrentUser(ctx context.Context) (*AccountSummary, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AccountSummary{
		UserID:           account.UserID,
		Username:         account.Username,
		Default:          true,
		Premium:          account.PremiumStatus,
		Schedulable:      account.Schedulable,
		PoolFrozenUntil:  account.PoolFrozenUntil,
		PoolLastSelected: account.PoolLastSelected,
		Eligible:         account.Schedulable && (account.PoolFrozenUntil == nil || *account.PoolFrozenUntil <= time.Now().UTC().Unix()),
		PoolStatusKnown:  true,
	}, nil
}

// CheckAccount 在线校验指定账号的 refresh token，并持久化 OAuth 返回的 rotation。
func (s *Service) CheckAccount(ctx context.Context, userID int64) (AccountSummary, error) {
	return s.CheckAccountWith(ctx, userID, pixiv.Options{})
}

// CheckAccountWith 是 CheckAccount 的显式连接选项变体。
func (s *Service) CheckAccountWith(ctx context.Context, userID int64, options pixiv.Options) (AccountSummary, error) {
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return AccountSummary{}, err
	}
	client, credentials, err := pixiv.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	if err := RotateCredential(ctx, s.repo, account.UserID, credentials.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		return AccountSummary{}, fmt.Errorf("persist rotated pixiv credentials: %w", err)
	}
	return AccountSummary{UserID: credentials.UserID, Username: credentials.Username}, nil
}

// CheckRefreshToken 在线校验一个 refresh token，不写库。
func (s *Service) CheckRefreshToken(ctx context.Context, token string) (AccountSummary, error) {
	return s.checkRefreshToken(ctx, 0, token, pixiv.Options{}, false)
}

// CheckRefreshTokenForUser 在线校验目标账号的 refresh token，不写库，并验证
// OAuth 返回的 UID 与本地账号身份一致。
func (s *Service) CheckRefreshTokenForUser(ctx context.Context, expectedUserID int64, token string) (AccountSummary, error) {
	return s.CheckRefreshTokenWith(ctx, expectedUserID, token, pixiv.Options{})
}

// CheckRefreshTokenWith 是 CheckRefreshTokenForUser 的显式连接选项变体。
func (s *Service) CheckRefreshTokenWith(ctx context.Context, expectedUserID int64, token string, options pixiv.Options) (AccountSummary, error) {
	return s.checkRefreshToken(ctx, expectedUserID, token, options, true)
}

func (s *Service) checkRefreshToken(ctx context.Context, expectedUserID int64, token string, options pixiv.Options, verifyIdentity bool) (AccountSummary, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AccountSummary{}, errors.New("pixiv refresh token is required")
	}
	client, credentials, err := pixiv.OpenWith(ctx, token, options)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	if verifyIdentity {
		if err := VerifyAccountIdentity(expectedUserID, credentials.UserID); err != nil {
			return AccountSummary{}, err
		}
	}
	return AccountSummary{UserID: credentials.UserID, Username: credentials.Username}, nil
}

// RefreshAccount 刷新指定账号的凭据并强制读取当前 profile，把会员资格快照写回
// database，最后返回账号摘要。
func (s *Service) RefreshAccount(ctx context.Context, userID int64) (AccountSummary, error) {
	return s.RefreshAccountWith(ctx, userID, pixiv.Options{})
}

// RefreshAccountWith 是 RefreshAccount 的显式连接选项变体。
func (s *Service) RefreshAccountWith(ctx context.Context, userID int64, options pixiv.Options) (AccountSummary, error) {
	client, err := s.OpenAccountClientWith(ctx, userID, options)
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

// CompleteLogin 保存一次完成登录会话后得到的账号凭据，并按 setDefault 或
// 默认账号缺失规则设置默认账号。
func (s *Service) CompleteLogin(ctx context.Context, credentials pixiv.Credentials, setDefault bool) (AccountSummary, error) {
	if credentials.UserID <= 0 || credentials.RefreshToken() == "" {
		return AccountSummary{}, errors.New("login credentials are incomplete")
	}
	account := New(credentials.UserID, credentials.Username, []byte(credentials.RefreshToken()))
	account.CredentialRevision = 1
	if err := s.repo.SavePixivCredential(ctx, account); err != nil {
		return AccountSummary{}, fmt.Errorf("save pixiv account: %w", err)
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return AccountSummary{}, fmt.Errorf("read pixiv default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.setDefaultUserID(credentials.UserID); err != nil {
			return AccountSummary{}, fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	return s.summary(ctx, credentials.UserID)
}

// AccountsWithTokens 返回全部账号及其 refresh token，仅用于显式 auth export。
func (s *Service) AccountsWithTokens(ctx context.Context) ([]AccountWithToken, error) {
	accounts, err := s.repo.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountWithToken, 0, len(accounts))
	for _, account := range accounts {
		isDefault, err := s.isDefault(ctx, account.UserID)
		if err != nil {
			return nil, err
		}
		out = append(out, NewAccountWithToken(
			account.UserID,
			account.Username,
			isDefault,
			string(account.RefreshTokenCopy()),
		))
	}
	return out, nil
}

// RestoreAccount 离线恢复单个 bundle 账号，不做网络验证；已有 default 始终保持不变。
//
// 单账号恢复无法提供 bundle 恢复的原子合并语义；多账号 bundle 恢复必须使用
// RestoreAccounts，使任一写入失败时不会留下部分提交。
func (s *Service) RestoreAccount(ctx context.Context, account AccountSummary, refreshToken string, _ bool) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if account.UserID <= 0 || refreshToken == "" {
		return errors.New("pixiv refresh token is required")
	}
	stored := New(account.UserID, account.Username, []byte(refreshToken))
	stored.CredentialRevision = 1
	stored.PremiumStatus = account.Premium
	if err := s.repo.SavePixivCredential(ctx, stored); err != nil {
		return fmt.Errorf("restore pixiv account: %w", err)
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return fmt.Errorf("read pixiv default account: %w", err)
	}
	if !hasDefault {
		if err := s.setDefaultUserID(account.UserID); err != nil {
			return fmt.Errorf("set pixiv default account: %w", err)
		}
	}
	return nil
}

// RestoreAccountInput 是一次 bundle 恢复的单账号输入。
type RestoreAccountInput struct {
	Account         AccountSummary
	RefreshToken    string
	IsBundleDefault bool
}

// RestoreAccountsResult 是一次原子 bundle 恢复的结果。ResultingDefault 为最终
// 选中默认账号；IsDefaultReplacement 区分本次写入是否替换了已有凭据。
type RestoreAccountsResult struct {
	Accounts         []RestoreAccountOutcome
	ResultingDefault int64
}

// RestoreAccountOutcome 是单账号恢复结果。IsReplacement 区分新增与替换。
type RestoreAccountOutcome struct {
	Account       AccountSummary
	IsReplacement bool
}

// RestoreAccounts 原子合并一个 bundle：先读取恢复前 default 与已有 UID 集合，
// 再用一次批量事务写入全部凭据，最后仅在本地无 default 时采用 bundle default。
// 任一凭据写入失败时事务整体回滚，default 配置保持不变。
func (s *Service) RestoreAccounts(ctx context.Context, inputs []RestoreAccountInput) (RestoreAccountsResult, error) {
	if len(inputs) == 0 {
		return RestoreAccountsResult{}, errors.New("pixiv restore bundle has no accounts")
	}
	stored := make([]Account, 0, len(inputs))
	for _, input := range inputs {
		refreshToken := strings.TrimSpace(input.RefreshToken)
		if input.Account.UserID <= 0 || refreshToken == "" {
			return RestoreAccountsResult{}, errors.New("pixiv refresh token is required")
		}
		account := New(input.Account.UserID, input.Account.Username, []byte(refreshToken))
		account.CredentialRevision = 1
		account.PremiumStatus = input.Account.Premium
		stored = append(stored, account)
	}
	preExistingDefault, hasDefault, err := s.readDefaultUserID()
	if err != nil {
		return RestoreAccountsResult{}, fmt.Errorf("read pixiv default account: %w", err)
	}
	existingUIDs := make(map[int64]struct{})
	if before, err := s.repo.ListPixiv(ctx); err != nil {
		return RestoreAccountsResult{}, fmt.Errorf("read pixiv accounts: %w", err)
	} else {
		for _, account := range before {
			existingUIDs[account.UserID] = struct{}{}
		}
	}
	if err := s.repo.SavePixivCredentials(ctx, stored); err != nil {
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
		if err := s.setDefaultUserID(resultingDefault); err != nil {
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

func (s *Service) setPremiumStatus(ctx context.Context, userID int64, premium *bool) error {
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return err
	}
	checkedAt := time.Now().UTC().Unix()
	account.PremiumStatus = premium
	account.PremiumCheckedAt = &checkedAt
	return s.repo.UpdatePixivMetadata(ctx, account.UserID, account.Username, account.PremiumStatus, account.PremiumCheckedAt)
}

func (s *Service) summary(ctx context.Context, userID int64) (AccountSummary, error) {
	account, err := s.repo.GetPixiv(ctx, userID)
	if err != nil {
		return AccountSummary{}, err
	}
	isDefault, err := s.isDefault(ctx, account.UserID)
	if err != nil {
		return AccountSummary{}, err
	}
	return accountSummary(account, isDefault), nil
}

func accountSummary(account Account, isDefault bool) AccountSummary {
	now := time.Now().UTC().Unix()
	frozenUntil := account.PoolFrozenUntil
	if frozenUntil != nil && *frozenUntil <= now {
		frozenUntil = nil
	}
	return AccountSummary{
		UserID:           account.UserID,
		Username:         account.Username,
		Default:          isDefault,
		Premium:          account.PremiumStatus,
		Schedulable:      account.Schedulable,
		PoolFrozenUntil:  frozenUntil,
		PoolLastSelected: account.PoolLastSelected,
		Eligible:         account.Schedulable && frozenUntil == nil,
		PoolStatusKnown:  true,
	}
}

func (s *Service) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := s.defaultUserID(ctx); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.repo.GetPixiv(ctx, userID); err != nil {
			return 0, fmt.Errorf("configured default pixiv account %d is missing", userID)
		}
		return userID, nil
	}
	return 0, sdk.NewError("pixiv", "auth", sdk.Unauthorized, sdk.WithDetail("no pixiv account is authenticated"))
}

// defaultUserID 返回当前默认账号：优先显式配置，否则回退到 sort_order 最小的
// 首个入库账号。
func (s *Service) defaultUserID(ctx context.Context) (int64, bool, error) {
	if userID, ok, err := s.readDefaultUserID(); err != nil {
		return 0, false, err
	} else if ok {
		return userID, true, nil
	}
	accounts, err := s.repo.ListPixiv(ctx)
	if err != nil {
		return 0, false, err
	}
	if len(accounts) == 0 {
		return 0, false, nil
	}
	return accounts[0].UserID, true, nil
}

func (s *Service) hasDefault() (bool, error) {
	_, ok, err := s.readDefaultUserID()
	return ok, err
}

func (s *Service) isDefault(ctx context.Context, userID int64) (bool, error) {
	defaultID, ok, err := s.defaultUserID(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return defaultID == userID, nil
}

func (s *Service) readDefaultUserID() (int64, bool, error) {
	if s.defaults == nil {
		return 0, false, errors.New("pixiv default account store is not configured")
	}
	return s.defaults.ReadPixivDefaultUserID()
}

func (s *Service) setDefaultUserID(userID int64) error {
	if s.defaults == nil {
		return errors.New("pixiv default account store is not configured")
	}
	return s.defaults.SetPixivDefaultUserID(userID)
}

func (s *Service) clearDefaultUserID() error {
	if s.defaults == nil {
		return errors.New("pixiv default account store is not configured")
	}
	return s.defaults.ClearPixivDefaultUserID()
}

// RotateCredential performs the identity check and revision CAS at the product
// account repository. Session policy decides when to rotate; this function owns
// the repository-side commit.
func RotateCredential(ctx context.Context, repository interface {
	RotatePixivCredentials(context.Context, int64, int64, []byte) error
}, selectedUserID, authenticatedUserID, revision int64, refreshToken []byte) error {
	if err := VerifyAccountIdentity(selectedUserID, authenticatedUserID); err != nil {
		return err
	}
	if repository == nil {
		return errors.New("pixiv credential repository is not configured")
	}
	return repository.RotatePixivCredentials(ctx, selectedUserID, revision, refreshToken)
}

// VerifyAccountIdentity rejects a credential whose authenticated UID does not
// match the selected local account before rotation or content requests.
func VerifyAccountIdentity(selectedUserID, authenticatedUserID int64) error {
	if selectedUserID <= 0 || authenticatedUserID <= 0 || selectedUserID != authenticatedUserID {
		return sdk.NewError("pixiv", "OpenAccountClient", sdk.LocalStateError, sdk.WithDetail("credential identity does not match selected account"))
	}
	return nil
}
