// Package pixivcmd 提供 Pixiv 数据命令共用的请求值、SDK 重放边界和分页执行。
//
// 包的组成：[Executor] 在账号池重放边界内执行一次请求，[Runner] 负责分页与输出提交。
// [ArtworkList]、[NovelList] 和 [UserList] 描述三类列表读取，行式输出在 presentation.go。
package pixivcmd

import (
	"context"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Request 保存一次 Pixiv 数据执行的账号和传输选择；它不携带 client，也不会另建认证路径。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Executor 在账号池的重放边界内执行一次读取或修改；回调报告输出或修改是否已经提交。
type Executor func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error

// Runner 用 command owner 选定的端口执行 Pixiv SDK 工作；账号选择、重试策略和展示都归调用方。
type Runner struct {
	out      io.Writer
	executor Executor
}

// New 绑定一次命令执行的输出 writer 与重放端口。
func New(out io.Writer, executor Executor) Runner {
	return Runner{out: out, executor: executor}
}

// Execute 把一次请求接到共享遍历契约上；逻辑分页、收藏策略和 cursor 处理仍由 command owner 决定。
func (r Runner) Execute(request Request) pagination.Execute[*pixiv.Client] {
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return r.executor(ctx, request, attempt)
	}
}

// Read 执行一次读取，只在边界整体成功时返回结果，不会返回半截数据。
func Read[T any](r Runner, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
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
func (r Runner) Write(ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) error) error {
	return r.executor(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// BuildRequest 从共用代理 flag 构造一次执行请求。
func BuildRequest(cmd *cobra.Command, opts command.Options) (Request, error) {
	proxy, err := command.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return Request{}, err
	}
	return Request{HTTPSProxyOverride: proxy}, nil
}

// CurrentUserID 返回已打开本地账号的 ID；取不到就报错，CLI 不会回退到未知用户。
func CurrentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, errors.New("cannot determine current user id")
}
