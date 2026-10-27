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
	"fmt"
	"sync"
	"time"

	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	fanboxdownload "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

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
	service := pixivaccount.NewService(db, settings.DefaultStore())
	return authcommands.AccountService{Pixiv: service, LoadRuntime: a.runtimeConfig}, pixivaccount.LoginService{Pixiv: service}, nil
}

func (a app) newPixivSDKPorts() (pixivSDKPorts, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return pixivSDKPorts{}, err
	}
	service := pixivaccount.NewService(db, settings.DefaultStore())
	gate := pool.NewGate()
	readSessions := pixivaccount.NewReadSessions(service, gate)
	a.closeState.add(readSessions.Close)
	facade := a.newPixivFacade(db, service, gate)
	return a.newPixivSDKPortClosures(db, facade, readSessions), nil
}

// newPixivFacade 把账号池配置读取与执行器接到 session 编排层；两者都按调用重新读取。
func (a app) newPixivFacade(db *database.DB, service *pixivaccount.Service, gate *pool.Gate) *session.Pixiv {
	return session.NewPixiv(service, gate, func() (session.PoolConfig, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return session.PoolConfig{}, err
		}
		return session.PoolConfig{Enabled: runtime.AccountPool.Enabled, Strategy: string(runtime.AccountPool.Strategy)}, nil
	}, func(config session.PoolConfig) (session.PoolExecutor, error) {
		return pool.Scheduler{
			Config: settings.AccountPoolConfig{Enabled: config.Enabled, Strategy: settings.AccountPoolStrategy(config.Strategy)},
			State:  db,
			Now:    time.Now,
		}, nil
	})
}

func (a app) newPixivSDKPortClosures(db *database.DB, facade *session.Pixiv, readSessions *pixivaccount.ReadSessions) pixivSDKPorts {
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
		localAccounts: func(ctx context.Context) (accounts.LocalSnapshot, error) {
			return localPixivAccounts(ctx, db)
		},
		openLease: openLease,
		execute:   execute,
	}
}

// newReadSessionPort 复用账号读取快照，并按请求选择代理覆写与请求节奏。
func (a app) newReadSessionPort(readSessions *pixivaccount.ReadSessions) func(context.Context, pixivcmd.Request) (mediaproxy.Session, error) {
	return func(ctx context.Context, request pixivcmd.Request) (mediaproxy.Session, error) {
		cfg, err := a.runtimeConfig()
		if err != nil {
			return mediaproxy.Session{}, err
		}
		options, err := pixivOptionsFromRequest(request, func() (settings.RuntimeConfig, error) { return cfg, nil }, a.logger())
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

func localPixivAccounts(ctx context.Context, db *database.DB) (accounts.LocalSnapshot, error) {
	records, err := db.ListPixiv(ctx)
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	defaultID, explicit, err := settings.DefaultStore().ReadPixivDefaultUserID()
	if err != nil {
		return accounts.LocalSnapshot{}, err
	}
	local := accounts.LocalSnapshot{Accounts: make([]accounts.Account, 0, len(records))}
	if explicit {
		local.DefaultUserID = defaultID
	}
	for _, record := range records {
		local.Accounts = append(local.Accounts, accounts.Account{UserID: record.UserID, Username: record.Username, HasCredentials: record.HasRefreshToken()})
	}
	return local, nil
}

func (a app) newFanboxAccountService() (*fanboxaccount.Service, error) {
	db, err := a.openAuthDatabase()
	if err != nil {
		return nil, err
	}
	return fanboxaccount.NewService(db, settings.DefaultStore(), func() (fanbox.Options, error) {
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
	return command.Env{In: a.in, Out: a.out, Err: a.errOut, WrapUsage: newUsageError, EnablePipelineSignal: a.enablePipelineSignal, ResolveJSON: func(override *bool) (bool, error) {
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
		once.Do(func() { ports, portsErr = a.newPixivSDKPorts() })
		if portsErr != nil {
			return portsErr
		}
		return ports.execute(ctx, request, attempt)
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
			account, login, loadErr = a.newPixivAccountServices()
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
		WriteBundle: secretfile.WriteSecretFile,
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
	}, defaultDownloadService)
}

func (a app) newFanboxCommandGroup(env command.Env) *cobra.Command {
	var once sync.Once
	var service *session.Fanbox
	var serviceErr error
	load := func() (*session.Fanbox, error) {
		once.Do(func() { service, serviceErr = a.newFanboxService() })
		return service, serviceErr
	}
	accounts := func() (*fanboxaccount.Service, error) { return a.newFanboxAccountService() }
	children := []*cobra.Command{
		fanboxauth.New(env, accounts, fanboxauth.SystemBrowserProvider{}),
		fanboxdownload.New(env, load, func() (string, error) {
			runtime, err := a.runtimeConfig()
			return runtime.DownloadPath, err
		}),
	}
	children = append(children, post.Commands(env, load)...)
	return newFanboxCommand(env, children)
}

type pixivSDKPorts struct {
	watchReadSessions func(context.Context) error
	readSession       func(context.Context, pixivcmd.Request) (mediaproxy.Session, error)
	read              func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	localAccounts     func(context.Context) (accounts.LocalSnapshot, error)
	openLease         func(context.Context, pixivcmd.Request) (*lifecycle.Lease[*pixiv.Client], error)
	execute           func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

func openCLIAuthDatabase() (*database.DB, error) {
	directory, err := paths.AppDataDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	return database.Open(directory)
}
