// Package cli 是 CLI 组合根：它把命令树、执行生命周期与生产依赖装配
// 组装成一个可运行的 pixiv 命令（以及 MCP stdio runtime）。
//
// 本文件持有命令树本身：app、命令树构建、常用 positional-args 与
// prompt/flag 绑定等所有 command owner 共享的接线。
//
// 其余两个同包文件按职责拆分，便于在所属文件内理解一条完整流程：
//   - execution.go：执行入口、退出码、资源关闭、启动与结束处理；
//   - composition.go：Pixiv/FANBOX/MCP 的生产依赖装配。
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	configcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/config"
	fanboxcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/auth"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/download"
	fanboxmcpcommand "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/mcp"
	fanboxpost "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/post"
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
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	stdiotransport "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/assembly"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
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
	runMCPStdio          = stdiotransport.RunStdio
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
	newCLIReverseSearch   = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		return reverseassembly.New(options)
	}
	newCLIMCPReverseSearch = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		return reverseassembly.New(options)
	}
)

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

func outputIsTTY(writer io.Writer) bool {
	file, ok := writer.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

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

func (a app) runtimeConfig() (configapp.RuntimeConfig, error) {
	return loadCLIRuntimeConfig()
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
