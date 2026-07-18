package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

type app struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	logger *slog.Logger
}

type commandOptions struct {
	proxyOptions
	profile          string
	uid              string
	refreshToken     string
	downloadPath     string
	filenameTemplate string
	jsonOut          bool
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

type clientConfig struct {
	config.RuntimeConfig
}

var (
	runMCPServer                = runMCP
	newCLIServices              = bootstrap.NewServices
	cleanupPendingWindowsUpdate = update.CleanupPendingWindowsUpdate
)

func Run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return RunContext(context.Background(), args, in, out, errOut)
}

// RunContext 让嵌入式调用方把取消信号传到每一条网络数据命令。
func RunContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	if err := cleanupPendingWindowsUpdate(); err != nil {
		fmt.Fprintf(errOut, "clean pending update: %v\n", err)
		return 1
	}
	logger, err := applicationLoggerForArgs(args, errOut)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	a := app{in: in, out: out, errOut: errOut, logger: logger}
	cmd := a.newRootCommand()
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	cmd.SetContext(ctx)
	operation := cmd.CommandPath()
	if target, _, findErr := cmd.Find(args[1:]); findErr == nil && target != nil {
		operation = target.CommandPath()
	}
	started := time.Now()
	err = cmd.Execute()
	// Cobra 将根帮助和 `pixiv --version` 都报告为 `pixiv`；后者是稳定的单行
	// machine-readable 输出，不能混入日志，前者则仍应留下命令诊断。
	a.commandLog(operation, started, err, len(args) == 2 && args[1] == "--version")
	return a.exit(err)
}

// applicationLoggerForArgs 在 Cobra 解析前识别凭据导出前缀，使成功、help 与参数
// 错误路径都不依赖 config/logging 环境。其他命令仍沿用完整配置型 logger。
func applicationLoggerForArgs(args []string, errOut io.Writer) (*slog.Logger, error) {
	if len(args) >= 3 && args[1] == "auth" && args[2] == "export" {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), nil
	}
	return bootstrap.NewApplicationLogger(errOut)
}

// commandLog 仅记录命令名和稳定结果，不能记录 args：其中可能含 refresh token、
// OAuth code 或本地路径。stdout 始终只保留命令业务输出。
func (a app) commandLog(operation string, started time.Time, err error, suppress bool) {
	if a.logger == nil {
		return
	}
	// 纯本地 metadata/config 命令的 stdout/stderr 是稳定机器接口；它们不触发业务
	// 网络流程，因此不额外产生日志。根命令覆盖 `pixiv --version` 的 Cobra 路径。
	if suppress || operation == "pixiv auth export" || strings.HasPrefix(operation, "pixiv config") || strings.HasPrefix(operation, "pixiv version") || strings.HasPrefix(operation, "pixiv update") {
		return
	}
	result, code, backend, status := "success", "", "local", 0
	level := slog.LevelInfo
	var illustID, userID int64
	if err != nil {
		result = "error"
		level = slog.LevelError
		var typed *sdk.Error
		if errors.As(err, &typed) {
			code, backend, status = string(typed.Code), string(typed.Backend), typed.UpstreamStatus
			if backend == "" {
				backend = "local"
			}
			illustID, userID = typed.IllustID, typed.UserID
		}
	}
	attrs := []slog.Attr{
		slog.String("component", "cli"), slog.String("operation", operation), slog.String("backend", backend),
		slog.Duration("duration", time.Since(started)), slog.String("result", result), slog.String("error_code", code), slog.Int("status", status),
	}
	if illustID != 0 {
		attrs = append(attrs, slog.Int64("illust_id", illustID))
	}
	if userID != 0 {
		attrs = append(attrs, slog.Int64("user_id", userID))
	}
	a.logger.LogAttrs(nil, level, "pixiv operation", attrs...)
}

func (a app) exit(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

func runMCP(ctx context.Context, errOut io.Writer, proxyOverride *string) error {
	return bootstrap.RunMCP(ctx, errOut, proxyOverride)
}

func (a app) services() application.Services {
	if a.logger != nil {
		return newCLIServices(a.logger)
	}
	// 仅供包内直接构造 app 的测试；生产入口始终显式创建根 logger。
	return newCLIServices(slog.New(slog.NewTextHandler(io.Discard, nil)))
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
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			a.checkAutomaticUpdate(cmd)
		},
	}
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(
		a.newAccountCommand(),
		a.newConfigCommand(),
		a.newSearchCommand(),
		a.newSearchOptionsCommand(),
		a.newDetailCommand(),
		a.newRankingCommand(),
		a.newRecommendedCommand(),
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
			return runMCPServer(cmd.Context(), a.errOut, proxyOverride)
		},
	}
	a.bindProxyFlags(cmd, &opts)
	return cmd
}

func (a app) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.uid, "uid", "", "Pixiv UID from auth.json")
	flags.StringVar(&opts.profile, "profile", "", "deprecated alias for --uid")
	_ = flags.MarkDeprecated("profile", "use --uid instead")
	flags.StringVar(&opts.refreshToken, "refresh-token", "", "Pixiv App API refresh token")
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

func (a app) bindProxyFlags(cmd *cobra.Command, opts *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear HTTP(S) proxy for this command")
}

func (a app) clientRequest(cmd *cobra.Command, opts commandOptions, needsAuth bool) (application.ClientRequest, error) {
	userID := int64(0)
	if cmd.Flags().Changed("uid") && cmd.Flags().Changed("profile") {
		return application.ClientRequest{}, fmt.Errorf("use either --uid or deprecated --profile, not both")
	}
	rawUID := opts.uid
	if rawUID == "" {
		rawUID = opts.profile
	}
	if rawUID != "" {
		parsed, err := application.ParseUID(rawUID)
		if err != nil {
			return application.ClientRequest{}, err
		}
		userID = parsed
	}
	req := application.ClientRequest{
		UserID:       userID,
		RefreshToken: opts.refreshToken,
		NeedsAuth:    needsAuth,
	}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return application.ClientRequest{}, err
	}
	req.HTTPSProxyOverride = proxyOverride
	if cmd.Flags().Changed("download-path") {
		req.DownloadPathOverride = &opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		req.FilenameTemplateOverride = &opts.filenameTemplate
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
