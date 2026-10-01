// Package deps defines the narrow common wiring used by Pixiv data commands.
package deps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Request 是数据命令解析 flags 后的本地请求值；只携带显式传输覆写与账号选择。
// 它不持有 Pixiv client，也不建立第二条认证或内容调用链。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Pooled 是"在账号池安全重放边界内执行一次 public SDK 调用"的命名函数类型。
// 类型参数 R 让每个 command owner 保留自己的 Request 类型，而无需把各自的请求
// 结构统一到同一个宽类型上。
type Pooled[R any] func(
	context.Context,
	R,
	func(context.Context, *pixiv.Client) (committed bool, err error),
) error

// Read 在账号池安全重放边界内执行一次**只读**调用，并在成功后才把结果交付给
// 调用方。
//
// 契约：
//   - 回调始终以 committed=false 结束：读取尚未向调用方提交结果，因此账号池可以
//     在失败时安全换号重放。
//   - 失败时不交付部分结果，始终返回 T 的零值与该错误。
//   - Pooled 为 nil（端口未配置）时返回明确错误，而不是 panic。
//
// 注意：Go 的泛型方法不能用于实现 interface 方法，因此这是一个围绕具体类型组织
// 操作的辅助方法，不构成新的抽象层。
func (p Pooled[R]) Read[T any](ctx context.Context, request R, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	if p == nil {
		return zero, errors.New("pixiv pooled operation is not configured")
	}
	var result T
	err := p(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Data 是 Pixiv 数据命令共同需要的最小运行依赖。它只描述输入、输出、usage
// 包装和已按 command requirement 构造的 SDK 端口；不暴露根 CLI 或服务定位器。
type Data struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	// Open 为一次 execution scope 打开独立认证快照的 public SDK client。
	Open func(Request) (*pixiv.Client, error)
	// Pooled 在账号池安全重放边界内执行一次内容读取；回调接收同一 execution
	// snapshot 的 public SDK client。
	Pooled Pooled[Request]
	// JSONOut 返回 JSON 输出开关（nil override 时读取 runtime config）。
	JSONOut func(*bool) (bool, error)
}

// ProxyOptions 表示一条数据命令自己的代理覆盖参数。
type ProxyOptions struct {
	Proxy   string
	NoProxy bool
}

// CommandOptions 是同时支持 --json 和代理覆盖的命令选项。
type CommandOptions struct {
	ProxyOptions
	JSON bool
}

func (d Data) Usage(err error) error {
	if err == nil {
		return nil
	}
	if d.UsageError == nil {
		panic("pixiv command usage error wrapper is not configured")
	}
	return d.UsageError(err)
}

// ExactArgs、MinArgs 与 MaxArgs 返回普通错误：位置参数数量错误按命令失败报告
// 为 exit code 1；usage exit code 2 只保留给 unknown option 与显式输入契约违规。
func (d Data) ExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) MinArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) MaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) BindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       d.Input,
		UsageError:   d.Usage,
	})
}

func (d Data) BindTextValueWhen(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       d.Input,
		UsageError:   d.Usage,
		Enabled:      enabled,
	})
}

func (d Data) BindTextOrRecord(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextOrRecord,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       d.Input,
		UsageError:   d.Usage,
	})
}

func (d Data) BindNoInput(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:      pipeline.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     d.Input,
		UsageError: d.Usage,
	})
}

// ActionInputArgs applies the shared positional contract for a TextOrRecord
// action while leaving allowed record types and mutation semantics to its
// command owner.
func (d Data) ActionInputArgs(usage string) cobra.PositionalArgs {
	return pipeline.ActionInputArgs(d.Input, usage, d.Usage)
}

// ConsumeActionRecords keeps canonical Record parsing and line diagnostics in
// pipeline. The caller owns its allowed entity types and invoked mutation.
func (d Data) ConsumeActionRecords(cmd *cobra.Command, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error) error {
	return pipeline.ConsumeActionRecords(cmd.Context(), pipeline.Reader(cmd, d.Input), d.ErrorOutput, operation, onError, allowedTypes, invoke, d.Usage)
}

func (d Data) BindCommonFlags(cmd *cobra.Command, opts *CommandOptions) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	d.BindProxyFlags(cmd, &opts.ProxyOptions)
}

func (d Data) BindActionFlags(cmd *cobra.Command, opts *ProxyOptions) {
	d.BindProxyFlags(cmd, opts)
}

func (d Data) BindProxyFlags(cmd *cobra.Command, opts *ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

// Request 解析命令本地的传输覆写，不创建 client。公开 SDK client 由 composition
// root 按 command requirement 构造，并通过 Open/Pooled 端口注入。
func (d Data) Request(cmd *cobra.Command, opts CommandOptions) (Request, error) {
	request := Request{}
	proxy, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return Request{}, err
	}
	request.HTTPSProxyOverride = proxy
	return request, nil
}

// JSONOverride 只在 --json flag 显式出现时返回覆盖值；nil 表示沿用 runtime
// config 的 JSON 输出开关。
func (d Data) JSONOverride(cmd *cobra.Command, opts CommandOptions) *bool {
	if !cmd.Flags().Changed("json") {
		return nil
	}
	value := opts.JSON
	return &value
}

func proxyOverrideFromFlags(cmd *cobra.Command, opts ProxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && opts.NoProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &opts.Proxy, nil
	}
	return nil, nil
}

// Executor 把账号池执行端口适配为 search/pagination workflow 的通用
// 可重入执行函数。
func (d Data) Executor(request Request) func(context.Context, func(context.Context, *pixiv.Client) (bool, error)) error {
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		if d.Pooled == nil {
			return errors.New("pixiv pooled operation is not configured")
		}
		return d.Pooled(ctx, request, attempt)
	}
}

// Client 打开一次独立认证快照的 public SDK client。
func (d Data) Client(request Request) (*pixiv.Client, error) {
	if d.Open == nil {
		return nil, errors.New("pixiv client factory is not configured")
	}
	return d.Open(request)
}

// Write 在账号池安全重放边界内执行一次 mutation。
//
// 契约：
//   - 一旦调用 public SDK 就标记 committed=true：写操作无法从所有网络错误可靠
//     判断服务端是否已接受请求，因此在未知提交状态下禁止账号池换号重放。
//   - 与 Read 的 committed=false 语义**刻意分开**，不合并成带布尔模式的单一函数：
//     两者对"失败后能否重放"的回答相反，合并会把这一差异隐藏在参数里。
//   - 端口未配置（nil）返回 "pixiv pooled operation is not configured"，不 panic。
func Write(d Data, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) error) error {
	return d.Pooled(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

func (d Data) WriteJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(d.Output, out.String()+"\n")
	return err
}

func (d Data) ShouldAutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	if ndjson || jsonOut || cmd.Flags().Changed("json") {
		return ndjson
	}
	file, ok := d.Output.(interface{ Fd() uintptr })
	return ok && !term.IsTerminal(int(file.Fd()))
}

// CurrentUserID 返回当前 execution snapshot 的身份 UID。CLI 只通过本地账号
// 打开 client，因此 client 一定携带选中账号；取不到时按错误处理，不静默回退。
func CurrentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, errors.New("cannot determine current user id")
}
