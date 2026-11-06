// Package session 将命令与 MCP 的默认账号、配置和 SDK 生命周期接到账号 owner。
//
// [Local] 按需装配 [Accounts]、[FanboxAccounts]、[Pixiv] 与 [Fanbox]；
// [Executor] 和 [Runner] 把一次命令的资源与安全重放边界连接起来。
// 账号文件、轮换持久化与账号池事务由 internal/account 负责。
package session

import (
	"context"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
)

// Request 明确指定固定账号或池调度；Options 原样传给旧 SDK。
type Request struct {
	Selection account.Selection
	Options   pixiv.Options
}

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

// Open 打开固定账号租约；池请求在此处采用调用方配置的默认账号。
func (f *Pixiv) Open(ctx context.Context, request Request) (*lifecycle.Lease[*pixiv.Client], error) {
	if ctx == nil {
		return nil, lifecycle.ErrNilContext
	}
	id := request.Selection.UserID
	switch request.Selection.Mode {
	case account.SelectionPool:
		selected, err := f.accounts.selectedUserID(ctx)
		if err != nil {
			return nil, err
		}
		id = selected
	case account.SelectionFixed:
		if id <= 0 {
			return nil, errors.New("account: fixed user id must be positive")
		}
	default:
		return nil, errors.New("account: invalid selection mode")
	}
	if err := f.gate.Acquire(ctx); err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			f.gate.Release()
		}
	}() // 租约接管前的失败同样释放轮换门。
	client, err := f.accounts.store.OpenAccountClient(ctx, id, request.Options)
	if err != nil {
		return nil, err
	}
	lease := lifecycle.NewLease(client, func() error { defer f.gate.Release(); client.CloseIdleConnections(); return nil })
	transferred = true
	return lease, nil
}

// Use 只在尚未提交且 SDK 允许安全重放时切换池账号；固定账号始终保持身份。
func (f *Pixiv) Use(ctx context.Context, request Request, callback func(context.Context, *pixiv.Client) (bool, error)) error {
	if ctx == nil {
		return lifecycle.ErrNilContext
	}
	settings, err := f.loadPoolConfig()
	if err != nil {
		return err
	}
	selection := request.Selection
	if selection.Mode == account.SelectionPool && !settings.Enabled {
		id, err := f.accounts.selectedUserID(ctx)
		if err != nil {
			return err
		}
		selection = account.Selection{Mode: account.SelectionFixed, UserID: id}
	}
	scheduler := account.Scheduler{State: f.accounts.store, Strategy: account.Strategy(settings.Strategy), Now: time.Now}
	return scheduler.Run(ctx, selection, func(ctx context.Context, id int64, attempt *lifecycle.Attempt) (err error) {
		fixed := request
		fixed.Selection = account.Selection{Mode: account.SelectionFixed, UserID: id}
		lease, err := f.Open(ctx, fixed)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, lease.Close()) }()
		committed, err := callback(ctx, lease.Value())
		if committed {
			attempt.Commit()
		}
		return err
	})
}
