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
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		Version:       version.Current().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				update.RunAutomaticCheck(cmd, a.errOut, a.runtimeConfig)
			}
		},
	}
	command.Bind(cmd, command.Execution{})
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return normalizeFlagError(err) })
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	// 不把 Cobra 的 help handler 作为公开 root subcommand；根命令仍通过
	// --help 与 --version 提供内置帮助/版本入口。
	cmd.SetHelpCommand(&cobra.Command{Use: "_help [command]", Hidden: true})
	cmd.CompletionOptions.DisableDefaultCmd = true
	env, exec := a.commandEnv(), a.commandExecutor()
	cmd.AddCommand(a.newAuthCommand())
	cmd.AddCommand(config.New(env, settings.DefaultStore()))
	cmd.AddCommand(a.pixivCommands(env, exec)...)
	cmd.AddCommand(a.newDownloadCommand())
	cmd.AddCommand(a.newFanboxCommandGroup(env))
	cmd.AddCommand(mcp.New(env, a.runPixivMCP, initMCPAuth))
	cmd.AddCommand(update.New(env, a.runtimeConfig))
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

func (a app) runtimeConfig() (settings.RuntimeConfig, error) {
	return defaultCLIRuntimeConfig()
}
