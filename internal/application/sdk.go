package application

import (
	"context"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// SDKClient 是 CLI 数据命令所需的窄 facade，直接使用 v1 公开 SDK 的模型与分页。
type SDKClient interface {
	// 账号与认证。
	ImportAccount(context.Context, string) (*Account, error)
	ListAccounts() (*AccountsResult, error)
	SelectAccount(int64) error
	RemoveAccount(int64) error
	CheckAccount(context.Context, int64) (*Account, error)
	CheckRefreshToken(context.Context, string) (*Account, error)
	ExportAccountRefreshToken(int64) (string, error)
	Refresh(context.Context) (*Account, error)
	CurrentUserID(context.Context) (int64, error)
	StartLogin() (*pixiv.LoginSession, error)
	CompleteLogin(context.Context, *pixiv.LoginSession, string, pixiv.LoginOptions) (*Account, error)

	// 作品。
	SearchArtworks(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	RelatedArtworks(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	ArtworkRanking(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	RecommendedArtworks(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	FollowingArtworks(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	LatestArtworks(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	UserArtworks(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	UserArtworkBookmarks(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	MyPixivArtworks(context.Context, pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	ArtworkComments(context.Context, pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error)
	TrendingArtworkTags(context.Context, pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error)
	UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)

	// 小说。
	SearchNovels(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	Novel(context.Context, pixiv.NovelRequest) (pixiv.Novel, error)
	RecommendedNovels(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	FollowingNovels(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	LatestNovels(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	UserNovels(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	MyPixivNovels(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)

	// 用户。
	SearchUsers(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	User(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error)
	RecommendedUsers(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	RelatedUsers(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	UserFollowing(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	UserFollowers(context.Context, pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error)
	MyPixivUsers(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)

	// Mutation。
	AddBookmark(context.Context, pixiv.AddBookmarkRequest) error
	RemoveBookmark(context.Context, pixiv.RemoveBookmarkRequest) error
	FollowUser(context.Context, pixiv.FollowUserRequest) error
	UnfollowUser(context.Context, pixiv.UnfollowUserRequest) error

	// 资源。
	ParseResourceRef(string) (sdk.ResourceRef, error)
	OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error)
	SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

// SDKClientRequest 只携带 CLI 显式覆写；账号与凭据选择由 application 服务负责。
type SDKClientRequest struct {
	UserID                  int64
	RefreshToken            string
	HTTPSProxyOverride      *string
	RequestIntervalOverride *time.Duration
}

// SDKClientFactory 打开一个 SDKClient。
type SDKClientFactory func(SDKClientRequest) (SDKClient, error)

// SDKPooledOperation 在账号池安全重放边界内执行一次内容读取。
type SDKPooledOperation func(context.Context, SDKClientRequest, func(context.Context, SDKClient) (committed bool, err error)) error

// SDKService 是 CLI/MCP 的 SDK 应用服务。
type SDKService struct {
	NewClient   SDKClientFactory
	LoadRuntime func() (config.RuntimeConfig, error)
	RunPooled   SDKPooledOperation
}

// RunPooledOperation 委托账号池执行器。
func (s SDKService) RunPooledOperation(ctx context.Context, req SDKClientRequest, attempt func(context.Context, SDKClient) (committed bool, err error)) error {
	return s.RunPooled(ctx, req, attempt)
}

// Client 打开一个 SDKClient。
func (s SDKService) Client(req SDKClientRequest) (SDKClient, error) {
	return s.NewClient(req)
}

// OpenOperation 打开并返回可用于读取的 SDKClient。
func (s SDKService) OpenOperation(ctx context.Context, req SDKClientRequest) (SDKClient, error) {
	return s.NewClient(req)
}

// JSONOut 返回 JSON 输出开关。
func (s SDKService) JSONOut(override *bool) (bool, error) {
	if override != nil {
		return *override, nil
	}
	runtime, err := s.LoadRuntime()
	if err != nil {
		return false, err
	}
	return runtime.OutputJSON, nil
}

// Runtime 返回运行时配置。
func (s SDKService) Runtime() (config.RuntimeConfig, error) {
	return s.LoadRuntime()
}

// CurrentUserID 打开客户端并返回当前用户 ID。
func (s SDKService) CurrentUserID(ctx context.Context, req SDKClientRequest) (SDKClient, int64, error) {
	client, err := s.NewClient(req)
	if err != nil {
		return nil, 0, err
	}
	userID, err := client.CurrentUserID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return client, userID, nil
}
