// Package fanbox 拥有 FANBOX 账号 domain 与 CLI/MCP 共用的账号/session 服务。
package fanbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// Service 是 CLI/MCP 的 FANBOX 账号服务。
type Service struct {
	repo     Repository
	defaults DefaultStore

	// LoadOptionsFunc supplies explicit runtime network settings at the point a
	// native SDK client is opened. Keeping it lazy lets config errors surface at
	// the operation boundary without making the auth database constructor read
	// network configuration.
	LoadOptionsFunc func() (fanbox.Options, error)

	// OpenClientFunc 覆盖 OpenClient 的客户端打开逻辑；nil 时按默认账号的已保存
	// session 打开。测试用它注入不拨号的 httptest transport。
	OpenClientFunc func(ctx context.Context) (*fanbox.Client, error)
	// OpenSessionFunc 覆盖 ImportSession 的客户端打开逻辑；nil 时使用
	// fanbox.Open（生产 transport）。测试用它注入验证用 httptest transport。
	OpenSessionFunc func(sessionValue string) (*fanbox.Client, error)
}

// NewService 构造 FANBOX 账号服务；repository/defaults 由 composition root 注入。
func NewService(repo Repository, defaults DefaultStore) *Service {
	return &Service{repo: repo, defaults: defaults}
}

// AccountSummary 是 auth list 的输出行。
type AccountSummary struct {
	UserID      int64
	DisplayName string
	CreatorID   string
	Default     bool
}

// ImportSession 使用显式 FANBOXSESSID 在线验证并保存账号；验证失败不产生记录。
func (s *Service) ImportSession(ctx context.Context, sessionValue string, setDefault bool) (AccountSummary, error) {
	return s.ImportSessionWithProxy(ctx, sessionValue, setDefault, nil)
}

// ImportSessionWithProxy validates and stores a session using an optional
// command-scoped native proxy override. The solver addresses remain unchanged.
func (s *Service) ImportSessionWithProxy(ctx context.Context, sessionValue string, setDefault bool, proxyOverride *string) (AccountSummary, error) {
	sessionValue = strings.TrimSpace(sessionValue)
	if sessionValue == "" {
		return AccountSummary{}, errors.New("FANBOX session value is required")
	}
	open := s.OpenSessionFunc
	if open == nil {
		open = func(value string) (*fanbox.Client, error) {
			options, err := s.connectionOptions(proxyOverride)
			if err != nil {
				return nil, err
			}
			return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, options)
		}
	}
	client, err := open(sessionValue)
	if err != nil {
		return AccountSummary{}, err
	}
	defer client.CloseIdleConnections()
	user, err := client.CurrentUser(ctx, fanbox.CurrentUserRequest{})
	if err != nil {
		return AccountSummary{}, err
	}
	now := time.Now().UTC().Unix()
	account := New(user.UserID, user.DisplayName, user.CreatorID, []byte(sessionValue))
	account.CredentialRevision = 1
	account.ValidatedAt = now
	if err := s.repo.SaveFanboxCredential(ctx, account); err != nil {
		return AccountSummary{}, fmt.Errorf("save fanbox account: %w", err)
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return AccountSummary{}, fmt.Errorf("read fanbox default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.setDefaultUserID(user.UserID); err != nil {
			return AccountSummary{}, fmt.Errorf("set fanbox default account: %w", err)
		}
	}
	isDefault, err := s.isDefault(user.UserID)
	if err != nil {
		return AccountSummary{}, err
	}
	return AccountSummary{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Default: isDefault}, nil
}

// ListAccounts 返回按 sort_order 排列的 FANBOX 账号。
func (s *Service) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.repo.ListFanbox(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountSummary, 0, len(accounts))
	for _, account := range accounts {
		isDefault, err := s.isDefault(account.UserID)
		if err != nil {
			return nil, err
		}
		out = append(out, AccountSummary{
			UserID:      account.UserID,
			DisplayName: account.DisplayName,
			CreatorID:   account.CreatorID,
			Default:     isDefault,
		})
	}
	return out, nil
}

// RemoveAccount 删除一个 FANBOX 账号。
func (s *Service) RemoveAccount(ctx context.Context, userID int64) error {
	defaultID, explicit, err := s.readDefaultUserID()
	if err != nil {
		return fmt.Errorf("read fanbox default account: %w", err)
	}
	if explicit && defaultID == userID {
		return fmt.Errorf("cannot remove fanbox account %d while it is the explicit default; use `pixiv fanbox auth use --auto` or select another account first", userID)
	}
	if err := s.repo.RemoveFanbox(ctx, userID); err != nil {
		return err
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Service) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.repo.GetFanbox(ctx, userID); err != nil {
		return err
	}
	return s.setDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Service) UseAuto() error {
	return s.clearDefaultUserID()
}

// Status 返回当前默认账号身份。
func (s *Service) Status(ctx context.Context) (*AccountSummary, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.repo.GetFanbox(ctx, userID)
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
	return s.OpenClientWithProxy(ctx, nil)
}

// OpenClientWithProxy opens the selected account with a command-scoped native
// proxy override. A nil override preserves the configured service/global
// precedence; the FlareSolverr service and upstream proxy are never changed.
func (s *Service) OpenClientWithProxy(ctx context.Context, proxyOverride *string) (*fanbox.Client, error) {
	if s.OpenClientFunc != nil {
		return s.OpenClientFunc(ctx)
	}
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.repo.GetFanbox(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select fanbox account: %w", err)
	}
	options, err := s.connectionOptions(proxyOverride)
	if err != nil {
		return nil, err
	}
	return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: string(account.SessionIDCopy())}, options)
}

func (s *Service) connectionOptions(proxyOverride *string) (fanbox.Options, error) {
	var options fanbox.Options
	if s.LoadOptionsFunc != nil {
		loaded, err := s.LoadOptionsFunc()
		if err != nil {
			return fanbox.Options{}, err
		}
		options = loaded
	}
	if proxyOverride != nil {
		options.ProxyURL = *proxyOverride
	}
	return options, nil
}

func (s *Service) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := s.readDefaultUserID(); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.repo.GetFanbox(ctx, userID); err != nil {
			return 0, fmt.Errorf("configured default fanbox account %d is missing", userID)
		}
		return userID, nil
	}
	accounts, err := s.repo.ListFanbox(ctx)
	if err != nil {
		return 0, err
	}
	if len(accounts) == 0 {
		return 0, sdk.NewError("fanbox", "", sdk.Unauthorized, sdk.WithDetail("no fanbox account is authenticated"))
	}
	return accounts[0].UserID, nil
}

func (s *Service) hasDefault() (bool, error) {
	_, ok, err := s.readDefaultUserID()
	return ok, err
}

func (s *Service) isDefault(userID int64) (bool, error) {
	configured, ok, err := s.readDefaultUserID()
	if err != nil {
		return false, err
	}
	if ok {
		return configured == userID, nil
	}
	// 未配置时默认是首个入库账号。
	accounts, err := s.repo.ListFanbox(context.Background())
	if err != nil {
		return false, err
	}
	return len(accounts) > 0 && accounts[0].UserID == userID, nil
}

func (s *Service) readDefaultUserID() (int64, bool, error) {
	if s.defaults == nil {
		return 0, false, errors.New("fanbox default account store is not configured")
	}
	return s.defaults.ReadFanboxDefaultUserID()
}

func (s *Service) setDefaultUserID(userID int64) error {
	if s.defaults == nil {
		return errors.New("fanbox default account store is not configured")
	}
	return s.defaults.SetFanboxDefaultUserID(userID)
}

func (s *Service) clearDefaultUserID() error {
	if s.defaults == nil {
		return errors.New("fanbox default account store is not configured")
	}
	return s.defaults.ClearFanboxDefaultUserID()
}
