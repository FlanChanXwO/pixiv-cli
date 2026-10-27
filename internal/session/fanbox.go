package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// nilClient 表示账号叶服务成功返回了 nil client。
var nilClient = errors.New("fanbox account service returned a nil client")

// AccountOpener 定义 account leaf 为 Fanbox 提供的最窄打开端口。
//
// account.Service 实现此接口，并负责默认账号选择以及通过其
// LoadOptionsFunc 加载 public sdk/fanbox.Options。ProxyOverride 只作用于
// 当前调用，不改变持久化配置或其他连接策略。
type AccountOpener interface {
	OpenClient(context.Context, *string) (*fanbox.Client, error)
}

// OpenRequest 描述一次默认 FANBOX 账号 client 打开请求。
type OpenRequest struct {
	// ProxyOverride 为 nil 时使用 account leaf 注入的 sdk/fanbox.Options；
	// 非 nil 时仅覆盖本次请求的原生 HTTP proxy，空字符串表示禁用 proxy。
	ProxyOverride *string
}

// Fanbox 编排 FANBOX 账号 client 的生命周期。
//
// Fanbox 不持有全局 client；每次 Open 都创建一个独立 Lease，调用方可以
// 明确决定生命周期。Use 用于无法把 Lease 继续向上传递的 callback 场景。
type Fanbox struct {
	accounts AccountOpener
}

// NewFanbox 构造 FANBOX 根业务编排对象。
func NewFanbox(accounts AccountOpener) *Fanbox {
	return &Fanbox{accounts: accounts}
}

// Open 打开当前默认账号并返回显式 Lease。
func (f *Fanbox) Open(ctx context.Context, request OpenRequest) (*lifecycle.Lease[*fanbox.Client], error) {
	if ctx == nil {
		return nil, lifecycle.ErrNilContext
	}
	client, err := f.accounts.OpenClient(ctx, request.ProxyOverride)
	if err != nil {
		if client != nil {
			client.CloseIdleConnections()
		}
		return nil, err
	}
	if client == nil {
		return nil, nilClient
	}
	return lifecycle.NewLease(client, func() error {
		client.CloseIdleConnections()
		return nil
	}), nil
}

// Use 在一个 callback 内使用默认账号 client，并在 callback 返回后关闭它。
// Attempt 参数保留 internal/lifecycle 的统一提交边界；FANBOX 当前没有
// Pixiv 账号池 replay 策略，但调用方仍可用它标记外部副作用已发生。
func (f *Fanbox) Use(
	ctx context.Context,
	request OpenRequest,
	use func(context.Context, *fanbox.Client, *lifecycle.Attempt) error,
) error {
	return lifecycle.Run(ctx,
		func(ctx context.Context) (*lifecycle.Lease[*fanbox.Client], error) {
			return f.Open(ctx, request)
		},
		use,
	)
}
