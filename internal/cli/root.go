package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	configcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/config"
	fanboxcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/auth"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/download"
	fanboxmcpcommand "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/mcp"
	fanboxpost "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/post"
	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginhelper"
	pixivbookmark "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/bookmark"
	pixivcomment "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/comment"
	pixivdetail "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/detail"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download"
	pixivfollow "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/follow"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mcp"
	pixivmypixiv "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mypixiv"
	pixivranking "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/ranking"
	pixivrecommended "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/recommended"
	pixivsearch "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/search"
	pixivseries "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/series"
	pixivtimeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/timeline"
	pixivuser "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/user"
	updatecommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/update"
	clidiagnostics "github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/invocation"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	stdiotransport "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	mcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	pixivpool "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
	coreDiagnostics "github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/network"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
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
	closeState          *closeState
	diagnostics         *diagnosticState
}

type diagnosticState struct {
	ctx       context.Context
	operation string
	presenter *clidiagnostics.Presenter
}

// closeState tracks resources opened by the current invocation. It is only a
// reverse-order close list; it does not cache services or expose a graph.
type closeState struct {
	mu      sync.Mutex
	closers []func() error
	err     error
	once    sync.Once
}

func (s *closeState) add(closer func() error) {
	if s == nil || closer == nil {
		return
	}
	s.mu.Lock()
	s.closers = append(s.closers, closer)
	s.mu.Unlock()
}

func (s *closeState) close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for index := len(s.closers) - 1; index >= 0; index-- {
			s.err = errors.Join(s.err, s.closers[index]())
		}
	})
	return s.err
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
// 位于 commands/config。
const configMissingPlaceholder = "<unset>"

type proxyOptions struct {
	proxy   string
	noProxy bool
}

var (
	runMCPServer = func(a app, ctx context.Context, request mcpcommands.Request) error {
		return a.runPixivMCP(ctx, request)
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
	// 级别的测试 seam，生产默认实现位于 commands/fanbox/auth。
	fanboxBrowserSessionReader          fanboxcommands.BrowserProvider = fanboxauth.SystemBrowserProvider{}
	cleanupPendingWindowsUpdate                                        = update.CleanupPendingWindowsUpdate
	automaticPersistentHandlerSupported                                = loginhelper.AutomaticPersistentHandlerSupported
	newUpdateCommandCoordinator                                        = updatecommands.NewCoordinator
	newCLIAutomaticUpdateChecker                                       = updatecommands.NewAutomaticChecker
	newCLIPixivSDKPorts                                                = func(a app) (pixivSDKPorts, error) { return a.newPixivSDKPorts() }
	newCLIAccountServices                                              = func(a app) (authcommands.AccountService, pixivaccount.LoginService, error) {
		return a.newPixivAccountServices()
	}
	newCLIFanboxService        = func(a app) (*fanboxapp.Facade, error) { return a.newFanboxService() }
	newCLIFanboxAccountService = func(a app) (*fanboxaccount.Service, error) {
		return a.newFanboxAccountService()
	}
	newCLIDownloadService = func() downloader.DownloadService { return defaultDownloadService() }
)

const internalURLCallbackCommand = loginhelper.CallbackCommand
const internalURLHandlerInstallCommand = "_install-handler"

// systemFanboxBrowserSessionReader 保留根包测试对默认 provider 的类型引用；实现
// 本身属于 commands/fanbox/auth，不在根包复制浏览器逻辑。
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

// RunContextWithDefaultBrokenPipeSignals 为二进制入口装配当前平台的默认 SIGPIPE
// 控制器：普通 NDJSON 输出与 MCP stdio 各自独立。嵌入式调用方若需要自定义信号
// 策略，应直接调用 RunContext 或 RunContextWithBrokenPipeSignals。
func RunContextWithDefaultBrokenPipeSignals(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return RunContextWithBrokenPipeSignals(ctx, args, in, out, errOut, enablePipelineBrokenPipeSignal, enableMCPBrokenPipeSignal)
}

func runContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, pipelineSignal, mcpBrokenPipeSignal *brokenPipeSignalState) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	streams := invocation.NewStreams(in, out, errOut)
	a := app{
		in:                  streams.In,
		out:                 streams.Out,
		errOut:              streams.Err,
		pipelineSignal:      pipelineSignal,
		mcpBrokenPipeSignal: mcpBrokenPipeSignal,
		closeState:          &closeState{},
		diagnostics:         &diagnosticState{},
	}
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

func (a app) closeResources() error {
	return a.closeState.close()
}

func (a app) openAuthDatabase() (*database.DB, error) {
	db, err := openCLIAuthDatabase()
	if err != nil {
		return nil, err
	}
	a.closeState.add(db.Close)
	return db, nil
}

func (a app) newPixivAccountServices() (authcommands.AccountService, pixivaccount.LoginService, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return authcommands.AccountService{}, pixivaccount.LoginService{}, err
	}
	service := pixivaccount.NewService(db, configDefaultStore{store: configapp.DefaultStore()})
	if service == nil {
		return authcommands.AccountService{}, pixivaccount.LoginService{}, errors.New("pixiv account service is not configured")
	}
	return authcommands.AccountService{Pixiv: service, LoadRuntime: a.runtimeConfig}, pixivaccount.LoginService{Pixiv: service}, nil
}

func (a app) newPixivSDKPorts() (pixivSDKPorts, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return pixivSDKPorts{}, err
	}
	service := pixivaccount.NewService(db, configDefaultStore{store: configapp.DefaultStore()})
	if service == nil {
		return pixivSDKPorts{}, errors.New("pixiv account service is not configured")
	}
	gate := pixivpool.NewGate()
	facade := pixivapp.New(pixivapp.Dependencies{
		Accounts: service,
		Gate:     gate,
		LoadPoolConfig: func() (pixivapp.PoolConfig, error) {
			runtime, err := a.runtimeConfig()
			if err != nil {
				return pixivapp.PoolConfig{}, err
			}
			return pixivapp.PoolConfig{Enabled: runtime.AccountPool.Enabled, Strategy: string(runtime.AccountPool.Strategy)}, nil
		},
		Pool: func(config pixivapp.PoolConfig) (pixivapp.PoolExecutor, error) {
			return pixivpool.Scheduler{
				Config: configapp.AccountPoolConfig{Enabled: config.Enabled, Strategy: configapp.AccountPoolStrategy(config.Strategy)},
				State:  db,
				Now:    time.Now,
			}, nil
		},
	})
	if facade == nil {
		return pixivSDKPorts{}, errors.New("pixiv service facade is not configured")
	}
	open := func(request pixivdeps.Request) (*pixiv.Client, error) {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return nil, err
		}
		lease, err := facade.Open(context.Background(), pixivapp.Request{UserID: request.UserID, Options: options})
		if err != nil {
			return nil, err
		}
		a.closeState.add(lease.Close)
		return lease.Value(), nil
	}
	openLease := func(ctx context.Context, request pixivdeps.Request) (*lifecycle.Lease[*pixiv.Client], error) {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return nil, err
		}
		return facade.Open(ctx, pixivapp.Request{UserID: request.UserID, Options: options})
	}
	execute := func(ctx context.Context, request pixivdeps.Request, callback func(context.Context, *pixiv.Client) (bool, error)) error {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return err
		}
		return facade.Use(ctx, pixivapp.Request{UserID: request.UserID, Options: options}, callback)
	}
	return pixivSDKPorts{
		open:      open,
		openLease: openLease,
		execute:   execute,
		pooled:    execute,
		jsonOut: func(override *bool) (bool, error) {
			if override != nil {
				return *override, nil
			}
			runtime, err := a.runtimeConfig()
			if err != nil {
				return false, err
			}
			return runtime.OutputJSON, nil
		},
	}, nil
}

func (a app) newFanboxAccountService() (*fanboxaccount.Service, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return nil, err
	}
	service := fanboxaccount.NewService(db, configDefaultStore{store: configapp.DefaultStore()})
	service.LoadOptionsFunc = func() (fanboxsdkOptions, error) {
		return fanboxOptionsFromRuntime(a.runtimeConfig)
	}
	return service, nil
}

func (a app) newFanboxService() (*fanboxapp.Facade, error) {
	accounts, err := a.newFanboxAccountService()
	if err != nil {
		return nil, err
	}
	return fanboxapp.NewFacade(accounts), nil
}

func defaultDownloadService() downloader.DownloadService {
	return downloader.DownloadService{NewManager: func(client downloader.DownloadClient, downloadPath, filenameTemplate string) (downloader.DownloadManager, error) {
		return downloader.NewManager(client, downloadPath, filenameTemplate), nil
	}}
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
			requirement := requirements.For(cmd)
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
				if err := a.startDiagnostics(cmd, requirement); err != nil {
					return err
				}
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
	// 不把 Cobra 的 help handler 作为公开 root subcommand；根命令仍通过
	// --help 与 --version 提供内置帮助/版本入口。
	cmd.SetHelpCommand(&cobra.Command{Use: "_help [command]", Hidden: true})
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(authcommands.New(a.authDeps()))
	configcommands.Register(cmd, a)
	cmd.AddCommand(a.pixivCommands()...)
	cmd.AddCommand(downloadcommands.New(a.downloadDeps()))
	fanboxData := a.fanboxDataDeps()
	cmd.AddCommand(fanboxcommands.New(fanboxData, fanboxcommands.CommandSet{
		Auth:     fanboxauth.New(fanboxData),
		Posts:    fanboxpost.Commands(fanboxData),
		Download: fanboxdownload.New(fanboxData),
		MCP:      fanboxmcpcommand.New(fanboxData),
	}))
	mcpcommands.Register(cmd, a)
	updatecommands.Register(cmd, a)
	return cmd
}

// 以下 host methods 是根控制器对命令子包暴露的窄输出/依赖端口。子包不导入
// internal/cli，也不接触 app 的内部状态字段。
func (a app) authDeps() authcommands.Deps {
	var once sync.Once
	var account authcommands.AccountService
	var login pixivaccount.LoginService
	load := func() {
		once.Do(func() {
			account, login, _ = newCLIAccountServices(a)
		})
	}
	return authcommands.Deps{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Account: func() authcommands.AccountService {
			load()
			return account
		},
		Login: func() pixivaccount.LoginService {
			load()
			return login
		},
		LoadRuntime:  a.runtimeConfig,
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
	var once sync.Once
	var ports pixivSDKPorts
	var portsErr error
	load := func() (pixivSDKPorts, error) {
		once.Do(func() { ports, portsErr = newCLIPixivSDKPorts(a) })
		return ports, portsErr
	}
	return downloadcommands.Deps{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Open: func(request downloadcommands.CommandRequest) (*pixiv.Client, error) {
			sdk, err := load()
			if err != nil {
				return nil, err
			}
			return sdk.open(downloadcommands.ToPixivRequest(request))
		},
		Pooled: func(ctx context.Context, request downloadcommands.CommandRequest, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			sdk, err := load()
			if err != nil {
				return err
			}
			return sdk.run(ctx, downloadcommands.ToPixivRequest(request), attempt)
		},
		Runtime: func() (downloadcommands.Runtime, error) {
			runtime, err := a.runtimeConfig()
			if err != nil {
				return downloadcommands.Runtime{}, err
			}
			return downloadcommands.Runtime{
				DownloadPath:      runtime.DownloadPath,
				FilenameTemplate:  runtime.FilenameTemplate,
				DirectoryTemplate: runtime.DirectoryTemplate,
			}, nil
		},
		Download: newCLIDownloadService,
	}
}

func (a app) pixivDataDeps() pixivdeps.Data {
	var once sync.Once
	var ports pixivSDKPorts
	var portsErr error
	load := func() (pixivSDKPorts, error) {
		once.Do(func() { ports, portsErr = newCLIPixivSDKPorts(a) })
		return ports, portsErr
	}
	return pixivdeps.Data{
		Input:       a.in,
		Output:      a.out,
		ErrorOutput: a.errOut,
		UsageError:  newUsageError,
		Open: func(request pixivdeps.Request) (*pixiv.Client, error) {
			sdk, err := load()
			if err != nil {
				return nil, err
			}
			return sdk.open(request)
		},
		Pooled: func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			sdk, err := load()
			if err != nil {
				return err
			}
			return sdk.run(ctx, request, attempt)
		},
		JSONOut: func(override *bool) (bool, error) {
			sdk, err := load()
			if err != nil {
				return false, err
			}
			return sdk.jsonOut(override)
		},
	}
}

func (a app) pixivCommands() []*cobra.Command {
	return []*cobra.Command{
		pixivsearch.New(a.searchDeps()),
		pixivsearch.NewNovel(a.searchDeps()),
		pixivdetail.New(a.detailDeps()),
		pixivranking.New(a.pixivDataDeps()),
		pixivseries.New(a.pixivDataDeps()),
		pixivcomment.New(a.pixivDataDeps()),
		pixivrecommended.New(a.recommendedDeps()),
		pixivtimeline.New(a.pixivDataDeps()),
		pixivmypixiv.New(a.pixivDataDeps()),
		pixivuser.New(a.userDeps()),
		pixivbookmark.New(a.pixivDataDeps()),
		pixivfollow.New(a.pixivDataDeps()),
	}
}

// searchDeps 只把输入、输出、JSON 配置和公开 SDK 的 pooled read 端口交给
// search owner；该 owner 不导入旧 resource graph 或内部 service adapter。
func (a app) searchDeps() pixivsearch.Dependencies {
	data := a.pixivDataDeps()
	return pixivsearch.Dependencies{
		Input:      data.Input,
		Output:     data.Output,
		UsageError: data.UsageError,
		JSONOut:    data.JSONOut,
		Pooled: func(ctx context.Context, request pixivsearch.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return data.Pooled(ctx, pixivdeps.Request(request), attempt)
		},
	}
}

// recommendedDeps 为推荐 owner 提供同一条公开 SDK pooled-read 端口，保留账号池
// safe replay 与 config JSON 语义，不向 command 泄漏 root 内部状态。
func (a app) recommendedDeps() pixivrecommended.Dependencies {
	data := a.pixivDataDeps()
	return pixivrecommended.Dependencies{
		Input:      data.Input,
		Output:     data.Output,
		UsageError: data.UsageError,
		JSONOut:    data.JSONOut,
		Pooled: func(ctx context.Context, request pixivrecommended.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return data.Pooled(ctx, pixivdeps.Request(request), attempt)
		},
	}
}

// userDeps 保留 user group 的公开 SDK read 端口；嵌套 follow 子命令继续由它自己
// 的 owner 构造，避免 user package 反向依赖其他 command owner。
func (a app) userDeps() pixivuser.Dependencies {
	data := a.pixivDataDeps()
	return pixivuser.Dependencies{
		Input:      data.Input,
		Output:     data.Output,
		UsageError: data.UsageError,
		JSONOut:    data.JSONOut,
		Pooled: func(ctx context.Context, request pixivuser.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return data.Pooled(ctx, pixivdeps.Request(request), attempt)
		},
		Follow: func() *cobra.Command { return pixivfollow.New(data) },
	}
}

// detailDeps 是 vertical slice 的专属窄端口。detail owner 不依赖通用 Data，
// root 只负责把共享 SDK 端口适配为该 owner 的请求类型。
func (a app) detailDeps() pixivdetail.Dependencies {
	data := a.pixivDataDeps()
	return pixivdetail.Dependencies{
		Input:      a.in,
		Output:     a.out,
		UsageError: newUsageError,
		BuildRequest: func(cmd *cobra.Command, options pixivdetail.Options) (pixivdetail.Request, error) {
			proxyOverride, err := proxyOverrideFromFlags(cmd, proxyOptions{proxy: options.Proxy, noProxy: options.NoProxy})
			if err != nil {
				return pixivdetail.Request{}, err
			}
			return pixivdetail.Request{HTTPSProxyOverride: proxyOverride}, nil
		},
		JSONOut: data.JSONOut,
		Pooled: func(ctx context.Context, request pixivdetail.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return data.Pooled(ctx, pixivdeps.Request{
				UserID:             request.UserID,
				HTTPSProxyOverride: request.HTTPSProxyOverride,
			}, attempt)
		},
	}
}

func (a app) fanboxDataDeps() fanboxcommands.Data {
	var once sync.Once
	var service *fanboxapp.Facade
	var serviceErr error
	load := func() (*fanboxapp.Facade, error) {
		once.Do(func() { service, serviceErr = newCLIFanboxService(a) })
		return service, serviceErr
	}
	return fanboxcommands.Data{
		Reader:                a.in,
		Writer:                a.out,
		WrapUsage:             newUsageError,
		ServiceFactory:        load,
		AccountServiceFactory: func() (*fanboxaccount.Service, error) { return newCLIFanboxAccountService(a) },
		Browser:               fanboxBrowserSessionReader,
		Runtime:               a.runtimeConfig,
		CanPromptFn: func() bool {
			return canPrompt(a)
		},
		PromptSecretFn: func(message string) (string, error) {
			return promptSecret(a, message)
		},
		PromptConfirmFn: func(message string, defaultValue bool) (bool, error) {
			return promptConfirm(a, message, defaultValue)
		},
		RunMCPServer: func(cmd *cobra.Command, service *fanboxapp.Facade, proxy *string) error {
			ports := fanboxmcpserver.SDKPorts{
				OpenLease: func(ctx context.Context, account fanboxmcpserver.Account) (*lifecycle.Lease[*fanbox.Client], error) {
					return service.Open(ctx, fanboxapp.OpenRequest{ProxyOverride: account.HTTPSProxyOverride})
				},
			}
			return stdiotransport.RunStdio(cmd.Context(), fanboxmcpserver.NewWithProxy(ports, proxy))
		},
	}
}

func (a app) ConfigService() configapp.Store { return configapp.DefaultStore() }

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

func (a app) FanboxService() (*fanboxapp.Facade, error) {
	return newCLIFanboxService(a)
}

func (a app) AccountService() authcommands.AccountService {
	account, _, _ := newCLIAccountServices(a)
	return account
}

func (a app) LoginService() pixivaccount.LoginService {
	_, login, _ := newCLIAccountServices(a)
	return login
}

func (a app) DownloadService() downloader.DownloadService {
	return newCLIDownloadService()
}

func (app) WriteAuthExportBundle(path string, body []byte, force bool) error {
	return writeAuthExportBundle(path, body, force)
}

func (app) FanboxBrowserProvider() fanboxcommands.BrowserProvider {
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
	return newUpdateCommandCoordinator(proxy, out, errOut)
}

func (a app) LoadAutomaticUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) NewAutomaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	return newCLIAutomaticUpdateChecker(proxy)
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
	return request, nil
}

func (a app) RunMCP(ctx context.Context, request mcpcommands.Request) error {
	return runMCPServer(a, ctx, request)
}

func (a app) runPixivMCP(ctx context.Context, request mcpcommands.Request) error {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	if request.HTTPSProxyOverride != nil {
		if _, err := network.HTTPClient(*request.HTTPSProxyOverride); err != nil {
			return err
		}
	}
	ports, err := newCLIPixivSDKPorts(a)
	if err != nil {
		return err
	}
	account := mcpserver.Account{
		HTTPSProxyOverride: request.HTTPSProxyOverride,
	}
	manager := downloader.NewManager(nil, runtime.DownloadPath, runtime.FilenameTemplate)
	manager.SetDirectoryTemplate(runtime.DirectoryTemplate)
	server := mcpserver.NewWithSDKDownloadFactory(manager, func(client *pixiv.Client) mcpserver.DownloadManager {
		snapshot := downloader.NewManager(client, runtime.DownloadPath, runtime.FilenameTemplate)
		snapshot.SetDirectoryTemplate(runtime.DirectoryTemplate)
		return snapshot
	}, mcpserver.SDKPorts{
		Open: func(account mcpserver.Account) (*pixiv.Client, error) {
			return ports.open(pixivdeps.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
		},
		OpenLease: func(ctx context.Context, account mcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
			return ports.openLease(ctx, pixivdeps.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
		},
		Execute: func(ctx context.Context, account mcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return ports.run(ctx, pixivdeps.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride}, attempt)
		},
	}, account)
	return stdiotransport.RunStdio(ctx, server)
}

func (a app) runtimeConfig() (configapp.RuntimeConfig, error) {
	return loadCLIRuntimeConfig()
}

func (a app) startDiagnostics(cmd *cobra.Command, requirement requirements.Execution) error {
	if a.diagnostics == nil || !requirement.EnsureConfig || isQuietConfigCommand(cmd) {
		return nil
	}
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	if runtime.LogLevel != "debug" {
		return nil
	}
	module := coreDiagnostics.ModulePixivCLI
	if strings.HasPrefix(cmd.CommandPath(), "pixiv fanbox") {
		module = coreDiagnostics.ModuleFanboxCLI
	}
	presenter := clidiagnostics.NewPresenterWithFormat(a.errOut, runtime.LogFormat, nil)
	scoped := coreDiagnostics.WithScope(cmd.Context(), presenter, module, 0)
	a.diagnostics.ctx = scoped
	a.diagnostics.operation = cmd.CommandPath()
	a.diagnostics.presenter = presenter
	coreDiagnostics.Emit(scoped, coreDiagnostics.Event{
		Kind:      coreDiagnostics.EventStarted,
		Operation: cmd.CommandPath(),
	})
	cmd.SetContext(scoped)
	return nil
}

func (a app) finishDiagnostics(err error) error {
	if a.diagnostics == nil || a.diagnostics.presenter == nil {
		return err
	}
	event := coreDiagnostics.Event{Operation: a.diagnostics.operation}
	if err == nil {
		event.Kind = coreDiagnostics.EventCompleted
	} else {
		event.Kind = coreDiagnostics.EventFailed
		event.Reason = coreDiagnostics.ReasonCommandFailed
	}
	coreDiagnostics.Emit(a.diagnostics.ctx, event)
	if diagnosticErr := a.diagnostics.presenter.Err(); diagnosticErr != nil {
		if err == nil {
			return fmt.Errorf("write diagnostics: %w", diagnosticErr)
		}
		return errors.Join(err, fmt.Errorf("write diagnostics: %w", diagnosticErr))
	}
	return err
}

func isQuietConfigCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.CommandPath() == "pixiv config" || strings.HasPrefix(cmd.CommandPath(), "pixiv config ")
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

func defaultCLIRuntimeConfig() (configapp.RuntimeConfig, error) {
	snapshot, err := configapp.DefaultStore().Current()
	if err != nil {
		return configapp.RuntimeConfig{}, err
	}
	return snapshot.Runtime()
}

// loadCLIRuntimeConfig 是 composition root 的窄测试 seam。
var loadCLIRuntimeConfig = defaultCLIRuntimeConfig

type pixivSDKPorts struct {
	open      func(pixivdeps.Request) (*pixiv.Client, error)
	openLease func(context.Context, pixivdeps.Request) (*lifecycle.Lease[*pixiv.Client], error)
	execute   func(context.Context, pixivdeps.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	// pooled 保留为当前 CLI 测试 seam 的兼容字段；生产组合根只注入 execute。
	pooled  func(context.Context, pixivdeps.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	jsonOut func(*bool) (bool, error)
}

func (p pixivSDKPorts) run(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
	if p.execute != nil {
		return p.execute(ctx, request, attempt)
	}
	if p.pooled != nil {
		return p.pooled(ctx, request, attempt)
	}
	return errors.New("pixiv sdk execution port is not configured")
}

func openCLIAuthDatabase() (*database.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	if err != nil {
		return nil, err
	}
	return db, nil
}

// configDefaultStore adapts the concrete CLI config store to the narrow
// account-domain default selection ports.
type configDefaultStore struct{ store configapp.Store }

func (s configDefaultStore) ReadPixivDefaultUserID() (int64, bool, error) {
	return s.store.ReadPixivDefaultUserID()
}

func (s configDefaultStore) SetPixivDefaultUserID(userID int64) error {
	return s.store.SetPixivDefaultUserID(userID)
}

func (s configDefaultStore) ClearPixivDefaultUserID() error {
	return s.store.ClearPixivDefaultUserID()
}

func (s configDefaultStore) ReadFanboxDefaultUserID() (int64, bool, error) {
	return s.store.ReadFanboxDefaultUserID()
}

func (s configDefaultStore) SetFanboxDefaultUserID(userID int64) error {
	return s.store.SetFanboxDefaultUserID(userID)
}

func (s configDefaultStore) ClearFanboxDefaultUserID() error {
	return s.store.ClearFanboxDefaultUserID()
}

func pixivOptionsFromRequest(request pixivdeps.Request, loadRuntime func() (configapp.RuntimeConfig, error)) (pixiv.Options, error) {
	if loadRuntime == nil {
		return pixiv.Options{}, errors.New("pixiv runtime loader is not configured")
	}
	runtime, err := loadRuntime()
	if err != nil {
		return pixiv.Options{}, err
	}
	options := pixiv.Options{}
	options.Pacing.MinInterval = runtime.RequestInterval
	proxyValue := request.HTTPSProxyOverride
	if proxyValue == nil {
		if runtime.PixivNetwork.ProxyURL.Present {
			value := runtime.PixivNetwork.ProxyURL.Value
			proxyValue = &value
		} else {
			value := runtime.HTTPSProxy
			proxyValue = &value
		}
	}
	if proxyValue != nil {
		httpClient, err := network.HTTPClient(*proxyValue)
		if err != nil {
			return pixiv.Options{}, err
		}
		options.HTTPClient = httpClient
	}
	return options, nil
}

// fanboxsdkOptions names the exact public SDK options type locally so the
// resource graph remains the only place translating one config snapshot.
type fanboxsdkOptions = fanbox.Options

func fanboxOptionsFromRuntime(loadRuntime func() (configapp.RuntimeConfig, error)) (fanboxsdkOptions, error) {
	cfg, err := loadRuntime()
	if err != nil {
		return fanboxsdkOptions{}, err
	}
	options := fanboxsdkOptions{}
	if cfg.FanboxNetwork.ProxyURL.Present {
		options.ProxyURL = cfg.FanboxNetwork.ProxyURL.Value
	} else {
		options.ProxyURL = cfg.HTTPSProxy
	}
	if cfg.FanboxNetwork.UserAgent.Present {
		options.UserAgent = cfg.FanboxNetwork.UserAgent.Value
	}
	if cfg.FanboxFlareSolverr != nil {
		options.FlareSolverr = &fanbox.FlareSolverrOptions{
			URL:      cfg.FanboxFlareSolverr.URL,
			ProxyURL: cfg.FanboxFlareSolverr.ProxyURL,
		}
	}
	return options, nil
}

func writeAuthExportBundle(path string, body []byte, force bool) error {
	return filesecret.WriteSecretFile(path, body, force)
}
