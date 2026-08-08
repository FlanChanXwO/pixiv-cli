package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/network"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	removeLegacyAccountPoolConfig = func(path string) error {
		return configapp.RemoveLegacyAccountPoolAccountsWithFileStore(path, filesystemConfigFiles{})
	}
)

func openAuthDB() (*authdb.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: determine home directory: %w", err)
	}
	appDataDir := filepath.Join(home, filesystem.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	if err != nil {
		return nil, err
	}
	if err := migrateLegacyAccountPoolConfig(context.Background(), db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// migrateLegacyAccountPoolConfig 将旧 UID allowlist 迁移为 authdb 的
// schedulable 状态，再删除旧配置键。DB 与 config 不是跨文件原子操作；任一
// config 写失败都返回错误，下一次启动会重复同一 DB 映射并重试删除。
func migrateLegacyAccountPoolConfig(ctx context.Context, db *authdb.DB) error {
	files := filesystemConfigFiles{}
	path, err := files.Path()
	if err != nil {
		return fmt.Errorf("account pool migration: resolve config path: %w", err)
	}
	state, err := configapp.LoadSettingsStateAtWithFileStore(path, files)
	if err != nil {
		return fmt.Errorf("account pool migration: read config: %w", err)
	}
	userIDs, present, err := state.LegacyAccountPoolUIDs()
	if err != nil {
		return fmt.Errorf("account pool migration: validate legacy accounts: %w", err)
	}
	if !present {
		return nil
	}
	if err := db.MigratePixivSchedulable(ctx, userIDs); err != nil {
		return fmt.Errorf("account pool migration: update authdb: %w", err)
	}
	if err := removeLegacyAccountPoolConfig(path); err != nil {
		return fmt.Errorf("account pool migration: remove legacy accounts: %w", err)
	}
	return nil
}

// newPixivService 构造 authdb-backed 的 Pixiv 应用服务。本地状态失败时返回 nil：
// pixiv 命令会给出明确错误。
func newPixivService(db *authdb.DB, store configapp.ConfigFileStore) *pixivapp.Service {
	if db == nil {
		return nil
	}
	return pixivapp.New(pixivPersistenceAdapter{db: db}, configDefaultStore{store: store})
}

// authDBPoolStateStore 将账号池事务委托给 authdb。数据库保存 schedulable、
// freeze 和 round-robin marker；application 不再读取或写入第二份 scheduler 状态。
type authDBPoolStateStore struct {
	db *authdb.DB
}

func (s authDBPoolStateStore) Select(ctx context.Context, now time.Time, strategy configapp.AccountPoolStrategy, attempted []int64) (int64, error) {
	account, err := s.db.SelectPooledPixiv(ctx, now.UTC().Unix(), authdb.PoolStrategy(strategy), attempted)
	if err != nil {
		var selectionErr *authdb.PoolSelectionError
		if errors.As(err, &selectionErr) {
			return 0, &pixivapp.PoolSelectionError{Kind: string(selectionErr.Kind), EarliestFrozenUntil: cloneInt64(selectionErr.EarliestFrozenUntil)}
		}
		return 0, err
	}
	return account.UserID, nil
}

func (s authDBPoolStateStore) Freeze(ctx context.Context, userID int64, until, now time.Time) error {
	_ = now
	return s.db.FreezePooledPixiv(ctx, userID, until.UTC().Unix())
}

// newDownloadManager 是 CLI 与 MCP 唯一的生产下载器构造链，避免两处 wiring 漂移。
func newDownloadManager(client downloadapp.DownloadClient, downloadPath, filenameTemplate string) *downloader.Manager {
	return downloader.NewManager(client, downloadPath, filenameTemplate)
}

// newSDKClient 将 CLI 的显式账号、代理和请求间隔覆写交给 public SDK。账号选择与
// refresh token rotation 由 pixivapp.Service（authdb-backed）负责。
func newSDKClient(request pixivapp.SDKClientRequest, auth *pixivapp.Service) (pixivapp.ClientSet, error) {
	if auth == nil {
		return pixivapp.ClientSet{}, errors.New("pixiv auth database is not available")
	}
	options := pixiv.Options{}
	if request.RequestIntervalOverride != nil {
		options.Pacing.MinInterval = *request.RequestIntervalOverride
	}
	proxyValue := request.HTTPSProxyOverride
	if proxyValue == nil {
		runtime, err := LoadRuntimeConfig()
		if err != nil {
			return pixivapp.ClientSet{}, err
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
			return pixivapp.ClientSet{}, err
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
		return pixivapp.ClientSet{}, err
	}
	return pixivapp.NewPixivSDKClients(client, auth), nil
}

func LoadRuntimeConfig() (configapp.RuntimeConfig, error) {
	store := runtimeConfigFileStore()
	path, err := store.Path()
	if err != nil {
		return configapp.RuntimeConfig{}, err
	}
	settings, err := configapp.LoadSettingsStateAtWithFileStore(path, store.Files)
	if err != nil {
		return configapp.RuntimeConfig{}, err
	}
	return settings.Runtime()
}

// EnsureDefaultConfigFile 将首次配置文件创建绑定到 bootstrap 注入的
// filesystem 端口，CLI 不需要直接持有文件系统实现。
func EnsureDefaultConfigFile() error {
	return runtimeConfigFileStore().EnsureDefaultConfigFile()
}

// RunMCP 创建单一 runtime，并在 stdio server 返回后显式关闭其资源。
func RunMCP(ctx context.Context, proxyOverride *string, requestIntervalOverride *time.Duration) (returnErr error) {
	runtime, err := NewRuntime(RuntimeOptions{
		ProxyOverride: proxyOverride, RequestIntervalOverride: requestIntervalOverride,
	})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, closeErr)
		}
	}()
	cfg, err := LoadRuntimeConfig()
	if err != nil {
		return err
	}
	request := pixivapp.SDKClientRequest{
		HTTPSProxyOverride: proxyOverride, RequestIntervalOverride: requestIntervalOverride,
	}
	client, err := runtime.SDK.Client(request)
	if err != nil {
		return err
	}
	manager := newDownloadManager(client, cfg.DownloadPath, cfg.FilenameTemplate)
	// 普通 MCP download 使用 SDK 的 src 高层 API；Manager 仅保留给随机推荐下载。
	server := mcpserver.NewWithSDK(nil, manager, runtime.SDK, request)
	return server.Run(ctx, &mcp.StdioTransport{})
}
