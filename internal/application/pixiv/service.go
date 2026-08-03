// Package pixiv 编排 Pixiv 账号选择、refresh token 轮换持久化与公开 SDK 调用。
package pixiv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Service 是 Pixiv 新应用层服务：账号选择 + rotation 持久化 + 公开 SDK。
type Service struct {
	db         *authdb.DB
	appDataDir string
}

// New 构造 Pixiv 应用服务。
func New(db *authdb.DB, appDataDir string) *Service {
	return &Service{db: db, appDataDir: appDataDir}
}

// Account 是 Pixiv 账号的公开摘要。
type Account struct {
	UserID   int64
	Username string
	Default  bool
	Premium  *bool
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
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select pixiv account: %w", err)
	}
	client, credentials, err := pixiv.OpenWith(ctx, string(account.RefreshToken), options)
	if err != nil {
		return nil, err
	}
	if err := s.db.RotatePixivCredentials(ctx, userID, []byte(credentials.RefreshToken())); err != nil {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("persist rotated pixiv credentials: %w", err)
	}
	return client, nil
}

// ImportAccount 导入一个 refresh token 并在线验证；验证失败不产生记录。
func (s *Service) ImportAccount(ctx context.Context, refreshToken string, setDefault bool) (Account, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return Account{}, errors.New("pixiv refresh token is required")
	}
	client, credentials, err := pixiv.Open(ctx, refreshToken)
	if err != nil {
		return Account{}, err
	}
	defer client.CloseIdleConnections()
	account := authdb.PixivAccount{
		UserID:             credentials.UserID,
		Username:           credentials.Username,
		RefreshToken:       []byte(credentials.RefreshToken()),
		CredentialRevision: 1,
	}
	if err := s.db.UpsertPixiv(ctx, account); err != nil {
		return Account{}, fmt.Errorf("save pixiv account: %w", err)
	}
	if setDefault || !s.hasDefault() {
		_ = config.SetPixivDefaultUserID(credentials.UserID)
	}
	return Account{UserID: credentials.UserID, Username: credentials.Username, Default: s.isDefault(credentials.UserID)}, nil
}

// ListAccounts 返回按 sort_order 排列的 Pixiv 账号。
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	accounts, err := s.db.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, Account{
			UserID:   account.UserID,
			Username: account.Username,
			Default:  s.isDefault(account.UserID),
			Premium:  account.PremiumStatus,
		})
	}
	return out, nil
}

// RemoveAccount 删除一个 Pixiv 账号。
func (s *Service) RemoveAccount(ctx context.Context, userID int64) error {
	if err := s.db.RemovePixiv(ctx, userID); err != nil {
		return err
	}
	if s.isDefault(userID) {
		_ = config.ClearPixivDefaultUserID()
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Service) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.db.GetPixiv(ctx, userID); err != nil {
		return err
	}
	return config.SetPixivDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Service) UseAuto() error {
	return config.ClearPixivDefaultUserID()
}

// ExportRefreshToken 返回选中账号的 raw refresh token，只允许显式 auth export
// 写入 stdout。
func (s *Service) ExportRefreshToken(ctx context.Context, userID int64) (string, error) {
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return "", err
	}
	return string(account.RefreshToken), nil
}

// CurrentUser 返回当前选中账号的身份摘要。
func (s *Service) CurrentUser(ctx context.Context) (*Account, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &Account{UserID: account.UserID, Username: account.Username, Default: true, Premium: account.PremiumStatus}, nil
}

// CheckAccount 在线校验指定账号的 refresh token，不写库。
func (s *Service) CheckAccount(ctx context.Context, userID int64) (Account, error) {
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	return s.CheckRefreshToken(ctx, string(account.RefreshToken))
}

// CheckRefreshToken 在线校验一个 refresh token，不写库。
func (s *Service) CheckRefreshToken(ctx context.Context, token string) (Account, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Account{}, errors.New("pixiv refresh token is required")
	}
	client, credentials, err := pixiv.Open(ctx, token)
	if err != nil {
		return Account{}, err
	}
	defer client.CloseIdleConnections()
	return Account{UserID: credentials.UserID, Username: credentials.Username}, nil
}

// RefreshAccount 刷新指定账号的凭据并强制读取当前 profile，把会员资格快照写回
// authdb，最后返回账号摘要。
func (s *Service) RefreshAccount(ctx context.Context, userID int64) (Account, error) {
	client, err := s.OpenAccountClient(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	defer client.CloseIdleConnections()
	detail, err := client.CurrentUser(ctx, pixiv.CurrentUserRequest{})
	if err != nil {
		return Account{}, err
	}
	premium := detail.Profile.IsPremium
	if err := s.setPremiumStatus(ctx, userID, &premium); err != nil {
		return Account{}, err
	}
	return s.summary(ctx, userID)
}

// CompleteLogin 保存一次完成登录会话后得到的账号凭据，并按 setDefault 或
// 默认账号缺失规则设置默认账号。
func (s *Service) CompleteLogin(ctx context.Context, credentials pixiv.Credentials, setDefault bool) (Account, error) {
	if credentials.UserID <= 0 || credentials.RefreshToken() == "" {
		return Account{}, errors.New("login credentials are incomplete")
	}
	account := authdb.PixivAccount{
		UserID:             credentials.UserID,
		Username:           credentials.Username,
		RefreshToken:       []byte(credentials.RefreshToken()),
		CredentialRevision: 1,
	}
	if err := s.db.UpsertPixiv(ctx, account); err != nil {
		return Account{}, fmt.Errorf("save pixiv account: %w", err)
	}
	if setDefault || !s.hasDefault() {
		_ = config.SetPixivDefaultUserID(credentials.UserID)
	}
	return Account{UserID: credentials.UserID, Username: credentials.Username, Default: s.isDefault(credentials.UserID)}, nil
}

// AccountWithToken 是仅用于显式 auth export 的账号+token 摘要。
type AccountWithToken struct {
	UserID       int64
	Username     string
	Default      bool
	RefreshToken string
}

// AccountsWithTokens 返回全部账号及其 refresh token，仅用于显式 auth export。
func (s *Service) AccountsWithTokens(ctx context.Context) ([]AccountWithToken, error) {
	accounts, err := s.db.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountWithToken, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, AccountWithToken{
			UserID:       account.UserID,
			Username:     account.Username,
			Default:      s.isDefault(account.UserID),
			RefreshToken: string(account.RefreshToken),
		})
	}
	return out, nil
}

// RestoreAccount 离线恢复一个 bundle 账号，不做网络验证。
func (s *Service) RestoreAccount(ctx context.Context, account Account, refreshToken string, setDefault bool) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if account.UserID <= 0 || refreshToken == "" {
		return errors.New("pixiv refresh token is required")
	}
	stored := authdb.PixivAccount{
		UserID:             account.UserID,
		Username:           account.Username,
		RefreshToken:       []byte(refreshToken),
		CredentialRevision: 1,
		PremiumStatus:      account.Premium,
	}
	if err := s.db.UpsertPixiv(ctx, stored); err != nil {
		return fmt.Errorf("restore pixiv account: %w", err)
	}
	if setDefault || !s.hasDefault() {
		_ = config.SetPixivDefaultUserID(account.UserID)
	}
	return nil
}

func (s *Service) setPremiumStatus(ctx context.Context, userID int64, premium *bool) error {
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return err
	}
	checkedAt := time.Now().UTC().Unix()
	account.PremiumStatus = premium
	account.PremiumCheckedAt = &checkedAt
	return s.db.UpsertPixiv(ctx, account)
}

func (s *Service) summary(ctx context.Context, userID int64) (Account, error) {
	account, err := s.db.GetPixiv(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	return Account{UserID: account.UserID, Username: account.Username, Default: s.isDefault(account.UserID), Premium: account.PremiumStatus}, nil
}

func (s *Service) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := s.defaultUserID(ctx); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.db.GetPixiv(ctx, userID); err != nil {
			return 0, fmt.Errorf("configured default pixiv account %d is missing", userID)
		}
		return userID, nil
	}
	return 0, sdk.NewError("pixiv", "auth", sdk.CodeUnauthorized, sdk.WithDetail("no pixiv account is authenticated"))
}

// defaultUserID 返回当前默认账号：优先显式配置，否则回退到 sort_order 最小的
// 首个入库账号。
func (s *Service) defaultUserID(ctx context.Context) (int64, bool, error) {
	if userID, ok, err := config.ReadPixivDefaultUserID(); err != nil {
		return 0, false, err
	} else if ok {
		return userID, true, nil
	}
	accounts, err := s.db.ListPixiv(ctx)
	if err != nil {
		return 0, false, err
	}
	if len(accounts) == 0 {
		return 0, false, nil
	}
	return accounts[0].UserID, true, nil
}

func (s *Service) hasDefault() bool {
	_, ok, _ := config.ReadPixivDefaultUserID()
	return ok
}

func (s *Service) isDefault(userID int64) bool {
	defaultID, ok, _ := s.defaultUserID(context.Background())
	if !ok {
		return false
	}
	return defaultID == userID
}
