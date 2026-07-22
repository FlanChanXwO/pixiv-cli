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
		Download: application.DownloadService{NewManager: func(client application.DownloadClient, downloadPath, filenameTemplate string) (application.DownloadManager, error) {
			return newDownloadManager(client, logger, downloadPath, filenameTemplate), nil
		}},
	}
}

// newDownloadManager 是 CLI 与 MCP 唯一的生产下载器构造链，避免两处 wiring 漂移。
func newDownloadManager(client application.DownloadClient, logger *slog.Logger, downloadPath, filenameTemplate string) *download.Manager {
	return download.NewManager(client, logger, downloadPath, filenameTemplate)
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
// CLI 与 MCP 将其显式传入所有下游组件。返回的 closer 必须在命令或 MCP 会话结束时关闭，
// 以释放 Windows 上不能由临时目录删除的 JSONL 文件句柄。
// 终端（errOut）默认不输出日志痕迹；操作摘要写入 UserStateDir/pixiv/logs 按日 JSONL。
// 日志目录创建/轮转/清理失败时静默继续。errOut 参数保留以兼容调用签名。
func NewApplicationLogger(errOut io.Writer) (*slog.Logger, io.Closer, error) {
	_ = errOut
	writer := openFileLogWriter()
	settings, err := config.LoadSettingsState()
	if err != nil {
		// 根 logger 的初始化不得抢在 Cobra 前把帮助、config path 等本地协议变成
		// 失败。配置文件不可读或语法损坏时静默 logger；只要文件可解析，下面对
		// log_level/log_format 的显式校验仍会把非法日志配置返回给调用方。
		return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelWarn})), writer, nil
	}
	// 根 logger 只依赖 logging 自己的两项设置。这样无关 runtime 配置（例如
	// web.fallback_enabled）的错误不会破坏 help/config path 等离线协议；反之
	// 无效 log_level/log_format 仍明确失败，绝不静默回退。
	level, err := settings.Effective("log_level")
	if err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	format, err := settings.Effective("log_format")
	if err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	// 仍校验 log_format 配置合法性，但文件日志固定 JSONL，不把文本日志写回终端。
	if _, err := config.NewLogger(io.Discard, config.RuntimeConfig{LogLevel: level.Value.(string), LogFormat: format.Value.(string)}); err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	logger, err := config.NewLogger(writer, config.RuntimeConfig{LogLevel: level.Value.(string), LogFormat: "json"})
	if err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	return logger, writer, nil
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
	manager := newDownloadManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
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
	logger, closer, err := NewApplicationLogger(errOut)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	runtime, err := NewMCPRuntime(ctx, logger, proxyOverride)
	if err != nil {
		return err
	}
	server := mcpserver.NewWithSDKDownloadFactory(runtime.Manager, func(client application.SDKClient) mcpserver.DownloadManager {
		return newDownloadManager(client, runtime.Logger, runtime.Manager.DownloadPath(), runtime.Config.FilenameTemplate)
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
