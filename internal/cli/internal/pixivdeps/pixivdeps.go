// Package deps defines the narrow common wiring used by Pixiv data commands.
package deps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Request 是数据命令解析 flags 后的本地请求值；只携带显式传输覆写与账号选择。
// 它不持有 Pixiv client，也不建立第二条认证或内容调用链。
type Request struct {
	UserID                  int64
	HTTPSProxyOverride      *string
	RequestIntervalOverride *time.Duration
}

// Data 是 Pixiv 数据命令共同需要的最小运行依赖。它只描述输入、输出、usage
// 包装和已按 command requirement 构造的 SDK 端口；不暴露根 CLI 或服务定位器。
type Data struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	// Open 为一次 operation 打开独立认证快照的 public SDK client。
	Open func(Request) (*pixiv.Client, error)
	// Pooled 在账号池安全重放边界内执行一次内容读取；回调接收同一 operation
	// snapshot 的 public SDK client。
	Pooled func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error
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

// Request resolves command-local transport overrides without creating a
// client. The composition root has already constructed the exact Pixiv SDK
// resource graph selected by the command requirement.
func (d Data) Request(cmd *cobra.Command, opts CommandOptions) (Request, error) {
	request := Request{}
	proxy, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return Request{}, err
	}
	request.HTTPSProxyOverride = proxy
	if flag := cmd.Flags().Lookup("sleep-request"); flag != nil && flag.Changed {
		value, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			return Request{}, fmt.Errorf("invalid --sleep-request: %w", err)
		}
		if value < 0 {
			return Request{}, errors.New("--sleep-request must not be negative")
		}
		request.RequestIntervalOverride = &value
	}
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

// PooledOperation 把账号池重放端口适配为 search/pagination workflow 的通用
// operation 形状。
func (d Data) PooledOperation(request Request) func(context.Context, func(context.Context, *pixiv.Client) (bool, error)) error {
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

// Read 在账号池安全重放边界内执行一次只读用例。读取尚未向调用方提交结果，
// 因此回调始终以 committed=false 结束；调用方只在成功后拿到结果。
func Read[T any](d Data, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	var result T
	err := d.Pooled(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Write 在账号池安全重放边界内执行一次 mutation。public SDK 一旦被调用，无法
// 从所有网络错误可靠判断服务端是否已接受请求，因此即使返回错误也标记
// committed=true，禁止账号池在未知提交状态下换号重放。
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

// CurrentUserID 返回当前 operation snapshot 的身份 UID。CLI 只通过本地账号
// 打开 client，因此 client 一定携带选中账号；取不到时按错误处理，不静默回退。
func CurrentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, errors.New("cannot determine current user id")
}
