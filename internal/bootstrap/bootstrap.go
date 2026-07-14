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
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
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
	repo := AuthFileRepository{}
	resolver := application.ClientResolver{
		Auth:            repo,
		LoadRuntime:     LoadRuntimeConfig,
		RefreshTokenEnv: config.RefreshTokenFromEnv,
		NewClient: func(cfg config.RuntimeConfig) (application.ClientBundle, error) {
			client, err := NewPixivClient(cfg)
			if err != nil {
				return application.ClientBundle{}, err
			}
			return application.ClientBundle{Auth: client, Artwork: client, Download: client}, nil
		},
	}
	return application.Services{
		Account: application.AccountService{
			Auth:            repo,
			LoadRuntime:     LoadRuntimeConfig,
			RefreshTokenEnv: config.RefreshTokenFromEnv,
			NewClient: func(cfg config.RuntimeConfig) (application.AuthenticatedPixivClient, error) {
				return NewPixivClient(cfg)
			},
		},
		Config: application.ConfigService{Store: ConfigFileStore{}},
		Artwork: application.ArtworkService{
			Resolver: resolver,
		},
		Download: application.DownloadService{
			Resolver: resolver,
			NewDownloader: func(client application.DownloadClient, cfg config.RuntimeConfig) application.Downloader {
				return download.NewManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
			},
		},
		Login: application.LoginService{
			Auth:        repo,
			LoadRuntime: LoadRuntimeConfig,
			NewOAuth: func(cfg config.RuntimeConfig, oauthBase string) (application.OAuthExchanger, error) {
				client, err := pixiv.NewOAuthClient(pixiv.OAuthConfig{HTTPSProxy: cfg.HTTPSProxy}, oauthBase)
				if err != nil {
					return nil, err
				}
				return oauthClient{client: client}, nil
			},
		},
		SDK: application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
			return newSDKClient(logger, request)
		}, LoadRuntime: LoadRuntimeConfig},
	}
}

// newSDKClient 将 CLI 的显式账号和代理覆写交给公共 SDK。没有 --proxy 时，
// OpenDefault 自己在每个操作读取当前 config 快照；有覆写时才固定本次 transport。
func newSDKClient(logger *slog.Logger, request application.SDKClientRequest) (application.SDKClient, error) {
	options := publicpixiv.Options{UserID: request.UserID, RefreshToken: request.RefreshToken, AuthFilePath: request.AuthFilePath, Logger: logger}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := pixiv.HTTPClient(*request.HTTPSProxyOverride)
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

func NewPixivClient(cfg config.RuntimeConfig) (*pixiv.Source, error) {
	return pixiv.NewSource(pixiv.SourceConfig{
		RefreshToken:       cfg.RefreshToken,
		HTTPSProxy:         cfg.HTTPSProxy,
		WebFallbackEnabled: cfg.WebFallbackEnabled,
	})
}

type MCPRuntime struct {
	Config     config.RuntimeConfig
	Client     *pixiv.Source
	Manager    *download.Manager
	Logger     *slog.Logger
	SDK        application.SDKService
	SDKRequest application.SDKClientRequest
	AuthPath   string
}

func NewMCPRuntime(logger *slog.Logger, proxyOverride *string) (MCPRuntime, error) {
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
	if cfg.RefreshToken = config.RefreshTokenFromEnv(); cfg.RefreshToken == "" {
		if _, acct, ok := auth.SelectAuthAccount(store, 0); ok {
			cfg.RefreshToken = acct.RefreshToken
		}
	}
	client, err := NewPixivClient(cfg)
	if err != nil {
		return MCPRuntime{}, err
	}
	manager := download.NewManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
	return MCPRuntime{
		Config:     cfg,
		Client:     client,
		Manager:    manager,
		Logger:     logger,
		SDK:        NewServices(logger).SDK,
		SDKRequest: application.SDKClientRequest{HTTPSProxyOverride: proxyOverride, AuthFilePath: authPath},
		AuthPath:   authPath,
	}, nil
}

func applyRuntimeProxyOverride(cfg *config.RuntimeConfig, override *string) {
	if override != nil {
		cfg.HTTPSProxy = *override
	}
}

func (r MCPRuntime) AutoAuthenticate(ctx context.Context) {
	started := time.Now()
	if r.Config.RefreshToken == "" {
		return
	}
	if err := r.Client.Refresh(ctx); err != nil {
		r.mcpLog("auto_authenticate", started, "error", r.Client.UserID())
		return
	}
	if err := r.persistAuthenticatedSource(); err != nil {
		r.mcpLog("auto_authenticate", started, "error", r.Client.UserID())
		return
	}
	r.mcpLog("auto_authenticate", started, "success", r.Client.UserID())
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

// persistAuthenticatedSource 将 legacy Source 刚完成 OAuth refresh 后的旋转 token 写回
// MCP 与公共 SDK 共用的 auth store。它只在已验证身份后操作，且不会记录凭据。
func (r MCPRuntime) persistAuthenticatedSource() error {
	if r.Client.UserID() <= 0 || r.Client.RefreshTokenValue() == "" {
		return fmt.Errorf("authenticated source did not provide account state")
	}
	store, err := auth.LoadAuthStore(r.AuthPath)
	if err != nil {
		return err
	}
	store.Upsert(auth.Account{UserID: r.Client.UserID(), Username: r.Client.UserName(), RefreshToken: r.Client.RefreshTokenValue()})
	store.DefaultUserID = r.Client.UserID()
	return auth.SaveAuthStore(r.AuthPath, store)
}

func RunMCP(ctx context.Context, errOut io.Writer, proxyOverride *string) error {
	logger, err := NewApplicationLogger(errOut)
	if err != nil {
		return err
	}
	runtime, err := NewMCPRuntime(logger, proxyOverride)
	if err != nil {
		return err
	}
	runtime.AutoAuthenticate(ctx)
	// AutoAuthenticate 已把环境/默认 token 的 rotation 保存到 auth store。随后固定
	// 已验证 UID，使 MCP SDK 在后续操作中选择 store 而不是再次使用已消费的环境 token。
	if userID := runtime.Client.UserID(); userID > 0 {
		runtime.SDKRequest.UserID = userID
	}
	server := mcpserver.NewWithSDK(runtime.Client, runtime.Manager, runtime.Logger, runtime.SDK, runtime.SDKRequest)
	started := time.Now()
	err = server.Run(ctx, &mcp.StdioTransport{})
	result := "success"
	if err != nil {
		result = "error"
	}
	runtime.mcpLog("run", started, result, runtime.Client.UserID())
	return err
}

type oauthClient struct {
	client *oauth.Client
}

func (c oauthClient) ExchangeAuthorizationCode(ctx context.Context, code, verifier string) (application.OAuthToken, error) {
	token, err := c.client.ExchangeAuthorizationCode(ctx, code, verifier)
	if err != nil {
		return application.OAuthToken{}, err
	}
	return application.OAuthToken{RefreshToken: token.RefreshToken, UserID: token.UserID, Username: token.Username}, nil
}
