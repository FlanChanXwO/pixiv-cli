package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	publicpixiv "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
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
		SDK: application.SDKService{NewClient: newSDKClient, LoadRuntime: LoadRuntimeConfig},
	}
}

// newSDKClient 将 CLI 的显式账号和代理覆写交给公共 SDK。没有 --proxy 时，
// OpenDefault 自己在每个操作读取当前 config 快照；有覆写时才固定本次 transport。
func newSDKClient(request application.SDKClientRequest) (application.SDKClient, error) {
	options := publicpixiv.Options{UserID: request.UserID, RefreshToken: request.RefreshToken}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := pixiv.HTTPClient(*request.HTTPSProxyOverride)
		if err != nil {
			return nil, err
		}
		options.HTTPClient = httpClient
	}
	return publicpixiv.OpenDefault(options)
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
		SDKRequest: application.SDKClientRequest{HTTPSProxyOverride: proxyOverride},
	}, nil
}

func applyRuntimeProxyOverride(cfg *config.RuntimeConfig, override *string) {
	if override != nil {
		cfg.HTTPSProxy = *override
	}
}

func (r MCPRuntime) AutoAuthenticate(ctx context.Context) {
	if r.Config.RefreshToken == "" {
		return
	}
	if err := r.Client.Refresh(ctx); err != nil {
		r.Logger.Warn("auto-authentication failed", "error", err)
		return
	}
	r.Logger.Info("auto-authentication successful", "user_id", r.Client.UserID())
}

func RunMCP(ctx context.Context, errOut io.Writer, proxyOverride *string) error {
	logger := slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runtime, err := NewMCPRuntime(logger, proxyOverride)
	if err != nil {
		return err
	}
	server := mcpserver.NewWithSDK(runtime.Client, runtime.Manager, runtime.Logger, runtime.SDK, runtime.SDKRequest)
	runtime.AutoAuthenticate(ctx)
	return server.Run(ctx, &mcp.StdioTransport{})
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
