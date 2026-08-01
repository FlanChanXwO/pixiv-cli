package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/accountpool"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	publicpixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AuthFileRepository struct{}

func (AuthFileRepository) Load() (auth.AuthStore, error) {
	path, err := auth.AuthFilePath()
	if err != nil {
		return auth.AuthStore{}, err
	}
	return auth.LoadAuthStore(path)
}

func (AuthFileRepository) Save(store auth.AuthStore) error {
	path, err := auth.AuthFilePath()
	if err != nil {
		return err
	}
	return auth.SaveAuthStore(path, store)
}

func NewServices() application.Services {
	sdkService := application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
		return newSDKClient(request)
	}, LoadRuntime: LoadRuntimeConfig, RunPooled: newPooledSDKOperation()}
	return application.Services{
		Account: application.AccountService{SDK: sdkService, RefreshTokenFromEnv: config.RefreshTokenFromEnv},
		Config:  application.ConfigService{Store: ConfigFileStore{}},
		Login:   application.LoginService{SDK: sdkService, LoadRuntime: LoadRuntimeConfig},
		SDK:     sdkService,
		Download: application.DownloadService{NewManager: func(client application.DownloadClient, downloadPath, filenameTemplate string) (application.DownloadManager, error) {
			return newDownloadManager(client, downloadPath, filenameTemplate), nil
		}},
	}
}

func newPooledSDKOperation() application.SDKPooledOperation {
	return func(ctx context.Context, request application.SDKClientRequest, attempt func(context.Context, application.SDKClient) (bool, error)) error {
		runtime, err := LoadRuntimeConfig()
		if err != nil {
			return err
		}
		if !runtime.AccountPool.Enabled {
			client, err := newSDKClient(request)
			if err != nil {
				return err
			}
			operation, err := snapshotSDKClient(ctx, client)
			if err != nil {
				return err
			}
			_, err = attempt(ctx, operation)
			return err
		}
		authPath := request.AuthFilePath
		if authPath == "" {
			authPath, err = auth.AuthFilePath()
			if err != nil {
				return err
			}
		}
		executor := application.AccountPoolExecutor{
			Config: runtime.AccountPool,
			State:  accountpool.DefaultStore(),
			AvailableAccounts: func(context.Context) ([]int64, error) {
				store, err := auth.LoadAuthStore(authPath)
				if err != nil {
					return nil, err
				}
				return store.UserIDs(), nil
			},
		}
		return executor.Run(ctx, func(ctx context.Context, userID int64) (bool, error) {
			poolRequest := request
			poolRequest.UserID = userID
			poolRequest.RefreshToken = ""
			poolRequest.AuthFilePath = authPath
			poolRequest.DisableRetryAfterRetry = true
			client, err := newSDKClient(poolRequest)
			if err != nil {
				return false, err
			}
			operation, err := snapshotSDKClient(ctx, client)
			if err != nil {
				return false, err
			}
			return attempt(ctx, operation)
		})
	}
}

func snapshotSDKClient(ctx context.Context, client application.SDKClient) (application.SDKClient, error) {
	if snapshotter, ok := client.(interface {
		Snapshot(context.Context) (*publicpixiv.Client, error)
	}); ok {
		return snapshotter.Snapshot(ctx)
	}
	return client, nil
}

// newDownloadManager 是 CLI 与 MCP 唯一的生产下载器构造链，避免两处 wiring 漂移。
func newDownloadManager(client application.DownloadClient, downloadPath, filenameTemplate string) *download.Manager {
	return download.NewManager(client, downloadPath, filenameTemplate)
}

// newSDKClient 将 CLI 的显式账号和代理覆写交给公共 SDK。没有 --proxy 时，
// OpenDefault 自己在每个操作读取当前 config 快照；有覆写时才固定本次 transport。
func newSDKClient(request application.SDKClientRequest) (application.SDKClient, error) {
	options := publicpixiv.OpenDefaultOptions{
		UserID:                        request.UserID,
		RefreshToken:                  request.RefreshToken,
		AuthFilePath:                  request.AuthFilePath,
		IgnoreEnvironmentRefreshToken: true,
		DisableRetryAfterRetry:        request.DisableRetryAfterRetry,
	}
	if request.RequestIntervalOverride != nil {
		options.RequestInterval = *request.RequestIntervalOverride
	}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := internalpixiv.HTTPClient(*request.HTTPSProxyOverride)
		if err != nil {
			return nil, err
		}
		options.HTTPClient = httpClient
	}
	return publicpixiv.OpenDefaultWith(options)
}

func LoadRuntimeConfig() (config.RuntimeConfig, error) {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	return settings.Runtime()
}

type ConfigFileStore struct{}

func (ConfigFileStore) Path() (string, error) {
	return config.ConfigFilePath()
}

func (ConfigFileStore) Get(alias string) (config.SettingValue, error) {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return config.SettingValue{}, err
	}
	value, err := settings.Effective(alias)
	if err != nil {
		return config.SettingValue{}, fmt.Errorf("%w. valid keys: %s", err, strings.Join(config.ValidSettingAliases(), ", "))
	}
	return value, nil
}

func (ConfigFileStore) Set(alias, raw string) (application.ConfigMutationResult, error) {
	spec, ok := config.SettingSpecByAlias(alias)
	if !ok {
		return application.ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(config.ValidSettingAliases(), ", "))
	}
	_, value, err := config.ParseSettingInput(alias, raw)
	if err != nil {
		return application.ConfigMutationResult{}, err
	}
	path, err := config.ConfigFilePath()
	if err != nil {
		return application.ConfigMutationResult{}, err
	}
	if err := config.SetConfigValue(path, alias, value); err != nil {
		return application.ConfigMutationResult{}, err
	}
	envRaw, hasOverride := config.EnvValue(spec)
	return application.ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

func (ConfigFileStore) Unset(alias string) (application.ConfigMutationResult, error) {
	spec, ok := config.SettingSpecByAlias(alias)
	if !ok {
		return application.ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(config.ValidSettingAliases(), ", "))
	}
	path, err := config.ConfigFilePath()
	if err != nil {
		return application.ConfigMutationResult{}, err
	}
	if _, err := config.UnsetConfigValue(path, alias); err != nil {
		return application.ConfigMutationResult{}, err
	}
	envRaw, hasOverride := config.EnvValue(spec)
	return application.ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

type MCPRuntime struct {
	Config     config.RuntimeConfig
	Manager    *download.Manager
	SDK        application.SDKService
	SDKRequest application.SDKClientRequest
	AuthPath   string
}

func NewMCPRuntime(_ context.Context, proxyOverride *string, requestIntervalOverride *time.Duration) (MCPRuntime, error) {
	cfg, err := LoadRuntimeConfig()
	if err != nil {
		return MCPRuntime{}, err
	}
	applyRuntimeProxyOverride(&cfg, proxyOverride)
	store, err := AuthFileRepository{}.Load()
	if err != nil {
		return MCPRuntime{}, err
	}
	authPath, err := auth.AuthFilePath()
	if err != nil {
		return MCPRuntime{}, err
	}
	request := application.SDKClientRequest{
		HTTPSProxyOverride: proxyOverride, RequestIntervalOverride: requestIntervalOverride, AuthFilePath: authPath,
	}
	if _, account, ok := auth.SelectAuthAccount(store, 0); ok {
		request.UserID = account.UserID
	}
	client, err := newSDKClient(request)
	if err != nil {
		return MCPRuntime{}, err
	}
	manager := newDownloadManager(client, cfg.DownloadPath, cfg.FilenameTemplate)
	return MCPRuntime{
		Config:     cfg,
		Manager:    manager,
		SDK:        NewServices().SDK,
		SDKRequest: request,
		AuthPath:   authPath,
	}, nil
}

func applyRuntimeProxyOverride(cfg *config.RuntimeConfig, override *string) {
	if override != nil {
		cfg.HTTPSProxy = *override
	}
}

func RunMCP(ctx context.Context, proxyOverride *string, requestIntervalOverride *time.Duration) error {
	runtime, err := NewMCPRuntime(ctx, proxyOverride, requestIntervalOverride)
	if err != nil {
		return err
	}
	// 普通 MCP download 使用 SDK 的 src 高层 API；Manager 仅保留给随机推荐下载。
	// 这样 CLI、MCP 与嵌入 SDK 共享同一缓存、续传与并发语义。
	server := mcpserver.NewWithSDK(nil, runtime.Manager, runtime.SDK, runtime.SDKRequest)
	return server.Run(ctx, &mcp.StdioTransport{})
}
