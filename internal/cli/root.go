// Package cli 是 CLI 组合根：它把命令树、执行生命周期与生产依赖装配
// 组装成一个可运行的 pixiv 命令（以及 MCP stdio/HTTP runtime）。
//
// 本文件持有命令树本身：app、命令树构建、常用 positional-args 与
// prompt/flag 绑定等所有 command owner 共享的接线。
//
// 其余两个同包文件按职责拆分，便于在所属文件内理解一条完整流程：
//   - execution.go：执行入口、退出码、资源关闭、启动与结束处理；
//   - composition.go：Pixiv/FANBOX/MCP 的生产依赖装配。
package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/follow"
	command "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mypixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/ranking"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/series"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/timeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/update"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	selfupdate "github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"
	thirdparty "github.com/FlanChanXwO/pixiv-cli/third_party"
	"github.com/spf13/cobra"
)

type app struct {
	in                  io.Reader
	out                 io.Writer
	errOut              io.Writer
	pipelineSignal      *brokenPipeSignalState
	mcpBrokenPipeSignal *brokenPipeSignalState
	closeState          *closeState
	logs                *logState
}

// logger 返回本次执行的 logger；quiet 命令使用构造时注入的丢弃 logger。
func (a app) logger() *slog.Logger { return a.logs.logger }

func (a app) newRootCommand() *cobra.Command {
	var licenses bool
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		Version:       version.Current().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if licenses {
				return thirdparty.WriteNotices(cmd.OutOrStdout())
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			requirement := command.For(cmd)
			a.enablePipelineSignal(cmd)
			if !cmd.Flags().Changed("port") {
				a.enableMCPBrokenPipeSignal(requirement)
			}
			if requirement.StartupHooks {
				if err := selfupdate.CleanupPendingWindowsUpdate(); err != nil {
					return &startupError{err: fmt.Errorf("clean pending update: %w", err)}
				}
				if loginhelper.AutomaticPersistentHandlerSupported() {
					if err := loginhelper.EnsurePersistentIfNeeded(cmd.Context()); err != nil {
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
			if command.For(cmd).AutomaticUpdate {
				update.RunAutomaticCheck(cmd, a)
			}
		},
	}
	command.Bind(cmd, command.Execution{})
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return normalizeFlagError(err) })
	cmd.Flags().BoolVar(&licenses, "licenses", false, "Print embedded third-party license notices")
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	// 不把 Cobra 的 help handler 作为公开 root subcommand；根命令仍通过
	// --help 与 --version 提供内置帮助/版本入口。
	cmd.SetHelpCommand(&cobra.Command{Use: "_help [command]", Hidden: true})
	cmd.CompletionOptions.DisableDefaultCmd = true
	env, exec := a.commandEnv(), a.commandExecutor()
	cmd.AddCommand(a.newAuthCommand())
	config.Register(cmd, a)
	cmd.AddCommand(a.pixivCommands(env, exec)...)
	cmd.AddCommand(a.newDownloadCommand())
	cmd.AddCommand(a.newFanboxCommandGroup(env))
	mcp.Register(cmd, a)
	update.Register(cmd, a)
	return cmd
}

func (a app) pixivCommands(env command.Env, exec pixivcmd.Executor) []*cobra.Command {
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
		user.New(env, pixivcmd.New(env.Out, exec), func() *cobra.Command { return follow.New(env, exec) }),
		bookmark.New(env, exec),
		follow.New(env, exec),
	}
}

// 以下 host methods 是根控制器对命令子包暴露的窄输出/依赖端口。子包不导入
// internal/cli，也不接触 app 的内部状态字段。
func (a app) Output() io.Writer { return a.out }

func (a app) Input() io.Reader { return a.in }

func (a app) ErrorOutput() io.Writer { return a.errOut }

func (a app) PrintJSON(value any) error { return command.Env{Out: a.out}.PrintJSON(value) }

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

func (a app) BindProxyFlags(cmd *cobra.Command, options *mcp.ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&options.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&options.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a app) ClientRequest(cmd *cobra.Command, options mcp.ProxyOptions) (mcp.Request, error) {
	proxyOverride, err := command.Env{}.ProxyOverride(cmd, command.ProxyOptions{Proxy: options.Proxy, NoProxy: options.NoProxy})
	if err != nil {
		return mcp.Request{}, err
	}
	request := mcp.Request{HTTPSProxyOverride: proxyOverride}
	return request, nil
}

func (a app) runtimeConfig() (settings.RuntimeConfig, error) {
	return defaultCLIRuntimeConfig()
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
