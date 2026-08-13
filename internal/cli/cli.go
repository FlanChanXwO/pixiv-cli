package cli

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	mcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	stdioruntime "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/stdio"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/network"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	sessionpixiv "github.com/FlanChanXwO/pixiv-cli/internal/session/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/migration"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type commandResources = requirements.Resources

// updateFactories 只保存 update command 与 automatic check 的窄构造入口。
// proxy 选择、输出格式和安装策略仍由各自 command owner 决定。
type updateFactories struct {
	newCoordinator    func(string, io.Writer, io.Writer) (*update.UpdateCoordinator, error)
	newAutomaticCheck func(string) (*update.AutomaticUpdateChecker, error)
}

// runResources 是一次 CLI invocation 的私有 resource graph。命令只声明其
// 生命周期需求；具体资源永不通过它暴露给命令 package。
type runResources struct {
	configStore *config.Store

	runtimeLoaded bool
	runtimeConfig config.RuntimeConfig
	runtimeErr    error

	databaseLoaded bool
	database       *database.DB
	databaseErr    error

	pixivLoaded bool
	pixiv       *pixivapp.Service
	pixivErr    error

	accountLoaded bool
	account       pixivapp.AccountService
	accountErr    error

	loginLoaded bool
	login       pixivapp.LoginService
	loginErr    error

	sdkLoaded bool
	sdk       pixivSDKPorts
	sdkErr    error

	downloadLoaded bool
	download       downloader.DownloadService
	downloadErr    error

	fanboxLoaded bool
	fanbox       *fanboxapp.Service
	fanboxErr    error

	updateLoaded bool
	updates      updateFactories

	closers   []func() error
	closeOnce sync.Once
	closeErr  error
}

// newCLIRunResources 是每次运行私有 graph 的窄测试 seam；其结果不会作为
// exported Runtime facade 暴露给命令包。
var newCLIRunResources = func() (*runResources, error) { return &runResources{}, nil }

func (r *runResources) configStoreValue() config.Store {
	if r.configStore == nil {
		store := config.DefaultStore()
		r.configStore = &store
	}
	return *r.configStore
}

func defaultCLIRuntimeConfig() (config.RuntimeConfig, error) {
	snapshot, err := config.DefaultStore().Current()
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	return snapshot.Runtime()
}

// loadCLIRuntimeConfig 只在 composition root 中读取一次，并由 runResources
// 缓存。测试可替换它验证输入失败和启动路径不会读取配置。
var loadCLIRuntimeConfig = defaultCLIRuntimeConfig

func (r *runResources) runtime() (config.RuntimeConfig, error) {
	if r.runtimeLoaded {
		return r.runtimeConfig, r.runtimeErr
	}
	r.runtimeLoaded = true
	r.runtimeConfig, r.runtimeErr = loadCLIRuntimeConfig()
	return r.runtimeConfig, r.runtimeErr
}

func (r *runResources) authDatabase() (*database.DB, error) {
	if r.databaseLoaded {
		return r.database, r.databaseErr
	}
	r.databaseLoaded = true
	r.database, r.databaseErr = openCLIAuthDatabase()
	if r.databaseErr != nil {
		return r.database, r.databaseErr
	}
	if r.database == nil {
		r.databaseErr = errors.New("auth database factory returned nil")
		return nil, r.databaseErr
	}
	r.closers = append(r.closers, r.database.Close)
	return r.database, nil
}

func (r *runResources) pixivService() (*pixivapp.Service, error) {
	if r.pixivLoaded {
		return r.pixiv, r.pixivErr
	}
	r.pixivLoaded = true
	db, err := r.authDatabase()
	if err != nil {
		r.pixivErr = err
		return nil, err
	}
	r.pixiv = pixivapp.NewService(db, configDefaultStore{store: r.configStoreValue()})
	if r.pixiv == nil {
		r.pixivErr = errors.New("pixiv account service is not configured")
	}
	return r.pixiv, r.pixivErr
}

func (r *runResources) accountService() (pixivapp.AccountService, error) {
	if r.accountLoaded {
		return r.account, r.accountErr
	}
	r.accountLoaded = true
	service, err := r.pixivService()
	if err != nil {
		r.accountErr = err
		return pixivapp.AccountService{}, err
	}
	r.account = pixivapp.AccountService{Pixiv: service, LoadRuntime: r.runtime}
	return r.account, nil
}

// pixivSDKPorts 是一次 CLI invocation 的 Pixiv SDK 窄端口：打开独立认证快照、
// 在账号池重放边界内执行操作、解析 JSON 输出开关。它不是 service locator。
type pixivSDKPorts struct {
	open    func(pixivdeps.Request) (*pixiv.Client, error)
	pooled  func(context.Context, pixivdeps.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	jsonOut func(*bool) (bool, error)
}

func (r *runResources) sdkService() (pixivSDKPorts, error) {
	if r.sdkLoaded {
		return r.sdk, r.sdkErr
	}
	r.sdkLoaded = true
	service, err := r.pixivService()
	if err != nil {
		r.sdkErr = err
		return pixivSDKPorts{}, err
	}
	db, err := r.authDatabase()
	if err != nil {
		r.sdkErr = err
		return pixivSDKPorts{}, err
	}
	r.sdk = pixivSDKPorts{
		open: func(request pixivdeps.Request) (*pixiv.Client, error) {
			return openPixivSDKClient(request, service, r.runtime)
		},
		pooled: newPixivPooledOperation(db, r.runtime, func(request pixivdeps.Request) (*pixiv.Client, error) {
			return openPixivSDKClient(request, service, r.runtime)
		}, time.Now),
		jsonOut: func(override *bool) (bool, error) {
			if override != nil {
				return *override, nil
			}
			runtime, err := r.runtime()
			if err != nil {
				return false, err
			}
			return runtime.OutputJSON, nil
		},
	}
	return r.sdk, nil
}

func (r *runResources) loginService() (pixivapp.LoginService, error) {
	if r.loginLoaded {
		return r.login, r.loginErr
	}
	r.loginLoaded = true
	service, err := r.pixivService()
	if err != nil {
		r.loginErr = err
		return pixivapp.LoginService{}, err
	}
	r.login = pixivapp.LoginService{
		Pixiv:           service,
		LoadRuntime:     r.runtime,
		ProxyHTTPClient: network.HTTPClient,
	}
	return r.login, nil
}

func (r *runResources) downloadService() (downloader.DownloadService, error) {
	if r.downloadLoaded {
		return r.download, r.downloadErr
	}
	r.downloadLoaded = true
	r.download = downloader.DownloadService{NewManager: func(client downloader.DownloadClient, downloadPath, filenameTemplate string) (downloader.DownloadManager, error) {
		return downloader.NewManager(client, downloadPath, filenameTemplate), nil
	}}
	return r.download, nil
}

func (r *runResources) fanboxService() (*fanboxapp.Service, error) {
	if r.fanboxLoaded {
		return r.fanbox, r.fanboxErr
	}
	r.fanboxLoaded = true
	db, err := r.authDatabase()
	if err != nil {
		r.fanboxErr = err
		return nil, err
	}
	service := fanboxapp.NewService(db, configDefaultStore{store: r.configStoreValue()})
	service.LoadOptionsFunc = func() (fanboxsdkOptions, error) {
		return fanboxOptionsFromRuntime(r.runtime)
	}
	r.fanbox = service
	return r.fanbox, nil
}

func (r *runResources) updateFactories() updateFactories {
	if !r.updateLoaded {
		r.updateLoaded = true
		r.updates = updateFactories{
			newCoordinator:    newUpdateCommandCoordinator,
			newAutomaticCheck: newCLIAutomaticUpdateChecker,
		}
	}
	return r.updates
}

func (r *runResources) updateCoordinator(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
	return r.updateFactories().newCoordinator(proxy, out, errOut)
}

func (r *runResources) automaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	return r.updateFactories().newAutomaticCheck(proxy)
}

// prepare constructs only the graph nodes declared by the resolved command.
// It is called after Cobra and stdin input validation have both succeeded.
func (r *runResources) prepare(resources commandResources) error {
	if resources.ConfigSnapshot {
		if _, err := r.runtime(); err != nil {
			return err
		}
	}
	if resources.Database {
		if _, err := r.authDatabase(); err != nil {
			return err
		}
	}
	if resources.PixivAccount {
		if _, err := r.accountService(); err != nil {
			return err
		}
	}
	if resources.PixivLogin {
		if _, err := r.loginService(); err != nil {
			return err
		}
	}
	if resources.PixivSDK {
		if _, err := r.sdkService(); err != nil {
			return err
		}
	}
	if resources.Download {
		if _, err := r.downloadService(); err != nil {
			return err
		}
	}
	if resources.Fanbox {
		if _, err := r.fanboxService(); err != nil {
			return err
		}
	}
	if resources.Update {
		r.updateFactories()
	}
	return nil
}

func (r *runResources) close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		for index := len(r.closers) - 1; index >= 0; index-- {
			r.closeErr = errors.Join(r.closeErr, r.closers[index]())
		}
	})
	return r.closeErr
}

func openCLIAuthDatabase() (*database.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	db, err := database.Open(filepath.Join(home, localstate.AppDataDirName))
	if err != nil {
		return nil, err
	}
	store := config.DefaultStore()
	if err := migration.LegacyAccountPool(context.Background(), db, store.Files, nil); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// configDefaultStore adapts the concrete CLI config store to the narrow
// account-domain default selection ports.
type configDefaultStore struct{ store config.Store }

func (s configDefaultStore) ReadPixivDefaultUserID() (int64, bool, error) {
	return s.store.ReadPixivDefaultUserID()
}

func (s configDefaultStore) SetPixivDefaultUserID(userID int64) error {
	return s.store.SetPixivDefaultUserID(userID)
}

func (s configDefaultStore) ClearPixivDefaultUserID() error {
	return s.store.ClearPixivDefaultUserID()
}

func (s configDefaultStore) ReadFanboxDefaultUserID() (int64, bool, error) {
	return s.store.ReadFanboxDefaultUserID()
}

func (s configDefaultStore) SetFanboxDefaultUserID(userID int64) error {
	return s.store.SetFanboxDefaultUserID(userID)
}

func (s configDefaultStore) ClearFanboxDefaultUserID() error {
	return s.store.ClearFanboxDefaultUserID()
}

// openPixivSDKClient 打开一次独立认证快照的 public SDK client。CLI 只使用本地
// 账号（显式 UID 或默认账号），不接受 refresh-token 数据命令入口。
func openPixivSDKClient(request pixivdeps.Request, auth *pixivapp.Service, loadRuntime func() (config.RuntimeConfig, error)) (*pixiv.Client, error) {
	if auth == nil {
		return nil, errors.New("pixiv auth database is not available")
	}
	options := pixiv.Options{}
	if request.RequestIntervalOverride != nil {
		options.Pacing.MinInterval = *request.RequestIntervalOverride
	}
	proxyValue := request.HTTPSProxyOverride
	if proxyValue == nil {
		runtime, err := loadRuntime()
		if err != nil {
			return nil, err
		}
		if runtime.PixivNetwork.ProxyURL.Present {
			value := runtime.PixivNetwork.ProxyURL.Value
			proxyValue = &value
		} else {
			value := runtime.HTTPSProxy
			proxyValue = &value
		}
	}
	if proxyValue != nil {
		httpClient, err := network.HTTPClient(*proxyValue)
		if err != nil {
			return nil, err
		}
		options.HTTPClient = httpClient
	}
	ctx := context.Background()
	if request.UserID != 0 {
		return auth.OpenAccountClientWith(ctx, request.UserID, options)
	}
	return auth.OpenClientWith(ctx, options)
}

// newPixivPooledOperation 构造 CLI 的账号池 operation。未启用账号池时直接执行
// 一次 factory；启用后由 sessionpixiv.Scheduler 决定是否可在 commit 前换号。
func newPixivPooledOperation(state sessionpixiv.PoolState, loadRuntime func() (config.RuntimeConfig, error), factory func(pixivdeps.Request) (*pixiv.Client, error), now func() time.Time) func(context.Context, pixivdeps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
	return func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		if loadRuntime == nil {
			return errors.New("account pool runtime loader is not configured")
		}
		runtime, err := loadRuntime()
		if err != nil {
			return err
		}
		if factory == nil {
			return errors.New("pixiv sdk client factory is not configured")
		}
		openAndUse := func(ctx context.Context, request pixivdeps.Request, committed *session.Attempt) error {
			return session.Run(ctx,
				func(context.Context) (*pixiv.Client, func() error, error) {
					client, err := factory(request)
					return client, func() error { return nil }, err
				},
				func(ctx context.Context, client *pixiv.Client, attemptState *session.Attempt) error {
					wasCommitted, err := attempt(ctx, client)
					if wasCommitted || attemptState.Committed() {
						committed.Commit()
					}
					return err
				},
			)
		}
		if !runtime.AccountPool.Enabled {
			return openAndUse(ctx, request, &session.Attempt{})
		}
		executor := sessionpixiv.Scheduler{
			Config: runtime.AccountPool,
			State:  state,
			Now:    now,
		}
		return executor.Run(ctx, func(ctx context.Context, userID int64, committed *session.Attempt) error {
			poolRequest := request
			poolRequest.UserID = userID
			return openAndUse(ctx, poolRequest, committed)
		})
	}
}

// fanboxsdkOptions names the exact public SDK options type locally so the
// resource graph remains the only place translating one config snapshot.
type fanboxsdkOptions = fanbox.Options

func fanboxOptionsFromRuntime(loadRuntime func() (config.RuntimeConfig, error)) (fanboxsdkOptions, error) {
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

func newUpdateCoordinator(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
	httpClient, err := network.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector:   update.SourceDetectorFunc(update.DetectInstallSource),
		ReleaseChecker:   releaseClient,
		CommandRunner:    update.NewCommandRunner(out, errOut),
		ReleaseInstaller: update.NewReleaseInstaller(withPublicReleaseSources(productionReleaseInstallerOptions(httpClient), usePublicReleaseSources)),
	})
}

func newAutomaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	httpClient, err := network.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
		SourceDetector: update.SourceDetectorFunc(update.DetectInstallSource),
		ReleaseChecker: releaseClient,
	})
}

func withPublicReleaseSources(options update.ReleaseInstallerOptions, enabled bool) update.ReleaseInstallerOptions {
	options.EnablePublicReleaseSources = enabled
	return options
}

func productionReleaseInstallerOptions(httpClient *http.Client) update.ReleaseInstallerOptions {
	return update.ReleaseInstallerOptions{
		HTTPClient: httpClient,
		TrustedKeys: map[string]ed25519.PublicKey{
			update.ReleaseSigningKeyID: append(ed25519.PublicKey(nil), update.ReleaseSigningPublicKey[:]...),
		},
	}
}

func (r *runResources) runPixivMCP(ctx context.Context, request mcpcommands.Request) error {
	runtime, err := r.runtime()
	if err != nil {
		return err
	}
	if request.HTTPSProxyOverride != nil {
		if _, err := network.HTTPClient(*request.HTTPSProxyOverride); err != nil {
			return err
		}
	}
	ports, err := r.sdkService()
	if err != nil {
		return err
	}
	account := mcpserver.Account{
		HTTPSProxyOverride:      request.HTTPSProxyOverride,
		RequestIntervalOverride: request.RequestIntervalOverride,
	}
	manager := downloader.NewManager(nil, runtime.DownloadPath, runtime.FilenameTemplate)
	server := mcpserver.NewWithSDKDownloadFactory(manager, func(client *pixiv.Client) mcpserver.DownloadManager {
		return downloader.NewManager(client, runtime.DownloadPath, runtime.FilenameTemplate)
	}, mcpserver.SDKPorts{
		Open: func(a mcpserver.Account) (*pixiv.Client, error) {
			return ports.open(pixivdeps.Request{UserID: a.UserID, HTTPSProxyOverride: a.HTTPSProxyOverride, RequestIntervalOverride: a.RequestIntervalOverride})
		},
		Pooled: func(ctx context.Context, a mcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return ports.pooled(ctx, pixivdeps.Request{UserID: a.UserID, HTTPSProxyOverride: a.HTTPSProxyOverride, RequestIntervalOverride: a.RequestIntervalOverride}, attempt)
		},
	}, account)
	return stdioruntime.Run(ctx, server)
}

func (r *runResources) runFanboxMCP(ctx context.Context, service *fanboxapp.Service, proxy *string) error {
	return stdioruntime.Run(ctx, fanboxmcpserver.NewWithProxy(service, proxy))
}
