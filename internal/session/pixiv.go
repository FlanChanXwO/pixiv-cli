// Package session 编排 Pixiv 与 FANBOX 账号的公开 SDK client 生命周期。
//
// 账号凭据与 pool 策略由叶模块实现，CLI/MCP 的请求模型和配置映射由组合根负责；
// 本包只接收协议无关的请求值与行为端口。
//
// 包的组成：[Pixiv] 与 [Fanbox] 每次打开都返回独立 [lifecycle.Lease]；
// [Request]、[OpenRequest]、[ProxyRequest] 与 [PoolConfig] 是请求值；
// [Executor] 与 [Runner] 把命令 owner 的读取/修改接到重放边界；
// [AccountService]、[AccountOpener]、[PoolExecutor] 与 [RotationGate] 是注入的行为端口。
package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account/pool"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

var (
	// poolExecutorNotConfigured 表示启用账号池时未注入 pool 叶执行器。
	poolExecutorNotConfigured = errors.New("account pool executor is not configured")
	// clientNotConfigured 表示 account 叶服务返回了 nil client。
	clientNotConfigured = errors.New("pixiv account service returned a nil client")
)

// Request 保存一次 Pixiv client 使用请求。
//
// UserID 为零表示使用 account 叶服务的默认账号；非零值表示显式打开该
// 本地账号。Options 原样传给 public SDK，不在 services 层解释代理或配置。
type Request struct {
	UserID  int64
	Options pixiv.Options
}

// PoolConfig 保存 Pixiv 会话需要的最小账号池配置，不携带 settings 包类型。
// Strategy 使用稳定的字符串值，由组合根负责把配置层的枚举映射到该值。
type PoolConfig struct {
	Enabled  bool
	Strategy string
}

// ConfigLoader 读取当前账号池是否启用。具体配置文件、环境变量和错误映射
// 由组合根负责。
type ConfigLoader func() (PoolConfig, error)

// AccountService 定义 account 叶服务为 client 生命周期提供的最小端口。
// *account.Service 直接满足该接口；账号选择与持久化由该叶服务负责。
type AccountService interface {
	OpenClient(context.Context, pixiv.Options) (*pixiv.Client, error)
	OpenAccountClient(context.Context, int64, pixiv.Options) (*pixiv.Client, error)
}

// PoolExecutor 定义 pool 叶服务的重放端口。执行器负责选择账号、判断 SDK
// retry advice 和维护 attempt exclusion；Pixiv 只负责每次 attempt 的资源边界。
type PoolExecutor interface {
	Run(context.Context, func(context.Context, int64, *lifecycle.Attempt) error) error
}

// PoolFactory 按当前账号池配置创建一次执行器。每次 Use 都重新调用它，
// 使 strategy 的运行时变化不会被 Pixiv 中的长期对象缓存。
type PoolFactory func(PoolConfig) (PoolExecutor, error)

// RotationGate 串行化会消耗同一 refresh token 的 SDK client 生命周期。
// *pool.Gate 直接满足该接口。
type RotationGate interface {
	Acquire(context.Context) error
	Release()
}

// Pixiv 编排 Pixiv 账号 client 的生命周期。
type Pixiv struct {
	accounts       AccountService
	poolFactory    PoolFactory
	gate           RotationGate
	loadPoolConfig ConfigLoader
}

var (
	_ AccountService = (*account.Service)(nil)
	_ PoolExecutor   = pool.Scheduler{}
	_ RotationGate   = (*pool.Gate)(nil)
)

// NewPixiv 接收组合根已装配的账号、轮换门与按调用读取的账号池策略。
func NewPixiv(accounts AccountService, gate RotationGate, loadPoolConfig ConfigLoader, pool PoolFactory) *Pixiv {
	return &Pixiv{
		accounts:       accounts,
		poolFactory:    pool,
		gate:           gate,
		loadPoolConfig: loadPoolConfig,
	}
}

// Open 打开一个显式的 SDK client snapshot，并返回负责释放 client 与 rotation
// gate 的 Lease。调用方必须关闭 Lease；重复关闭是安全的，gate 和 client 只会
// 各释放一次。
func (f *Pixiv) Open(ctx context.Context, request Request) (*lifecycle.Lease[*pixiv.Client], error) {
	if ctx == nil {
		return nil, lifecycle.ErrNilContext
	}
	if err := f.gate.Acquire(ctx); err != nil {
		return nil, err
	}
	gateReleased := false
	defer func() {
		// opener 是注入边界；若它在 Lease 接管所有权前 panic，不能让轮换门永久占用。
		if !gateReleased {
			f.gate.Release()
		}
	}()

	var (
		client *pixiv.Client
		err    error
	)
	if request.UserID == 0 {
		client, err = f.accounts.OpenClient(ctx, request.Options)
	} else {
		client, err = f.accounts.OpenAccountClient(ctx, request.UserID, request.Options)
	}
	if err != nil {
		if client != nil {
			client.CloseIdleConnections()
		}
		return nil, err
	}
	if client == nil {
		return nil, clientNotConfigured
	}

	gate := f.gate
	lease := lifecycle.NewLease(client, func() error {
		// defer 保证即使 SDK 释放实现未来返回 panic，gate 也不会永久占用。
		defer gate.Release()
		client.CloseIdleConnections()
		return nil
	})
	gateReleased = true
	return lease, nil
}

// Use 在一次账号池安全重放边界内使用 client。callback 返回 committed=true
// 时，Pixiv 将该边界提交给 pool 执行器；之后执行器不得再切换账号。每个
// attempt 都独立获得并关闭一个 Lease。
func (f *Pixiv) Use(ctx context.Context, request Request, callback func(context.Context, *pixiv.Client) (committed bool, err error)) error {
	if ctx == nil {
		return lifecycle.ErrNilContext
	}
	config, err := f.loadPoolConfig()
	if err != nil {
		return err
	}

	useAttempt := func(ctx context.Context, request Request, attempt *lifecycle.Attempt) (err error) {
		lease, err := f.Open(ctx, request)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := lease.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()

		committed, err := callback(ctx, lease.Value())
		if committed {
			attempt.Commit()
		}
		return err
	}

	// 显式账号属于调用快照，账号池不能改写身份或在错误后换号。
	if request.UserID != 0 || !config.Enabled {
		return useAttempt(ctx, request, &lifecycle.Attempt{})
	}
	pool, err := f.poolFactory(config)
	if err != nil {
		return err
	}
	if pool == nil {
		return poolExecutorNotConfigured
	}
	return pool.Run(ctx, func(ctx context.Context, userID int64, attempt *lifecycle.Attempt) error {
		pooledRequest := request
		pooledRequest.UserID = userID
		return useAttempt(ctx, pooledRequest, attempt)
	})
}
