package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"syscall"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

type app struct {
	in                  io.Reader
	out                 io.Writer
	errOut              io.Writer
	pipelineSignal      *brokenPipeSignalState
	mcpBrokenPipeSignal *brokenPipeSignalState
}

// brokenPipeSignalState 临时安装平台的 broken-pipe 信号策略，使 stdout 写失败能以
// EPIPE 返回 Go 调用链。NDJSON 与 MCP 使用独立状态，且只有前者可把 EPIPE 归为成功。
type brokenPipeSignalState struct {
	enable func() func()
	stop   func()
}

type commandOptions struct {
	proxyOptions
	jsonOut bool
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

type clientConfig struct {
	config.RuntimeConfig
}

var (
	runMCPServer                        = runMCP
	newCLIServices                      = bootstrap.NewServices
	cleanupPendingWindowsUpdate         = update.CleanupPendingWindowsUpdate
	automaticPersistentHandlerSupported = loginhelper.AutomaticPersistentHandlerSupported
)

func Run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return runContext(context.Background(), args, in, out, errOut, nil, nil)
}

// RunContext 让嵌入式调用方把取消信号传到每一条网络数据命令。
func RunContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return runContext(ctx, args, in, out, errOut, nil, nil)
}

// RunContextWithPipelineSignal 仅由二进制入口传入 SIGPIPE 控制器。控制器会在
// filter 或已解析的 --ndjson 查询命令运行期间启用，并在命令退出时恢复。
func RunContextWithPipelineSignal(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, enablePipelineSignal func() func()) int {
	return RunContextWithBrokenPipeSignals(ctx, args, in, out, errOut, enablePipelineSignal, nil)
}

// RunContextWithBrokenPipeSignals 仅供二进制入口传入平台的 SIGPIPE 控制器。普通
// NDJSON 输出和 MCP stdio 必须分别传入控制器：前者的 EPIPE 是下游正常停止，后者
// 则是 JSON-RPC transport 错误。
func RunContextWithBrokenPipeSignals(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, enablePipelineSignal, enableMCPBrokenPipeSignal func() func()) int {
	var pipelineSignal, mcpBrokenPipeSignal *brokenPipeSignalState
	if enablePipelineSignal != nil {
		pipelineSignal = &brokenPipeSignalState{enable: enablePipelineSignal}
	}
	if enableMCPBrokenPipeSignal != nil {
		mcpBrokenPipeSignal = &brokenPipeSignalState{enable: enableMCPBrokenPipeSignal}
	}
	return runContext(ctx, args, in, out, errOut, pipelineSignal, mcpBrokenPipeSignal)
}

func runContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, pipelineSignal, mcpBrokenPipeSignal *brokenPipeSignalState) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	if !isAuthExportInvocation(args) && !isInternalLoginCallbackInvocation(args) && !isFilterInvocation(args) {
		if err := cleanupPendingWindowsUpdate(); err != nil {
			fmt.Fprintf(errOut, "clean pending update: %v\n", err)
			return 1
		}
	}
	if shouldAutomaticallyEnsurePersistentHandler(args) {
		if err := ensureURLSchemeRelay(ctx); err != nil {
			// 这项桌面集成不能阻断原命令，也不能把系统/本机路径写入 stderr。
			fmt.Fprintln(errOut, "warning: persistent pixiv:// callback handler was not initialized")
		}
	}
	a := app{in: in, out: out, errOut: errOut, pipelineSignal: pipelineSignal, mcpBrokenPipeSignal: mcpBrokenPipeSignal}
	if pipelineSignal != nil {
		defer func() {
			if pipelineSignal.stop != nil {
				pipelineSignal.stop()
			}
		}()
	}
	if mcpBrokenPipeSignal != nil {
		defer func() {
			if mcpBrokenPipeSignal.stop != nil {
				mcpBrokenPipeSignal.stop()
			}
		}()
	}
	cmd := a.newRootCommand()
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	cmd.SetContext(ctx)
	target := cmd
	if found, _, findErr := cmd.Find(args[1:]); findErr == nil && found != nil {
		target = found
	}
	err := cmd.Execute()
	return a.exitWithNDJSONScope(err, commandWritesNDJSON(target))
}

func shouldAutomaticallyEnsurePersistentHandler(args []string) bool {
	return automaticPersistentHandlerSupported() && !isAuthExportInvocation(args) && !isInternalLoginCallbackInvocation(args) && !isFilterInvocation(args)
}

func isAuthExportInvocation(args []string) bool {
	if len(args) < 3 {
		return false
	}
	state := rootBooleanFlagState{}
	index := scanRootBooleanFlags(args, 1, &state)
	if index >= len(args) || args[index] != "auth" {
		return false
	}
	index = scanRootBooleanFlags(args, index+1, &state)
	if index >= len(args) || args[index] != "export" || state.invalid {
		return false
	}
	return !state.help && !state.version
}

// isFilterInvocation 让 filter 保持纯本地 stdin→stdout 变换，不初始化默认配置文件。
func isFilterInvocation(args []string) bool {
	if len(args) < 2 {
		return false
	}
	state := rootBooleanFlagState{}
	index := scanRootBooleanFlags(args, 1, &state)
	return !state.invalid && !state.help && !state.version && index < len(args) && args[index] == "filter"
}

// isInternalLoginCallbackInvocation 识别由操作系统协议关联或安装器启动的隐藏 helper。
// callback 可能携带一次性 authorization code；两者都不能触发更新清理或配置加载。
func isInternalLoginCallbackInvocation(args []string) bool {
	// `_callback` 可由未来不经 argv 传递 callback 的桌面 helper 调用，
	// `_install-handler` 本身也没有位置参数；两者最短都是三个 argv 元素。
	if len(args) < 3 {
		return false
	}
	state := rootBooleanFlagState{}
	index := scanRootBooleanFlags(args, 1, &state)
	if index >= len(args) || args[index] != "auth" {
		return false
	}
	index = scanRootBooleanFlags(args, index+1, &state)
	return index < len(args) && (args[index] == internalURLCallbackCommand || args[index] == internalURLHandlerInstallCommand) && !state.invalid && !state.help && !state.version
}

// rootBooleanFlagState 按 pflag 的重复 flag 语义保存每个 logical flag 的最后值。
// -h 与 --help 属于同一 logical flag；非法值由 Cobra 报错，不能被后值覆盖。
type rootBooleanFlagState struct {
	help    bool
	version bool
	invalid bool
}

type rootBooleanFlag uint8

const (
	rootBooleanFlagHelp rootBooleanFlag = iota + 1
	rootBooleanFlagVersion
)

func scanRootBooleanFlags(args []string, index int, state *rootBooleanFlagState) int {
	for index < len(args) {
		flag, value, recognized, err := rootBooleanFlagValue(args[index])
		if !recognized {
			return index
		}
		if err != nil {
			state.invalid = true
		} else {
			switch flag {
			case rootBooleanFlagHelp:
				state.help = value
			case rootBooleanFlagVersion:
				state.version = value
			}
		}
		index++
	}
	return index
}

func rootBooleanFlagValue(argument string) (flag rootBooleanFlag, value bool, recognized bool, err error) {
	name, rawValue, assigned := strings.Cut(argument, "=")
	switch name {
	case "--help", "-h":
		flag = rootBooleanFlagHelp
	case "--version":
		flag = rootBooleanFlagVersion
	default:
		return 0, false, false, nil
	}
	if !assigned {
		return flag, true, true, nil
	}
	value, err = strconv.ParseBool(rawValue)
	return flag, value, true, err
}

func (a app) exit(err error) int {
	return a.exitWithNDJSONScope(err, false)
}

func (a app) exitWithNDJSONScope(err error, ndjsonOutput bool) int {
	if err == nil {
		return 0
	}
	if ndjsonOutput && errors.Is(err, syscall.EPIPE) {
		return 0
	}
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(a.errOut, "error:", usageErr.err)
		return 2
	}
	var pipelineErr *pipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

func commandWritesNDJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.CommandPath() == "pixiv filter" {
		return true
	}
	flag := cmd.Flags().Lookup("ndjson")
	return flag != nil && flag.Changed && flag.Value.String() == "true"
}

// usageError 标记由 CLI 参数、flag 或显式输入契约验证导致的错误；上游 SDK、
// 网络和本地 I/O 错误不得包裹为此类型，以保持 shell 可区分的退出码语义。
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func newUsageError(err error) error {
	if err == nil {
		return nil
	}
	var existing *usageError
	if errors.As(err, &existing) {
		return err
	}
	return &usageError{err: err}
}

func runMCP(ctx context.Context, proxyOverride *string) error {
	return bootstrap.RunMCP(ctx, proxyOverride)
}

func (a app) services() application.Services {
	return newCLIServices()
}

func (a app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		Version:       buildinfo.Current().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			a.enablePipelineSignal(cmd)
			a.enableMCPBrokenPipeSignal(cmd)
			if !shouldInitializeConfigForCommand(cmd) {
				return nil
			}
			return config.EnsureDefaultConfigFile()
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if cmd.CommandPath() == "pixiv filter" {
				return
			}
			a.checkAutomaticUpdate(cmd)
		},
	}
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(
		a.newAccountCommand(),
		a.newConfigCommand(),
		a.newSearchCommand(),
		a.newFilterCommand(),
		a.newNovelCommand(),
		a.newDetailCommand(),
		a.newRankingCommand(),
		a.newRecommendedCommand(),
		a.newTimelineCommand(),
		a.newMyPixivCommand(),
		a.newUserCommand(),
		a.newBookmarkCommand(),
		a.newFollowCommand(),
		a.newDownloadCommand(),
		a.newMCPCommand(),
		a.newVersionCommand(),
		a.newUpdateCommand(),
	)
	return cmd
}

func (a app) enablePipelineSignal(cmd *cobra.Command) {
	if a.pipelineSignal == nil || a.pipelineSignal.enable == nil || a.pipelineSignal.stop != nil || !commandWritesNDJSON(cmd) {
		return
	}
	a.pipelineSignal.stop = a.pipelineSignal.enable()
}

func (a app) enableMCPBrokenPipeSignal(cmd *cobra.Command) {
	if a.mcpBrokenPipeSignal == nil || a.mcpBrokenPipeSignal.enable == nil || a.mcpBrokenPipeSignal.stop != nil || cmd == nil || cmd.CommandPath() != "pixiv mcp" {
		return
	}
	a.mcpBrokenPipeSignal.stop = a.mcpBrokenPipeSignal.enable()
}

// shouldInitializeConfigForCommand 仅让真正执行的普通 CLI 命令生成配置。帮助、版本、
// credential export 与操作系统回调不得因只读/敏感流程而产生本地配置文件。
func shouldInitializeConfigForCommand(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "pixiv", "pixiv auth", "pixiv config", "pixiv version", "pixiv filter", "pixiv auth export", "pixiv auth " + internalURLCallbackCommand, "pixiv auth " + internalURLHandlerInstallCommand:
		return false
	default:
		return true
	}
}

func (a app) newMCPCommand() *cobra.Command {
	var opts proxyOptions
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP stdio server",
		Example: "pixiv mcp",
		Args:    requireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			proxyOverride, err := proxyOverrideFromFlags(cmd, opts)
			if err != nil {
				return err
			}
			return runMCPServer(cmd.Context(), proxyOverride)
		},
	}
	a.bindProxyFlags(cmd, &opts)
	return cmd
}

func (a app) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

// bindActionFlags 仅给会产生远端或文件副作用的命令暴露传输参数。动作的可组合
// 结果在 stdin 管道上用 stderr 逐条诊断表达，不能再接受与其冲突的 --json 输出。
func (a app) bindActionFlags(cmd *cobra.Command, opts *commandOptions) {
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

func (a app) bindProxyFlags(cmd *cobra.Command, opts *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear HTTP(S) proxy for this command")
}

func (a app) clientRequest(cmd *cobra.Command, opts commandOptions, needsAuth bool) (application.ClientRequest, error) {
	req := application.ClientRequest{
		NeedsAuth: needsAuth,
	}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return application.ClientRequest{}, err
	}
	req.HTTPSProxyOverride = proxyOverride
	if cmd.Flags().Changed("json") {
		req.JSONOverride = &opts.jsonOut
	}
	return req, nil
}

func proxyOverrideFromFlags(cmd *cobra.Command, opts proxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, fmt.Errorf("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && opts.noProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &opts.proxy, nil
	}
	return nil, nil
}

func (a app) printJSON(v any) error {
	encoder := json.NewEncoder(a.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func textBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func requireExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func requireMinArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func requireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}
