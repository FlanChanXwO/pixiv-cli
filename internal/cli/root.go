// Package cli 把命令树、执行生命周期和生产依赖装配成 pixiv 命令，也启动 MCP 的 stdio 与 HTTP runtime。
//
// 包的组成：[Run] 是唯一的执行入口，负责退出码与资源关闭；本文件同时装配命令树、
// 生产依赖、配置到 SDK 选项的翻译与反向搜图端口。MCP runtime 在 internal/cli/mcp。
// 打开的数据库与 SDK 资源登记到本次执行的 [lifecycle.CloseState]，由 [Run] 按逆序关闭。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"syscall"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	fanboxcli "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/browse"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/follow"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mypixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/ranking"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/series"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/timeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/update"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/whoami"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/FlanChanXwO/pixiv-cli/internal/urlscheme"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

type app struct {
	in             io.Reader
	out, errOut    io.Writer
	pipelineSignal environment.BrokenPipe
	mcpSignal      environment.BrokenPipe
	closeState     *lifecycle.CloseState
	local          *session.Local
	logs           *logging.State
}

func (a app) logger() *slog.Logger { return a.logs.Logger }

func (a app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		Version:       version.Current().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			requirement := environment.For(cmd)
			// 显式 --ndjson 在执行前就可知；自动 NDJSON 由 command owner 经 AutoNDJSON 决定。
			if environment.WritesNDJSON(cmd) {
				a.pipelineSignal.Enable()
			}
			if !cmd.Flags().Changed("port") && requirement.MCP {
				a.mcpSignal.Enable()
			}
			if requirement.StartupHooks {
				if err := selfupdate.CleanupPendingWindowsUpdate(); err != nil {
					return &environment.StartupError{Err: fmt.Errorf("clean pending update: %w", err)}
				}
				if urlscheme.AutomaticPersistentHandlerSupported() {
					if err := urlscheme.EnsurePersistentIfNeeded(cmd.Context()); err != nil {
						// 这项桌面集成不能阻断原命令，也不能把系统/本机路径写入 stderr。
						fmt.Fprintln(a.errOut, "warning: persistent pixiv:// callback handler was not initialized")
					}
				}
			}
			if requirement.EnsureConfig {
				if err := settings.DefaultStore().EnsureDefaultConfigFile(); err != nil {
					return err
				}
				if err := a.startLogging(cmd, requirement); err != nil {
					return err
				}
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if environment.For(cmd).AutomaticUpdate {
				update.RunAutomaticCheck(cmd, a.errOut, a.runtimeConfig)
			}
		},
	}
	environment.Bind(cmd, environment.Execution{})
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return environment.NormalizeFlagError(err) })
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	// 不把 Cobra 的 help handler 作为公开 root subcommand；根命令仍通过
	// --help 与 --version 提供内置帮助/版本入口。
	cmd.SetHelpCommand(&cobra.Command{Use: "_help [command]", Hidden: true})
	cmd.CompletionOptions.DisableDefaultCmd = true
	env, exec := a.commandEnv(), a.local.Execute
	cmd.AddCommand(a.newAuthCommand())
	cmd.AddCommand(whoami.New(env, a.local.CurrentUser))
	cmd.AddCommand(config.New(env, settings.DefaultStore()))
	cmd.AddCommand(a.pixivCommands(env, exec)...)
	cmd.AddCommand(download.New(env, exec, download.RuntimeFrom(a.runtimeConfig), downloader.NewService))
	cmd.AddCommand(a.newFanboxCommandGroup(env))
	cmd.AddCommand(mcp.New(env, a.runPixivMCP, mcp.InitAuth))
	cmd.AddCommand(update.New(env, a.runtimeConfig))
	return cmd
}

func (a app) pixivCommands(env environment.Env, exec session.Executor) []*cobra.Command {
	reverseSearch := a.reverseSearchPort()
	return []*cobra.Command{
		search.New(env, exec, reverseSearch),
		search.NewNovel(env, exec, reverseSearch),
		detail.New(env, exec),
		ranking.New(env, exec),
		series.New(env, exec),
		comment.New(env, exec),
		recommended.New(env, exec),
		timeline.New(env, exec),
		mypixiv.New(env, exec),
		user.New(env, session.NewRunner(env.Out, exec), func() *cobra.Command { return follow.New(env, exec) }),
		bookmark.New(env, exec),
		follow.New(env, exec),
	}
}

func (a app) runtimeConfig() (settings.RuntimeConfig, error) {
	snapshot, err := settings.DefaultStore().Current()
	if err != nil {
		return settings.RuntimeConfig{}, err
	}
	return snapshot.Runtime()
}

// Run 执行一条 CLI 命令并返回进程退出码，阶段顺序固定：
//
// 1. 只用输入输出流和资源登记表构造 app，不打开资源；
// 2. 装配命令树并绑定流与参数；
// 3. 执行命令，资源在命令真正请求时才创建，help、version 不会提前打开数据库或网络 client；
// 4. 无论成功与否都按登记逆序关闭资源，关闭错误并入退出原因；
// 5. 记录失败并映射退出码。
//
// 普通 NDJSON 输出与 MCP stdio 各自持有平台的 SIGPIPE 控制器：前者的 EPIPE 表示下游
// 正常停止，后者属于 JSON-RPC transport 错误。
func Run(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	closeState := &lifecycle.CloseState{}
	a := app{in: in, out: out, errOut: errOut, closeState: closeState, logs: &logging.State{Logger: logging.Discard()}}
	a.local = session.NewLocal(closeState, a.pixivSettings, a.fanboxOptions, a.poolConfig)
	defer a.pipelineSignal.Restore()
	defer a.mcpSignal.Restore()
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
	// 即使执行失败也要关闭资源，避免"命令成功但资源未释放"被报成成功。
	if closeErr := closeState.Close(); closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	ndjsonOutput := environment.WritesNDJSON(target) || environment.OutputNDJSON(target)
	if err != nil && !(ndjsonOutput && errors.Is(err, syscall.EPIPE)) && a.logs.Ctx != nil {
		attrs := append([]slog.Attr{slog.Any(logging.AttrError, err)}, logging.OriginAttr(err)...)
		a.logs.Logger.LogAttrs(a.logs.Ctx, a.logs.FailureLevel, "Command failed", attrs...)
	}
	return a.exitWithNDJSONScope(err, ndjsonOutput)
}

func (a app) exitWithNDJSONScope(err error, ndjsonOutput bool) int {
	if err == nil {
		return 0
	}
	if ndjsonOutput && errors.Is(err, syscall.EPIPE) {
		return 0
	}
	var startupErr *environment.StartupError
	if errors.As(err, &startupErr) {
		fmt.Fprintln(a.errOut, startupErr.Err)
		return 1
	}
	var usageErr *environment.UsageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(a.errOut, "error:", usageErr.Err)
		return 2
	}
	var pipelineErr *record.PipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

func (a app) startLogging(cmd *cobra.Command, requirement environment.Execution) error {
	if !requirement.EnsureConfig || isQuietConfigCommand(cmd) {
		return nil
	}
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	call, err := logging.Start(cmd.Context(), a.errOut, cmd.CommandPath(), string(runtime.LogLevel), string(runtime.LogFormat))
	if err != nil {
		return err
	}
	*a.logs = call
	cmd.SetContext(call.Ctx)
	return nil
}

func isQuietConfigCommand(cmd *cobra.Command) bool {
	return cmd.CommandPath() == "pixiv config" || strings.HasPrefix(cmd.CommandPath(), "pixiv config ")
}

// enablePipelineSignal 安装 NDJSON 专用的 SIGPIPE 策略。command owner 只在确定输出记录流时
// 调用，其他命令保留平台的默认 SIGPIPE 行为，否则普通输出被关闭管道截断会被误判为成功。
func (a app) commandEnv() environment.Env {
	return environment.New(a.in, a.out, a.errOut, environment.NewUsageError, a.pipelineSignal.Enable, func(override *bool) (bool, error) {
		if override != nil {
			return *override, nil
		}
		runtime, err := a.runtimeConfig()
		if err != nil {
			return false, err
		}
		return runtime.OutputJSON, nil
	})
}

// newAuthCommand 组装 pixiv auth 组；账号服务延迟到第一次访问 auth 子命令时才打开数据库。
func (a app) newAuthCommand() *cobra.Command {
	return auth.New(a.commandEnv(), auth.SupportFor(a.local.PixivAccounts, a.runtimeConfig, secretfile.WriteSecretFile))
}

func (a app) newFanboxCommandGroup(env environment.Env) *cobra.Command {
	children := []*cobra.Command{
		fanboxauth.New(env, a.local.FanboxAccounts, fanboxauth.SystemBrowserProvider{}),
		fanboxdownload.New(env, a.local.Fanbox, func() (string, error) {
			runtime, err := a.runtimeConfig()
			return runtime.DownloadPath, err
		}),
	}
	return fanboxcli.NewCommand(env, append(children, browse.Commands(env, a.local.Fanbox)...))
}

func (a app) pixivSettings(request session.ProxyRequest) (session.PixivSettings, error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return session.PixivSettings{}, err
	}
	return session.PixivOptions(runtime, request, a.logger())
}

func (a app) fanboxOptions() (fanbox.Options, error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return fanbox.Options{}, err
	}
	return session.FanboxOptions(runtime, a.logger()), nil
}

func (a app) poolConfig() (session.PoolConfig, error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return session.PoolConfig{}, err
	}
	return session.PoolConfig{Enabled: runtime.AccountPool.Enabled, Strategy: string(runtime.AccountPool.Strategy)}, nil
}

// runPixivMCP 构造 MCP 启动快照，把反向搜图与本地账号端口注入 mcp.Runtime。
func (a app) runPixivMCP(ctx context.Context, request mcp.Request) error {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	reverseSearch, err := newMCPReverseSearchPorts(runtime, request)
	if err != nil {
		return err
	}
	return mcp.Runtime{
		In:            a.in,
		Out:           a.out,
		ErrOut:        a.errOut,
		Logger:        a.logger,
		Settings:      a.runtimeConfig,
		Downloads:     downloader.NewService(),
		Local:         a.local,
		Fanbox:        a.local.NewFanbox,
		ReverseSearch: reverseSearch,
		PixivAccounts: a.local.PixivAccounts,
		CloseState:    a.closeState,
	}.Run(ctx, request)
}

// newReverseSearcher 固定 standard/ascii2d 两个代理网络面、凭据和 solver 配置。
func newReverseSearcher(runtime settings.RuntimeConfig, proxyOverride *string) (reversesearch.Searcher, error) {
	standardProxy, ascii2dProxy := runtime.HTTPSProxy, runtime.HTTPSProxy
	if runtime.ReverseSearchNetwork.ProxyURL.Present {
		ascii2dProxy = runtime.ReverseSearchNetwork.ProxyURL.Value
	}
	if proxyOverride != nil {
		standardProxy, ascii2dProxy = *proxyOverride, *proxyOverride
	}
	var flareSolverr *assembly.FlareSolverrOptions
	if runtime.ReverseSearchFlareSolverr != nil {
		flareSolverr = &assembly.FlareSolverrOptions{URL: runtime.ReverseSearchFlareSolverr.URL, ProxyURL: runtime.ReverseSearchFlareSolverr.ProxyURL}
	}
	return assembly.New(assembly.Options{
		Proxy:        standardProxy,
		ASCII2DProxy: &ascii2dProxy,
		UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
		SauceNAOKey:  runtime.SauceNAOAPIKey,
		FlareSolverr: flareSolverr,
	})
}

// newMCPReverseSearchPorts 在 MCP 启动时创建一次反向搜图 Facade，并固定 provider 与
// 两个代理网络面；后续 tool input 只能覆盖 provider，不能改变传输或凭据依赖。
func newMCPReverseSearchPorts(runtime settings.RuntimeConfig, request mcp.Request) (pixiv.ReverseSearchPorts, error) {
	searcher, err := newReverseSearcher(runtime, request.HTTPSProxyOverride)
	if err != nil {
		return pixiv.ReverseSearchPorts{}, err
	}
	provider := reversesearch.Provider(runtime.ReverseSearchProvider)
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	return pixiv.ReverseSearchPorts{Searcher: searcher, Provider: provider, PixivOnly: runtime.ReverseSearchPixivOnly}, nil
}

// reverseSearchPort 为每次命令树构造反向搜图端口；每次调用按当前配置重新读取 provider 和代理。
func (a app) reverseSearchPort() search.ReverseSearchPort {
	return func(ctx context.Context, request search.ReverseSearchRequest) (reversesearch.Response, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return reversesearch.Response{}, err
		}
		provider := reversesearch.Provider(runtime.ReverseSearchProvider)
		if request.Provider != "" {
			provider = request.Provider
		}
		searcher, err := newReverseSearcher(runtime, request.HTTPSProxyOverride)
		if err != nil {
			return reversesearch.Response{}, err
		}
		if closer, ok := searcher.(reversesearch.Closer); ok {
			a.closeState.Add(closer.Close)
		}
		return searcher.Search(ctx, reversesearch.Request{Source: request.Source, Provider: provider, PixivOnly: runtime.ReverseSearchPixivOnly})
	}
}
