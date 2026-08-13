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

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	configcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/download"
	fanboxmcp "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/mcp"
	fanboxpost "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/post"
	fanboxdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxdeps"
	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginhelper"
	pixivbookmark "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/bookmark"
	pixivcomment "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/comment"
	pixivdetail "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/detail"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/download"
	pixivfollow "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/follow"
	pixivmypixiv "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/mypixiv"
	pixivranking "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/ranking"
	pixivrecommended "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/recommended"
	pixivsearch "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/search"
	pixivseries "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/series"
	pixivtimeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/timeline"
	pixivuser "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	updatecommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/update"
	versioncommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/version"
	diagnosticscore "github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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
	resourcesState      *resourcesState
	debugState          *debugState
}

type resourcesState struct {
	initialized bool
	resources   *runResources
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
	runMCPServer = func(resources *runResources, ctx context.Context, request mcpcommands.Request) error {
		return resources.runPixivMCP(ctx, request)
	}
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
	// FANBOX 浏览器读取是 auth command 的注入端口；根包只保留一次 Run
	// 级别的测试 seam，生产默认实现位于 internal/cli/fanbox/auth。
	fanboxBrowserSessionReader          fanboxdeps.BrowserProvider = fanboxauth.SystemBrowserProvider{}
	cleanupPendingWindowsUpdate                                    = update.CleanupPendingWindowsUpdate
	automaticPersistentHandlerSupported                            = loginhelper.AutomaticPersistentHandlerSupported
	newUpdateCommandCoordinator                                    = newUpdateCoordinator
	newCLIAutomaticUpdateChecker                                   = newAutomaticUpdateChecker
)

const internalURLCallbackCommand = loginhelper.CallbackCommand
const internalURLHandlerInstallCommand = "_install-handler"

// systemFanboxBrowserSessionReader 保留根包测试对默认 provider 的类型引用；实现
// 本身属于 internal/cli/fanbox，不在根包复制浏览器逻辑。
type systemFanboxBrowserSessionReader = fanboxauth.SystemBrowserProvider

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
	a := app{in: in, out: out, errOut: errOut, pipelineSignal: pipelineSignal, mcpBrokenPipeSignal: mcpBrokenPipeSignal, resourcesState: &resourcesState{}, debugState: &debugState{}}
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
	defer pipeline.Clear(cmd)
	defer authcommands.ClearInputState(cmd)
	defer requirements.Clear(cmd)
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
	if closeErr := a.closeResources(); closeErr != nil {
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
	var pipelineErr *pipeline.PipelineDiagnosticError
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

func (a app) runResources() (*runResources, error) {
	if a.resourcesState == nil {
		return nil, errors.New("CLI resource state is not initialized")
	}
	if !a.resourcesState.initialized {
		a.resourcesState.resources, a.resourcesState.err = newCLIRunResources()
		a.resourcesState.initialized = true
	}
	if a.resourcesState.err != nil {
		return nil, a.resourcesState.err
	}
	if a.resourcesState.resources == nil {
		return nil, errors.New("resource factory returned nil")
	}
	return a.resourcesState.resources, nil
}

func (a app) closeResources() error {
	if a.resourcesState == nil || a.resourcesState.resources == nil {
		return nil
	}
	return a.resourcesState.resources.close()
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
			requirement := requirements.For(cmd)
			a.startDiagnostics(cmd, debug, requirement)
			a.enablePipelineSignal(cmd)
			a.enableMCPBrokenPipeSignal(requirement)
			if requirement.StartupHooks {
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
			if requirement.EnsureConfig {
				if err := configapp.DefaultStore().EnsureDefaultConfigFile(); err != nil {
					return err
				}
			}
			if err := a.prepareResources(requirement); err != nil {
				return err
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if requirements.For(cmd).AutomaticUpdate {
				updatecommands.RunAutomaticCheck(cmd, a)
			}
		},
	}
	requirements.Bind(cmd, requirements.Execution{})
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return normalizeFlagError(err) })
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	cmd.PersistentFlags().DurationVar(&sleepRequest, "sleep-request", 0, "minimum interval between network request starts for this command")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "write safe execution diagnostics to stderr")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(authcommands.New(a.authDeps()))
	configcommands.Register(cmd, a)
	cmd.AddCommand(a.pixivCommands()...)
	cmd.AddCommand(downloadcommands.New(a.downloadDeps()))
	cmd.AddCommand(a.newFanboxCommand())
	mcpcommands.Register(cmd, a)
	versioncommands.Register(cmd, a)
	updatecommands.Register(cmd, a)
	return cmd
}

// 以下 host methods 是根控制器对命令子包暴露的窄输出/依赖端口。子包不导入
// internal/cli，也不接触 app 的内部状态字段。
func (a app) authDeps() authcommands.Deps {
	return authcommands.Deps{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Account: func() pixivapp.AccountService {
			resources, err := a.runResources()
			if err != nil {
				return pixivapp.AccountService{}
			}
			service, _ := resources.accountService()
			return service
		},
		Login: func() pixivapp.LoginService {
			resources, err := a.runResources()
			if err != nil {
				return pixivapp.LoginService{}
			}
			service, _ := resources.loginService()
			return service
		},
		WriteBundle:  writeAuthExportBundle,
		CanPrompt:    func() bool { return canPrompt(a) },
		PromptInput:  func(message, defaultValue string) (string, error) { return promptInput(a, message, defaultValue) },
		PromptSecret: func(message string) (string, error) { return promptSecret(a, message) },
		PromptSelect: func(message string, options []string) (string, error) { return promptSelect(a, message, options) },
		PromptConfirm: func(message string, defaultValue bool) (bool, error) {
			return promptConfirm(a, message, defaultValue)
		},
	}
}

func (a app) downloadDeps() downloadcommands.Deps {
	return downloadcommands.Deps{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Open: func(request downloadcommands.CommandRequest) (*pixiv.Client, error) {
			resources, err := a.runResources()
			if err != nil {
				return nil, err
			}
			sdk, err := resources.sdkService()
			if err != nil {
				return nil, err
			}
			return sdk.open(downloadRequestToPixiv(request))
		},
		Pooled: func(ctx context.Context, request downloadcommands.CommandRequest, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			resources, err := a.runResources()
			if err != nil {
				return err
			}
			sdk, err := resources.sdkService()
			if err != nil {
				return err
			}
			return sdk.pooled(ctx, downloadRequestToPixiv(request), attempt)
		},
		Runtime: func() (downloadcommands.Runtime, error) {
			runtime, err := a.runtimeConfig()
			if err != nil {
				return downloadcommands.Runtime{}, err
			}
			return downloadcommands.Runtime{
				DownloadPath:     runtime.DownloadPath,
				FilenameTemplate: runtime.FilenameTemplate,
			}, nil
		},
		Download: func() downloader.DownloadService {
			resources, err := a.runResources()
			if err != nil {
				return downloader.DownloadService{}
			}
			service, _ := resources.downloadService()
			return service
		},
	}
}

func (a app) pixivDataDeps() pixivdeps.Data {
	return pixivdeps.Data{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Open: func(request pixivdeps.Request) (*pixiv.Client, error) {
			resources, err := a.runResources()
			if err != nil {
				return nil, err
			}
			sdk, err := resources.sdkService()
			if err != nil {
				return nil, err
			}
			return sdk.open(request)
		},
		Pooled: func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			resources, err := a.runResources()
			if err != nil {
				return err
			}
			sdk, err := resources.sdkService()
			if err != nil {
				return err
			}
			return sdk.pooled(ctx, request, attempt)
		},
		JSONOut: func(override *bool) (bool, error) {
			resources, err := a.runResources()
			if err != nil {
				return false, err
			}
			sdk, err := resources.sdkService()
			if err != nil {
				return false, err
			}
			return sdk.jsonOut(override)
		},
	}
}

func (a app) pixivCommands() []*cobra.Command {
	data := a.pixivDataDeps()
	return []*cobra.Command{
		pixivsearch.New(data),
		pixivsearch.NewNovel(data),
		pixivdetail.New(data),
		pixivranking.New(data),
		pixivseries.New(data),
		pixivcomment.New(data),
		pixivrecommended.New(data),
		pixivtimeline.New(data),
		pixivmypixiv.New(data),
		pixivuser.New(data),
		pixivbookmark.New(data),
		pixivfollow.New(data),
	}
}

func (a app) fanboxDataDeps() fanboxdeps.Data {
	return fanboxdeps.Data{
		Reader:    a.in,
		Writer:    a.out,
		WrapUsage: newUsageError,
		ServiceFactory: func() (*fanboxapp.Service, error) {
			resources, err := a.runResources()
			if err != nil {
				return nil, err
			}
			return resources.fanboxService()
		},
		Browser: fanboxBrowserSessionReader,
		Runtime: a.runtimeConfig,
		CanPromptFn: func() bool {
			return canPrompt(a)
		},
		PromptSecretFn: func(message string) (string, error) {
			return promptSecret(a, message)
		},
		PromptConfirmFn: func(message string, defaultValue bool) (bool, error) {
			return promptConfirm(a, message, defaultValue)
		},
		RunMCPServer: func(cmd *cobra.Command, service *fanboxapp.Service, proxy *string) error {
			resources, err := a.runResources()
			if err != nil {
				return err
			}
			return resources.runFanboxMCP(cmd.Context(), service, proxy)
		},
	}
}

func (a app) newFanboxCommand() *cobra.Command {
	data := a.fanboxDataDeps()
	var proxy string
	var noProxy bool
	cmd := &cobra.Command{
		Use:   "fanbox",
		Short: "Browse and download Pixiv FANBOX content",
		Args:  data.RequireExactArgs(0, "pixiv fanbox <command>"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVar(&proxy, "proxy", "", "native FANBOX proxy URL (HTTP or HTTPS CONNECT)")
	flags.BoolVar(&noProxy, "no-proxy", false, "use a direct native FANBOX connection for this command")
	cmd.AddCommand(fanboxauth.New(data))
	cmd.AddCommand(fanboxpost.Commands(data)...)
	cmd.AddCommand(fanboxdownload.New(data), fanboxmcp.New(data))
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.Execution{})
	return cmd
}

func (a app) ConfigService() configapp.Store {
	if a.resourcesState != nil && a.resourcesState.resources != nil {
		return a.resourcesState.resources.configStoreValue()
	}
	return configapp.DefaultStore()
}

func (a app) ConfigPath() (string, error) { return a.ConfigService().Path() }

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
	resources, err := a.runResources()
	if err != nil {
		return nil, err
	}
	return resources.fanboxService()
}

func (a app) AccountService() pixivapp.AccountService {
	resources, err := a.runResources()
	if err != nil {
		return pixivapp.AccountService{}
	}
	service, _ := resources.accountService()
	return service
}

func (a app) LoginService() pixivapp.LoginService {
	resources, err := a.runResources()
	if err != nil {
		return pixivapp.LoginService{}
	}
	service, _ := resources.loginService()
	return service
}

func (a app) DownloadService() downloader.DownloadService {
	resources, err := a.runResources()
	if err != nil {
		return downloader.DownloadService{}
	}
	service, _ := resources.downloadService()
	return service
}

func (app) WriteAuthExportBundle(path string, body []byte, force bool) error {
	return writeAuthExportBundle(path, body, force)
}

func (app) FanboxBrowserProvider() fanboxdeps.BrowserProvider {
	return fanboxBrowserSessionReader
}

func (a app) FanboxRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
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

func (a app) LoadUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) NewUpdateCoordinator(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
	resources, err := a.runResources()
	if err != nil {
		return nil, err
	}
	return resources.updateCoordinator(proxy, out, errOut)
}

func (a app) LoadAutomaticUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) NewAutomaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	resources, err := a.runResources()
	if err != nil {
		return nil, err
	}
	return resources.automaticUpdateChecker(proxy)
}

func (a app) BindProxyFlags(cmd *cobra.Command, options *mcpcommands.ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&options.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&options.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a app) ClientRequest(cmd *cobra.Command, options mcpcommands.ProxyOptions) (mcpcommands.Request, error) {
	proxyOverride, err := proxyOverrideFromFlags(cmd, proxyOptions{proxy: options.Proxy, noProxy: options.NoProxy})
	if err != nil {
		return mcpcommands.Request{}, err
	}
	request := mcpcommands.Request{HTTPSProxyOverride: proxyOverride}
	if flag := cmd.Flags().Lookup("sleep-request"); flag != nil && flag.Changed {
		value, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			return mcpcommands.Request{}, fmt.Errorf("invalid --sleep-request: %w", err)
		}
		if value < 0 {
			return mcpcommands.Request{}, errors.New("--sleep-request must not be negative")
		}
		request.RequestIntervalOverride = &value
	}
	return request, nil
}

func (a app) RunMCP(ctx context.Context, request mcpcommands.Request) error {
	resources, err := a.runResources()
	if err != nil {
		return err
	}
	return runMCPServer(resources, ctx, request)
}

func (a app) startDiagnostics(cmd *cobra.Command, enabled bool, requirement requirements.Execution) {
	if !enabled || !requirement.Diagnostics || a.debugState == nil {
		return
	}
	module := diagnosticscore.ModulePixivCLI
	if cmd != nil && strings.HasPrefix(cmd.CommandPath(), "pixiv fanbox") {
		module = diagnosticscore.ModuleFanboxCLI
	}
	presenter := diagnostics.NewPresenter(a.errOut)
	scoped := diagnosticscore.WithScope(cmd.Context(), presenter, module, 0)
	cmd.SetContext(scoped)
	a.debugState.presenter = presenter
	a.debugState.ctx = scoped
	a.debugState.operation = cmd.CommandPath()
	a.debugState.startedAt = time.Now()
	diagnosticscore.Emit(scoped, diagnosticscore.Event{Kind: diagnosticscore.EventStarted, Operation: cmd.CommandPath()})
}

func (a app) finishDiagnostics(err error) error {
	if a.debugState == nil || a.debugState.presenter == nil || a.debugState.ctx == nil {
		return err
	}
	if err == nil {
		diagnosticscore.Emit(a.debugState.ctx, diagnosticscore.Event{
			Kind:      diagnosticscore.EventCompleted,
			Operation: a.debugState.operation,
			Duration:  time.Since(a.debugState.startedAt),
		})
	} else {
		diagnosticscore.Emit(a.debugState.ctx, diagnosticscore.Event{
			Kind:      diagnosticscore.EventFailed,
			Operation: a.debugState.operation,
			Reason:    diagnosticscore.ReasonCommandFailed,
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

func (a app) prepareResources(requirement requirements.Execution) error {
	resources := requirement.Resources
	if !resources.ConfigSnapshot && !resources.Database && !resources.PixivAccount && !resources.PixivLogin && !resources.PixivSDK && !resources.Download && !resources.Fanbox && !resources.Update {
		return nil
	}
	graph, err := a.runResources()
	if err != nil {
		return &startupError{err: fmt.Errorf("initialize local state: %w", err)}
	}
	if err := graph.prepare(resources); err != nil {
		return &startupError{err: fmt.Errorf("initialize local state: %w", err)}
	}
	return nil
}

func (a app) runtimeConfig() (configapp.RuntimeConfig, error) {
	if a.resourcesState != nil && a.resourcesState.resources != nil {
		return a.resourcesState.resources.runtime()
	}
	return loadCLIRuntimeConfig()
}

func (a app) enablePipelineSignal(cmd *cobra.Command) {
	if a.pipelineSignal == nil || a.pipelineSignal.enable == nil || a.pipelineSignal.stop != nil || !commandWritesNDJSON(cmd) {
		return
	}
	a.pipelineSignal.stop = a.pipelineSignal.enable()
}

func (a app) enableMCPBrokenPipeSignal(requirement requirements.Execution) {
	if a.mcpBrokenPipeSignal == nil || a.mcpBrokenPipeSignal.enable == nil || a.mcpBrokenPipeSignal.stop != nil || !requirement.MCP {
		return
	}
	a.mcpBrokenPipeSignal.stop = a.mcpBrokenPipeSignal.enable()
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
	body, err := json.Marshal(v)
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

// downloadRequestToPixiv 把 download command 的本地请求映射为共享 Pixiv SDK
// 请求；download owner 不导入 pixivdeps。
func downloadRequestToPixiv(request downloadcommands.CommandRequest) pixivdeps.Request {
	return pixivdeps.Request{
		HTTPSProxyOverride:      request.HTTPSProxyOverride,
		RequestIntervalOverride: request.RequestIntervalOverride,
	}
}
