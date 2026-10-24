// Package session 编排 Pixiv 与 FANBOX 账号的 public SDK client 生命周期。
//
// 账号凭据与 pool 策略由叶模块实现，具体的 CLI/MCP 请求模型和配置映射由
// 组合根负责。本包只接收协议无关的请求值与行为端口。
package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

var (
	// pixivAccountServiceNotConfigured 表示未注入 Pixiv account 叶服务。
	pixivAccountServiceNotConfigured = errors.New("pixiv account service is not configured")
	// rotationGateNotConfigured 表示未注入用于串行 OAuth rotation 的 gate。
	rotationGateNotConfigured = errors.New("pixiv rotation gate is not configured")
	// poolRuntimeLoaderNotConfigured 表示未注入账号池启用状态读取器。
	poolRuntimeLoaderNotConfigured = errors.New("account pool runtime loader is not configured")
	// poolExecutorNotConfigured 表示启用账号池时未注入 pool 叶执行器。
	poolExecutorNotConfigured = errors.New("account pool executor is not configured")
	// useNotConfigured 表示调用方没有提供 client callback。
	useNotConfigured = errors.New("pixiv client callback is not configured")
	// clientNotConfigured 表示 account 叶服务返回了 nil client。
	clientNotConfigured = errors.New("pixiv account service returned a nil client")
)

// Request 是一次 Pixiv client 使用请求。
//
// UserID 为零表示使用 account 叶服务的默认账号；非零值表示显式打开该
// 本地账号。Options 原样传给 public SDK，不在 services 层解释代理或配置。
type Request struct {
	UserID  int64
	Options pixiv.Options
}

// PoolConfig 是 Pixiv 会话需要的最小账号池配置，不携带 settings 包类型。
// Strategy 使用稳定的字符串值，由组合根负责把配置层的枚举映射到该值。
type PoolConfig struct {
	Enabled  bool
	Strategy string
}

// ConfigLoader 读取当前账号池是否启用。具体配置文件、环境变量和错误映射
// 由组合根负责。
type ConfigLoader func() (PoolConfig, error)

// AccountService 是 account 叶服务为 client 生命周期提供的最小端口。
// *account.Service 直接满足该接口；接口也让 Pixiv 会话测试可以使用无网络 fake。
type AccountService interface {
	OpenClient(context.Context, pixiv.Options) (*pixiv.Client, error)
	OpenAccountClient(context.Context, int64, pixiv.Options) (*pixiv.Client, error)
}

// PoolExecutor 是 pool 叶服务的重放端口。执行器负责选择账号、判断 SDK
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

// Pixiv 是 Pixiv 的业务编排层。
type Pixiv struct {
	accounts       AccountService
	poolFactory    PoolFactory
	gate           RotationGate
	loadPoolConfig ConfigLoader
	closeClient    func(*pixiv.Client) error
}

var (
	_ AccountService = (*pixivaccount.Service)(nil)
	_ PoolExecutor   = pool.Scheduler{}
	_ RotationGate   = (*pool.Gate)(nil)
)

// NewPixiv 创建 Pixiv 根编排对象。依赖缺失不会在构造时 panic，而是在对应的
// Open/Use 路径返回稳定配置错误，便于 CLI/MCP 组合根保留自己的启动语义。
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
	if f == nil || f.accounts == nil {
		return nil, pixivAccountServiceNotConfigured
	}
	if f.gate == nil {
		return nil, rotationGateNotConfigured
	}
	if err := f.gate.Acquire(ctx); err != nil {
		return nil, err
	}
	closeClient := f.closeClient
	if closeClient == nil {
		closeClient = func(client *pixiv.Client) error {
			if client != nil {
				client.CloseIdleConnections()
			}
			return nil
		}
	}
	gateReleased := false
	defer func() {
		// An opener is an injected boundary. If it panics before the Lease takes
		// ownership, do not leave the rotation gate permanently occupied.
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
			return nil, errors.Join(err, closeClient(client))
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
		return closeClient(client)
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
	if callback == nil {
		return useNotConfigured
	}
	if f == nil || f.loadPoolConfig == nil {
		return poolRuntimeLoaderNotConfigured
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
	if f.poolFactory == nil {
		return poolExecutorNotConfigured
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
