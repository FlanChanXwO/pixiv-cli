package pixiv

import (
	"context"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// AuthClient 是 CLI/MCP 认证和本地账号命令所需的最小能力集。
type AuthClient interface {
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
}

// ArtworkClient 是作品查询能力的最小端口。
type ArtworkClient interface {
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
}

// NovelClient 是小说查询能力的最小端口。
type NovelClient interface {
	SearchNovels(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	Novel(context.Context, pixiv.NovelRequest) (pixiv.Novel, error)
	RecommendedNovels(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	FollowingNovels(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	LatestNovels(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	UserNovels(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	MyPixivNovels(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)
}

// UserClient 是用户查询能力的最小端口。
type UserClient interface {
	SearchUsers(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	User(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error)
	RecommendedUsers(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	RelatedUsers(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	UserFollowing(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	UserFollowers(context.Context, pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error)
	MyPixivUsers(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)
}

// MutationClient 是收藏和关注写操作的最小端口。
type MutationClient interface {
	AddBookmark(context.Context, pixiv.AddBookmarkRequest) error
	RemoveBookmark(context.Context, pixiv.RemoveBookmarkRequest) error
	FollowUser(context.Context, pixiv.FollowUserRequest) error
	UnfollowUser(context.Context, pixiv.UnfollowUserRequest) error
}

// ResourceClient 是资源引用解析、读取和保存的最小端口。
type ResourceClient interface {
	ParseResourceRef(string) (sdk.ResourceRef, error)
	OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error)
	SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

// ClientSet 是一次 public SDK operation snapshot 的能力集合。
//
// 它用具名窄接口字段表达认证、catalog、mutation 和 resource 的边界，避免
// application 通过一个“什么都能做”的 interface 偷渡跨用例依赖。调用方应只
// 取本次用例所需的字段；bootstrap 负责把同一 snapshot 的 adapter 注入各字段。
type ClientSet struct {
	AuthClient
	ArtworkClient
	NovelClient
	UserClient
	MutationClient
	ResourceClient
}

// NewClientSet 组装同一 operation snapshot 的能力字段。六个参数刻意保持
// 独立，调用方无法只实现一个“大而全”的 application interface 就绕过能力边界。
func NewClientSet(auth AuthClient, artwork ArtworkClient, novel NovelClient, user UserClient, mutation MutationClient, resource ResourceClient) ClientSet {
	return ClientSet{
		AuthClient:     auth,
		ArtworkClient:  artwork,
		NovelClient:    novel,
		UserClient:     user,
		MutationClient: mutation,
		ResourceClient: resource,
	}
}

// SDKClientRequest 只携带 CLI 显式覆写；账号与凭据选择由 application 服务负责。
type SDKClientRequest struct {
	UserID                  int64
	RefreshToken            string
	HTTPSProxyOverride      *string
	RequestIntervalOverride *time.Duration
}

// ClientFactory 打开一次带独立认证快照的 public SDK 能力集合。
type ClientFactory func(SDKClientRequest) (ClientSet, error)

// PooledOperation 在账号池安全重放边界内执行一次内容读取。回调接收同一
// operation snapshot 的能力集合，具体用例仍必须选择窄接口字段。
type PooledOperation func(context.Context, SDKClientRequest, func(context.Context, ClientSet) (committed bool, err error)) error

// SDKService 是 CLI/MCP 的 SDK 应用服务。
type SDKService struct {
	NewClient   ClientFactory
	LoadRuntime func() (config.RuntimeConfig, error)
	RunPooled   PooledOperation
}

// RunPooledOperation 委托账号池执行器。
func (s SDKService) RunPooledOperation(ctx context.Context, req SDKClientRequest, attempt func(context.Context, ClientSet) (committed bool, err error)) error {
	return s.RunPooled(ctx, req, attempt)
}

// Client 打开一次 public SDK operation snapshot。
func (s SDKService) Client(req SDKClientRequest) (ClientSet, error) {
	return s.NewClient(req)
}

// OpenOperation 打开并返回可用于读取的 public SDK operation snapshot。
func (s SDKService) OpenOperation(ctx context.Context, req SDKClientRequest) (ClientSet, error) {
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
func (s SDKService) CurrentUserID(ctx context.Context, req SDKClientRequest) (ClientSet, int64, error) {
	client, err := s.NewClient(req)
	if err != nil {
		return ClientSet{}, 0, err
	}
	if client.AuthClient == nil {
		return ClientSet{}, 0, errors.New("pixiv authentication client is not configured")
	}
	userID, err := client.CurrentUserID(ctx)
	if err != nil {
		return ClientSet{}, 0, err
	}
	return client, userID, nil
}
