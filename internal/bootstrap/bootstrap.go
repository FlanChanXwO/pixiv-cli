package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
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

func NewServices(logger *slog.Logger) application.Services {
	sdkService := application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
		return newSDKClient(logger, request)
	}, LoadRuntime: LoadRuntimeConfig}
	return application.Services{
		Account: application.AccountService{SDK: sdkService, RefreshTokenFromEnv: config.RefreshTokenFromEnv},
		Config:  application.ConfigService{Store: ConfigFileStore{}},
		Login:   application.LoginService{SDK: sdkService, LoadRuntime: LoadRuntimeConfig},
		SDK:     sdkService,
	}
}

// newSDKClient 将 CLI 的显式账号和代理覆写交给公共 SDK。没有 --proxy 时，
// OpenDefault 自己在每个操作读取当前 config 快照；有覆写时才固定本次 transport。
func newSDKClient(logger *slog.Logger, request application.SDKClientRequest) (application.SDKClient, error) {
	options := publicpixiv.Options{UserID: request.UserID, RefreshToken: request.RefreshToken, AuthFilePath: request.AuthFilePath, Logger: logger}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := internalpixiv.HTTPClient(*request.HTTPSProxyOverride)
		if err != nil {
			return nil, err
		}
		options.HTTPClient = httpClient
	}
	return publicpixiv.OpenDefault(options)
}

// NewApplicationLogger 从当前配置建立一次应用根 logger。它不触碰 slog 全局默认值；
// CLI 与 MCP 将其显式传入所有下游组件，确保诊断永远离开 stdout 协议通道。
func NewApplicationLogger(errOut io.Writer) (*slog.Logger, error) {
	settings, err := config.LoadSettingsState()
	if err != nil {
		// 根 logger 的初始化不得抢在 Cobra 前把帮助、config path 等本地协议变成
		// 失败。配置文件不可读或语法损坏时静默 logger；只要文件可解析，下面对
		// log_level/log_format 的显式校验仍会把非法日志配置返回给调用方。
		return slog.New(slog.NewTextHandler(io.Discard, nil)), nil
	}
	// 根 logger 只依赖 logging 自己的两项设置。这样无关 runtime 配置（例如
	// web.fallback_enabled）的错误不会破坏 help/config path 等离线协议；反之
	// 无效 log_level/log_format 仍明确失败，绝不静默回退。
	level, err := settings.Effective("log_level")
	if err != nil {
		return nil, err
	}
	format, err := settings.Effective("log_format")
	if err != nil {
		return nil, err
	}
	return config.NewLogger(errOut, config.RuntimeConfig{LogLevel: level.Value.(string), LogFormat: format.Value.(string)})
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
	Logger     *slog.Logger
	SDK        application.SDKService
	SDKRequest application.SDKClientRequest
	AuthPath   string
}

func NewMCPRuntime(ctx context.Context, logger *slog.Logger, proxyOverride *string) (MCPRuntime, error) {
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
	request := application.SDKClientRequest{HTTPSProxyOverride: proxyOverride, AuthFilePath: authPath}
	if _, account, ok := auth.SelectAuthAccount(store, 0); ok {
		request.UserID = account.UserID
	}
	client, err := newSDKClient(logger, request)
	if err != nil {
		return MCPRuntime{}, err
	}
	if token, err := config.RefreshTokenFromEnv(); err != nil {
		return MCPRuntime{}, err
	} else if token != "" {
		account, err := client.ImportAccount(ctx, token)
		if err != nil {
			return MCPRuntime{}, err
		}
		if err := client.SelectAccount(account.UserID); err != nil {
			return MCPRuntime{}, err
		}
		request.UserID = account.UserID
	}
	manager := download.NewManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
	return MCPRuntime{
		Config:     cfg,
		Manager:    manager,
		Logger:     logger,
		SDK:        NewServices(logger).SDK,
		SDKRequest: request,
		AuthPath:   authPath,
	}, nil
}

func applyRuntimeProxyOverride(cfg *config.RuntimeConfig, override *string) {
	if override != nil {
		cfg.HTTPSProxy = *override
	}
}

func (r MCPRuntime) mcpLog(operation string, started time.Time, result string, userID int64) {
	if r.Logger == nil {
		return
	}
	level := slog.LevelInfo
	if result == "error" {
		level = slog.LevelError
	}
	r.Logger.LogAttrs(nil, level, "pixiv operation",
		slog.String("component", "mcp"), slog.String("operation", operation), slog.String("backend", "local"), slog.Duration("duration", time.Since(started)),
		slog.String("result", result), slog.String("error_code", ""), slog.Int("status", 0), slog.Int64("user_id", userID),
	)
}

func RunMCP(ctx context.Context, errOut io.Writer, proxyOverride *string) error {
	logger, err := NewApplicationLogger(errOut)
	if err != nil {
		return err
	}
	runtime, err := NewMCPRuntime(ctx, logger, proxyOverride)
	if err != nil {
		return err
	}
	server := mcpserver.NewWithSDKDownloadFactory(runtime.Manager, func(client application.SDKClient) mcpserver.DownloadManager {
		return download.NewManager(client, runtime.Logger, runtime.Manager.DownloadPath(), runtime.Config.FilenameTemplate)
	}, runtime.Logger, runtime.SDK, runtime.SDKRequest)
	started := time.Now()
	err = server.Run(ctx, &mcp.StdioTransport{})
	result := "success"
	if err != nil {
		result = "error"
	}
	runtime.mcpLog("run", started, result, runtime.SDKRequest.UserID)
	return err
}
