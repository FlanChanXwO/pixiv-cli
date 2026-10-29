package session

import (
	"context"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ProxyRequest 保存一次 Pixiv 数据执行的账号与代理覆盖。代理到 SDK Options 的翻译
// 由组合根负责；本包不读取配置或 flag。
type ProxyRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Executor 在账号池的重放边界内执行一次读取或修改；回调报告输出或修改是否已经提交。
type Executor func(context.Context, ProxyRequest, func(context.Context, *pixiv.Client) (bool, error)) error

// Runner 用命令 owner 选定的端口执行 Pixiv SDK 工作；账号选择、重试策略和展示都归调用方。
type Runner struct {
	Out      io.Writer
	executor Executor
}

// NewRunner 绑定一次命令执行的输出 writer 与重放端口。
func NewRunner(out io.Writer, executor Executor) Runner {
	return Runner{Out: out, executor: executor}
}

// Execute 把一次请求接到共享遍历契约上；逻辑分页、收藏策略和 cursor 处理仍由 command owner 决定。
func (r Runner) Execute(request ProxyRequest) pagination.Execute[*pixiv.Client] {
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return r.executor(ctx, request, attempt)
	}
}

// Read 执行一次读取，只在边界整体成功时返回结果，不会返回半截数据。
func Read[T any](r Runner, ctx context.Context, request ProxyRequest, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero, result T
	err := r.executor(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Write 执行一次修改。从进入修改回调起就按已提交处理，即使 invoke 在发出请求前返回错误，
// 因为错误无法证明请求没有送达，不能让账号池把它当作未提交操作重放。
func (r Runner) Write(ctx context.Context, request ProxyRequest, invoke func(context.Context, *pixiv.Client) error) error {
	return r.executor(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// CurrentUserID 返回已打开本地账号的 ID；取不到就报错，CLI 不会回退到未知用户。
func CurrentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, errors.New("cannot determine current user id")
}

// Open 打开延迟构造的 FANBOX 会话的默认账号租约；session 延迟解析，构造命令树时不打开账号库。
func Open(ctx context.Context, open func() (*Fanbox, error), request OpenRequest) (*lifecycle.Lease[*fanbox.Client], error) {
	service, err := open()
	if err != nil {
		return nil, err
	}
	return service.Open(ctx, request)
}

// Use 让租约只覆盖一次命令回调；回调和关闭的错误都会保留并合并返回。
func Use(ctx context.Context, open func() (*Fanbox, error), request OpenRequest, use func(*fanbox.Client) error) error {
	lease, err := Open(ctx, open, request)
	if err != nil {
		return err
	}
	useErr := use(lease.Value())
	return errors.Join(useErr, lease.Close())
}
