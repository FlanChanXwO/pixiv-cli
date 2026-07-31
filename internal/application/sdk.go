package application

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// SDKClient 是 CLI 数据命令所需的窄 facade。它刻意不复用或导出旧 Source 的大接口，
// 让 CLI 通过公共 SDK 获得 cursor、错误和路由语义。
type SDKClient interface {
	ImportAccount(context.Context, string) (*sdk.Account, error)
	ListAccounts() (*sdk.AccountsResult, error)
	SelectAccount(int64) error
	RemoveAccount(int64) error
	CheckAccount(context.Context, int64) (*sdk.Account, error)
	CheckRefreshToken(context.Context, string) (*sdk.Account, error)
	ExportAccountRefreshToken(int64) (string, error)
	Refresh(context.Context) (*sdk.Account, error)
	StartLogin() (*sdk.LoginSession, error)
	CompleteLogin(context.Context, *sdk.LoginSession, string, sdk.LoginOptions) (*sdk.Account, error)
	CurrentUserID(context.Context) (int64, error)
	SearchIllust(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error)
	SearchNovel(context.Context, sdk.SearchNovelRequest) (*sdk.NovelListResult, error)
	IllustDetail(context.Context, int64) (*sdk.IllustDetail, error)
	IllustRelated(context.Context, sdk.IllustRelatedRequest) (*sdk.IllustListResult, error)
	IllustRanking(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error)
	IllustRecommended(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	MangaRecommended(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	NovelRecommended(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error)
	UserRecommended(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error)
	UserDetail(context.Context, sdk.UserDetailRequest) (*sdk.UserDetailResult, error)
	UserArtworks(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
	UserBookmarks(context.Context, sdk.UserBookmarksRequest) (*sdk.IllustListResult, error)
	UserBookmarksCursor(context.Context, sdk.UserBookmarksRequest, int64) (sdk.Cursor, error)
	UserFollowing(context.Context, sdk.UserFollowingRequest) (*sdk.UserListResult, error)
	FollowingIllusts(context.Context, sdk.FollowingIllustsRequest) (*sdk.IllustListResult, error)
	FollowingNovels(context.Context, sdk.FollowingNovelsRequest) (*sdk.NovelListResult, error)
	LatestIllusts(context.Context, sdk.LatestIllustsRequest) (*sdk.IllustListResult, error)
	LatestNovels(context.Context, sdk.LatestNovelsRequest) (*sdk.NovelListResult, error)
	MyPixivUsers(context.Context, sdk.MyPixivUsersRequest) (*sdk.UserListResult, error)
	MyPixivIllusts(context.Context, sdk.MyPixivIllustsRequest) (*sdk.IllustListResult, error)
	MyPixivNovels(context.Context, sdk.MyPixivNovelsRequest) (*sdk.NovelListResult, error)
	UserNovels(context.Context, sdk.UserNovelsRequest) (*sdk.NovelListResult, error)
	SearchUser(context.Context, sdk.SearchUserRequest) (*sdk.UserListResult, error)
	TrendingTagsIllust(context.Context) (*sdk.TrendingTagsIllustResult, error)
	UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error)
	ParseResourceRef(string) (sdk.ResourceRef, error)
	OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error)
	DownloadResource(context.Context, sdk.ResourceRef, string) (sdk.ResourceDownloadResult, error)
	AddBookmark(context.Context, sdk.AddBookmarkRequest) error
	RemoveBookmark(context.Context, sdk.RemoveBookmarkRequest) error
	FollowUser(context.Context, sdk.FollowUserRequest) error
	UnfollowUser(context.Context, sdk.UnfollowUserRequest) error
}

// AuthBundleSDKClient 是离线认证 bundle 所需的独立 public SDK facade，避免把
// secret-bearing 方法扩散到内容命令和 MCP 的通用测试替身。
type AuthBundleSDKClient interface {
	ExportAuthBundle(sdk.AuthExportSelection) (*sdk.AuthExportBundle, error)
	RestoreAuthBundle(*sdk.AuthExportBundle) (*sdk.AuthRestoreResult, error)
}

// SDKClientRequest 只携带 CLI 显式覆写；其余 config/auth 优先级由 OpenDefault
// 在每次 SDK 操作快照中处理。
type SDKClientRequest struct {
	UserID             int64
	RefreshToken       string
	HTTPSProxyOverride *string
	// DisableRetryAfterRetry 只供账号池首个 429 的安全切换路径使用。
	DisableRetryAfterRetry bool
	// AuthFilePath 允许 MCP 等长驻调用方把 OAuth rotation 固定到同一个受保护 store。
	// 为空时继续使用 OpenDefault 的标准路径。
	AuthFilePath string
}

type SDKClientFactory func(SDKClientRequest) (SDKClient, error)

// SDKPooledOperation 是生产组装提供的账号池执行边界。attempt 的 committed=true
// 表示已有记录输出或文件落盘，此后不得因 429 重放该次操作。
type SDKPooledOperation func(context.Context, SDKClientRequest, func(context.Context, SDKClient) (committed bool, err error)) error

type SDKService struct {
	NewClient   SDKClientFactory
	LoadRuntime func() (config.RuntimeConfig, error)
	RunPooled   SDKPooledOperation
}

// RunPooledOperation 在未启用账号池时保持单账号 operation 语义；生产组装提供
// RunPooled 时由其负责本地账号校验、持久状态和有限的 Retry-After 切换。
func (s SDKService) RunPooledOperation(ctx context.Context, req SDKClientRequest, attempt func(context.Context, SDKClient) (committed bool, err error)) error {
	if attempt == nil {
		return errors.New("pixiv sdk pooled operation attempt is not configured")
	}
	if s.RunPooled != nil {
		return s.RunPooled(ctx, req, attempt)
	}
	client, err := s.OpenOperation(ctx, req)
	if err != nil {
		return err
	}
	_, err = attempt(ctx, client)
	return err
}

func (s SDKService) Client(req SDKClientRequest) (SDKClient, error) {
	if s.NewClient == nil {
		return nil, errors.New("pixiv sdk client factory is not configured")
	}
	return s.NewClient(req)
}

func (s SDKService) AuthBundleClient(req SDKClientRequest) (AuthBundleSDKClient, error) {
	client, err := s.Client(req)
	if err != nil {
		return nil, err
	}
	authClient, ok := client.(AuthBundleSDKClient)
	if !ok {
		return nil, errors.New("pixiv sdk auth bundle client is not configured")
	}
	return authClient, nil
}

// OpenOperation 让一个 CLI 命令的 self 身份解析和所有 cursor 页面共享同一个
// OpenDefault 快照，避免 OAuth rotation 在命令进行中使旧 refresh token 失效。
func (s SDKService) OpenOperation(ctx context.Context, req SDKClientRequest) (SDKClient, error) {
	client, err := s.Client(req)
	if err != nil {
		return nil, err
	}
	if snapshotter, ok := client.(interface {
		Snapshot(context.Context) (*sdk.Client, error)
	}); ok {
		return snapshotter.Snapshot(ctx)
	}
	// Test/embedding adapters may already represent one stable operation. Product
	// factory returns *sdk.Client and always takes the concrete Snapshot branch.
	return client, nil
}

// JSONOut 与既有 CLI config 语义一致：命令显式 --json 覆盖 runtime 配置。
func (s SDKService) JSONOut(override *bool) (bool, error) {
	if s.LoadRuntime == nil {
		return false, errors.New("runtime config loader is not configured")
	}
	runtime, err := s.LoadRuntime()
	if err != nil {
		return false, err
	}
	if override != nil {
		return *override, nil
	}
	return runtime.OutputJSON, nil
}

// Runtime 返回当前本地运行配置，供下载路径这类 SDK 之外的本地输出策略使用。
func (s SDKService) Runtime() (config.RuntimeConfig, error) {
	if s.LoadRuntime == nil {
		return config.RuntimeConfig{}, errors.New("runtime config loader is not configured")
	}
	return s.LoadRuntime()
}

// CurrentUserID 通过 SDK OAuth 快照获取真实身份。这样 --refresh-token 和环境 token
// 都不会被本地默认账号误替代。
func (s SDKService) CurrentUserID(ctx context.Context, req SDKClientRequest) (SDKClient, int64, error) {
	client, err := s.OpenOperation(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	userID, err := client.CurrentUserID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return client, userID, nil
}
