package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Runtime 是组合根交给 MCP 命令的生产依赖。
//
// 本地账号、只读会话、反向搜图与配置快照都由 root.go 构造；本包只把它们接到
// mcpserver 的 stdio 或 HTTP runtime，不打开账号库或读取配置。
type Runtime struct {
	In        io.Reader
	Out       io.Writer
	ErrOut    io.Writer
	Logger    func() *slog.Logger
	Settings  func() (settings.RuntimeConfig, error)
	Downloads downloader.DownloadService
	// Local 提供 Pixiv 重放执行、只读会话与本地账号摘要。
	Local *session.Local
	// Fanbox 为每次请求打开独立的 FANBOX 会话。
	Fanbox func() (*session.Fanbox, error)
	// ReverseSearch 是启动时固定配置的反向搜图端口。
	ReverseSearch pixivmcpserver.ReverseSearchPorts
	// PixivAccounts 延迟打开账号与登录服务，供 HTTP 登录接力复用。
	PixivAccounts func() (*account.Service, account.LoginService, error)
	// CloseState 登记 MCP 打开的反向搜图 client。
	CloseState *lifecycle.CloseState
}

// mcpStateFileName 保存 HTTP MCP 的 owner 与 OAuth grant，位于应用数据目录。
const mcpStateFileName = "mcp-state.json"

// Run 启动 stdio 或 HTTP runtime；request.ListenAddr 为 nil 时使用 stdio。
func (r Runtime) Run(ctx context.Context, request Request) error {
	settings, err := r.Settings()
	if err != nil {
		return err
	}
	if request.HTTPSProxyOverride != nil {
		if _, err := httpclient.HTTPClient(*request.HTTPSProxyOverride); err != nil {
			return err
		}
	}
	if closer, ok := r.ReverseSearch.Searcher.(reversesearch.Closer); ok {
		r.CloseState.Add(closer.Close)
	}
	sdkPorts, fanboxPorts := r.sdkPorts()
	if request.ListenAddr == nil {
		return r.runStdio(ctx, settings, sdkPorts, fanboxPorts, request)
	}
	return r.runHTTP(ctx, settings, sdkPorts, fanboxPorts, request)
}

// sdkPorts 把组合根的 SDK 边界翻译成 MCP 产品端口。
func (r Runtime) sdkPorts() (pixivmcpserver.SDKPorts, fanboxmcpserver.SDKPorts) {
	sdkPorts := pixivmcpserver.SDKPorts{
		Logger: r.Logger(),
		Execute: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return r.Local.Execute(ctx, pixivRequest(account), attempt)
		},
		ReverseSearch: r.ReverseSearch,
	}
	fanboxPorts := fanboxmcpserver.SDKPorts{
		Logger: r.Logger(),
		OpenLease: func(ctx context.Context, account fanboxmcpserver.Account) (*lifecycle.Lease[*fanbox.Client], error) {
			service, err := r.Fanbox()
			if err != nil {
				return nil, err
			}
			return service.Open(ctx, session.OpenRequest{ProxyOverride: account.HTTPSProxyOverride})
		},
	}
	return sdkPorts, fanboxPorts
}

func pixivRequest(account pixivmcpserver.Account) session.ProxyRequest {
	return session.ProxyRequest{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride}
}

// runStdio 使用每个请求独立打开本地账号，stdout 只承载 JSON-RPC。
func (r Runtime) runStdio(ctx context.Context, settings settings.RuntimeConfig, sdkPorts pixivmcpserver.SDKPorts, fanboxPorts fanboxmcpserver.SDKPorts, request Request) error {
	account := pixivmcpserver.Account{HTTPSProxyOverride: request.HTTPSProxyOverride}
	sdkPorts.Downloads = &pixivmcpserver.LocalDownloads{Service: r.Downloads, Path: settings.DownloadPath, FilenameTemplate: settings.FilenameTemplate, DirectoryTemplate: settings.DirectoryTemplate}
	// 只经本地 Facade 解析一次身份，后续阶段固定使用同一账号，避免并发请求各选到不同账号。
	sdkPorts.ResolveAccount = func(ctx context.Context, account pixivmcpserver.Account) (pixivmcpserver.Account, error) {
		err := r.Local.Execute(ctx, session.ProxyRequest{HTTPSProxyOverride: account.HTTPSProxyOverride}, func(_ context.Context, client *pixiv.Client) (bool, error) {
			account.UserID = client.UserID()
			return false, nil
		})
		return account, err
	}
	sdkPorts.OpenLease = func(ctx context.Context, account pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
		return r.Local.OpenLease(ctx, pixivRequest(account))
	}
	return mcpserver.RunStdio(ctx, mcpserver.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride), r.In, r.Out)
}

// runHTTP 在显式 --port 时启动共享账号选择、登录 relay 与媒体代理。
func (r Runtime) runHTTP(ctx context.Context, settings settings.RuntimeConfig, sdkPorts pixivmcpserver.SDKPorts, fanboxPorts fanboxmcpserver.SDKPorts, request Request) (result error) {
	if err := r.Local.WatchReadSessions(ctx); err != nil {
		return err
	}
	listenAddr, baseURL := *request.ListenAddr, *request.BaseURL
	filename, err := paths.UserDataFile(paths.AppDataDirName, mcpStateFileName)
	if err != nil {
		return err
	}
	store := auth.Store{Path: filename}
	if _, err := auth.NewHandler(baseURL, store); err != nil {
		return err
	}
	if _, err := store.Read(ctx); err != nil {
		return err
	}
	selection := accounts.Selection{Store: store, Load: r.localAccounts}
	login, err := r.newLogin(ctx, baseURL, selection, request.HTTPSProxyOverride)
	if err != nil {
		return err
	}
	defer login.Close()
	media, err := mediaproxy.New(baseURL, store, func(ctx context.Context, userID int64) (*lifecycle.Lease[*pixiv.Client], error) {
		return r.Local.OpenLease(ctx, session.ProxyRequest{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	})
	if err != nil {
		return err
	}
	if err := configureMedia(ctx, media, settings, r.readSession, request); err != nil {
		return err
	}
	if err := media.ConfigureAnimations(ctx, settings.MCPGalleryAnimationTempCapacityMiB*(1<<20)); err != nil {
		return err
	}
	defer func() { result = errors.Join(result, media.CloseAnimations()) }()
	sdkPorts.Media, sdkPorts.Accounts, sdkPorts.Login = media, selection, login
	sdkPorts.Read = func(ctx context.Context, account pixivmcpserver.Account, callback func(context.Context, *pixiv.Client) (bool, error)) error {
		return r.Local.Read(ctx, pixivRequest(account), callback)
	}
	sdkPorts.OpenLease = func(ctx context.Context, account pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
		snapshot, err := r.Local.ReadSession(ctx, pixivRequest(account))
		return snapshot.Lease, err
	}
	sdkPorts.ResolveAccount = func(ctx context.Context, account pixivmcpserver.Account) (pixivmcpserver.Account, error) {
		userID, err := selection.Resolve(ctx)
		if err != nil {
			return pixivmcpserver.Account{}, err
		}
		account.UserID = userID
		return account, nil
	}
	account := pixivmcpserver.Account{HTTPSProxyOverride: request.HTTPSProxyOverride}
	return mcpserver.RunHTTP(ctx, mcpserver.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride), listenAddr, baseURL, store, r.ErrOut, login, media)
}

// localAccounts 把 session 的本地账号摘要投影成 MCP 的选择视图。
func (r Runtime) localAccounts(ctx context.Context) (accounts.LocalSnapshot, error) {
	local, err := r.Local.LocalAccounts(ctx)
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	snapshot := accounts.LocalSnapshot{DefaultUserID: local.DefaultUserID, Accounts: make([]accounts.Account, 0, len(local.Accounts))}
	for _, account := range local.Accounts {
		snapshot.Accounts = append(snapshot.Accounts, accounts.Account{UserID: account.UserID, Username: account.Username, HasCredentials: account.HasCredentials})
	}
	return snapshot, nil
}

// readSession 把 session 的只读快照投影成媒体代理使用的会话值。
func (r Runtime) readSession(ctx context.Context, request session.ProxyRequest) (mediaproxy.Session, error) {
	snapshot, err := r.Local.ReadSession(ctx, request)
	if err != nil {
		return mediaproxy.Session{}, err
	}
	return mediaproxy.Session{Lease: snapshot.Lease, Version: snapshot.Version, Expires: snapshot.Expires, Invalidated: snapshot.Invalidated, Invalidate: snapshot.Invalidate}, nil
}

// configureMedia 挂媒体阶段观测；日志只带固定阶段名和计数。
func configureMedia(lifecycleCtx context.Context, media *mediaproxy.Service, settings settings.RuntimeConfig, readSession func(context.Context, session.ProxyRequest) (mediaproxy.Session, error), request Request) error {
	return media.Configure(mediaproxy.Config{
		Capacity: settings.MCPGalleryCacheCapacityMiB * (1 << 20),
		ObserveAnimation: func(requestCtx context.Context, event mediaproxy.AnimationObservation) {
			logAnimation(lifecycleCtx, requestCtx, event)
		},
		Observe: func(_ context.Context, stage string, duration time.Duration) {
			logging.From(lifecycleCtx).LogAttrs(lifecycleCtx, slog.LevelDebug, "Gallery media step completed",
				slog.String(logging.AttrStage, stage), slog.Duration(logging.AttrDuration, duration))
		},
	}, func(ctx context.Context, userID int64) (mediaproxy.Session, error) {
		return readSession(ctx, session.ProxyRequest{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	})
}

// logAnimation 把媒体阶段的安全指标写到执行边界 logger；原因只取固定分类，不回显调用方字符串。
func logAnimation(lifecycleCtx, requestCtx context.Context, observation mediaproxy.AnimationObservation) {
	// 请求可能脱离 MCP 调用 ctx（HTTP 断线/服务退出）；此时用生命周期 logger 记录。
	ctx := lifecycleCtx
	if logging.HasLogger(requestCtx) {
		ctx = requestCtx
	}
	logger := logging.From(ctx)
	attrs := []slog.Attr{
		slog.String(logging.AttrStage, observation.Stage),
		slog.Duration(logging.AttrDuration, observation.Duration),
	}
	if observation.Bytes > 0 {
		attrs = append(attrs, slog.Int(logging.AttrCount, int(observation.Bytes)))
	}
	reason := ""
	switch {
	case observation.CancellationSource == "request":
		reason = "request cancelled"
	case observation.CancellationSource == "transport":
		reason = "transport interrupted"
	case observation.CancellationSource == "subscription":
		reason = "subscription released"
	case observation.CancellationSource == "service":
		reason = "service closed"
	case observation.CancellationSource == "expiry":
		reason = "authorization expired"
	case observation.CancellationSource == "authorization":
		reason = "authorization invalidated"
	case observation.CancellationSource != "" || observation.Failed:
		reason = "tool failed"
	}
	if reason != "" {
		logger.LogAttrs(ctx, slog.LevelWarn, "Animation "+observation.Stage+" failed",
			append(attrs, slog.String(logging.AttrReason, reason))...)
		return
	}
	logger.LogAttrs(ctx, slog.LevelDebug, "Animation "+observation.Stage+" completed", attrs...)
}

// InitAuth 初始化 HTTP MCP 的 owner 与 OAuth grant。
func InitAuth(ctx context.Context, reset bool) (string, error) {
	filename, err := paths.UserDataFile(paths.AppDataDirName, mcpStateFileName)
	if err != nil {
		return "", err
	}
	return (auth.Store{Path: filename}).Init(ctx, reset)
}

func (r Runtime) newLogin(ctx context.Context, baseURL string, selection accounts.Selection, proxy *string) (*accounts.Login, error) {
	// 登录启动串行执行；成功后保留同一个账号服务，后续完成步骤复用它。
	var service *account.LoginService
	return accounts.NewLogin(ctx, strings.TrimRight(baseURL, "/")+"/pixiv-login", func() (accounts.LoginAttempt, error) {
		if service == nil {
			_, loaded, err := r.PixivAccounts()
			if err != nil {
				return accounts.LoginAttempt{}, err
			}
			service = &loaded
		}
		options := pixiv.LoginOptions{}
		if proxy != nil {
			client, err := httpclient.HTTPClient(*proxy)
			if err != nil {
				return accounts.LoginAttempt{}, err
			}
			options.HTTPClient = client
		}
		start, err := service.Start(account.LoginRequest{Options: options})
		if err != nil {
			return accounts.LoginAttempt{}, err
		}
		return accounts.LoginAttempt{
			AuthorizationURL: start.AuthorizationURL,
			AcceptsCallback:  start.AcceptsCallbackURL,
			Complete: func(ctx context.Context, callback string) (accounts.LoginResult, error) {
				// 显式代理 client 由组合根创建，关闭也由它负责。
				if options.HTTPClient != nil {
					defer options.HTTPClient.CloseIdleConnections()
				}
				account, err := service.Complete(ctx, start, account.LoginCompleteRequest{CallbackOrCode: callback, PreserveDefault: true})
				result := accounts.LoginResult{Account: accounts.Account{UserID: account.UserID, Username: account.Username, HasCredentials: account.UserID > 0}, AccountSaved: account.UserID > 0}
				if err != nil {
					return result, err
				}
				result.SelectionUpdated, err = selection.SelectAfterLogin(ctx, account.UserID)
				return result, err
			},
		}, nil
	})
}
