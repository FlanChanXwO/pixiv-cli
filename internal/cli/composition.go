package cli

// 本文件负责 CLI 生产依赖装配：账号服务、Pixiv/FANBOX SDK 端口、下载服务与
// 命令树依赖。MCP runtime 在 composition_mcp.go，反向搜图在 composition_search.go，
// SDK options 翻译在 composition_options.go。
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
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	mcpaccounts "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	pixivpool "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	selfupdate "github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
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
	return authcommands.AccountService{Pixiv: service, LoadRuntime: a.runtimeConfig}, pixivaccount.LoginService{Pixiv: service}, nil
}

func (a app) newPixivSDKPorts() (pixivSDKPorts, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return pixivSDKPorts{}, err
	}
	service := pixivaccount.NewService(db, configapp.DefaultStore())
	gate := pixivpool.NewGate()
	readSessions := pixivaccount.NewReadSessions(service, gate)
	a.closeState.add(readSessions.Close)
	facade := a.newPixivFacade(db, service, gate)
	return a.newPixivSDKPortClosures(db, facade, readSessions), nil
}

// newPixivFacade 把账号池配置读取与执行器接到 session 编排层；两者都按调用重新读取。
func (a app) newPixivFacade(db *database.DB, service *pixivaccount.Service, gate *pixivpool.Gate) *session.Pixiv {
	return session.NewPixiv(service, gate, func() (session.PoolConfig, error) {
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
}

func (a app) newPixivSDKPortClosures(db *database.DB, facade *session.Pixiv, readSessions *pixivaccount.ReadSessions) pixivSDKPorts {
	open := func(request pixivcmd.Request) (*pixiv.Client, error) {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig, a.logger())
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
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig, a.logger())
		if err != nil {
			return nil, err
		}
		return facade.Open(ctx, session.Request{UserID: request.UserID, Options: options})
	}
	execute := func(ctx context.Context, request pixivcmd.Request, callback func(context.Context, *pixiv.Client) (bool, error)) error {
		options, err := pixivOptionsFromRequest(request, a.runtimeConfig, a.logger())
		if err != nil {
			return err
		}
		return facade.Use(ctx, session.Request{UserID: request.UserID, Options: options}, callback)
	}
	readSession := a.newReadSessionPort(readSessions)
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
			return localPixivAccounts(ctx, db)
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
	}
}

// newReadSessionPort 复用账号读取快照，并按请求选择代理覆写与请求节奏。
func (a app) newReadSessionPort(readSessions *pixivaccount.ReadSessions) func(context.Context, pixivcmd.Request) (mediaproxy.Session, error) {
	return func(ctx context.Context, request pixivcmd.Request) (mediaproxy.Session, error) {
		cfg, err := a.runtimeConfig()
		if err != nil {
			return mediaproxy.Session{}, err
		}
		options, err := pixivOptionsFromRequest(request, func() (configapp.RuntimeConfig, error) { return cfg, nil }, a.logger())
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
}

func localPixivAccounts(ctx context.Context, db *database.DB) (mcpaccounts.LocalSnapshot, error) {
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
}

func (a app) newFanboxAccountService() (*fanboxaccount.Service, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return nil, err
	}
	return fanboxaccount.NewService(db, configapp.DefaultStore(), func() (fanboxsdkOptions, error) {
		return fanboxOptionsFromRuntime(a.runtimeConfig, a.logger())
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

// readSessionLease 优先复用 MCP 读取快照；没有快照时退回普通 opening lease。
func (p pixivSDKPorts) readSessionLease(ctx context.Context, userID int64, proxyOverride *string) (*lifecycle.Lease[*pixiv.Client], error) {
	if p.readSession == nil {
		return p.openLease(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: proxyOverride})
	}
	snapshot, err := p.readSession(ctx, pixivcmd.Request{UserID: userID, HTTPSProxyOverride: proxyOverride})
	return snapshot.Lease, err
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

func writeAuthExportBundle(path string, body []byte, force bool) error {
	return filesecret.WriteSecretFile(path, body, force)
}
