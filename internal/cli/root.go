package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth/loginhelper"
	configcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	fanboxcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	output "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
	pixivcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv"
	cliruntime "github.com/FlanChanXwO/pixiv-cli/internal/cli/runtime"
	updatecommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/update"
	versioncommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/version"
	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

type app struct {
	in                  io.Reader
	out                 io.Writer
	errOut              io.Writer
	pipelineSignal      *brokenPipeSignalState
	mcpBrokenPipeSignal *brokenPipeSignalState
	servicesState       *servicesState
	debugState          *debugState
}

type servicesState struct {
	initialized bool
	runtime     *bootstrap.Runtime
	err         error
}

// debugState is created per CLI invocation. It deliberately stores only the
// in-memory presenter and scoped context; no diagnostic event is persisted.
type debugState struct {
	presenter *diagnostics.Presenter
	ctx       context.Context
	operation string
	startedAt time.Time
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

// configMissingPlaceholder 保留根测试与文案契约的稳定名称；实际 config handler
// 位于 internal/cli/config。
const configMissingPlaceholder = "<unset>"

type proxyOptions struct {
	proxy   string
	noProxy bool
}

var (
	runMCPServer         = runMCP
	ensureURLSchemeRelay = loginhelper.EnsurePersistentIfNeeded
	canPrompt            = func(a app) bool { return authcommands.CanPrompt(a.in, a.out) }
	promptInput          = func(a app, message, defaultValue string) (string, error) {
		return authcommands.PromptInput(a.in, a.out, a.errOut, message, defaultValue)
	}
	promptSecret = func(a app, message string) (string, error) {
		return authcommands.PromptSecret(a.in, a.out, a.errOut, message)
	}
	promptSelect = func(a app, message string, options []string) (string, error) {
		return authcommands.PromptSelect(a.in, a.out, a.errOut, message, options)
	}
	promptConfirm = func(a app, message string, defaultValue bool) (bool, error) {
		return authcommands.PromptConfirm(a.in, a.out, a.errOut, message, defaultValue)
	}
	// FANBOX 浏览器读取是 command package 的注入端口；根包只保留一次 Run
	// 级别的测试 seam，生产默认实现位于 internal/cli/fanbox。
	fanboxBrowserSessionReader fanboxcommands.BrowserProvider = fanboxcommands.SystemBrowserProvider{}
	// CLI 在一次 Run 内只创建一个 Runtime；测试可注入不持有外部资源的 Runtime。
	newCLIServices                      = cliruntime.DefaultFactory()
	cleanupPendingWindowsUpdate         = update.CleanupPendingWindowsUpdate
	automaticPersistentHandlerSupported = loginhelper.AutomaticPersistentHandlerSupported
	loadUpdateRuntimeConfig             = bootstrap.LoadRuntimeConfig
	newUpdateCommandCoordinator         = bootstrap.NewUpdateCoordinator
	loadAutomaticUpdateRuntimeConfig    = bootstrap.LoadRuntimeConfig
	newCLIAutomaticUpdateChecker        = bootstrap.NewAutomaticUpdateChecker
)

const internalURLCallbackCommand = loginhelper.CallbackCommand
const internalURLHandlerInstallCommand = "_install-handler"

// systemFanboxBrowserSessionReader 保留根包测试对默认 provider 的类型引用；实现
// 本身属于 internal/cli/fanbox，不在根包复制浏览器逻辑。
type systemFanboxBrowserSessionReader = fanboxcommands.SystemBrowserProvider

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
	a := app{in: in, out: out, errOut: errOut, pipelineSignal: pipelineSignal, mcpBrokenPipeSignal: mcpBrokenPipeSignal, servicesState: &servicesState{}, debugState: &debugState{}}
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
	if closeErr := a.closeRuntime(); closeErr != nil {
		if err == nil {
			err = closeErr
		} else {
			err = errors.Join(err, closeErr)
		}
	}
	err = a.finishDiagnostics(err)
	return a.exitWithNDJSONScope(err, commandWritesNDJSON(target) || commandAutoWritesNDJSON(target, out))
}

func (a app) exitWithNDJSONScope(err error, ndjsonOutput bool) int {
	if err == nil {
		return 0
	}
	if ndjsonOutput && errors.Is(err, syscall.EPIPE) {
		return 0
	}
	var startupErr *startupError
	if errors.As(err, &startupErr) {
		fmt.Fprintln(a.errOut, startupErr.err)
		return 1
	}
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(a.errOut, "error:", usageErr.err)
		return 2
	}
	var pipelineErr *output.PipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

// startupError 保持解析成功后的启动副作用失败契约：它是命令启动失败，
// 不是用户参数错误，因此使用普通 process-level exit code 1 且不伪装成 usage。
type startupError struct{ err error }

func (e *startupError) Error() string { return e.err.Error() }
func (e *startupError) Unwrap() error { return e.err }

func normalizeFlagError(err error) error {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return err
	}

	if shortnames := notExist.GetSpecifiedShortnames(); shortnames != "" {
		return newUsageError(fmt.Errorf("unknown option '-%c'", []rune(shortnames)[0]))
	}
	if name := notExist.GetSpecifiedName(); name != "" {
		return newUsageError(fmt.Errorf("unknown option '--%s'", name))
	}
	return err
}

func commandWritesNDJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup("ndjson")
	return flag != nil && flag.Changed && flag.Value.String() == "true"
}

// commandAutoWritesNDJSON 与各视觉列表实际的 stdout 判定保持一致，让下游主动
// 关闭管道时可按 Unix 习惯结束而不把 EPIPE 误报为命令失败。
func commandAutoWritesNDJSON(cmd *cobra.Command, out io.Writer) bool {
	if cmd == nil || cmd.Flags().Changed("json") {
		return false
	}
	file, ok := out.(interface{ Fd() uintptr })
	if !ok || term.IsTerminal(int(file.Fd())) {
		return false
	}
	return slices.Contains([]string{
		"pixiv search", "pixiv ranking", "pixiv recommended",
		"pixiv timeline following", "pixiv timeline latest",
		"pixiv mypixiv works", "pixiv user artworks", "pixiv user bookmarks",
	}, cmd.CommandPath())
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

func runMCP(ctx context.Context, proxyOverride *string, requestIntervalOverride *time.Duration) error {
	return bootstrap.RunMCP(ctx, proxyOverride, requestIntervalOverride)
}

func (a app) services() *bootstrap.Runtime {
	if a.servicesState == nil {
		// 直接构造 app 的旧测试只需要注册命令，不应隐式创建 runtime；真正
		// 执行命令的 runContext 总会提供 servicesState 并由 pre-run 报告错误。
		return nil
	}
	if !a.servicesState.initialized {
		a.servicesState.runtime, a.servicesState.err = newCLIServices()
		a.servicesState.initialized = true
	}
	return a.servicesState.runtime
}

func (a app) closeRuntime() error {
	if a.servicesState == nil || a.servicesState.runtime == nil {
		return nil
	}
	return a.servicesState.runtime.Close()
}

func (a app) newRootCommand() *cobra.Command {
	var sleepRequest time.Duration
	var debug bool
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
			a.startDiagnostics(cmd, debug)
			a.enablePipelineSignal(cmd)
			a.enableMCPBrokenPipeSignal(cmd)
			if !shouldSkipStartupSideEffects(cmd) {
				if err := cleanupPendingWindowsUpdate(); err != nil {
					return &startupError{err: fmt.Errorf("clean pending update: %w", err)}
				}
				if automaticPersistentHandlerSupported() {
					if err := ensureURLSchemeRelay(cmd.Context()); err != nil {
						// 这项桌面集成不能阻断原命令，也不能把系统/本机路径写入 stderr。
						fmt.Fprintln(a.errOut, "warning: persistent pixiv:// callback handler was not initialized")
					}
				}
			}
			if !shouldInitializeConfigForCommand(cmd) {
				return a.checkServiceInitialization(cmd)
			}
			if err := bootstrap.EnsureDefaultConfigFile(); err != nil {
				return err
			}
			return a.checkServiceInitialization(cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) { updatecommands.RunAutomaticCheck(cmd, a) },
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return normalizeFlagError(err) })
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	cmd.PersistentFlags().DurationVar(&sleepRequest, "sleep-request", 0, "minimum interval between network request starts for this command")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "write safe execution diagnostics to stderr")
	cmd.CompletionOptions.DisableDefaultCmd = true
	authcommands.Register(cmd, a)
	configcommands.Register(cmd, a)
	pixivcommands.Register(cmd, a)
	downloadcommands.Register(cmd, a)
	fanboxcommands.Register(cmd, a)
	mcpcommands.Register(cmd, a)
	versioncommands.Register(cmd, a)
	updatecommands.Register(cmd, a)
	return cmd
}

// 以下公开的 host seam 只负责把根控制器交给对应的 command package；命令实现
// 仍可在增量重组期间共享既有 app state，子包不反向导入 internal/cli。
func (a app) AuthCommand() *cobra.Command { return authcommands.NewCommand(a) }

// newAccountCommand 保留根测试与嵌入式调用方的旧构造入口；实际实现已在
// internal/cli/auth，根包只负责传递 Host。
func (a app) newAccountCommand() *cobra.Command { return authcommands.NewCommand(a) }

func (a app) newAccountImportCommand() *cobra.Command {
	for _, command := range a.newAccountCommand().Commands() {
		if command.Name() == "import" {
			return command
		}
	}
	return nil
}

func (a app) ConfigService() configapp.ConfigService { return a.services().Config }

func (app) ConfigPath() (string, error) { return bootstrap.ConfigFilePath() }

func (a app) PixivCommands() []*cobra.Command {
	return pixivcommands.NewCommands(a)
}

func (a app) DownloadCommand() *cobra.Command { return downloadcommands.NewCommand(a) }

func (a app) FanboxCommand() *cobra.Command { return fanboxcommands.NewCommand(a) }

func (a app) MCPCommand() *cobra.Command { return mcpcommands.NewCommand(a) }

func (a app) VersionCommand() *cobra.Command { return versioncommands.NewCommand(a) }

func (a app) UpdateCommand() *cobra.Command { return updatecommands.NewCommand(a) }

// 以下 host methods 是根控制器对命令子包暴露的窄输出/依赖端口。子包不导入
// internal/cli，也不接触 app 的内部状态字段。
func (a app) Output() io.Writer { return a.out }

func (a app) Input() io.Reader { return a.in }

func (a app) ErrorOutput() io.Writer { return a.errOut }

func (a app) PrintJSON(value any) error { return a.printJSON(value) }

func (app) UsageError(err error) error { return newUsageError(err) }

func (a app) RequireExactArgs(count int, usage string) cobra.PositionalArgs {
	return requireExactArgs(count, usage)
}

func (a app) RequireMinArgs(count int, usage string) cobra.PositionalArgs {
	return requireMinArgs(count, usage)
}

func (a app) RequireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return requireMaxArgs(count, usage)
}

func (a app) FanboxService() (*fanboxapp.Service, error) {
	return a.services().Fanbox, nil
}

func (a app) AccountService() pixivapp.AccountService { return a.services().Account }

func (a app) LoginService() pixivapp.LoginService { return a.services().Login }

func (a app) SDKService() pixivapp.SDKService { return a.services().SDK }

func (a app) DownloadService() downloadapp.DownloadService { return a.services().Download }

func (app) WriteAuthExportBundle(path string, body []byte, force bool) error {
	return bootstrap.WriteAuthExportBundle(path, body, force)
}

func (app) FanboxBrowserProvider() fanboxcommands.BrowserProvider {
	return fanboxBrowserSessionReader
}

func (app) FanboxRuntimeConfig() (configapp.RuntimeConfig, error) {
	return bootstrap.LoadRuntimeConfig()
}

func (a app) CanPrompt() bool { return canPrompt(a) }

func (a app) PromptInput(message, defaultValue string) (string, error) {
	return promptInput(a, message, defaultValue)
}

func (a app) PromptSecret(message string) (string, error) {
	return promptSecret(a, message)
}

func (a app) PromptSelect(message string, options []string) (string, error) {
	return promptSelect(a, message, options)
}

func (a app) PromptConfirm(message string, defaultValue bool) (bool, error) {
	return promptConfirm(a, message, defaultValue)
}

func (app) LoadUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return loadUpdateRuntimeConfig()
}

func (app) NewUpdateCoordinator(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
	return newUpdateCommandCoordinator(proxy, out, errOut)
}

func (app) LoadAutomaticUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return loadAutomaticUpdateRuntimeConfig()
}

func (app) NewAutomaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	return newCLIAutomaticUpdateChecker(proxy)
}

func (a app) BindProxyFlags(cmd *cobra.Command, options *mcpcommands.ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&options.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&options.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a app) ClientRequest(cmd *cobra.Command, options mcpcommands.ProxyOptions) (application.ClientRequest, error) {
	return a.clientRequest(cmd, commandOptions{proxyOptions: proxyOptions{proxy: options.Proxy, noProxy: options.NoProxy}}, false)
}

func (app) RunMCP(ctx context.Context, proxy *string, interval *time.Duration) error {
	return runMCPServer(ctx, proxy, interval)
}

func (a app) startDiagnostics(cmd *cobra.Command, enabled bool) {
	if !enabled || shouldSkipDiagnostics(cmd) || a.debugState == nil {
		return
	}
	module := diagnostics.ModulePixivCLI
	if cmd != nil && strings.HasPrefix(cmd.CommandPath(), "pixiv fanbox") {
		module = diagnostics.ModuleFanboxCLI
	}
	presenter := diagnostics.NewPresenter(a.errOut)
	scoped := diagnostics.WithScope(cmd.Context(), presenter, module, 0)
	cmd.SetContext(scoped)
	a.debugState.presenter = presenter
	a.debugState.ctx = scoped
	a.debugState.operation = cmd.CommandPath()
	a.debugState.startedAt = time.Now()
	diagnostics.Emit(scoped, diagnostics.Event{Kind: diagnostics.EventStarted, Operation: cmd.CommandPath()})
}

func (a app) finishDiagnostics(err error) error {
	if a.debugState == nil || a.debugState.presenter == nil || a.debugState.ctx == nil {
		return err
	}
	if err == nil {
		diagnostics.Emit(a.debugState.ctx, diagnostics.Event{
			Kind:      diagnostics.EventCompleted,
			Operation: a.debugState.operation,
			Duration:  time.Since(a.debugState.startedAt),
		})
	} else {
		diagnostics.Emit(a.debugState.ctx, diagnostics.Event{
			Kind:      diagnostics.EventFailed,
			Operation: a.debugState.operation,
			Reason:    diagnostics.ReasonCommandFailed,
			Duration:  time.Since(a.debugState.startedAt),
		})
	}
	if diagnosticErr := a.debugState.presenter.Err(); diagnosticErr != nil {
		wrapped := fmt.Errorf("write debug diagnostics: %w", diagnosticErr)
		if err == nil {
			return wrapped
		}
		return errors.Join(err, wrapped)
	}
	return err
}

func (a app) checkServiceInitialization(cmd *cobra.Command) error {
	if !shouldInitializeServicesForCommand(cmd) {
		return nil
	}
	if a.servicesState == nil {
		return &startupError{err: errors.New("initialize local state: CLI runtime state is not initialized")}
	}
	runtime := a.services()
	if a.servicesState.err != nil {
		return &startupError{err: fmt.Errorf("initialize local state: %w", a.servicesState.err)}
	}
	if runtime == nil {
		return &startupError{err: errors.New("initialize local state: runtime factory returned nil")}
	}
	return nil
}

// shouldInitializeServicesForCommand 避免纯帮助、版本和隐藏回调为只读流程
// 创建 authdb；需要应用服务的命令在业务 RunE 前必须暴露真实初始化错误。
func shouldInitializeServicesForCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	switch cmd.CommandPath() {
	case "pixiv", "pixiv auth", "pixiv config path", "pixiv version", "pixiv update",
		"pixiv mcp", "pixiv fanbox mcp",
		"pixiv auth " + internalURLCallbackCommand, "pixiv auth " + internalURLHandlerInstallCommand:
		return false
	default:
		return true
	}
}

func shouldSkipStartupSideEffects(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	switch cmd.CommandPath() {
	case "pixiv auth export", "pixiv auth " + internalURLCallbackCommand, "pixiv auth " + internalURLHandlerInstallCommand:
		return true
	default:
		return false
	}
}

func shouldSkipDiagnostics(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	switch cmd.CommandPath() {
	case "pixiv auth export", "pixiv auth " + internalURLCallbackCommand, "pixiv auth " + internalURLHandlerInstallCommand:
		return true
	default:
		return false
	}
}

func (a app) enablePipelineSignal(cmd *cobra.Command) {
	if a.pipelineSignal == nil || a.pipelineSignal.enable == nil || a.pipelineSignal.stop != nil || !commandWritesNDJSON(cmd) {
		return
	}
	a.pipelineSignal.stop = a.pipelineSignal.enable()
}

func (a app) enableMCPBrokenPipeSignal(cmd *cobra.Command) {
	if a.mcpBrokenPipeSignal == nil || a.mcpBrokenPipeSignal.enable == nil || a.mcpBrokenPipeSignal.stop != nil || cmd == nil {
		return
	}
	path := cmd.CommandPath()
	if path != "pixiv mcp" && path != "pixiv fanbox mcp" {
		return
	}
	a.mcpBrokenPipeSignal.stop = a.mcpBrokenPipeSignal.enable()
}

// shouldInitializeConfigForCommand 仅让真正执行的普通 CLI 命令生成配置。帮助、版本、
// credential export、操作系统回调与 FANBOX 认证流程不得因只读/敏感流程而产生本地配置文件。
func shouldInitializeConfigForCommand(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "pixiv", "pixiv auth", "pixiv config", "pixiv version", "pixiv auth export", "pixiv auth " + internalURLCallbackCommand, "pixiv auth " + internalURLHandlerInstallCommand:
		return false
	default:
		return !strings.HasPrefix(cmd.CommandPath(), "pixiv fanbox auth")
	}
}

func (a app) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

func (a app) bindProxyFlags(cmd *cobra.Command, opts *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear the configured proxy for this command")
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
	if flag := cmd.Flags().Lookup("sleep-request"); flag != nil && flag.Changed {
		value, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			return application.ClientRequest{}, fmt.Errorf("invalid --sleep-request: %w", err)
		}
		if value < 0 {
			return application.ClientRequest{}, errors.New("--sleep-request must not be negative")
		}
		req.RequestIntervalOverride = &value
	}
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
	body, err := output.MarshalJSONValue(v, false)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	if _, err := io.WriteString(a.out, out.String()+"\n"); err != nil {
		return err
	}
	return nil
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
