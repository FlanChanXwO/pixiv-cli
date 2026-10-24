package cli

// 本文件负责生产依赖装配：把公开 SDK、账号服务、下载服务与 MCP runtime
// 按一次 execution 的需求组装成窄端口，并把它交给命令树。
//
// 装配顺序即资源所有权顺序：这里打开的数据库与 SDK 资源都登记到 a.closeState，
// 由 execution.go 统一关闭。

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth/loginhelper"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/download"
	fanboxpost "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	pixivsearch "github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	coreDiagnostics "github.com/FlanChanXwO/pixiv-cli/internal/logging"
	unifiedmcp "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	mcpauth "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	mcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	mcpaccounts "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	pixivpool "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
	selfupdate "github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

const internalURLCallbackCommand = loginhelper.CallbackCommand

const internalURLHandlerInstallCommand = "_install-handler"

// systemFanboxBrowserSessionReader 保留根包测试对默认 provider 的类型引用；实现
// 本身属于 commands/fanbox/auth，不在根包复制浏览器逻辑。
type systemFanboxBrowserSessionReader = fanboxauth.SystemBrowserProvider

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
	service := pixivaccount.NewService(db, configapp.DefaultStore())
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
	service := pixivaccount.NewService(db, configapp.DefaultStore())
	if service == nil {
		return pixivSDKPorts{}, errors.New("pixiv account service is not configured")
	}
	gate := pixivpool.NewGate()
	readSessions := pixivaccount.NewReadSessions(service, gate)
	a.closeState.add(readSessions.Close)
	facade := session.NewPixiv(service, gate, func() (session.PoolConfig, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return session.PoolConfig{}, err
		}
		return session.PoolConfig{Enabled: runtime.AccountPool.Enabled, Strategy: string(runtime.AccountPool.Strategy)}, nil
	}, func(config session.PoolConfig) (session.PoolExecutor, error) {
		return pixivpool.Scheduler{
			Config: configapp.AccountPoolConfig{Enabled: config.Enabled, Strategy: configapp.AccountPoolStrategy(config.Strategy)},
			State:  db,
			Now:    time.Now,
		}, nil
	})
	if facade == nil {
		return pixivSDKPorts{}, errors.New("pixiv service facade is not configured")
	}
	open := func(request pixivcmd.Request) (*pixiv.Client, error) {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return nil, err
		}
		lease, err := facade.Open(context.Background(), session.Request{UserID: request.UserID, Options: options})
		if err != nil {
			return nil, err
		}
		a.closeState.add(lease.Close)
		return lease.Value(), nil
	}
	openLease := func(ctx context.Context, request pixivcmd.Request) (*lifecycle.Lease[*pixiv.Client], error) {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return nil, err
		}
		return facade.Open(ctx, session.Request{UserID: request.UserID, Options: options})
	}
	execute := func(ctx context.Context, request pixivcmd.Request, callback func(context.Context, *pixiv.Client) (bool, error)) error {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig)
		if err != nil {
			return err
		}
		return facade.Use(ctx, session.Request{UserID: request.UserID, Options: options}, callback)
	}
	readSession := func(ctx context.Context, request pixivcmd.Request) (mediaproxy.Session, error) {
		cfg, err := a.runtimeConfig()
		if err != nil {
			return mediaproxy.Session{}, err
		}
		options, err := pixivOptionsFromRequest(request, func() (configapp.RuntimeConfig, error) { return cfg, nil })
		if err != nil {
			return mediaproxy.Session{}, err
		}
		proxy := cfg.HTTPSProxy
		if cfg.PixivNetwork.ProxyURL.Present {
			proxy = cfg.PixivNetwork.ProxyURL.Value
		}
		if request.HTTPSProxyOverride != nil {
			proxy = *request.HTTPSProxyOverride
		}
		networkID := sha256.Sum256([]byte(fmt.Sprintf("%q/%d/%q", proxy, options.Pacing.MinInterval, options.AcceptLanguage)))
		snapshot, err := readSessions.Open(ctx, request.UserID, fmt.Sprintf("%x", networkID), options)
		if err != nil {
			return mediaproxy.Session{}, err
		}
		return mediaproxy.Session{Lease: lifecycle.NewLease(snapshot.Client, nil), Version: fmt.Sprint(snapshot.Version), Expires: snapshot.Expires, Invalidated: snapshot.Invalidated, Invalidate: func(err error) { readSessions.Invalidate(request.UserID, snapshot.Version, err) }}, nil
	}
	return pixivSDKPorts{
		watchReadSessions: func(ctx context.Context) error { return readSessions.Watch(ctx, db.Path()) },
		readSession:       readSession,
		read: func(ctx context.Context, request pixivcmd.Request, callback func(context.Context, *pixiv.Client) (bool, error)) error {
			snapshot, err := readSession(ctx, request)
			if err != nil {
				return err
			}
			defer snapshot.Lease.Close()
			_, err = callback(ctx, snapshot.Lease.Value())
			snapshot.Invalidate(err)
			return err
		},
		localAccounts: func(ctx context.Context) (mcpaccounts.LocalSnapshot, error) {
			records, err := db.ListPixiv(ctx)
			if err != nil {
				return mcpaccounts.LocalSnapshot{}, err
			}
			defaultID, explicit, err := configapp.DefaultStore().ReadPixivDefaultUserID()
			if err != nil {
				return mcpaccounts.LocalSnapshot{}, err
			}
			local := mcpaccounts.LocalSnapshot{Accounts: make([]mcpaccounts.Account, 0, len(records))}
			if explicit {
				local.DefaultUserID = defaultID
			}
			for _, record := range records {
				local.Accounts = append(local.Accounts, mcpaccounts.Account{UserID: record.UserID, Username: record.Username, HasCredentials: record.HasRefreshToken()})
			}
			return local, nil
		},
		open:      open,
		openLease: openLease,
		execute:   execute,
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
	return fanboxaccount.NewService(db, configapp.DefaultStore(), func() (fanboxsdkOptions, error) {
		return fanboxOptionsFromRuntime(a.runtimeConfig)
	}), nil
}

func (a app) newFanboxService() (*session.Fanbox, error) {
	accounts, err := a.newFanboxAccountService()
	if err != nil {
		return nil, err
	}
	return session.NewFanbox(accounts), nil
}

func defaultDownloadService() downloader.DownloadService {
	return downloader.DownloadService{NewBatchDownloader: func(client downloader.DownloadClient, downloadPath, filenameTemplate string) (downloader.BatchDownloader, error) {
		return downloader.NewArtworkDownloader(client, downloadPath, filenameTemplate), nil
	}}
}

// commandEnv is the one execution environment every command owner receives.
func (a app) commandEnv() command.Env {
	return command.Env{In: a.in, Out: a.out, Err: a.errOut, WrapUsage: newUsageError, ResolveJSON: func(override *bool) (bool, error) {
		if override != nil {
			return *override, nil
		}
		runtime, err := a.runtimeConfig()
		if err != nil {
			return false, err
		}
		return runtime.OutputJSON, nil
	}}
}

// commandExecutor loads the pool once per command tree and runs requests
// through the public SDK replay boundary.
func (a app) commandExecutor() pixivcmd.Executor {
	var once sync.Once
	var ports pixivSDKPorts
	var portsErr error
	return func(ctx context.Context, request pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		once.Do(func() { ports, portsErr = newCLIPixivSDKPorts(a) })
		if portsErr != nil {
			return portsErr
		}
		return ports.run(ctx, request, attempt)
	}
}

// newAuthCommand constructs the Pixiv auth group with its account leaf.
func (a app) newAuthCommand() *cobra.Command {
	var once sync.Once
	var account authcommands.AccountService
	var login pixivaccount.LoginService
	var loadErr error
	load := func() {
		once.Do(func() {
			account, login, loadErr = newCLIAccountServices(a)
		})
	}
	return authcommands.New(a.commandEnv(), authcommands.Support{
		Account: func() (authcommands.AccountService, error) {
			load()
			return account, loadErr
		},
		Login: func() (pixivaccount.LoginService, error) {
			load()
			return login, loadErr
		},
		LoadRuntime: a.runtimeConfig,
		WriteBundle: writeAuthExportBundle,
	})
}

func (a app) newDownloadCommand() *cobra.Command {
	return downloadcommands.New(a.commandEnv(), a.commandExecutor(), func() (downloadcommands.Runtime, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return downloadcommands.Runtime{}, err
		}
		return downloadcommands.Runtime{
			DownloadPath:      runtime.DownloadPath,
			FilenameTemplate:  runtime.FilenameTemplate,
			DirectoryTemplate: runtime.DirectoryTemplate,
		}, nil
	}, newCLIDownloadService)
}

func (a app) newFanboxCommandGroup(env command.Env) *cobra.Command {
	var once sync.Once
	var service *session.Fanbox
	var serviceErr error
	load := func() (*session.Fanbox, error) {
		once.Do(func() { service, serviceErr = newCLIFanboxService(a) })
		return service, serviceErr
	}
	accounts := func() (*fanboxaccount.Service, error) { return newCLIFanboxAccountService(a) }
	children := []*cobra.Command{
		fanboxauth.New(env, accounts, fanboxBrowserSessionReader),
		fanboxdownload.New(env, load, func() (string, error) {
			runtime, err := a.runtimeConfig()
			return runtime.DownloadPath, err
		}),
	}
	children = append(children, fanboxpost.Commands(env, load)...)
	return newFanboxCommand(env, children)
}

func (a app) ConfigService() configapp.Store { return configapp.DefaultStore() }

func (a app) ConfigPath() (string, error) { return a.ConfigService().Path() }

func (a app) FanboxService() (*session.Fanbox, error) {
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

func (app) FanboxBrowserProvider() fanboxcmd.BrowserProvider {
	return fanboxBrowserSessionReader
}

func (a app) FanboxRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) LoadUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) NewUpdateCoordinator(proxy string, out, errOut io.Writer) (*selfupdate.UpdateCoordinator, error) {
	return newUpdateCommandCoordinator(proxy, out, errOut)
}

func (a app) LoadAutomaticUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return a.runtimeConfig()
}

func (a app) NewAutomaticUpdateChecker(proxy string) (*selfupdate.AutomaticUpdateChecker, error) {
	return newCLIAutomaticUpdateChecker(proxy)
}

func (a app) RunMCP(ctx context.Context, request mcpcommands.Request) error {
	return runMCPServer(a, ctx, request)
}

func (a app) runPixivMCP(ctx context.Context, request mcpcommands.Request) (result error) {
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
	ports, err := newCLIPixivSDKPorts(a)
	if err != nil {
		return err
	}
	account := mcpserver.Account{
		HTTPSProxyOverride: request.HTTPSProxyOverride,
	}
	fanboxService := func() (*session.Fanbox, error) { return newCLIFanboxService(a) }
	var readPort func(context.Context, mcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error
	if ports.read != nil {
		readPort = func(ctx context.Context, account mcpserver.Account, callback func(context.Context, *pixiv.Client) (bool, error)) error {
			return ports.read(ctx, pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride}, callback)
		}
	}
	sdkPorts := mcpserver.SDKPorts{
		Read: readPort,
		Open: func(account mcpserver.Account) (*pixiv.Client, error) {
			return ports.open(pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
		},
		OpenLease: func(ctx context.Context, account mcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
			if ports.readSession != nil {
				snapshot, err := ports.readSession(ctx, pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
				return snapshot.Lease, err
			}
			return ports.openLease(ctx, pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
		},
		Execute: func(ctx context.Context, account mcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return ports.run(ctx, pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride}, attempt)
		},
		ReverseSearch: reverseSearchPorts,
	}
	fanboxPorts := fanboxmcpserver.SDKPorts{
		OpenLease: func(ctx context.Context, account fanboxmcpserver.Account) (*lifecycle.Lease[*fanbox.Client], error) {
			service, err := fanboxService()
			if err != nil {
				return nil, err
			}
			return service.Open(ctx, session.OpenRequest{ProxyOverride: account.HTTPSProxyOverride})
		},
	}
	if request.ListenAddr == nil {
		sdkPorts.Downloads = &mcpserver.LocalDownloads{Service: a.DownloadService(), Path: runtime.DownloadPath, FilenameTemplate: runtime.FilenameTemplate, DirectoryTemplate: runtime.DirectoryTemplate}
		// Resolve once through the local Facade, then pin all phases to that identity.
		sdkPorts.ResolveAccount = func(ctx context.Context, account mcpserver.Account) (mcpserver.Account, error) {
			err := ports.run(ctx, pixivcmd.Request{HTTPSProxyOverride: account.HTTPSProxyOverride}, func(_ context.Context, client *pixiv.Client) (bool, error) {
				account.UserID = client.UserID()
				return false, nil
			})
			return account, err
		}
		sdkPorts.Read = nil
		sdkPorts.OpenLease = func(ctx context.Context, account mcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
			return ports.openLease(ctx, pixivcmd.Request{UserID: account.UserID, HTTPSProxyOverride: account.HTTPSProxyOverride})
		}
		return runMCPStdio(ctx, unifiedmcp.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride), a.in, a.out)
	}
	if ports.watchReadSessions != nil {
		if err := ports.watchReadSessions(ctx); err != nil {
			return err
		}
	}
	listenAddr, baseURL := *request.ListenAddr, *request.BaseURL
	filename, err := paths.UserDataFile(paths.AppDataDirName, "mcp-state.json")
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
	media, err := mediaproxy.New(baseURL, store, func(ctx context.Context, userID int64) (*lifecycle.Lease[*pixiv.Client], error) {
		return ports.openLease(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
	})
	if err != nil {
		return err
	}
	if ports.readSession != nil {
		if err := media.Configure(mediaproxy.Config{Capacity: runtime.MCPGalleryCacheCapacityMiB * (1 << 20), TTL: mediaproxy.DefaultCacheTTL, Concurrency: mediaproxy.DefaultConcurrency, ObserveAnimation: func(requestCtx context.Context, event mediaproxy.AnimationObservation) {
			emitMCPAnimationEvent(ctx, requestCtx, event)
		}, Observe: func(requestCtx context.Context, stage string, duration time.Duration) {
			emitMCPMediaEvent(ctx, requestCtx, coreDiagnostics.Event{Module: coreDiagnostics.ModulePixivMCP, Kind: coreDiagnostics.EventCompleted, Operation: stage, Duration: duration})
		}}, func(ctx context.Context, userID int64) (mediaproxy.Session, error) {
			return ports.readSession(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: request.HTTPSProxyOverride})
		}); err != nil {
			return err
		}
	}
	if ports.readSession != nil {
		if err := media.ConfigureAnimations(ctx, runtime.MCPGalleryAnimationTempCapacityMiB*(1<<20)); err != nil {
			return err
		}
		defer func() { result = errors.Join(result, media.CloseAnimations()) }()
	}
	sdkPorts.Media, sdkPorts.Accounts, sdkPorts.Login = media, selection, login
	sdkPorts.ResolveAccount = func(ctx context.Context, account mcpserver.Account) (mcpserver.Account, error) {
		userID, err := selection.Resolve(ctx)
		if err != nil {
			return mcpserver.Account{}, err
		}
		account.UserID = userID
		return account, nil
	}
	server := unifiedmcp.New(sdkPorts, account, fanboxPorts, request.HTTPSProxyOverride)

	return runMCPHTTP(ctx, server, listenAddr, baseURL, store, a.errOut, login, media)
}

func reverseSearchFlareSolverr(runtime configapp.RuntimeConfig) *reverseassembly.FlareSolverrOptions {
	if runtime.ReverseSearchFlareSolverr == nil {
		return nil
	}
	return &reverseassembly.FlareSolverrOptions{
		URL:      runtime.ReverseSearchFlareSolverr.URL,
		ProxyURL: runtime.ReverseSearchFlareSolverr.ProxyURL,
	}
}

func reverseSearchProxies(runtime configapp.RuntimeConfig, override *string) (standardProxy, ascii2dProxy string) {
	standardProxy = runtime.HTTPSProxy
	if override != nil {
		return *override, *override
	}
	if runtime.ReverseSearchNetwork.ProxyURL.Present {
		return standardProxy, runtime.ReverseSearchNetwork.ProxyURL.Value
	}
	return standardProxy, standardProxy
}

// newMCPReverseSearchPorts 在 MCP 启动时创建一次反向搜图 Facade，并
// 固定 standard/ascii2d 两个代理网络面、凭据和配置默认值。后续 tool input
// 只允许覆盖 provider，不能改变传输或 credential 依赖，避免同一 MCP session
// 的运行时语义漂移。
func newMCPReverseSearchPorts(runtime configapp.RuntimeConfig, request mcpcommands.Request) (mcpserver.ReverseSearchPorts, error) {
	standardProxy, ascii2dProxy := reverseSearchProxies(runtime, request.HTTPSProxyOverride)
	searcher, err := newCLIMCPReverseSearch(reverseassembly.Options{
		Proxy:        standardProxy,
		ASCII2DProxy: &ascii2dProxy,
		UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
		SauceNAOKey:  runtime.SauceNAOAPIKey,
		FlareSolverr: reverseSearchFlareSolverr(runtime),
	})
	if err != nil {
		return mcpserver.ReverseSearchPorts{}, err
	}
	provider := reversesearch.Provider(runtime.ReverseSearchProvider)
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	return mcpserver.ReverseSearchPorts{
		Searcher:  searcher,
		Provider:  provider,
		PixivOnly: runtime.ReverseSearchPixivOnly,
	}, nil
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
	watchReadSessions func(context.Context) error
	readSession       func(context.Context, pixivcmd.Request) (mediaproxy.Session, error)
	read              func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	localAccounts     func(context.Context) (mcpaccounts.LocalSnapshot, error)
	open              func(pixivcmd.Request) (*pixiv.Client, error)
	openLease         func(context.Context, pixivcmd.Request) (*lifecycle.Lease[*pixiv.Client], error)
	execute           func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	jsonOut           func(*bool) (bool, error)
}

func (p pixivSDKPorts) run(ctx context.Context, request pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
	if p.execute != nil {
		return p.execute(ctx, request, attempt)
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

func pixivOptionsFromRequest(request pixivcmd.Request, loadRuntime func() (configapp.RuntimeConfig, error)) (pixiv.Options, error) {
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
		httpClient, err := httpclient.HTTPClient(*proxyValue)
		if err != nil {
			return pixiv.Options{}, err
		}
		options.HTTPClient = httpClient
	}
	return options, nil
}

// fanboxsdkOptions 是本包对公开 SDK options 类型的本地别名。保留别名的原因：
// 让 composition root 成为**唯一**把一份 config snapshot 翻译为 SDK options 的
// 地方，命令 owner 不需要（也不应该）直接依赖 SDK 的 options 形状。
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

func (a app) InitMCPAuth(ctx context.Context, reset bool) (string, error) {
	filename, err := paths.UserDataFile(paths.AppDataDirName, "mcp-state.json")
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
			_, loaded, err := newCLIAccountServices(a)
			if err != nil {
				return mcpaccounts.LoginAttempt{}, err
			}
			service = &loaded
		}
		options := pixiv.LoginOptions{}
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

func emitMCPMediaEvent(lifecycleCtx, requestCtx context.Context, event coreDiagnostics.Event) {
	if _, ok := coreDiagnostics.ScopeFromContext(requestCtx); !ok {
		requestCtx = lifecycleCtx
	}
	coreDiagnostics.Emit(requestCtx, event)
}

func emitMCPAnimationEvent(lifecycleCtx, requestCtx context.Context, observation mediaproxy.AnimationObservation) {
	event := coreDiagnostics.Event{Module: coreDiagnostics.ModulePixivMCP, Kind: coreDiagnostics.EventCompleted, Operation: observation.Stage, Duration: observation.Duration, Count: int(observation.Bytes)}
	if observation.Failed {
		event.Kind = coreDiagnostics.EventFailed
		event.Reason = coreDiagnostics.ReasonToolFailed
	}
	if observation.CancellationSource != "" {
		event.Kind = coreDiagnostics.EventFailed
		switch observation.CancellationSource {
		case "request":
			event.Reason = coreDiagnostics.ReasonRequestCancelled
		case "transport":
			event.Reason = coreDiagnostics.ReasonTransportInterrupted
		case "subscription":
			event.Reason = coreDiagnostics.ReasonSubscriptionReleased
		case "service":
			event.Reason = coreDiagnostics.ReasonServiceClosed
		case "expiry":
			event.Reason = coreDiagnostics.ReasonAuthorizationExpired
		case "authorization":
			event.Reason = coreDiagnostics.ReasonAuthorizationInvalidated
		default:
			event.Reason = coreDiagnostics.ReasonToolFailed
		}
	}
	emitMCPMediaEvent(lifecycleCtx, requestCtx, event)
}

// reverseSearchPort builds the reverse-search executor once per command tree.
func (a app) reverseSearchPort() pixivsearch.ReverseSearchPort {
	return func(ctx context.Context, request pixivsearch.ReverseSearchRequest) (reversesearch.Response, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return reversesearch.Response{}, err
		}
		provider := reversesearch.Provider(runtime.ReverseSearchProvider)
		if request.Provider != "" {
			provider = request.Provider
		}
		standardProxy, ascii2dProxy := reverseSearchProxies(runtime, request.HTTPSProxyOverride)
		searcher, err := newCLIReverseSearch(reverseassembly.Options{
			Proxy:        standardProxy,
			ASCII2DProxy: &ascii2dProxy,
			UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
			SauceNAOKey:  runtime.SauceNAOAPIKey,
			FlareSolverr: reverseSearchFlareSolverr(runtime),
		})
		if err != nil {
			return reversesearch.Response{}, err
		}
		registerReverseSearchCloser(a.closeState, searcher)
		return searcher.Search(ctx, reversesearch.Request{
			Source: request.Source, Provider: provider, PixivOnly: runtime.ReverseSearchPixivOnly,
		})
	}
}
