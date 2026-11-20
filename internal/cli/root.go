// Package cli 把命令树、执行生命周期和生产依赖装配成 pixiv 命令，也启动 MCP 的 stdio 与 HTTP runtime。
//
// 包的组成：[Run] 是唯一的执行入口，负责退出码与资源关闭；本文件同时装配命令树、
// 生产依赖、配置到 SDK 选项的翻译与反向搜图端口。MCP runtime 在 internal/cli/mcp。
// 打开的 SDK 与其他需关闭的资源登记到本次执行的 [lifecycle.CloseState]，由 [Run] 按逆序关闭。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/comment"
	configcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/config"
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
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/vector"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/whoami"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/FlanChanXwO/pixiv-cli/internal/urlscheme"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type app struct {
	language       i18n.Locale
	in             io.Reader
	out, errOut    io.Writer
	pipelineSignal *environment.BrokenPipe
	mcpSignal      *environment.BrokenPipe
	closeState     *lifecycle.CloseState
	storeSlot      chan struct{}
	store          *account.Store
	gate           *account.Gate
	readsMu        sync.Mutex
	reads          *account.ReadSessions
	logs           *logging.State
}

func (a *app) logger() *slog.Logger { return a.logs.Logger }

func (a *app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "help.pixiv_cli_and_mcp_server",
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
					return &environment.StartupError{Err: i18n.Errorf("error.clean_pending_update", err)}
				}
				if urlscheme.AutomaticPersistentHandlerSupported() {
					if err := urlscheme.EnsurePersistentIfNeeded(cmd.Context()); err != nil {
						// 这项桌面集成不能阻断原命令，也不能把系统/本机路径写入 stderr。
						fmt.Fprintln(a.errOut, a.language.Text("output.warning_persistent_pixiv_callback_handler_was_not_initialized_351"))
					}
				}
			}
			if requirement.EnsureConfig {
				if err := (config.Store{Files: secretfile.ConfigFiles{}}).EnsureDefaultConfigFile(); err != nil {
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
				update.RunAutomaticCheck(cmd, a.errOut, a.language, a.runtimeConfig)
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
	env := a.commandEnv()
	cmd.AddCommand(a.newAuthCommand())
	cmd.AddCommand(whoami.New(env, a.currentUser))
	cmd.AddCommand(configcmd.New(env, config.Store{Files: secretfile.ConfigFiles{}}))
	cmd.AddCommand(a.pixivCommands(env)...)
	cmd.AddCommand(download.New(env, a.executePixiv, download.RuntimeFrom(a.runtimeConfig)))
	cmd.AddCommand(a.newFanboxCommandGroup(env))
	cmd.AddCommand(mcp.New(env, a.runPixivMCP, mcp.InitAuth))
	cmd.AddCommand(update.New(env, a.runtimeConfig))
	environment.LocalizeCommands(cmd, a.language)
	return cmd
}

func (a *app) pixivCommands(env environment.Env) []*cobra.Command {
	reverseSearch := a.reverseSearchPort()
	return []*cobra.Command{
		search.New(env, a.executePixiv, reverseSearch),
		search.NewNovel(env, a.executePixiv),
		detail.New(env, a.executePixiv),
		ranking.New(env, a.executePixiv),
		series.New(env, a.executePixiv),
		comment.New(env, a.executePixiv),
		recommended.New(env, a.executePixiv),
		timeline.New(env, a.executePixiv),
		mypixiv.New(env, a.executePixiv),
		user.New(env, a.executePixiv, func() *cobra.Command { return follow.New(env, a.executePixiv) }),
		vector.New(env, config.Store{Files: secretfile.ConfigFiles{}}, a.openVectorPixiv, a.searchVector),
		bookmark.New(env, a.executePixiv),
		follow.New(env, a.executePixiv),
	}
}

func (a *app) runtimeConfig() (config.Config, error) {
	return (config.Store{Files: secretfile.ConfigFiles{}}).Current()
}

func (a *app) openAccounts(ctx context.Context) (*account.Store, error) {
	// 用容量为 1 的 channel 串行初始化：排队的请求能响应各自的取消，Mutex 做不到。
	select {
	case a.storeSlot <- struct{}{}:
		defer func() { <-a.storeSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if a.store != nil {
		return a.store, nil
	}
	directory, err := config.AppDataDir()
	if err != nil {
		return nil, i18n.Errorf("error.determine_home_directory", err)
	}
	store, err := account.Open(ctx, directory)
	if err != nil {
		return nil, err
	}
	a.store = store
	return store, nil
}

func (a *app) pixivAccounts(ctx context.Context) (*auth.Accounts, auth.LoginService, error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return nil, auth.LoginService{}, err
	}
	service := auth.NewAccounts(store, config.Store{Files: secretfile.ConfigFiles{}})
	return service, auth.LoginService{Pixiv: service}, nil
}

func (a *app) currentUser(ctx context.Context) (*auth.AccountSummary, error) {
	service, _, err := a.pixivAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return service.CurrentUser(ctx)
}

func (a *app) fanboxAccounts(ctx context.Context) (*fanboxauth.Accounts, error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return fanboxauth.NewAccounts(store, config.Store{Files: secretfile.ConfigFiles{}}, a.fanboxOptions), nil
}

func (a *app) openFanbox(ctx context.Context, proxy *string) (*lifecycle.Lease[*fanbox.Client], error) {
	service, err := a.fanboxAccounts(ctx)
	if err != nil {
		return nil, err
	}
	client, err := service.OpenClient(ctx, proxy)
	if err != nil {
		return nil, err
	}
	return lifecycle.NewLease(client, func() error { return client.Close(context.WithoutCancel(ctx)) }), nil
}

func (a *app) selectedPixiv(ctx context.Context, userID int64) (int64, error) {
	if userID != 0 {
		return userID, nil
	}
	service, _, err := a.pixivAccounts(ctx)
	if err != nil {
		return 0, err
	}
	return service.SelectedUserID(ctx)
}

func (a *app) pixivLease(ctx context.Context, store *account.Store, userID int64, options pixiv.Options) (lease *lifecycle.Lease[*pixiv.Client], err error) {
	// 注入传输归组合根所有，在失败或租约关闭时释放。
	defer func() {
		if lease == nil {
			options.HTTPClient.CloseIdleConnections()
		}
	}()
	if err := a.gate.Acquire(ctx); err != nil {
		return nil, err
	}
	client, err := store.OpenSDKClient(ctx, userID, options)
	if err != nil {
		a.gate.Release()
		return nil, err
	}
	return lifecycle.NewLease(client, func() error {
		defer a.gate.Release()
		client.CloseIdleConnections()
		options.HTTPClient.CloseIdleConnections()
		return nil
	}), nil
}

func (a *app) executePixiv(ctx context.Context, request environment.PixivRequest, invoke func(context.Context, int64, *pixiv.Client) (bool, error)) error {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return err
	}
	options, _, err := a.pixivSettings(request)
	if err != nil {
		return err
	}
	defer options.HTTPClient.CloseIdleConnections()
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	selection := account.Selection{Mode: account.SelectionPool}
	if request.UserID != 0 || !runtime.AccountPool.Enabled {
		userID, err := a.selectedPixiv(ctx, request.UserID)
		if err != nil {
			return err
		}
		selection = account.Selection{Mode: account.SelectionFixed, UserID: userID}
	}
	scheduler := account.Scheduler{State: store, Strategy: account.Strategy(runtime.AccountPool.Strategy), Now: time.Now}
	return scheduler.Run(ctx, selection, func(ctx context.Context, userID int64, attempt *lifecycle.Attempt) (err error) {
		lease, err := a.pixivLease(ctx, store, userID, options)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, lease.Close()) }()
		committed, err := invoke(ctx, userID, lease.Value())
		if committed {
			attempt.Commit()
		}
		return err
	})
}

func (a *app) openPixiv(ctx context.Context, request environment.PixivRequest) (*lifecycle.Lease[*pixiv.Client], error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return nil, err
	}
	options, _, err := a.pixivSettings(request)
	if err != nil {
		return nil, err
	}
	userID, err := a.selectedPixiv(ctx, request.UserID)
	if err != nil {
		options.HTTPClient.CloseIdleConnections()
		return nil, err
	}
	return a.pixivLease(ctx, store, userID, options)
}

func (a *app) openVectorPixiv(ctx context.Context, request environment.PixivRequest) (int64, *pixiv.Client, func() error, error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return 0, nil, nil, err
	}
	options, _, err := a.pixivSettings(request)
	if err != nil {
		return 0, nil, nil, err
	}
	options.OnArtwork = nil
	userID, err := a.selectedPixiv(ctx, request.UserID)
	if err != nil {
		options.HTTPClient.CloseIdleConnections()
		return 0, nil, nil, err
	}
	lease, err := a.pixivLease(ctx, store, userID, options)
	if err != nil {
		return 0, nil, nil, err
	}
	return userID, lease.Value(), lease.Close, nil
}

func (a *app) searchVector(ctx context.Context, input vectorindex.SearchInput, source string, request environment.PixivRequest) (result vectorindex.SearchResponse, err error) {
	return a.searchVectorResolved(ctx, input, source, request, nil)
}

func (a *app) searchVectorResolved(ctx context.Context, input vectorindex.SearchInput, source string, request environment.PixivRequest, resolve func(context.Context) (int64, error)) (result vectorindex.SearchResponse, err error) {
	code := "model_unavailable"
	defer func() {
		if err != nil {
			var classified *vectorindex.SearchError
			if !errors.As(err, &classified) && reversesearch.CodeOf(err) == reversesearch.CodeUnknown {
				err = &vectorindex.SearchError{Code: code, Cause: err}
			}
		}
	}()
	choices := 0
	for _, present := range []bool{strings.TrimSpace(input.Query) != "", source != "", len(input.Image) > 0} {
		if present {
			choices++
		}
	}
	if choices != 1 {
		return result, &vectorindex.SearchError{Code: "invalid_request", Cause: i18n.Errorf("error.provide_exactly_one_text_query_or_image")}
	}
	c, err := (config.Store{Files: secretfile.ConfigFiles{}}).Current()
	if err != nil {
		return result, err
	}
	if c.Vector.Model == "" {
		return result, &vectorindex.SearchError{Code: "not_set_up", Cause: i18n.Errorf("error.vector_search_is_not_set_up_run_pixiv")}
	}
	proxy := c.Network.HTTPSProxy
	if request.HTTPSProxyOverride != nil {
		proxy = *request.HTTPSProxyOverride
	}
	client, err := config.HTTPClient(proxy)
	if err != nil {
		return result, err
	}
	defer client.CloseIdleConnections()
	if source != "" {
		snapshot, loadErr := reversesearch.NewSourceLoader(client).Load(ctx, source)
		if loadErr != nil {
			return result, loadErr
		}
		defer func() { err = errors.Join(err, snapshot.Close()) }()
		reader, readErr := snapshot.Open(ctx)
		if readErr != nil {
			return result, readErr
		}
		input.Image, err = io.ReadAll(reader)
		if err = errors.Join(err, reader.Close()); err != nil {
			return result, err
		}
	}
	identity, err := vectormodel.Identifier(c.Vector.Model, c.Vector.BaseURL)
	if err != nil {
		return result, err
	}
	code = "index_unavailable"
	root, err := config.AppDataDir()
	if err != nil {
		return result, err
	}
	code = "model_unavailable"
	model, err := vectormodel.Open(vectormodel.Options{Name: c.Vector.Model, BaseURL: c.Vector.BaseURL, APIKey: c.Vector.APIKey, Device: string(c.Vector.Device), Directory: vectorindex.Directory(root)}, client)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, model.Close()) }()
	code = "sync_failed"
	if resolve != nil {
		request.UserID, err = resolve(ctx)
		if err != nil {
			return result, &vectorindex.SearchError{Code: "sync_failed", Cause: err}
		}
	}
	accountID, err := a.selectedPixiv(ctx, request.UserID)
	if err != nil && sdk.ReasonOf(err) != sdk.Unauthorized {
		return result, err
	}
	request.UserID = accountID
	fixture, err := vectormodel.ProbeImage()
	if err != nil {
		return result, err
	}
	return vectorindex.Search(ctx, root, input, vectorindex.SearchOptions{AccountID: accountID, SyncOptions: vectorindex.SyncOptions{Model: identity, Embedder: model, ProbeImage: fixture, OpenClient: func(ctx context.Context) (int64, *pixiv.Client, func() error, error) {
		return a.openVectorPixiv(ctx, request)
	}}})
}

func (a *app) readSession(ctx context.Context, request environment.PixivRequest) (mediaproxy.Session, error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return mediaproxy.Session{}, err
	}
	a.readsMu.Lock()
	if a.reads == nil {
		a.reads = account.NewReadSessions(store, a.gate)
		a.closeState.Add(a.reads.Close)
	}
	reads := a.reads
	a.readsMu.Unlock()
	options, network, err := a.pixivSettings(request)
	if err != nil {
		return mediaproxy.Session{}, err
	}
	userID, err := a.selectedPixiv(ctx, request.UserID)
	if err != nil {
		options.HTTPClient.CloseIdleConnections()
		return mediaproxy.Session{}, err
	}
	snapshot, err := reads.OpenSDK(ctx, userID, network, options)
	if err != nil {
		options.HTTPClient.CloseIdleConnections()
		return mediaproxy.Session{}, err
	}
	return mediaproxy.Session{Lease: lifecycle.NewLease(snapshot.SDKClient, nil), Version: fmt.Sprint(snapshot.Version), Expires: snapshot.Expires, Invalidated: snapshot.Invalidated, Invalidate: func(err error) { reads.Invalidate(userID, snapshot.Version, err) }}, nil
}

func (a *app) localAccounts(ctx context.Context) (accounts.LocalSnapshot, error) {
	store, err := a.openAccounts(ctx)
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	records, err := store.ListPixiv(ctx)
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	defaultID, explicit, err := (config.Store{Files: secretfile.ConfigFiles{}}).ReadPixivDefaultUserID()
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	local := accounts.LocalSnapshot{Accounts: make([]accounts.Account, 0, len(records))}
	if explicit {
		local.DefaultUserID = defaultID
	}
	for _, record := range records {
		local.Accounts = append(local.Accounts, accounts.Account{UserID: record.UserID, Username: record.Username, HasCredentials: !record.Credential.Empty()})
	}
	return local, nil
}

// Run 执行一条 CLI 命令并返回进程退出码，阶段顺序固定：
//
// 1. 只用输入输出流和资源登记表构造 app，不打开资源；
// 2. 装配命令树并绑定流与参数；
// 3. 执行命令，资源在命令真正请求时才创建，help、version 不会提前打开账号文件或网络 client；
// 4. 无论成功与否都按登记逆序关闭资源，关闭错误并入退出原因；
// 5. 记录失败并映射退出码。
//
// 普通 NDJSON 输出与 MCP stdio 各自持有平台的 SIGPIPE 控制器：前者的 EPIPE 表示下游
// 正常停止，后者属于 JSON-RPC transport 错误。
func Run(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	previousLogger := slog.Default()
	defer slog.SetDefault(previousLogger)
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	closeState := &lifecycle.CloseState{}
	a := app{
		language: i18n.System(),
		in:       in, out: out, errOut: errOut, closeState: closeState,
		pipelineSignal: &environment.BrokenPipe{}, mcpSignal: &environment.BrokenPipe{},
		logs:      &logging.State{Logger: logging.Discard()},
		storeSlot: make(chan struct{}, 1),
	}
	a.gate = account.NewGate()
	defer a.pipelineSignal.Restore()
	defer a.mcpSignal.Restore()
	cmd := a.newRootCommand()
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	cmd.SetContext(i18n.WithLocale(ctx, a.language))
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
		attrs := []slog.Attr{slog.Any(logging.AttrError, err)}
		a.logs.Logger.LogAttrs(a.logs.Ctx, a.logs.FailureLevel, "Command failed", attrs...)
	}
	return a.exitWithNDJSONScope(err, ndjsonOutput)
}

func (a *app) exitWithNDJSONScope(err error, ndjsonOutput bool) int {
	if err == nil {
		return 0
	}
	if ndjsonOutput && errors.Is(err, syscall.EPIPE) {
		return 0
	}
	var startupErr *environment.StartupError
	if errors.As(err, &startupErr) {
		fmt.Fprintln(a.errOut, environment.ErrorText(startupErr.Err, a.language))
		return 1
	}
	var usageErr *environment.UsageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(a.errOut, a.language.Text("output.error"), environment.ErrorText(usageErr.Err, a.language))
		return 2
	}
	var pipelineErr *record.PipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, a.language.Text("output.error"), environment.ErrorText(err, a.language))
	return 1
}

func (a *app) startLogging(cmd *cobra.Command, requirement environment.Execution) error {
	if !requirement.EnsureConfig || isQuietConfigCommand(cmd) {
		return nil
	}
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	call, err := logging.Start(cmd.Context(), a.errOut, cmd.CommandPath(), string(runtime.Logging.Level), string(runtime.Logging.Format))
	if err != nil {
		return err
	}
	*a.logs = call
	slog.SetDefault(call.Logger)
	cmd.SetContext(call.Ctx)
	return nil
}

func isQuietConfigCommand(cmd *cobra.Command) bool {
	return cmd.CommandPath() == "pixiv config" || strings.HasPrefix(cmd.CommandPath(), "pixiv config ")
}

// enablePipelineSignal 安装 NDJSON 专用的 SIGPIPE 策略。command owner 只在确定输出记录流时
// 调用，其他命令保留平台的默认 SIGPIPE 行为，否则普通输出被关闭管道截断会被误判为成功。
func (a *app) commandEnv() environment.Env {
	env := environment.New(a.in, a.out, a.errOut, environment.NewUsageError, a.pipelineSignal.Enable, func(override *bool) (bool, error) {
		if override != nil {
			return *override, nil
		}
		runtime, err := a.runtimeConfig()
		if err != nil {
			return false, err
		}
		return runtime.Output.JSON, nil
	})
	env.Language = a.language
	return env
}

// newAuthCommand 组装 pixiv auth 组；账号服务延迟到第一次访问 auth 子命令时才打开账号文件。
func (a *app) newAuthCommand() *cobra.Command {
	return auth.New(a.commandEnv(), auth.SupportFor(a.pixivAccounts, a.runtimeConfig, secretfile.WriteSecretFile))
}

func (a *app) newFanboxCommandGroup(env environment.Env) *cobra.Command {
	children := []*cobra.Command{
		fanboxauth.New(env, a.fanboxAccounts, fanboxauth.SystemBrowserProvider{}),
		fanboxdownload.New(env, a.openFanbox, func() (string, error) {
			runtime, err := a.runtimeConfig()
			return runtime.Download.Path, err
		}),
	}
	return fanboxcli.NewCommand(env, append(children, browse.Commands(env, a.openFanbox)...))
}

func (a *app) pixivSettings(request environment.PixivRequest) (pixiv.Options, string, error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return pixiv.Options{}, "", err
	}
	interval, err := time.ParseDuration(runtime.Network.RequestInterval)
	if err != nil {
		return pixiv.Options{}, "", err
	}
	proxy := runtime.Network.HTTPSProxy
	if runtime.Pixiv.Network.ProxyURL != nil {
		proxy = *runtime.Pixiv.Network.ProxyURL
	}
	if request.HTTPSProxyOverride != nil {
		proxy = *request.HTTPSProxyOverride
	}
	httpClient, err := config.HTTPClient(proxy)
	if err != nil {
		return pixiv.Options{}, "", err
	}
	options := pixiv.Options{HTTPClient: httpClient, RequestInterval: interval}
	if runtime.Vector.Model != "" {
		root, err := config.AppDataDir()
		if err != nil {
			httpClient.CloseIdleConnections()
			return pixiv.Options{}, "", err
		}
		recorder := vectorindex.Recorder{Directory: root}
		options.OnArtwork = func(ctx context.Context, artwork *pixiv.Artwork) {
			if err := recorder.Append(ctx, artwork); err != nil {
				slog.WarnContext(ctx, "Could not record artwork for vector search", "reason", err)
			}
		}
	}
	return options, fmt.Sprintf("%q/%d/%q", proxy, interval, runtime.Vector.Model), nil
}

func (a *app) fanboxOptions() (fanbox.Options, error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return fanbox.Options{}, err
	}
	options := fanbox.Options{ProxyURL: runtime.Network.HTTPSProxy}
	if runtime.Fanbox.Network.ProxyURL != nil {
		options.ProxyURL = *runtime.Fanbox.Network.ProxyURL
	}
	if runtime.Fanbox.Network.UserAgent != nil {
		options.UserAgent = *runtime.Fanbox.Network.UserAgent
	}
	if runtime.Fanbox.FlareSolverr.URL != nil {
		options.FlareSolverrURL = *runtime.Fanbox.FlareSolverr.URL
		if runtime.Fanbox.FlareSolverr.ProxyURL != nil {
			options.FlareSolverrProxyURL = *runtime.Fanbox.FlareSolverr.ProxyURL
		}
	}
	return options, nil
}

// runPixivMCP 构造 MCP 启动快照，把反向搜图与本地账号端口注入 mcp.Runtime。
func (a *app) runPixivMCP(ctx context.Context, request mcp.Request) error {
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
		Settings:      runtime,
		ExecutePixiv:  a.executePixiv,
		OpenPixiv:     a.openPixiv,
		ReadSession:   a.readSession,
		LocalAccounts: a.localAccounts,
		PixivAccounts: a.pixivAccounts,
		OpenFanbox:    a.openFanbox,
		ReverseSearch: reverseSearch,
		VectorSearch: func(ctx context.Context, input vectorindex.SearchInput, source string, resolve func(context.Context) (pixivmcpserver.Account, error)) (vectorindex.SearchResponse, error) {
			return a.searchVectorResolved(ctx, input, source, environment.PixivRequest{HTTPSProxyOverride: request.HTTPSProxyOverride}, func(ctx context.Context) (int64, error) {
				account, err := resolve(ctx)
				return account.UserID, err
			})
		},
		CloseState: a.closeState,
	}.Run(ctx, request)
}

// newReverseSearcher 固定 standard/ascii2d 两个代理网络面、凭据和 solver 配置。
func newReverseSearcher(runtime config.Config, proxyOverride *string) (reversesearch.Searcher, error) {
	standardProxy, ascii2dProxy := runtime.Network.HTTPSProxy, runtime.Network.HTTPSProxy
	if runtime.ReverseSearch.Network.ProxyURL != nil {
		ascii2dProxy = *runtime.ReverseSearch.Network.ProxyURL
	}
	if proxyOverride != nil {
		standardProxy, ascii2dProxy = *proxyOverride, *proxyOverride
	}
	client, err := config.HTTPClient(standardProxy)
	if err != nil {
		return nil, err
	}
	options := reversesearch.Options{HTTPClient: client, ASCII2DProxy: ascii2dProxy, SauceNAOKey: runtime.ReverseSearch.SauceNAOAPIKey}
	if runtime.ReverseSearch.FlareSolverr.URL != nil {
		options.FlareSolverrURL = *runtime.ReverseSearch.FlareSolverr.URL
	}
	if runtime.ReverseSearch.FlareSolverr.ProxyURL != nil {
		options.FlareSolverrProxyURL = *runtime.ReverseSearch.FlareSolverr.ProxyURL
	}
	if runtime.ReverseSearch.Network.UserAgent != nil {
		options.UserAgent = *runtime.ReverseSearch.Network.UserAgent
	}
	return reversesearch.New(options)
}

// newMCPReverseSearchPorts 在 MCP 启动时创建一次反向搜图 Facade，并固定 provider 与
// 两个代理网络面；后续 tool input 只能覆盖 provider，不能改变传输或凭据依赖。
func newMCPReverseSearchPorts(runtime config.Config, request mcp.Request) (pixivmcpserver.ReverseSearchPorts, error) {
	searcher, err := newReverseSearcher(runtime, request.HTTPSProxyOverride)
	if err != nil {
		return pixivmcpserver.ReverseSearchPorts{}, err
	}
	provider := reversesearch.Provider(runtime.ReverseSearch.Provider)
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	return pixivmcpserver.ReverseSearchPorts{Searcher: searcher, Provider: provider, PixivOnly: runtime.ReverseSearch.PixivOnly}, nil
}

// reverseSearchPort 为每次命令树构造反向搜图端口；每次调用按当前配置重新读取 provider 和代理。
func (a *app) reverseSearchPort() search.ReverseSearchPort {
	return func(ctx context.Context, request search.ReverseSearchRequest) (reversesearch.Response, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return reversesearch.Response{}, err
		}
		provider := reversesearch.Provider(runtime.ReverseSearch.Provider)
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
		return searcher.Search(ctx, reversesearch.Request{Source: request.Source, Provider: provider, PixivOnly: runtime.ReverseSearch.PixivOnly})
	}
}
