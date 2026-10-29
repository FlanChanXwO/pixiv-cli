// Package cli 把命令树、执行生命周期和生产依赖装配成 pixiv 命令，也启动 MCP 的 stdio 与 HTTP runtime。
//
// 包的组成：[Run] 是唯一的执行入口，负责退出码与资源关闭。
// 命令树在 root.go 组装，生产依赖在 composition.go 打开。
// 配置到 SDK 选项的翻译在 composition_options.go，MCP runtime 在 composition_mcp.go，
// 反向搜图在 composition_search.go。
// 打开的数据库与 SDK 资源登记到本次执行的 closeState，由 [Run] 按逆序关闭。
package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/detail"
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
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/internal/urlscheme"
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
			requirement := environment.For(cmd)
			// 显式 --ndjson 在执行前就可知；自动 NDJSON 由 command owner 经 AutoNDJSON 决定，
			// 因此这里只处理前者。
			if commandWritesNDJSON(cmd) {
				a.enablePipelineSignal()
			}
			if !cmd.Flags().Changed("port") {
				a.enableMCPBrokenPipeSignal(requirement)
			}
			if requirement.StartupHooks {
				if err := selfupdate.CleanupPendingWindowsUpdate(); err != nil {
					return &startupError{err: fmt.Errorf("clean pending update: %w", err)}
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
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return normalizeFlagError(err) })
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	// 不把 Cobra 的 help handler 作为公开 root subcommand；根命令仍通过
	// --help 与 --version 提供内置帮助/版本入口。
	cmd.SetHelpCommand(&cobra.Command{Use: "_help [command]", Hidden: true})
	cmd.CompletionOptions.DisableDefaultCmd = true
	env, exec := a.commandEnv(), a.commandExecutor()
	cmd.AddCommand(a.newAuthCommand())
	cmd.AddCommand(whoami.New(env, a.currentPixivUser))
	cmd.AddCommand(config.New(env, settings.DefaultStore()))
	cmd.AddCommand(a.pixivCommands(env, exec)...)
	cmd.AddCommand(a.newDownloadCommand())
	cmd.AddCommand(a.newFanboxCommandGroup(env))
	cmd.AddCommand(mcp.New(env, a.runPixivMCP, initMCPAuth))
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
	return defaultCLIRuntimeConfig()
}
