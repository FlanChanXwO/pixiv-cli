// MCP 运行时装配：Pixiv/FANBOX SDK 端口、stdio/HTTP 生命周期与 MCP 登录。
// 依赖由 composition.go 打开并登记；本文件只把一次 mcp 请求组装成 server。

package cli

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	unifiedmcp "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	mcpauth "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	mcpaccounts "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	mediaproxy "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	sdkfanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func (a app) runPixivMCP(ctx context.Context, request mcp.Request) (result error) {
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	if request.HTTPSProxyOverride != nil {
		if _, err := httpclient.HTTPClient(*request.HTTPSProxyOverride); err != nil {
			return err
		}
	}
	reverseSearchPorts, err := newMCPReverseSearchPorts(runtime, request)
	if err != nil {
		return err
	}
	registerReverseSearchCloser(a.closeState, reverseSearchPorts.Searcher)
	ports, err := a.newPixivSDKPorts()
	if err != nil {
		return err
	}
	sdkPorts, fanboxPorts := a.newMCPSDKPorts(ports, reverseSearchPorts)
	if request.ListenAddr == nil {
		return a.runMCPStdio(ctx, runtime, sdkPorts, fanboxPorts, ports, request)
	}
	return a.runMCPHTTP(ctx, runtime, sdkPorts, fanboxPorts, ports, request)
}

// newMCPSDKPorts 把 composition root 的 SDK 边界翻译成 MCP 产品端口。读取策略由
// transport 决定：stdio 经 Execute 的账号池读取，HTTP 复用只读会话。
func (a app) newMCPSDKPorts(ports pixivSDKPorts, reverseSearchPorts pixivmcpserver.ReverseSearchPorts) (pixivmcpserver.SDKPorts, fanboxmcpserver.SDKPorts) {
	sdkPorts := pixivmcpserver.SDKPorts{
		Logger: a.logger(),
		Execute: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *sdkpixiv.Client) (bool, error)) error {
			return ports.execute(ctx, pixivRequest(account), attempt)
		},
		ReverseSearch: reverseSearchPorts,
	}
	fanboxPorts := fanboxmcpserver.SDKPorts{
		Logger: a.logger(),
		OpenLease: func(ctx context.Context, account fanboxmcpserver.Account) (*lifecycle.Lease[*sdkfanbox.Client], error) {
			service, err := a.newFanboxService()
			if err != nil {
				return nil, err
			}
			return service.Open(ctx, session.OpenRequest{ProxyOverride: account.HTTPSProxyOverride})
		},
	}
	return sdkPorts, fanboxPorts
}

func pixivRequest(account pixivmcpserver.Account) pixivcmd.Request {
	return pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride}
}

// runMCPStdio 使用每个请求独立打开本地账号，stdout 只承载 JSON-RPC。
func (a app) runMCPStdio(ctx context.Context, runtime settings.RuntimeConfig, sdkPorts pixivmcpserver.SDKPorts, fanboxPorts fanboxmcpserver.SDKPorts, ports pixivSDKPorts, request mcp.Request) error {
	account := pixivmcpserver.Account{HTTPSProxyOverride: request.HTTPSProxyOverride}
	sdkPorts.Downloads = &pixivmcpserver.LocalDownloads{Service: defaultDownloadService(), Path: runtime.DownloadPath, FilenameTemplate: runtime.FilenameTemplate, DirectoryTemplate: runtime.DirectoryTemplate}
	// Resolve once through the local Facade, then pin all phases to that identity.
	sdkPorts.ResolveAccount = func(ctx context.Context, account pixivmcpserver.Account) (pixivmcpserver.Account, error) {
		err := ports.execute(ctx, pixivcmd.Request{HTTPSProxyOverride: account.HTTPSProxyOverride}, func(_ context.Context, client *sdkpixiv.Client) (bool, error) {
			account.UserID = client.UserID()
			return false, nil
		})
		return account, err
	}
	sdkPorts.OpenLease = func(ctx context.Context, account pixivmcpserver.Account) (*lifecycle.Lease[*sdkpixiv.Client], error) {
		return ports.openLease(ctx, pixivRequest(account))
	}
	return unifiedmcp.RunStdio(ctx, unifiedmcp.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride), a.in, a.out)
}

// runMCPHTTP 在显式 --port 时启动共享账号选择、登录 relay 与媒体代理。
func (a app) runMCPHTTP(ctx context.Context, runtime settings.RuntimeConfig, sdkPorts pixivmcpserver.SDKPorts, fanboxPorts fanboxmcpserver.SDKPorts, ports pixivSDKPorts, request mcp.Request) (result error) {
	if err := ports.watchReadSessions(ctx); err != nil {
		return err
	}
	listenAddr, baseURL := *request.ListenAddr, *request.BaseURL
	filename, err := paths.UserDataFile(paths.AppDataDirName, mcpStateFileName)
	if err != nil {
		return err
	}
	store := mcpauth.Store{Path: filename}
	if _, err := mcpauth.NewHandler(baseURL, store); err != nil {
		return err
	}
	if _, err := store.Read(ctx); err != nil {
		return err
	}
	selection := mcpaccounts.Selection{Store: store, Load: ports.localAccounts}
	login, err := a.newMCPLogin(ctx, baseURL, selection, request.HTTPSProxyOverride)
	if err != nil {
		return err
	}
	defer login.Close()
	media, err := mediaproxy.New(baseURL, store, func(ctx context.Context, userID int64) (*lifecycle.Lease[*sdkpixiv.Client], error) {
		return ports.openLease(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	})
	if err != nil {
		return err
	}
	if err := configureMCPMedia(ctx, media, runtime, ports.readSession, request); err != nil {
		return err
	}
	if err := media.ConfigureAnimations(ctx, runtime.MCPGalleryAnimationTempCapacityMiB*(1<<20)); err != nil {
		return err
	}
	defer func() { result = errors.Join(result, media.CloseAnimations()) }()
	sdkPorts.Media, sdkPorts.Accounts, sdkPorts.Login = media, selection, login
	sdkPorts.Read = func(ctx context.Context, account pixivmcpserver.Account, callback func(context.Context, *sdkpixiv.Client) (bool, error)) error {
		return ports.read(ctx, pixivRequest(account), callback)
	}
	sdkPorts.OpenLease = func(ctx context.Context, account pixivmcpserver.Account) (*lifecycle.Lease[*sdkpixiv.Client], error) {
		snapshot, err := ports.readSession(ctx, pixivRequest(account))
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
	return unifiedmcp.RunHTTP(ctx, unifiedmcp.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride), listenAddr, baseURL, store, a.errOut, login, media)
}

// configureMCPMedia 挂媒体阶段观测；日志只带固定阶段名和计数。
func configureMCPMedia(lifecycleCtx context.Context, media *mediaproxy.Service, runtime settings.RuntimeConfig, readSession func(context.Context, pixivcmd.Request) (mediaproxy.Session, error), request mcp.Request) error {
	return media.Configure(mediaproxy.Config{
		Capacity:    runtime.MCPGalleryCacheCapacityMiB * (1 << 20),
		TTL:         mediaproxy.DefaultCacheTTL,
		Concurrency: mediaproxy.DefaultConcurrency,
		ObserveAnimation: func(requestCtx context.Context, event mediaproxy.AnimationObservation) {
			logAnimation(lifecycleCtx, requestCtx, event)
		},
		Observe: func(_ context.Context, stage string, duration time.Duration) {
			logging.From(lifecycleCtx).LogAttrs(lifecycleCtx, slog.LevelDebug, "Gallery media step completed",
				slog.String(logging.AttrStage, stage), slog.Duration(logging.AttrDuration, duration))
		},
	}, func(ctx context.Context, userID int64) (mediaproxy.Session, error) {
		return readSession(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
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

// mcpStateFileName 保存 HTTP MCP 的 owner 与 OAuth grant，位于应用数据目录。
const mcpStateFileName = "mcp-state.json"

func initMCPAuth(ctx context.Context, reset bool) (string, error) {
	filename, err := paths.UserDataFile(paths.AppDataDirName, mcpStateFileName)
	if err != nil {
		return "", err
	}
	return (mcpauth.Store{Path: filename}).Init(ctx, reset)
}

func (a app) newMCPLogin(ctx context.Context, baseURL string, selection mcpaccounts.Selection, proxy *string) (*mcpaccounts.Login, error) {
	// Login serializes starts; retain one successfully opened account service.
	var service *pixivaccount.LoginService
	return mcpaccounts.NewLogin(ctx, strings.TrimRight(baseURL, "/")+"/pixiv-login", func() (mcpaccounts.LoginAttempt, error) {
		if service == nil {
			_, loaded, err := a.newPixivAccountServices()
			if err != nil {
				return mcpaccounts.LoginAttempt{}, err
			}
			service = &loaded
		}
		options := sdkpixiv.LoginOptions{}
		if proxy != nil {
			client, err := httpclient.HTTPClient(*proxy)
			if err != nil {
				return mcpaccounts.LoginAttempt{}, err
			}
			options.HTTPClient = client
		}
		start, err := service.Start(pixivaccount.LoginRequest{Options: options})
		if err != nil {
			return mcpaccounts.LoginAttempt{}, err
		}
		return mcpaccounts.LoginAttempt{
			AuthorizationURL: start.AuthorizationURL,
			AcceptsCallback:  start.AcceptsCallbackURL,
			Complete: func(ctx context.Context, callback string) (mcpaccounts.LoginResult, error) {
				// This composition root owns explicit proxy/direct clients, not the SDK.
				if options.HTTPClient != nil {
					defer options.HTTPClient.CloseIdleConnections()
				}
				account, err := service.Complete(ctx, start, pixivaccount.LoginCompleteRequest{CallbackOrCode: callback, PreserveDefault: true})
				result := mcpaccounts.LoginResult{Account: mcpaccounts.Account{UserID: account.UserID, Username: account.Username, HasCredentials: account.UserID > 0}, AccountSaved: account.UserID > 0}
				if err != nil {
					return result, err
				}
				result.SelectionUpdated, err = selection.SelectAfterLogin(ctx, account.UserID)
				return result, err
			},
		}, nil
	})
}
