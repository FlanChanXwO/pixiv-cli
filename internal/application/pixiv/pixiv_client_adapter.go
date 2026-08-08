package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// pixivClientAdapter 是 public SDK 能力字段的生产实现：内容方法委托给 *pixiv.Client，
// 账号/认证方法委托给 authdb-backed 的 Service。
type pixivClientAdapter struct {
	client *pixiv.Client
	auth   *Service
}

// NewPixivSDKClients 包装一个已打开的 *pixiv.Client 与账号服务，构造一次
// operation snapshot 的具名能力集合。各字段引用同一个 adapter，因此不会
// 额外创建 client 或改变 public SDK 的认证快照语义。
func NewPixivSDKClients(client *pixiv.Client, auth *Service) ClientSet {
	adapter := &pixivClientAdapter{client: client, auth: auth}
	return NewClientSet(adapter, adapter, adapter, adapter, adapter, adapter)
}

// ---- 账号与认证（委托 Service）----

func (a *pixivClientAdapter) ImportAccount(ctx context.Context, refreshToken string) (*Account, error) {
	account, err := a.auth.ImportAccount(ctx, refreshToken, false)
	if err != nil {
		return nil, err
	}
	return accountFromPixivApp(account), nil
}

func (a *pixivClientAdapter) ListAccounts() (*AccountsResult, error) {
	accounts, err := a.auth.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	out := &AccountsResult{Accounts: make([]Account, 0, len(accounts))}
	for _, account := range accounts {
		out.Accounts = append(out.Accounts, *accountFromPixivApp(account))
		if account.Default {
			out.DefaultID = account.UserID
		}
	}
	return out, nil
}

func (a *pixivClientAdapter) SelectAccount(userID int64) error {
	return a.auth.UseAccount(context.Background(), userID)
}

func (a *pixivClientAdapter) RemoveAccount(userID int64) error {
	return a.auth.RemoveAccount(context.Background(), userID)
}

func (a *pixivClientAdapter) CheckAccount(ctx context.Context, userID int64) (*Account, error) {
	account, err := a.auth.CheckAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	return accountFromPixivApp(account), nil
}

func (a *pixivClientAdapter) CheckRefreshToken(ctx context.Context, refreshToken string) (*Account, error) {
	account, err := a.auth.CheckRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	return accountFromPixivApp(account), nil
}

func (a *pixivClientAdapter) ExportAccountRefreshToken(userID int64) (string, error) {
	return a.auth.ExportRefreshToken(context.Background(), userID)
}

func (a *pixivClientAdapter) Refresh(ctx context.Context) (*Account, error) {
	userID := a.client.UserID()
	if userID == 0 {
		current, err := a.auth.CurrentUser(ctx)
		if err != nil {
			return nil, err
		}
		userID = current.UserID
	}
	account, err := a.auth.RefreshAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	return accountFromPixivApp(account), nil
}

func (a *pixivClientAdapter) CurrentUserID(ctx context.Context) (int64, error) {
	if userID := a.client.UserID(); userID > 0 {
		return userID, nil
	}
	current, err := a.auth.CurrentUser(ctx)
	if err != nil {
		return 0, err
	}
	return current.UserID, nil
}

func (a *pixivClientAdapter) StartLogin() (*pixiv.LoginSession, error) {
	return pixiv.BeginLogin(pixiv.LoginOptions{})
}

func (a *pixivClientAdapter) CompleteLogin(ctx context.Context, session *pixiv.LoginSession, callbackOrCode string, _ pixiv.LoginOptions) (*Account, error) {
	credentials, err := session.Complete(ctx, callbackOrCode)
	if err != nil {
		return nil, err
	}
	account, err := a.auth.CompleteLogin(ctx, credentials, false)
	if err != nil {
		return nil, err
	}
	return accountFromPixivApp(account), nil
}

// ---- 作品 ----

func (a *pixivClientAdapter) SearchArtworks(ctx context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.SearchArtworks(ctx, request)
}

func (a *pixivClientAdapter) Artwork(ctx context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	return a.client.Artwork(ctx, request)
}

func (a *pixivClientAdapter) RelatedArtworks(ctx context.Context, request pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.RelatedArtworks(ctx, request)
}

func (a *pixivClientAdapter) ArtworkRanking(ctx context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.ArtworkRanking(ctx, request)
}

func (a *pixivClientAdapter) RecommendedArtworks(ctx context.Context, request pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.RecommendedArtworks(ctx, request)
}

func (a *pixivClientAdapter) FollowingArtworks(ctx context.Context, request pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.FollowingArtworks(ctx, request)
}

func (a *pixivClientAdapter) LatestArtworks(ctx context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.LatestArtworks(ctx, request)
}

func (a *pixivClientAdapter) UserArtworks(ctx context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.UserArtworks(ctx, request)
}

func (a *pixivClientAdapter) UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.UserArtworkBookmarks(ctx, request)
}

func (a *pixivClientAdapter) MyPixivArtworks(ctx context.Context, request pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return a.client.MyPixivArtworks(ctx, request)
}

func (a *pixivClientAdapter) ArtworkComments(ctx context.Context, request pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error) {
	return a.client.ArtworkComments(ctx, request)
}

func (a *pixivClientAdapter) TrendingArtworkTags(ctx context.Context, request pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error) {
	return a.client.TrendingArtworkTags(ctx, request)
}

func (a *pixivClientAdapter) UgoiraMetadata(ctx context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	return a.client.UgoiraMetadata(ctx, request)
}

// ---- 小说 ----

func (a *pixivClientAdapter) SearchNovels(ctx context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.SearchNovels(ctx, request)
}

func (a *pixivClientAdapter) Novel(ctx context.Context, request pixiv.NovelRequest) (pixiv.Novel, error) {
	return a.client.Novel(ctx, request)
}

func (a *pixivClientAdapter) RecommendedNovels(ctx context.Context, request pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.RecommendedNovels(ctx, request)
}

func (a *pixivClientAdapter) FollowingNovels(ctx context.Context, request pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.FollowingNovels(ctx, request)
}

func (a *pixivClientAdapter) LatestNovels(ctx context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.LatestNovels(ctx, request)
}

func (a *pixivClientAdapter) UserNovels(ctx context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.UserNovels(ctx, request)
}

func (a *pixivClientAdapter) MyPixivNovels(ctx context.Context, request pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return a.client.MyPixivNovels(ctx, request)
}

// ---- 用户 ----

func (a *pixivClientAdapter) SearchUsers(ctx context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.SearchUsers(ctx, request)
}

func (a *pixivClientAdapter) User(ctx context.Context, request pixiv.UserRequest) (pixiv.UserDetail, error) {
	return a.client.User(ctx, request)
}

func (a *pixivClientAdapter) RecommendedUsers(ctx context.Context, request pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.RecommendedUsers(ctx, request)
}

func (a *pixivClientAdapter) RelatedUsers(ctx context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.RelatedUsers(ctx, request)
}

func (a *pixivClientAdapter) UserFollowing(ctx context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.UserFollowing(ctx, request)
}

func (a *pixivClientAdapter) UserFollowers(ctx context.Context, request pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.UserFollowers(ctx, request)
}

func (a *pixivClientAdapter) MyPixivUsers(ctx context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return a.client.MyPixivUsers(ctx, request)
}

// ---- Mutation ----

func (a *pixivClientAdapter) AddBookmark(ctx context.Context, request pixiv.AddBookmarkRequest) error {
	return a.client.AddBookmark(ctx, request)
}

func (a *pixivClientAdapter) RemoveBookmark(ctx context.Context, request pixiv.RemoveBookmarkRequest) error {
	return a.client.RemoveBookmark(ctx, request)
}

func (a *pixivClientAdapter) FollowUser(ctx context.Context, request pixiv.FollowUserRequest) error {
	return a.client.FollowUser(ctx, request)
}

func (a *pixivClientAdapter) UnfollowUser(ctx context.Context, request pixiv.UnfollowUserRequest) error {
	return a.client.UnfollowUser(ctx, request)
}

// ---- 资源 ----

func (a *pixivClientAdapter) ParseResourceRef(text string) (sdk.ResourceRef, error) {
	return sdk.ParseResourceRef(text)
}

func (a *pixivClientAdapter) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	return a.client.OpenResource(ctx, request)
}

func (a *pixivClientAdapter) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	return a.client.SaveResource(ctx, ref, options)
}

func accountFromPixivApp(account Account) *Account {
	return &Account{UserID: account.UserID, Username: account.Username, Default: account.Default, Premium: account.Premium}
}
