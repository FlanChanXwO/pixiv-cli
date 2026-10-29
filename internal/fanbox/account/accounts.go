// Package account 提供 FANBOX 本地账号领域，以及 CLI/MCP 共用的账号与 session 服务。
//
// FANBOX 的账号选择、会话校验和存储端口独立于 Pixiv。
//
// 包的组成：[Account] 是保存的账号记录，[Repository] 与 [DefaultStore] 是存储端口，
// [Service] 负责导入、选择、列出与打开客户端，[AccountSummary] 是面向输出的摘要。
package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// Service 提供 FANBOX 账号导入、列出、选择、移除与 client 打开。
type Service struct {
	repo        Repository
	defaults    DefaultStore
	loadOptions func() (fanbox.Options, error)
}

// NewService 构造 FANBOX 账号服务。loadOptions 每次打开客户端时读取当前网络
// 配置，使配置错误留在执行边界，避免构造账号库时提前读取配置。
func NewService(repo Repository, defaults DefaultStore, loadOptions func() (fanbox.Options, error)) *Service {
	return &Service{repo: repo, defaults: defaults, loadOptions: loadOptions}
}

// AccountSummary 汇总 auth list 的输出行。
type AccountSummary struct {
	UserID      int64
	DisplayName string
	CreatorID   string
	Default     bool
}

// ImportSession 使用显式 FANBOXSESSID 在线验证并保存账号；验证失败不产生记录。
// proxyOverride 只覆盖本次请求的原生 HTTP proxy，空值保留配置的优先级。
func (s *Service) ImportSession(ctx context.Context, sessionValue string, setDefault bool, proxyOverride *string) (AccountSummary, error) {
	sessionValue = strings.TrimSpace(sessionValue)
	if sessionValue == "" {
		return AccountSummary{}, errors.New("FANBOX session value is required")
	}
	options, err := s.connectionOptions(proxyOverride)
	if err != nil {
		return AccountSummary{}, err
	}
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: sessionValue}, options)
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
	isDefault, err := s.isDefault(ctx, user.UserID)
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
		isDefault, err := s.isDefault(ctx, account.UserID)
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

// OpenClient 打开选中的账号。proxyOverride 只影响本次请求的原生代理，nil 时按配置的
// 服务级/全局优先级决定；FlareSolverr 服务与 upstream 代理保持不变。
func (s *Service) OpenClient(ctx context.Context, proxyOverride *string) (*fanbox.Client, error) {
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
	if s.loadOptions != nil {
		loaded, err := s.loadOptions()
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

func (s *Service) isDefault(ctx context.Context, userID int64) (bool, error) {
	configured, ok, err := s.readDefaultUserID()
	if err != nil {
		return false, err
	}
	if ok {
		return configured == userID, nil
	}
	// 未配置时默认是首个入库账号。
	accounts, err := s.repo.ListFanbox(ctx)
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
