package bootstrap

import (
	"errors"
	"sync"
	"time"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/network"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
)

// RuntimeOptions 是 composition root 的启动参数。命令级代理/间隔仍可通过
// SDKClientRequest 显式覆盖；这里的字段只为 runtime-owned MCP 等调用方提供
// 一次性默认值。
type RuntimeOptions struct {
	ProxyOverride           *string
	RequestIntervalOverride *time.Duration
}

// Runtime 持有一次 CLI/MCP 运行所需的全部应用服务和本地数据库连接。
// 它不把 wiring 复制到 CLI/MCP，也不使用进程级 registry。
type Runtime struct {
	Account  pixivapp.AccountService
	Config   configapp.ConfigService
	Login    pixivapp.LoginService
	SDK      pixivapp.SDKService
	Download downloadapp.DownloadService
	Fanbox   *fanboxapp.Service

	db        *authdb.DB
	closeOnce sync.Once
	closeErr  error
}

// NewRuntime 创建一个拥有独立 authdb 连接的运行时。构造失败会主动关闭已
// 打开的数据库，调用方只需对返回的 Runtime 调用 Close。
func NewRuntime(options RuntimeOptions) (*Runtime, error) {
	db, initializationErr := openAuthDB()
	if initializationErr != nil {
		if db != nil {
			initializationErr = errors.Join(initializationErr, db.Close())
		}
		return nil, initializationErr
	}
	configStore := runtimeConfigFileStore()
	pixivService := newPixivService(db, configStore)
	if pixivService == nil {
		return nil, errors.Join(errors.New("pixiv auth database is not available"), db.Close())
	}
	baseRequest := pixivapp.SDKClientRequest{
		HTTPSProxyOverride:      options.ProxyOverride,
		RequestIntervalOverride: options.RequestIntervalOverride,
	}
	sdkService := pixivapp.SDKService{
		NewClient: func(request pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			request = mergeSDKClientRequest(baseRequest, request)
			return newSDKClient(request, pixivService)
		},
		LoadRuntime: LoadRuntimeConfig,
		RunPooled: pixivapp.NewPooledSDKOperation(pixivapp.PooledSDKOperationOptions{
			LoadRuntime: LoadRuntimeConfig,
			State:       authDBPoolStateStore{db: db},
			Factory: func(request pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
				return newSDKClient(mergeSDKClientRequest(baseRequest, request), pixivService)
			},
			Now: time.Now,
		}),
	}
	return &Runtime{
		Account: pixivapp.AccountService{Pixiv: pixivService, RefreshTokenFromEnv: configapp.RefreshTokenFromEnv, LoadRuntime: LoadRuntimeConfig},
		Config:  configapp.ConfigService{Store: configStore},
		Login:   pixivapp.LoginService{SDK: sdkService, Pixiv: pixivService, LoadRuntime: LoadRuntimeConfig, ProxyHTTPClient: network.HTTPClient},
		SDK:     sdkService,
		Download: downloadapp.DownloadService{NewManager: func(client downloadapp.DownloadClient, downloadPath, filenameTemplate string) (downloadapp.DownloadManager, error) {
			return newDownloadManager(client, downloadPath, filenameTemplate), nil
		}},
		Fanbox: newFanboxService(db, configStore),
		db:     db,
	}, nil
}

func mergeSDKClientRequest(base, override pixivapp.SDKClientRequest) pixivapp.SDKClientRequest {
	if override.HTTPSProxyOverride == nil {
		override.HTTPSProxyOverride = base.HTTPSProxyOverride
	}
	if override.RequestIntervalOverride == nil {
		override.RequestIntervalOverride = base.RequestIntervalOverride
	}
	return override
}

// Close 以幂等方式关闭 runtime-owned 资源。所有关闭错误都显式返回，不再
// 依赖全局 fanbox registry 或进程退出时的隐式清理。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.db != nil {
			r.closeErr = r.db.Close()
		}
	})
	return r.closeErr
}
