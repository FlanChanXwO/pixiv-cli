// Package fanbox 提供 FANBOX 的业务 Facade。
//
// 账号选择与连接选项由 account leaf 负责；本包负责把默认账号 client
// 组合成明确的资源生命周期，避免调用方直接管理 SDK client 的释放。
package fanbox

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

var (
	// ErrAccountServiceNotConfigured 表示没有注入 FANBOX 账号叶服务。
	ErrAccountServiceNotConfigured = errors.New("fanbox account service is not configured")
	// ErrNilClient 表示账号叶服务成功返回了 nil client。
	ErrNilClient = errors.New("fanbox account service returned a nil client")
)

// AccountOpener 是 account leaf 为根 Facade 提供的最窄打开端口。
//
// account.Service 实现此接口，并负责默认账号选择以及通过其
// LoadOptionsFunc 加载 public sdk/fanbox.Options。ProxyOverride 只作用于
// 当前调用，不改变持久化配置或其他连接策略。
type AccountOpener interface {
	OpenClientWithProxy(context.Context, *string) (*fanboxsdk.Client, error)
}

// OpenRequest 描述一次默认 FANBOX 账号 client 打开请求。
type OpenRequest struct {
	// ProxyOverride 为 nil 时使用 account leaf 注入的 sdk/fanbox.Options；
	// 非 nil 时仅覆盖本次请求的原生 HTTP proxy，空字符串表示禁用 proxy。
	ProxyOverride *string
}

// Facade 是 FANBOX 业务编排层。
//
// Facade 不持有全局 client；每次 Open 都创建一个独立 Lease，调用方可以
// 明确决定生命周期。Use 用于无法把 Lease 继续向上传递的 callback 场景。
type Facade struct {
	accounts    AccountOpener
	closeClient func(*fanboxsdk.Client) error
}

// NewFacade 构造 FANBOX 根业务 Facade。
func NewFacade(accounts AccountOpener) *Facade {
	return NewFacadeWithCloseClient(accounts, nil)
}

// NewFacadeWithCloseClient 构造可注入 client 释放策略的 FANBOX Facade。
// closeClient 为 nil 时使用 public SDK 的 CloseIdleConnections。
func NewFacadeWithCloseClient(accounts AccountOpener, closeClient func(*fanboxsdk.Client) error) *Facade {
	if closeClient == nil {
		closeClient = func(client *fanboxsdk.Client) error {
			if client != nil {
				client.CloseIdleConnections()
			}
			return nil
		}
	}
	return &Facade{
		accounts:    accounts,
		closeClient: closeClient,
	}
}

// Open 打开当前默认账号并返回显式 Lease。
func (f *Facade) Open(ctx context.Context, request OpenRequest) (*lifecycle.Lease[*fanboxsdk.Client], error) {
	if ctx == nil {
		return nil, lifecycle.ErrNilContext
	}
	if f == nil || f.accounts == nil {
		return nil, ErrAccountServiceNotConfigured
	}
	closeClient := f.closeClient
	if closeClient == nil {
		closeClient = func(client *fanboxsdk.Client) error {
			if client != nil {
				client.CloseIdleConnections()
			}
			return nil
		}
	}
	client, err := f.accounts.OpenClientWithProxy(ctx, request.ProxyOverride)
	if err != nil {
		if client != nil {
			return nil, errors.Join(err, closeClient(client))
		}
		return nil, err
	}
	if client == nil {
		return nil, ErrNilClient
	}
	return lifecycle.NewLease(client, func() error { return closeClient(client) }), nil
}

// Use 在一个 callback 内使用默认账号 client，并在 callback 返回后关闭它。
// Attempt 参数保留 shared/lifecycle 的统一提交边界；FANBOX 当前没有
// Pixiv 账号池 replay 策略，但调用方仍可用它标记外部副作用已发生。
func (f *Facade) Use(
	ctx context.Context,
	request OpenRequest,
	use func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error,
) error {
	return lifecycle.Run(ctx,
		func(ctx context.Context) (*lifecycle.Lease[*fanboxsdk.Client], error) {
			return f.Open(ctx, request)
		},
		use,
	)
}
