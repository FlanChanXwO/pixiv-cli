// Package session 将命令与 MCP 的默认账号、配置和 SDK 生命周期接到账号 owner。
//
// [Local] 按需装配 [Accounts]、[FanboxAccounts]、[Pixiv] 与 [Fanbox]；
// [SDKExecutor] 把一次命令的资源与安全重放边界连接起来。
// 账号文件、轮换持久化与账号池事务由 internal/account 负责。
package session

import (
	"context"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
)

// PoolConfig 保存调用方读取的池设置；本包负责把策略值交给账号 owner。
type PoolConfig struct {
	Enabled  bool
	Strategy string
}

// ConfigLoader 按调用读取当前账号池设置。
type ConfigLoader func() (PoolConfig, error)

// Pixiv 为一次 client 使用取得和释放轮换门；账号池负责安全重放。
type Pixiv struct {
	accounts       *Accounts
	gate           *account.Gate
	loadPoolConfig ConfigLoader
}

// NewPixiv 接收已装配的账号与轮换门，配置每次操作读取。
func NewPixiv(accounts *Accounts, gate *account.Gate, loadPoolConfig ConfigLoader) *Pixiv {
	return &Pixiv{accounts: accounts, gate: gate, loadPoolConfig: loadPoolConfig}
}

func (f *Pixiv) useAccount(ctx context.Context, selection account.Selection, attempt func(context.Context, int64, *lifecycle.Attempt) error) error {
	settings, err := f.loadPoolConfig()
	if err != nil {
		return err
	}
	if selection.Mode == account.SelectionPool && !settings.Enabled {
		id, err := f.accounts.selectedUserID(ctx)
		if err != nil {
			return err
		}
		selection = account.Selection{Mode: account.SelectionFixed, UserID: id}
	}
	scheduler := account.Scheduler{State: f.accounts.store, Strategy: account.Strategy(settings.Strategy), Now: time.Now}
	return scheduler.Run(ctx, selection, attempt)
}
