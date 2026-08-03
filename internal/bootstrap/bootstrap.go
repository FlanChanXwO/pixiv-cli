package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServices() application.Services {
	db, appDataDir := openAuthDB()
	pixivService := newPixivService(db, appDataDir)
	sdkService := application.SDKService{
		NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
			return newSDKClient(request, pixivService)
		},
		LoadRuntime: LoadRuntimeConfig,
		RunPooled:   newPooledSDKOperation(db, pixivService),
	}
	return application.Services{
		Account: application.AccountService{Pixiv: pixivService, RefreshTokenFromEnv: config.RefreshTokenFromEnv},
		Config:  application.ConfigService{Store: ConfigFileStore{}},
		Login:   application.LoginService{SDK: sdkService, Pixiv: pixivService, LoadRuntime: LoadRuntimeConfig},
		SDK:     sdkService,
		Download: application.DownloadService{NewManager: func(client application.DownloadClient, downloadPath, filenameTemplate string) (application.DownloadManager, error) {
			return newDownloadManager(client, downloadPath, filenameTemplate), nil
		}},
		Fanbox: newFanboxService(db, appDataDir),
	}
}

func openAuthDB() (*authdb.DB, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, ""
	}
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	if err != nil {
		return nil, appDataDir
	}
	registerFanboxDB(db)
	return db, appDataDir
}

// newPixivService 构造 authdb-backed 的 Pixiv 应用服务，并把 legacy auth.json
// 一次性迁移到 SQLite。本地状态失败时返回 nil：pixiv 命令会给出明确错误。
func newPixivService(db *authdb.DB, appDataDir string) *pixivapp.Service {
	if db == nil {
		return nil
	}
	_, _ = authdb.MigrateLegacyAuthJSON(context.Background(), appDataDir, filepath.Join(appDataDir, "auth.json"))
	return pixivapp.New(db, appDataDir)
}

func newPooledSDKOperation(db *authdb.DB, auth *pixivapp.Service) application.SDKPooledOperation {
	return func(ctx context.Context, request application.SDKClientRequest, attempt func(context.Context, application.SDKClient) (bool, error)) error {
		runtime, err := LoadRuntimeConfig()
		if err != nil {
			return err
		}
		if !runtime.AccountPool.Enabled {
			client, err := newSDKClient(request, auth)
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		}
		if db == nil {
			return errors.New("pixiv auth database is not available")
		}
		configured := append([]int64(nil), runtime.AccountPool.Accounts...)
		if len(configured) == 0 {
			accounts, err := db.ListPixiv(ctx)
			if err != nil {
				return err
			}
			for _, account := range accounts {
				configured = append(configured, account.UserID)
			}
		}
		if len(configured) == 0 {
			return application.ErrAccountPoolExhausted
		}
		var lastRateLimit error
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			account, err := db.SelectPooledPixiv(ctx, time.Now().Unix(), configured)
			if err != nil {
				if errors.Is(err, authdb.ErrNotFound) {
					return poolExhaustedError(lastRateLimit)
				}
				return err
			}
			poolRequest := request
			poolRequest.UserID = account.UserID
			poolRequest.RefreshToken = ""
			client, err := newSDKClient(poolRequest, auth)
			if err != nil {
				return err
			}
			committed, attemptErr := attempt(ctx, client)
			if attemptErr == nil {
				return nil
			}
			if committed || ctx.Err() != nil {
				return attemptErr
			}
			retryAfter, ok := application.PoolRetryAfter(attemptErr)
			if !ok {
				return attemptErr
			}
			lastRateLimit = attemptErr
			if err := db.FreezePooledPixiv(ctx, account.UserID, time.Now().Add(retryAfter).Unix()); err != nil {
				return err
			}
		}
	}
}

func poolExhaustedError(lastRateLimit error) error {
	if lastRateLimit == nil {
		return application.ErrAccountPoolExhausted
	}
	return fmt.Errorf("%w: %w", application.ErrAccountPoolExhausted, lastRateLimit)
}

// newDownloadManager 是 CLI 与 MCP 唯一的生产下载器构造链，避免两处 wiring 漂移。
func newDownloadManager(client application.DownloadClient, downloadPath, filenameTemplate string) *download.Manager {
	return download.NewManager(client, downloadPath, filenameTemplate)
}

// newSDKClient 将 CLI 的显式账号、代理和请求间隔覆写交给 public SDK。账号选择与
// refresh token rotation 由 pixivapp.Service（authdb-backed）负责。
func newSDKClient(request application.SDKClientRequest, auth *pixivapp.Service) (application.SDKClient, error) {
	if auth == nil {
		return nil, errors.New("pixiv auth database is not available")
	}
	options := pixiv.Options{}
	if request.RequestIntervalOverride != nil {
		options.Pacing.MinInterval = *request.RequestIntervalOverride
	}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := internalpixiv.HTTPClient(*request.HTTPSProxyOverride)
		if err != nil {
			return nil, err
		}
		options.HTTPClient = httpClient
	}
	ctx := context.Background()
	var client *pixiv.Client
	var err error
	if request.RefreshToken != "" {
		var credentials pixiv.Credentials
		client, credentials, err = pixiv.OpenWith(ctx, request.RefreshToken, options)
		_ = credentials
	} else if request.UserID != 0 {
		client, err = auth.OpenAccountClientWith(ctx, request.UserID, options)
	} else {
		client, err = auth.OpenClientWith(ctx, options)
	}
	if err != nil {
		return nil, err
	}
	return application.NewPixivSDKClient(client, auth), nil
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
	db, appDataDir := openAuthDB()
	pixivService := newPixivService(db, appDataDir)
	request := application.SDKClientRequest{
		HTTPSProxyOverride: proxyOverride, RequestIntervalOverride: requestIntervalOverride,
	}
	client, err := newSDKClient(request, pixivService)
	if err != nil {
		return MCPRuntime{}, err
	}
	manager := newDownloadManager(client, cfg.DownloadPath, cfg.FilenameTemplate)
	sdkService := application.SDKService{
		NewClient: func(req application.SDKClientRequest) (application.SDKClient, error) {
			return newSDKClient(req, pixivService)
		},
		LoadRuntime: LoadRuntimeConfig,
		RunPooled:   newPooledSDKOperation(db, pixivService),
	}
	return MCPRuntime{
		Config:     cfg,
		Manager:    manager,
		SDK:        sdkService,
		SDKRequest: request,
		AuthPath:   "",
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
	server := mcpserver.NewWithSDK(nil, runtime.Manager, runtime.SDK, runtime.SDKRequest)
	return server.Run(ctx, &mcp.StdioTransport{})
}
