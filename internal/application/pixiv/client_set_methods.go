package pixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Validate 检查 composition root 是否把每个能力端口都绑定到同一 operation
// snapshot。失败在打开 operation 时暴露，避免后续通过 nil interface panic。
func (c ClientSet) Validate() error {
	if c.AuthClient == nil {
		return errors.New("pixiv authentication client is not configured")
	}
	if c.ArtworkClient == nil {
		return errors.New("pixiv artwork client is not configured")
	}
	if c.NovelClient == nil {
		return errors.New("pixiv novel client is not configured")
	}
	if c.UserClient == nil {
		return errors.New("pixiv user client is not configured")
	}
	if c.MutationClient == nil {
		return errors.New("pixiv mutation client is not configured")
	}
	if c.ResourceClient == nil {
		return errors.New("pixiv resource client is not configured")
	}
	return nil
}

func (c ClientSet) Configured() bool { return c.Validate() == nil }

// 以下方法只为 downloader 等已有窄 port 提供一个具体 operation adapter。
// application 业务优先直接使用 ClientSet 中对应的窄接口字段；这些委托不再
// 组成一个可以被实现方“全量满足”的 giant interface。
func (c ClientSet) ImportAccount(ctx context.Context, token string) (*Account, error) {
	return c.AuthClient.ImportAccount(ctx, token)
}
func (c ClientSet) ListAccounts() (*AccountsResult, error) { return c.AuthClient.ListAccounts() }
func (c ClientSet) SelectAccount(id int64) error           { return c.AuthClient.SelectAccount(id) }
func (c ClientSet) RemoveAccount(id int64) error           { return c.AuthClient.RemoveAccount(id) }
func (c ClientSet) CheckAccount(ctx context.Context, id int64) (*Account, error) {
	return c.AuthClient.CheckAccount(ctx, id)
}
func (c ClientSet) CheckRefreshToken(ctx context.Context, token string) (*Account, error) {
	return c.AuthClient.CheckRefreshToken(ctx, token)
}
func (c ClientSet) ExportAccountRefreshToken(id int64) (string, error) {
	return c.AuthClient.ExportAccountRefreshToken(id)
}
func (c ClientSet) Refresh(ctx context.Context) (*Account, error) { return c.AuthClient.Refresh(ctx) }
func (c ClientSet) CurrentUserID(ctx context.Context) (int64, error) {
	return c.AuthClient.CurrentUserID(ctx)
}
func (c ClientSet) StartLogin() (*pixiv.LoginSession, error) { return c.AuthClient.StartLogin() }
func (c ClientSet) CompleteLogin(ctx context.Context, session *pixiv.LoginSession, callback string, options pixiv.LoginOptions) (*Account, error) {
	return c.AuthClient.CompleteLogin(ctx, session, callback, options)
}

func (c ClientSet) SearchArtworks(ctx context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.SearchArtworks(ctx, request)
}
func (c ClientSet) Artwork(ctx context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	return c.ArtworkClient.Artwork(ctx, request)
}
func (c ClientSet) RelatedArtworks(ctx context.Context, request pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.RelatedArtworks(ctx, request)
}
func (c ClientSet) ArtworkRanking(ctx context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.ArtworkRanking(ctx, request)
}
func (c ClientSet) RecommendedArtworks(ctx context.Context, request pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.RecommendedArtworks(ctx, request)
}
func (c ClientSet) FollowingArtworks(ctx context.Context, request pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.FollowingArtworks(ctx, request)
}
func (c ClientSet) LatestArtworks(ctx context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.LatestArtworks(ctx, request)
}
func (c ClientSet) UserArtworks(ctx context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.UserArtworks(ctx, request)
}
func (c ClientSet) UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.UserArtworkBookmarks(ctx, request)
}
func (c ClientSet) MyPixivArtworks(ctx context.Context, request pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return c.ArtworkClient.MyPixivArtworks(ctx, request)
}
func (c ClientSet) ArtworkComments(ctx context.Context, request pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error) {
	return c.ArtworkClient.ArtworkComments(ctx, request)
}
func (c ClientSet) TrendingArtworkTags(ctx context.Context, request pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error) {
	return c.ArtworkClient.TrendingArtworkTags(ctx, request)
}
func (c ClientSet) UgoiraMetadata(ctx context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	return c.ArtworkClient.UgoiraMetadata(ctx, request)
}

func (c ClientSet) SearchNovels(ctx context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.SearchNovels(ctx, request)
}
func (c ClientSet) Novel(ctx context.Context, request pixiv.NovelRequest) (pixiv.Novel, error) {
	return c.NovelClient.Novel(ctx, request)
}
func (c ClientSet) RecommendedNovels(ctx context.Context, request pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.RecommendedNovels(ctx, request)
}
func (c ClientSet) FollowingNovels(ctx context.Context, request pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.FollowingNovels(ctx, request)
}
func (c ClientSet) LatestNovels(ctx context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.LatestNovels(ctx, request)
}
func (c ClientSet) UserNovels(ctx context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.UserNovels(ctx, request)
}
func (c ClientSet) MyPixivNovels(ctx context.Context, request pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	return c.NovelClient.MyPixivNovels(ctx, request)
}

func (c ClientSet) SearchUsers(ctx context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.SearchUsers(ctx, request)
}
func (c ClientSet) User(ctx context.Context, request pixiv.UserRequest) (pixiv.UserDetail, error) {
	return c.UserClient.User(ctx, request)
}
func (c ClientSet) RecommendedUsers(ctx context.Context, request pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.RecommendedUsers(ctx, request)
}
func (c ClientSet) RelatedUsers(ctx context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.RelatedUsers(ctx, request)
}
func (c ClientSet) UserFollowing(ctx context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.UserFollowing(ctx, request)
}
func (c ClientSet) UserFollowers(ctx context.Context, request pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.UserFollowers(ctx, request)
}
func (c ClientSet) MyPixivUsers(ctx context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	return c.UserClient.MyPixivUsers(ctx, request)
}

func (c ClientSet) AddBookmark(ctx context.Context, request pixiv.AddBookmarkRequest) error {
	return c.MutationClient.AddBookmark(ctx, request)
}
func (c ClientSet) RemoveBookmark(ctx context.Context, request pixiv.RemoveBookmarkRequest) error {
	return c.MutationClient.RemoveBookmark(ctx, request)
}
func (c ClientSet) FollowUser(ctx context.Context, request pixiv.FollowUserRequest) error {
	return c.MutationClient.FollowUser(ctx, request)
}
func (c ClientSet) UnfollowUser(ctx context.Context, request pixiv.UnfollowUserRequest) error {
	return c.MutationClient.UnfollowUser(ctx, request)
}

func (c ClientSet) ParseResourceRef(value string) (sdk.ResourceRef, error) {
	return c.ResourceClient.ParseResourceRef(value)
}
func (c ClientSet) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	return c.ResourceClient.OpenResource(ctx, request)
}
func (c ClientSet) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	return c.ResourceClient.SaveResource(ctx, ref, options)
}
