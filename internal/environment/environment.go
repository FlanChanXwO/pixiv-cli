// Package environment 提供 CLI 各 command owner 共用的执行环境：输入输出流、stdin 输入契约、
// flag 绑定、usage 错误标记与命令生命周期声明。
//
// 本包只供 internal/cli 使用；不含 Pixiv 或 FANBOX 的业务语义。
//
// 包的组成：[Env] 承载输入输出与 flag 绑定，[State] 保存一条命令解析后的 stdin 输入，
// [Execution] 声明启动、更新和 MCP 等执行要求。
package environment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Env 汇集所有 command owner 共用的输入输出流和 flag 绑定；root 构造一份，
// 各命令包通过构造函数参数取得。
type Env struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	// WrapUsage 给参数或输入契约错误打上 CLI usage 退出码的标记。
	WrapUsage func(error) error
	// ResolveJSON 返回本次执行生效的 JSON 设置；override 为 nil 时读取运行时配置。
	ResolveJSON func(*bool) (bool, error)
	// EnablePipelineSignal 在第一次 NDJSON 写出前安装执行边界的 broken-pipe 策略，
	// 重复调用不产生额外副作用。
	EnablePipelineSignal func()
}

// ProxyOptions 保存一条命令的 --proxy/--no-proxy 取值。
type ProxyOptions struct {
	Proxy   string
	NoProxy bool
}

// Options 保存数据命令共用的 --json 与代理 flag 取值。
type Options struct {
	ProxyOptions
	JSON bool
}

// ExactArgs 校验位置参数个数恰好为 count。个数不符按普通命令失败处理；
// usage 退出码只留给未知选项和显式输入契约违规。
func (e Env) ExactArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length == count })
}

// MinArgs 校验位置参数至少 count 个。
func (e Env) MinArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length >= count })
}

// MaxArgs 校验位置参数至多 count 个。
func (e Env) MaxArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length <= count })
}

func requireArgs(usage string, valid func(int) bool) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if !valid(len(args)) {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

// PrintJSON 向 stdout 写一个两空格缩进的 JSON 文档并追加换行。
func (e Env) PrintJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(e.Out, out.String()+"\n")
	return err
}

// JSONOverride 返回显式 --json 的取值；flag 未出现时返回 nil，让调用方沿用配置。
func (e Env) JSONOverride(cmd *cobra.Command, opts Options) *bool {
	if !cmd.Flags().Changed("json") {
		return nil
	}
	value := opts.JSON
	return &value
}

// ProxyOverride 解析共用的代理 flag；两者都未出现时返回 nil，表示沿用配置。
func ProxyOverride(cmd *cobra.Command, opts ProxyOptions) (*string, error) {
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

// BindCommonFlags 注册 --json（短选项 -j）和代理覆盖 flag。
func (e Env) BindCommonFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	e.BindProxyFlags(cmd, &opts.ProxyOptions)
}

// BindProxyFlags 只注册代理覆盖 flag，供没有 --json 的命令使用。
func (e Env) BindProxyFlags(cmd *cobra.Command, opts *ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

// BindTextValue 把 stdin 声明为单个文本值，并返回本次命令的输入状态。
func (e Env) BindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) *State {
	return BindInput(cmd, InputSpec{
		Codec:        TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.WrapUsage,
	})
}

// BindTextValueWhen 与 BindTextValue 相同，但 enabled 返回 false 时不读取 stdin。
func (e Env) BindTextValueWhen(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) *State {
	return BindInput(cmd, InputSpec{
		Codec:        TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.WrapUsage,
		Enabled:      enabled,
	})
}

// BindTextOrRecord 让 stdin 在单个文本值和 NDJSON 记录流之间自动判别。
func (e Env) BindTextOrRecord(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) *State {
	return BindInput(cmd, InputSpec{
		Codec:        TextOrRecord,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.WrapUsage,
	})
}

// BindNoInput 声明命令不读取 stdin，保留 stdin 给 MCP 等协议使用。
func (e Env) BindNoInput(cmd *cobra.Command) *State {
	return BindInput(cmd, InputSpec{
		Codec:      NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     e.In,
		UsageError: e.WrapUsage,
	})
}

// ActionInputArgs 给出 TextOrRecord 动作命令共用的位置参数校验。
func (e Env) ActionInputArgs(usage string) cobra.PositionalArgs {
	return ActionInputArgs(e.In, usage, e.WrapUsage)
}

// ConsumeActionRecords 把标准记录解析和逐行诊断交给 input 包；允许的实体类型、
// 已解析的输入状态和实际动作由调用方负责。
func (e Env) ConsumeActionRecords(cmd *cobra.Command, state *State, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error) error {
	return ConsumeActionRecords(cmd.Context(), state.Reader(e.In), e.Err, operation, onError, allowedTypes, invoke, e.WrapUsage)
}

// AutoNDJSON 决定列表命令是否输出记录流：显式 --ndjson 直接选择记录流；
// 否则显式 --json 或配置的 JSON 模式优先，两者都没有时只要 stdout 不是终端就输出记录流。
// 结果记录在命令上，执行边界据此把提前关闭的管道判为正常结束。
func (e Env) AutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	effective := ndjson
	if !ndjson && !jsonOut && !cmd.Flags().Changed("json") {
		file, ok := e.Out.(interface{ Fd() uintptr })
		effective = ok && !term.IsTerminal(int(file.Fd()))
	}
	SetOutputNDJSON(cmd, effective)
	if effective && e.EnablePipelineSignal != nil {
		e.EnablePipelineSignal()
	}
	return effective
}

// OutputIsTTY 报告 stdout 是否连接到终端。
func (e Env) OutputIsTTY() bool {
	file, ok := e.Out.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

// New 构造所有 command owner 共用的执行环境；WrapUsage 给参数错误打上 usage 标记，
// ResolveJSON 在 override 为 nil 时读取运行时配置。
func New(in io.Reader, out, errOut io.Writer, wrapUsage func(error) error, enablePipelineSignal func(), resolveJSON func(*bool) (bool, error)) Env {
	return Env{In: in, Out: out, Err: errOut, WrapUsage: wrapUsage, EnablePipelineSignal: enablePipelineSignal, ResolveJSON: resolveJSON}
}
