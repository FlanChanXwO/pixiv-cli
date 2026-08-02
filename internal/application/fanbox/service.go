// Package fanbox 编排 FANBOX 账号选择、session 持久化与公开 SDK 调用。
package fanbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// Service 是 CLI/MCP 的 FANBOX 应用层服务。
type Service struct {
	db         *authdb.DB
	appDataDir string

	// OpenClientFunc 覆盖 OpenClient 的客户端打开逻辑；nil 时按默认账号的已保存
	// session 打开。测试用它注入不拨号的 httptest transport。
	OpenClientFunc func(ctx context.Context) (*fanbox.Client, error)
	// OpenSessionFunc 覆盖 ImportSession 的客户端打开逻辑；nil 时使用
	// fanbox.Open（生产 transport）。测试用它注入验证用 httptest transport。
	OpenSessionFunc func(sessionValue string) (*fanbox.Client, error)
}

// New 构造 FANBOX 应用服务。
func New(db *authdb.DB, appDataDir string) *Service {
	return &Service{db: db, appDataDir: appDataDir}
}

// Account 是 FANBOX 账号的公开摘要。
type Account struct {
	UserID      int64
	DisplayName string
	CreatorID   string
	Default     bool
}

// AccountSummary 是 auth list 的输出行。
type AccountSummary struct {
	UserID      int64
	DisplayName string
	CreatorID   string
	Default     bool
}

// ImportSession 使用显式 FANBOXSESSID 在线验证并保存账号；验证失败不产生记录。
func (s *Service) ImportSession(ctx context.Context, sessionValue string, setDefault bool) (Account, error) {
	sessionValue = strings.TrimSpace(sessionValue)
	if sessionValue == "" {
		return Account{}, errors.New("FANBOX session value is required")
	}
	open := s.OpenSessionFunc
	if open == nil {
		open = func(value string) (*fanbox.Client, error) {
			return fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: value})
		}
	}
	client, err := open(sessionValue)
	if err != nil {
		return Account{}, err
	}
	defer client.CloseIdleConnections()
	user, err := client.CurrentUser(ctx, fanbox.CurrentUserRequest{})
	if err != nil {
		return Account{}, err
	}
	now := time.Now().UTC().Unix()
	account := authdb.FanboxAccount{
		UserID:             user.UserID,
		DisplayName:        user.DisplayName,
		CreatorID:          user.CreatorID,
		SessionID:          []byte(sessionValue),
		CredentialRevision: 1,
		ValidatedAt:        now,
	}
	if err := s.db.UpsertFanbox(ctx, account); err != nil {
		return Account{}, fmt.Errorf("save fanbox account: %w", err)
	}
	if setDefault || !s.hasDefault() {
		_ = config.SetFanboxDefaultUserID(user.UserID)
	}
	return Account{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Default: s.isDefault(user.UserID)}, nil
}

// ListAccounts 返回按 sort_order 排列的 FANBOX 账号。
func (s *Service) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.db.ListFanbox(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountSummary, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, AccountSummary{
			UserID:      account.UserID,
			DisplayName: account.DisplayName,
			CreatorID:   account.CreatorID,
			Default:     s.isDefault(account.UserID),
		})
	}
	return out, nil
}

// RemoveAccount 删除一个 FANBOX 账号。
func (s *Service) RemoveAccount(ctx context.Context, userID int64) error {
	if err := s.db.RemoveFanbox(ctx, userID); err != nil {
		return err
	}
	if s.isDefault(userID) {
		_ = config.ClearFanboxDefaultUserID()
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Service) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.db.GetFanbox(ctx, userID); err != nil {
		return err
	}
	return config.SetFanboxDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Service) UseAuto() error {
	return config.ClearFanboxDefaultUserID()
}

// Status 返回当前默认账号身份。
func (s *Service) Status(ctx context.Context) (*AccountSummary, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.db.GetFanbox(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AccountSummary{
		UserID:      account.UserID,
		DisplayName: account.DisplayName,
		CreatorID:   account.CreatorID,
		Default:     true,
	}, nil
}

// OpenClient 打开默认账号对应的 sdk/fanbox Client。
func (s *Service) OpenClient(ctx context.Context) (*fanbox.Client, error) {
	if s.OpenClientFunc != nil {
		return s.OpenClientFunc(ctx)
	}
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.db.GetFanbox(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select fanbox account: %w", err)
	}
	return fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: string(account.SessionID)})
}

func (s *Service) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := config.ReadFanboxDefaultUserID(); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.db.GetFanbox(ctx, userID); err != nil {
			return 0, fmt.Errorf("configured default fanbox account %d is missing", userID)
		}
		return userID, nil
	}
	accounts, err := s.db.ListFanbox(ctx)
	if err != nil {
		return 0, err
	}
	if len(accounts) == 0 {
		return 0, sdk.NewError("fanbox", "", sdk.CodeUnauthorized, sdk.WithDetail("no fanbox account is authenticated"))
	}
	return accounts[0].UserID, nil
}

func (s *Service) hasDefault() bool {
	_, ok, _ := config.ReadFanboxDefaultUserID()
	return ok
}

func (s *Service) isDefault(userID int64) bool {
	configured, ok, _ := config.ReadFanboxDefaultUserID()
	if ok {
		return configured == userID
	}
	// 未配置时默认是首个入库账号。
	return false
}
