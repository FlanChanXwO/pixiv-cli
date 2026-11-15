package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// Accounts 提供 FANBOX 账号导入、列出、选择、移除与 client 打开。
type Accounts struct {
	store       *account.Store
	defaults    config.Store
	loadOptions func() (fanbox.Options, error)
}

// NewAccounts 绑定账号文件、默认配置与按调用读取的连接选项。
func NewAccounts(store *account.Store, defaults config.Store, loadOptions func() (fanbox.Options, error)) *Accounts {
	return &Accounts{store: store, defaults: defaults, loadOptions: loadOptions}
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
func (s *Accounts) ImportSession(ctx context.Context, sessionValue string, setDefault bool, proxyOverride *string) (AccountSummary, error) {
	sessionValue = strings.TrimSpace(sessionValue)
	if sessionValue == "" {
		return AccountSummary{}, errors.New("FANBOX session value is required")
	}
	options, err := s.connectionOptions(proxyOverride)
	if err != nil {
		return AccountSummary{}, err
	}
	user, err := s.store.ImportFanbox(ctx, sessionValue, options)
	if err != nil {
		return AccountSummary{}, err
	}
	hasDefault, err := s.hasDefault()
	if err != nil {
		return AccountSummary{}, fmt.Errorf("read fanbox default account: %w", err)
	}
	if setDefault || !hasDefault {
		if err := s.defaults.SetFanboxDefaultUserID(user.UserID); err != nil {
			return AccountSummary{}, fmt.Errorf("set fanbox default account: %w", err)
		}
	}
	isDefault, err := s.isDefault(ctx, user.UserID)
	if err != nil {
		return AccountSummary{}, err
	}
	return AccountSummary{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Default: isDefault}, nil
}

// ListAccounts 返回按显示顺序排列的 FANBOX 账号。
func (s *Accounts) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.store.ListFanbox(ctx)
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
func (s *Accounts) RemoveAccount(ctx context.Context, userID int64) error {
	defaultID, explicit, err := s.defaults.ReadFanboxDefaultUserID()
	if err != nil {
		return fmt.Errorf("read fanbox default account: %w", err)
	}
	if explicit && defaultID == userID {
		return fmt.Errorf("cannot remove fanbox account %d while it is the explicit default; use `pixiv fanbox auth use --auto` or select another account first", userID)
	}
	if err := s.store.RemoveFanbox(ctx, userID); err != nil {
		return err
	}
	return nil
}

// UseAccount 设置显式默认账号。
func (s *Accounts) UseAccount(ctx context.Context, userID int64) error {
	if _, err := s.store.GetFanbox(ctx, userID); err != nil {
		return err
	}
	return s.defaults.SetFanboxDefaultUserID(userID)
}

// UseAuto 清除显式默认，恢复首个入库账号。
func (s *Accounts) UseAuto() error {
	return s.defaults.ClearFanboxDefaultUserID()
}

// Status 返回当前默认账号身份。
func (s *Accounts) Status(ctx context.Context) (*AccountSummary, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.store.GetFanbox(ctx, userID)
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
func (s *Accounts) OpenClient(ctx context.Context, proxyOverride *string) (*fanbox.Client, error) {
	userID, err := s.selectedUserID(ctx)
	if err != nil {
		return nil, err
	}
	options, err := s.connectionOptions(proxyOverride)
	if err != nil {
		return nil, err
	}
	return s.store.OpenFanboxClient(ctx, userID, options)
}

func (s *Accounts) connectionOptions(proxyOverride *string) (fanbox.Options, error) {
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

func (s *Accounts) selectedUserID(ctx context.Context) (int64, error) {
	if userID, ok, err := s.defaults.ReadFanboxDefaultUserID(); err != nil {
		return 0, err
	} else if ok {
		if _, err := s.store.GetFanbox(ctx, userID); err != nil {
			if errors.Is(err, account.ErrFanboxNotFound) {
				return 0, fmt.Errorf("configured default fanbox account %d is missing", userID)
			}
			return 0, err
		}
		return userID, nil
	}
	accounts, err := s.store.ListFanbox(ctx)
	if err != nil {
		return 0, err
	}
	if len(accounts) == 0 {
		return 0, report.Wrap(&sdk.Error{Reason: sdk.Unauthorized, Message: "no fanbox account is authenticated"}, "")
	}
	return accounts[0].UserID, nil
}

func (s *Accounts) hasDefault() (bool, error) {
	_, ok, err := s.defaults.ReadFanboxDefaultUserID()
	return ok, err
}

func (s *Accounts) isDefault(ctx context.Context, userID int64) (bool, error) {
	configured, ok, err := s.defaults.ReadFanboxDefaultUserID()
	if err != nil {
		return false, err
	}
	if ok {
		return configured == userID, nil
	}
	// 未配置时默认是首个入库账号。
	accounts, err := s.store.ListFanbox(ctx)
	if err != nil {
		return false, err
	}
	return len(accounts) > 0 && accounts[0].UserID == userID, nil
}
