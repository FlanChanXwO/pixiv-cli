package application

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

// SDKClient 是 CLI 数据命令所需的窄 facade。它刻意不复用或导出旧 Source 的大接口，
// 让 CLI 通过公共 SDK 获得 cursor、错误和路由语义。
type SDKClient interface {
	CurrentUserID(context.Context) (int64, error)
	SearchIllust(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error)
	IllustDetail(context.Context, int64) (*sdk.IllustDetail, error)
	IllustRanking(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error)
	IllustRecommended(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	UserArtworks(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
	UserBookmarks(context.Context, sdk.UserBookmarksRequest) (*sdk.IllustListResult, error)
	UserFollowing(context.Context, sdk.UserFollowingRequest) (*sdk.UserListResult, error)
	AddBookmark(context.Context, sdk.AddBookmarkRequest) error
	RemoveBookmark(context.Context, sdk.RemoveBookmarkRequest) error
	FollowUser(context.Context, sdk.FollowUserRequest) error
	UnfollowUser(context.Context, sdk.UnfollowUserRequest) error
}

// SDKClientRequest 只携带 CLI 显式覆写；其余 config/auth 优先级由 OpenDefault
// 在每次 SDK 操作快照中处理。
type SDKClientRequest struct {
	UserID             int64
	RefreshToken       string
	HTTPSProxyOverride *string
}

type SDKClientFactory func(SDKClientRequest) (SDKClient, error)

type SDKService struct {
	NewClient   SDKClientFactory
	LoadRuntime func() (config.RuntimeConfig, error)
}

func (s SDKService) Client(req SDKClientRequest) (SDKClient, error) {
	if s.NewClient == nil {
		return nil, errors.New("pixiv sdk client factory is not configured")
	}
	return s.NewClient(req)
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
