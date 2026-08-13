package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	recommendedcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sdkCommandFake struct {
	currentUserID     func(context.Context) (int64, error)
	search            func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	searchNovel       func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	detail            func(context.Context, int64) (pixiv.Artwork, error)
	novelDetail       func(context.Context, pixiv.NovelRequest) (pixiv.Novel, error)
	novelContent      func(context.Context, pixiv.NovelContentRequest) (pixiv.NovelContent, error)
	artworkSeries     func(context.Context, pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error)
	novelSeries       func(context.Context, pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error)
	artworkComments   func(context.Context, pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error)
	novelComments     func(context.Context, pixiv.NovelCommentsRequest) (pixiv.CommentPage, error)
	bookmarkTags      func(context.Context, pixiv.UserArtworkBookmarkTagsRequest) (sdk.Page[pixiv.BookmarkTag], error)
	bookmarkDetail    func(context.Context, pixiv.ArtworkBookmarkRequest) (pixiv.ArtworkBookmarkDetail, error)
	trendingTags      func(context.Context, pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error)
	ranking           func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	recommended       func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	novelRecommended  func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userRecommended   func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	userDetail        func(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error)
	artworks          func(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	bookmarks         func(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	following         func(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	followingArtworks func(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	followingNovels   func(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	latestArtworks    func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	latestNovels      func(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	myPixivUsers      func(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	myPixivArtworks   func(context.Context, pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	myPixivNovels     func(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userNovels        func(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	novelBookmarks    func(context.Context, pixiv.UserNovelBookmarksRequest) (sdk.Page[pixiv.Novel], error)
	searchUser        func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	userFollowers     func(context.Context, pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedUsers      func(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	blockedUsers      func(context.Context, pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	addBookmark       func(context.Context, pixiv.AddBookmarkRequest) error
	removeBookmark    func(context.Context, pixiv.RemoveBookmarkRequest) error
	follow            func(context.Context, pixiv.FollowUserRequest) error
	unfollow          func(context.Context, pixiv.UnfollowUserRequest) error
}

func unimplementedSDKCommand() error { return errors.New("unexpected sdk command") }

func (sdkCommandFake) ImportAccount(context.Context, string) (*pixivapp.AccountSummary, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) ListAccounts() (*pixivapp.AccountsResult, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) SelectAccount(int64) error { return unimplementedSDKCommand() }
func (sdkCommandFake) RemoveAccount(int64) error { return unimplementedSDKCommand() }
func (sdkCommandFake) CheckAccount(context.Context, int64) (*pixivapp.AccountSummary, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) CheckRefreshToken(context.Context, string) (*pixivapp.AccountSummary, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) ExportAccountRefreshToken(int64) (string, error) {
	return "", unimplementedSDKCommand()
}
func (sdkCommandFake) Refresh(context.Context) (*pixivapp.AccountSummary, error) {
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) CurrentUserID(ctx context.Context) (int64, error) {
	if f.currentUserID != nil {
		return f.currentUserID(ctx)
	}
	return 0, unimplementedSDKCommand()
}
func (sdkCommandFake) StartLogin() (*pixiv.LoginSession, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) CompleteLogin(context.Context, *pixiv.LoginSession, string, pixiv.LoginOptions) (*pixivapp.AccountSummary, error) {
	return nil, unimplementedSDKCommand()
}

func (f sdkCommandFake) SearchArtworks(ctx context.Context, r pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.search != nil {
		return f.search(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) SearchNovels(ctx context.Context, r pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.searchNovel != nil {
		return f.searchNovel(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) Artwork(ctx context.Context, r pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	if f.detail != nil {
		return f.detail(ctx, r.ArtworkID)
	}
	return pixiv.Artwork{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) ArtworkSeries(ctx context.Context, r pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.artworkSeries != nil {
		return f.artworkSeries(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (sdkCommandFake) RelatedArtworks(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) ArtworkRanking(ctx context.Context, r pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.ranking != nil {
		return f.ranking(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) RecommendedArtworks(ctx context.Context, r pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.recommended != nil {
		return f.recommended(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) FollowingArtworks(ctx context.Context, r pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.followingArtworks != nil {
		return f.followingArtworks(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) LatestArtworks(ctx context.Context, r pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.latestArtworks != nil {
		return f.latestArtworks(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserArtworks(ctx context.Context, r pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.artworks != nil {
		return f.artworks(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserArtworkBookmarks(ctx context.Context, r pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.bookmarks != nil {
		return f.bookmarks(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) MyPixivArtworks(ctx context.Context, r pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.myPixivArtworks != nil {
		return f.myPixivArtworks(ctx, r)
	}
	return sdk.Page[pixiv.Artwork]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) ArtworkComments(ctx context.Context, r pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error) {
	if f.artworkComments != nil {
		return f.artworkComments(ctx, r)
	}
	return pixiv.CommentPage{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserArtworkBookmarkTags(ctx context.Context, r pixiv.UserArtworkBookmarkTagsRequest) (sdk.Page[pixiv.BookmarkTag], error) {
	if f.bookmarkTags != nil {
		return f.bookmarkTags(ctx, r)
	}
	return sdk.Page[pixiv.BookmarkTag]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) ArtworkBookmark(ctx context.Context, r pixiv.ArtworkBookmarkRequest) (pixiv.ArtworkBookmarkDetail, error) {
	if f.bookmarkDetail != nil {
		return f.bookmarkDetail(ctx, r)
	}
	return pixiv.ArtworkBookmarkDetail{}, unimplementedSDKCommand()
}

func (f sdkCommandFake) TrendingArtworkTags(ctx context.Context, r pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error) {
	if f.trendingTags != nil {
		return f.trendingTags(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	return pixiv.UgoiraMetadata{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) RecommendedNovels(ctx context.Context, r pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.novelRecommended != nil {
		return f.novelRecommended(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) FollowingNovels(ctx context.Context, r pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.followingNovels != nil {
		return f.followingNovels(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) LatestNovels(ctx context.Context, r pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.latestNovels != nil {
		return f.latestNovels(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserNovels(ctx context.Context, r pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.userNovels != nil {
		return f.userNovels(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserNovelBookmarks(ctx context.Context, r pixiv.UserNovelBookmarksRequest) (sdk.Page[pixiv.Novel], error) {
	if f.novelBookmarks != nil {
		return f.novelBookmarks(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) MyPixivNovels(ctx context.Context, r pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.myPixivNovels != nil {
		return f.myPixivNovels(ctx, r)
	}
	return sdk.Page[pixiv.Novel]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) Novel(ctx context.Context, r pixiv.NovelRequest) (pixiv.Novel, error) {
	if f.novelDetail != nil {
		return f.novelDetail(ctx, r)
	}
	return pixiv.Novel{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) NovelContent(ctx context.Context, r pixiv.NovelContentRequest) (pixiv.NovelContent, error) {
	if f.novelContent != nil {
		return f.novelContent(ctx, r)
	}
	return pixiv.NovelContent{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) NovelSeries(ctx context.Context, r pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error) {
	if f.novelSeries != nil {
		return f.novelSeries(ctx, r)
	}
	return pixiv.NovelSeriesResult{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) NovelComments(ctx context.Context, r pixiv.NovelCommentsRequest) (pixiv.CommentPage, error) {
	if f.novelComments != nil {
		return f.novelComments(ctx, r)
	}
	return pixiv.CommentPage{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) SearchUsers(ctx context.Context, r pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.searchUser != nil {
		return f.searchUser(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) User(ctx context.Context, r pixiv.UserRequest) (pixiv.UserDetail, error) {
	if f.userDetail != nil {
		return f.userDetail(ctx, r)
	}
	return pixiv.UserDetail{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) RecommendedUsers(ctx context.Context, r pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.userRecommended != nil {
		return f.userRecommended(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) RelatedUsers(ctx context.Context, r pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.relatedUsers != nil {
		return f.relatedUsers(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserFollowing(ctx context.Context, r pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.following != nil {
		return f.following(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserFollowers(ctx context.Context, r pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.userFollowers != nil {
		return f.userFollowers(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserBlockedUsers(ctx context.Context, r pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.blockedUsers != nil {
		return f.blockedUsers(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) MyPixivUsers(ctx context.Context, r pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.myPixivUsers != nil {
		return f.myPixivUsers(ctx, r)
	}
	return sdk.Page[pixiv.UserPreview]{}, unimplementedSDKCommand()
}
func (f sdkCommandFake) AddBookmark(ctx context.Context, r pixiv.AddBookmarkRequest) error {
	if f.addBookmark != nil {
		return f.addBookmark(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) RemoveBookmark(ctx context.Context, r pixiv.RemoveBookmarkRequest) error {
	if f.removeBookmark != nil {
		return f.removeBookmark(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) FollowUser(ctx context.Context, r pixiv.FollowUserRequest) error {
	if f.follow != nil {
		return f.follow(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) UnfollowUser(ctx context.Context, r pixiv.UnfollowUserRequest) error {
	if f.unfollow != nil {
		return f.unfollow(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (sdkCommandFake) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, unimplementedSDKCommand()
}
func (sdkCommandFake) OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	return nil, unimplementedSDKCommand()
}
func (sdkCommandFake) SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error) {
	return sdk.SavedResource{}, unimplementedSDKCommand()
}

func setTestSDKCommandClient(t *testing.T, client *sdkCommandFake) {
	t.Helper()
	setTestSDKCommandFactory(t, client)
}

func commandArtwork(id int64) pixiv.Artwork {
	return pixiv.Artwork{ID: id, Title: "work", User: pixiv.User{Name: "artist"}}
}

// testCursor 构造一个共享 sdk.Cursor 的不透明测试值；token 只用于区分不同游标。
func testCursor(t *testing.T, token string) sdk.Cursor {
	t.Helper()
	cursor, err := sdk.NewCursor("test", "pagination", 1, "query", []byte(token))
	require.NoError(t, err)
	return cursor
}

func TestRecommendedAllJSONRoutesEverySDKKindThroughOneOperation(t *testing.T) {
	useTempPaths(t)
	var order []string
	setTestSDKCommandFactory(t, &sdkCommandFake{
		recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			if len(order) == 0 {
				order = append(order, "illust")
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}}, nil
			}
			order = append(order, "manga")
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(2)}}, nil
		},
		novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			order = append(order, "novel")
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 42}}}}, nil
		},
		userRecommended: func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			order = append(order, "user")
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "recommended", "all", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []string{"illust", "manga", "novel", "user"}, order)
	assert.Contains(t, stdout.String(), `"illusts"`)
	assert.Contains(t, stdout.String(), `"user_previews"`)
}

func TestRecommendedAllDefersTextOutputAndRejectsKindsBeforeOpeningSDK(t *testing.T) {
	useTempPaths(t)
	opened := 0
	setTestSDKCommandFactoryObserve(t, &sdkCommandFake{
		recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}}, nil
		},
		novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 42}}}}, nil
		},
		userRecommended: func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{}, sdk.NewError("pixiv", "recommend", sdk.MalformedUpstreamResponse)
		},
	}, func(pixivdeps.Request) { opened++ })

	var stdout, stderr bytes.Buffer
	assert.Equal(t, 1, Run([]string{"pixiv", "recommended", "all"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Equal(t, 1, opened)
	stdout.Reset()
	stderr.Reset()
	opened = 0
	assert.Equal(t, 1, Run([]string{"pixiv", "recommended", "unknown"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Equal(t, 0, opened)
}

func TestRecommendedAllAppliesPagePlanIndependentlyToEveryStream(t *testing.T) {
	useTempPaths(t)
	nextIllust := testCursor(t, "i")
	nextNovel := testCursor(t, "n")
	nextUser := testCursor(t, "u")
	var illust, novel, users []sdk.Cursor
	setTestSDKCommandClient(t, &sdkCommandFake{
		recommended: func(_ context.Context, r pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			illust = append(illust, r.Cursor)
			if r.Cursor.IsZero() {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}, Next: nextIllust}, nil
			}
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(11)}}, nil
		},
		novelRecommended: func(_ context.Context, r pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			novel = append(novel, r.Cursor)
			if r.Cursor.IsZero() {
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 42}}}, Next: nextNovel}, nil
			}
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 13, User: pixiv.User{ID: 42}}}}, nil
		},
		userRecommended: func(_ context.Context, r pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			users = append(users, r.Cursor)
			if r.Cursor.IsZero() {
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}}, Next: nextUser}, nil
			}
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 14}}}}, nil
		},
	})
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "recommended", "all", "--page", "2", "--limit", "1", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{{}, nextIllust, {}, nextIllust}, illust)
	assert.Equal(t, []sdk.Cursor{{}, nextNovel}, novel)
	assert.Equal(t, []sdk.Cursor{{}, nextUser}, users)
	for _, args := range [][]string{{"pixiv", "recommended"}, {"pixiv", "recommended", "unknown"}} {
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, Run(args, strings.NewReader(""), &out, &errOut))
		assert.Empty(t, out.String())
	}
}

func TestRecommendedMangaJSONUsesTheSameMangaEnvelopeAsAll(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandClient(t, &sdkCommandFake{
		recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(2)}}, nil
		},
		novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			return sdk.Page[pixiv.Novel]{}, nil
		},
		userRecommended: func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{}, nil
		},
	})
	var single, all, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "recommended", "manga", "--json"}, strings.NewReader(""), &single, &stderr), stderr.String())
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "recommended", "all", "--json"}, strings.NewReader(""), &all, &stderr), stderr.String())
	var one struct {
		Manga []pixiv.Artwork `json:"manga"`
	}
	var every struct {
		Manga []pixiv.Artwork `json:"manga"`
	}
	require.NoError(t, json.Unmarshal(single.Bytes(), &one))
	require.NoError(t, json.Unmarshal(all.Bytes(), &every))
	assert.Equal(t, every.Manga, one.Manga)
	assert.NotContains(t, single.String(), `"illusts"`)
}

// TestRecommendationJSONSpoolCleansUpWhenHeaderWriteFails 用注入的 spool header
// seam 模拟临时文档首段写失败，验证 stdout 完全为空且命令按失败结束。
func TestRecommendationJSONSpoolCleansUpWhenHeaderWriteFails(t *testing.T) {
	useTempPaths(t)
	original := recommendedcommands.WriteSpoolHeader
	recommendedcommands.WriteSpoolHeader = func(io.Writer, string) (int, error) { return 0, errors.New("header write failed") }
	t.Cleanup(func() { recommendedcommands.WriteSpoolHeader = original })
	setTestSDKCommandClient(t, &sdkCommandFake{recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(42)}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "recommended", "all", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "header write failed")
}

func TestUserDetailRoutesRequiredIDAndPrintsCompleteSDKJSON(t *testing.T) {
	useTempPaths(t)
	webpage := "https://example.test/artist"
	want := pixiv.UserDetail{
		User: pixiv.User{ID: 42, Name: "artist", Account: "artist_account", Comment: "hello"},
		Profile: pixiv.UserProfile{
			Webpage: webpage, Region: "Tokyo", CountryCode: "JP", Job: "illustrator",
			TotalIllusts: 10, TotalManga: 2, TotalNovels: 3, TotalFollowUsers: 4,
		},
		ProfilePublicity: pixiv.UserProfilePublicity{Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true},
		Workspace:        pixiv.UserWorkspace{PC: "desktop", Tool: "pen"},
	}
	var got pixiv.UserRequest
	var gotRequest pixivdeps.Request
	setTestSDKCommandFactoryObserve(t, &sdkCommandFake{userDetail: func(_ context.Context, request pixiv.UserRequest) (pixiv.UserDetail, error) {
		got = request
		return want, nil
	}}, func(request pixivdeps.Request) {
		gotRequest = request
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "detail", "42", "--json", "--proxy", "http://127.0.0.1:7890"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.UserRequest{UserID: 42}, got)
	require.NotNil(t, gotRequest.HTTPSProxyOverride)
	assert.Equal(t, "http://127.0.0.1:7890", *gotRequest.HTTPSProxyOverride)
	assert.Contains(t, stdout.String(), `"profile_publicity"`)
	assert.Contains(t, stdout.String(), `"workspace"`)
	var actual map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	user, ok := actual["user"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(42), user["id"])
	profile, ok := actual["profile"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://example.test/artist", profile["webpage"])
	assert.Equal(t, float64(10), profile["total_illusts"])
}

func TestUserDetailTextOmitsEmptyFieldsAndSanitizesWebpage(t *testing.T) {
	useTempPaths(t)
	webpage := "https://alice:secret@example.test/artist?token=secret#private"
	setTestSDKCommandClient(t, &sdkCommandFake{userDetail: func(_ context.Context, request pixiv.UserRequest) (pixiv.UserDetail, error) {
		assert.Equal(t, pixiv.UserRequest{UserID: 42}, request)
		return pixiv.UserDetail{
			User:    pixiv.User{ID: 42, Name: "artist", Account: "artist_account", Comment: "hello"},
			Profile: pixiv.UserProfile{Webpage: webpage, Region: "Tokyo", CountryCode: "JP", Job: "illustrator", TotalIllusts: 10, TotalManga: 2, TotalNovels: 3, TotalFollowUsers: 4},
			Workspace: pixiv.UserWorkspace{
				PC: "desktop",
			},
		}, nil
	}})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "detail", "42"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	output := stdout.String()
	assert.Contains(t, output, "user id: 42\n")
	assert.Contains(t, output, "name: artist\n")
	assert.Contains(t, output, "account: artist_account\n")
	assert.Contains(t, output, "comment: hello\n")
	assert.Contains(t, output, "webpage: https://example.test/artist\n")
	assert.Contains(t, output, "region: Tokyo\n")
	assert.Contains(t, output, "country: JP\n")
	assert.Contains(t, output, "job: illustrator\n")
	assert.Contains(t, output, "artworks: 10\n")
	assert.Contains(t, output, "manga: 2\n")
	assert.Contains(t, output, "novels: 3\n")
	assert.Contains(t, output, "following: 4\n")
	assert.Contains(t, output, "workspace pc: desktop\n")
	assert.NotContains(t, output, "token=secret")
	assert.NotContains(t, output, "alice:secret")
	assert.NotContains(t, output, "#private")
	assert.NotContains(t, output, "workspace monitor:")
	assert.NotContains(t, output, "workspace comment:")
}

func TestUserDetailBindsNoProxyFlag(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandFactoryObserve(t, &sdkCommandFake{userDetail: func(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error) {
		return pixiv.UserDetail{User: pixiv.User{ID: 42}}, nil
	}}, func(request pixivdeps.Request) {
		require.NotNil(t, request.HTTPSProxyOverride)
		assert.Equal(t, "", *request.HTTPSProxyOverride)
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "detail", "42", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
}

// TestUserTextPrintersReturnWriterFailure 通过命令路由验证用户文本输出把 stdout
// 写失败原样返回，而不是吞掉后伪装成功。
func TestUserTextPrintersReturnWriterFailure(t *testing.T) {
	want := errors.New("stdout unavailable")

	t.Run("search", func(t *testing.T) {
		useTempPaths(t)
		setTestSDKCommandClient(t, &sdkCommandFake{searchUser: func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 42, Name: "artist"}}}}, nil
		}})
		var stderr bytes.Buffer
		code := Run([]string{"pixiv", "user", "search", "artist"}, strings.NewReader(""), failingWriter{err: want}, &stderr)
		require.NotZero(t, code)
		assert.Contains(t, stderr.String(), want.Error())
	})

	t.Run("detail", func(t *testing.T) {
		useTempPaths(t)
		setTestSDKCommandClient(t, &sdkCommandFake{userDetail: func(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error) {
			return pixiv.UserDetail{User: pixiv.User{ID: 42, Name: "artist"}}, nil
		}})
		var stderr bytes.Buffer
		code := Run([]string{"pixiv", "user", "detail", "42"}, strings.NewReader(""), failingWriter{err: want}, &stderr)
		require.NotZero(t, code)
		assert.Contains(t, stderr.String(), want.Error())
	})
}

func TestUserDetailRejectsInvalidIDBeforeOpeningSDKAndPreservesTypedErrorOutput(t *testing.T) {
	useTempPaths(t)
	for _, args := range [][]string{
		{"pixiv", "user", "detail"},
		{"pixiv", "user", "detail", "not-a-number"},
		{"pixiv", "user", "detail", "0"},
	} {
		t.Run(strings.Join(args[3:], "/"), func(t *testing.T) {
			factoryCalls := 0
			setTestSDKCommandFactory(t, &sdkCommandFake{})

			var stdout, stderr bytes.Buffer
			assert.Equal(t, 1, Run(args, strings.NewReader(""), &stdout, &stderr))
			assert.Empty(t, stdout.String())
			assert.Equal(t, 0, factoryCalls)
		})
	}

	setTestSDKCommandClient(t, &sdkCommandFake{userDetail: func(context.Context, pixiv.UserRequest) (pixiv.UserDetail, error) {
		return pixiv.UserDetail{}, sdk.NewError("pixiv", "user", sdk.MalformedUpstreamResponse)
	}})
	var stdout, stderr bytes.Buffer
	assert.Equal(t, 1, Run([]string{"pixiv", "user", "detail", "42", "--json"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "malformed_upstream_response")
}

func TestUserCommandsRouteOptionalIDAndMutationsThroughSDK(t *testing.T) {
	useTempPaths(t)
	var gotArtwork pixiv.UserArtworksRequest
	var gotBookmarks pixiv.UserArtworkBookmarksRequest
	var gotFollowing pixiv.UserFollowingRequest
	var gotBookmark pixiv.AddBookmarkRequest
	var gotFollow pixiv.FollowUserRequest
	var removedBookmark pixiv.RemoveBookmarkRequest
	var unfollowed pixiv.UnfollowUserRequest
	currentCalls := 0
	setTestSDKCommandClient(t, &sdkCommandFake{
		currentUserID: func(context.Context) (int64, error) { currentCalls++; return 77, nil },
		artworks: func(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			gotArtwork = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}}, nil
		},
		bookmarks: func(_ context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
			gotBookmarks = request
			return sdk.Page[pixiv.Artwork]{}, nil
		},
		following: func(_ context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
			gotFollowing = request
			return sdk.Page[pixiv.UserPreview]{}, nil
		},
		addBookmark: func(_ context.Context, request pixiv.AddBookmarkRequest) error { gotBookmark = request; return nil },
		follow:      func(_ context.Context, request pixiv.FollowUserRequest) error { gotFollow = request; return nil },
		removeBookmark: func(_ context.Context, request pixiv.RemoveBookmarkRequest) error {
			removedBookmark = request
			return nil
		},
		unfollow: func(_ context.Context, request pixiv.UnfollowUserRequest) error { unfollowed = request; return nil },
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "artworks", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(77), gotArtwork.UserID)
	assert.Equal(t, 1, currentCalls)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "user", "bookmarks", "88", "--restrict", "private", "--tag", "favourite", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.UserArtworkBookmarksRequest{UserID: 88, Restrict: pixiv.RestrictPrivate, Tag: "favourite"}, gotBookmarks)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "user", "following", "99", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.UserFollowingRequest{UserID: 99, Restrict: pixiv.RestrictPublic}, gotFollowing)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "bookmark", "add", "42", "--restrict", "private", "--tag", "first", "--tag", "second"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.AddBookmarkRequest{ArtworkID: 42, Restrict: pixiv.RestrictPrivate, Tags: []string{"first", "second"}}, gotBookmark)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "follow", "add", "88", "--restrict", "private"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.FollowUserRequest{UserID: 88, Restrict: pixiv.RestrictPrivate}, gotFollow)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "bookmark", "remove", "42"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.RemoveBookmarkRequest{ArtworkID: 42}, removedBookmark)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "follow", "remove", "88"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.UnfollowUserRequest{UserID: 88}, unfollowed)
}

func TestSearchDefaultOneBatchSkipsLeadingEmptyUpstreamBatches(t *testing.T) {
	useTempPaths(t)
	nextCursor := testCursor(t, "next")
	laterCursor := testCursor(t, "later")
	var cursors []sdk.Cursor
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		cursors = append(cursors, request.Cursor)
		switch {
		case request.Cursor.IsZero():
			// 模拟本地筛选后的空上游批次，但仍有 continuation。
			return sdk.Page[pixiv.Artwork]{Items: nil, Next: nextCursor}, nil
		case request.Cursor.String() == nextCursor.String():
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(9)}, Next: laterCursor}, nil
		default:
			return sdk.Page[pixiv.Artwork]{}, errors.New("unexpected cursor " + request.Cursor.String())
		}
	}})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{{}, nextCursor}, cursors, "default one logical batch must skip leading empty batches")
	var out struct {
		Illusts []pixiv.Artwork `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Illusts, 1)
	assert.Equal(t, int64(9), out.Illusts[0].ID)
}

func TestListPaginationAndValidationUseOpaqueCursorWithoutCursorFlag(t *testing.T) {
	useTempPaths(t)
	nextCursor := testCursor(t, "next")
	var cursors []sdk.Cursor
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		cursors = append(cursors, request.Cursor)
		switch {
		case request.Cursor.IsZero():
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1), commandArtwork(2)}, Next: nextCursor}, nil
		case request.Cursor.String() == nextCursor.String():
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(3), commandArtwork(4)}}, nil
		default:
			return sdk.Page[pixiv.Artwork]{}, errors.New("unexpected cursor")
		}
	}})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{{}}, cursors, "default consumes exactly one upstream batch")

	cursors = nil
	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json", "--page", "2", "--limit", "2"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{{}, nextCursor}, cursors)
	var out struct {
		Illusts []pixiv.Artwork `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Equal(t, []int64{3, 4}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--page", "0", "--limit", "1"},
		{"pixiv", "search", "miku", "--page", "1"},
		{"pixiv", "search", "miku", "--limit", "-1"},
		{"pixiv", "search", "miku", "--cursor", "secret"},
	} {
		stdout.Reset()
		stderr.Reset()
		assert.NotZero(t, Run(args, strings.NewReader(""), &stdout, &stderr), args)
	}
}

func TestInvalidListOrExplicitUserIDDoesNotOpenSDKOperation(t *testing.T) {
	useTempPaths(t)
	calls := 0
	old := newCLIRunResources
	newCLIRunResources = func() (*runResources, error) {
		resources := newTestResources(t)
		wireClient := openCLIWireClient(t, &sdkCommandFake{})
		resources.sdk.open = func(pixivdeps.Request) (*pixiv.Client, error) {
			calls++
			return wireClient, nil
		}
		return resources, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--page", "0", "--limit", "1"},
		{"pixiv", "ranking", "--page", "1"},
		{"pixiv", "recommended", "illust", "--limit", "-1"},
		{"pixiv", "user", "artworks", "--page", "0", "--limit", "1"},
		{"pixiv", "user", "bookmarks", "not-a-user-id"},
		{"pixiv", "user", "following", "0"},
	} {
		var stdout, stderr bytes.Buffer
		if Run(args, strings.NewReader(""), &stdout, &stderr) == 0 {
			t.Fatalf("invalid command succeeded: %v", args)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid list/user ID opened SDK operation %d times", calls)
	}
}

func TestListJSONSpoolsUntilFullSuccessAndDetailKeepsPages(t *testing.T) {
	useTempPaths(t)
	searchCalls := 0
	setTestSDKCommandClient(t, &sdkCommandFake{
		search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			searchCalls++
			if request.Cursor.IsZero() {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}, Next: testCursor(t, "next")}, nil
			}
			return sdk.Page[pixiv.Artwork]{}, errors.New("upstream failed")
		},
		detail: func(_ context.Context, id int64) (pixiv.Artwork, error) {
			return pixiv.Artwork{ID: id, Pages: []pixiv.ArtworkPage{{PageIndex: 0, Width: 100, Height: 200}, {PageIndex: 1, Width: 300, Height: 400}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	assert.NotZero(t, Run([]string{"pixiv", "search", "miku", "--json", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Equal(t, 2, searchCalls)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "detail", "9", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	var detail pixiv.Artwork
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &detail))
	assert.Len(t, detail.Pages, 2)
}

func TestRunContextCancelsSDKNetworkCommand(t *testing.T) {
	useTempPaths(t)
	started := make(chan struct{})
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(ctx context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		close(started)
		<-ctx.Done()
		return sdk.Page[pixiv.Artwork]{}, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- RunContext(ctx, []string{"pixiv", "search", "miku"}, strings.NewReader(""), &stdout, &stderr)
	}()
	<-started
	cancel()
	assert.NotZero(t, <-done)
	assert.Contains(t, stderr.String(), "upstream_unavailable")
}

func TestListJSONCursorCycleDoesNotWritePartialStdout(t *testing.T) {
	useTempPaths(t)
	calls := 0
	repeatCursor := testCursor(t, "repeat")
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		calls++
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(1)}, Next: repeatCursor}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(2)}, Next: repeatCursor}, nil
	}})
	var stdout, stderr bytes.Buffer
	assert.NotZero(t, Run([]string{"pixiv", "search", "miku", "--json", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Equal(t, 2, calls)
	assert.Contains(t, stderr.String(), "pagination cursor repeated")
}

func TestSelfUserListsReuseOneConcreteOAuthSnapshotAcrossPages(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "artworks", path: "/v1/user/illusts"},
		{name: "bookmarks", path: "/v1/user/bookmarks/illust"},
		{name: "following", path: "/v1/user/following"},
	} {
		t.Run(test.name, func(t *testing.T) {
			authPath, _ := useTempPaths(t)
			require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{DefaultUserID: 202, Accounts: []testAuthAccount{{UserID: 202, RefreshToken: "stored-token"}}}))
			// 数据命令只使用本地 auth store；分页过程中的 token rotation
			// 不得改变这次操作已经选择的账号或 OAuth 快照。
			var oauthCalls int
			var requestedUserIDs []string
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/auth/token":
					oauthCalls++
					require.NoError(t, request.ParseForm())
					assert.Equal(t, "stored-token", request.Form.Get("refresh_token"))
					if oauthCalls > 1 {
						http.Error(w, "old token already rotated", http.StatusBadRequest)
						return
					}
					_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"rotated","user":{"id":202}}`)
				case test.path:
					requestedUserIDs = append(requestedUserIDs, request.URL.Query().Get("user_id"))
					if request.URL.Query().Get("offset") == "1" || request.URL.Query().Get("max_bookmark_id") == "1" {
						if test.name == "following" {
							_, _ = io.WriteString(w, `{"user_previews":[{"user":{"id":2}}]}`)
						} else {
							_, _ = io.WriteString(w, `{"illusts":[{"id":2,"type":"illust","page_count":1,"create_date":"2024-01-01T00:00:00Z","user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}]}`)
						}
						return
					}
					nextKey := "offset=1"
					if test.name == "bookmarks" {
						nextKey = "max_bookmark_id=1"
					}
					nextURL := server.URL + test.path + "?" + nextKey
					if test.name == "following" {
						_, _ = fmt.Fprintf(w, `{"user_previews":[{"user":{"id":1}}],"next_url":%q}`, nextURL)
					} else {
						_, _ = fmt.Fprintf(w, `{"illusts":[{"id":1,"type":"illust","page_count":1,"create_date":"2024-01-01T00:00:00Z","user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}],"next_url":%q}`, nextURL)
					}
				default:
					t.Fatalf("unexpected path=%s", request.URL.Path)
				}
			}))
			defer server.Close()

			old := newCLIRunResources
			newCLIRunResources = func() (*runResources, error) {
				resources := newTestResources(t)
				resources.sdk.open = func(request pixivdeps.Request) (*pixiv.Client, error) {
					client, _, err := pixiv.OpenWith(context.Background(), "stored-token", pixiv.Options{HTTPClient: rewriteHTTPClient(t, server, server.URL, server.URL)})
					if err != nil {
						return nil, err
					}
					_ = request
					return client, nil
				}
				// 此测试验证一条单账号分页操作共享同一个 OAuth 快照；账号池的
				// 选择和持久状态另有专门测试，不能让生产池 wiring 绕过 mock transport。
				resources.sdk.pooled = func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					client, err := resources.sdk.open(request)
					if err != nil {
						return err
					}
					_, err = attempt(ctx, client)
					return err
				}
				return resources, nil
			}
			t.Cleanup(func() { newCLIRunResources = old })

			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run([]string{"pixiv", "user", test.name, "--json", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
			assert.Equal(t, 1, oauthCalls)
			assert.Equal(t, []string{"202", "202"}, requestedUserIDs)
		})
	}
}

var _ io.Writer = (*bytes.Buffer)(nil)

// setTestSDKCommandFactory 让 CLI 测试用 wire-responder 的真实 client 观察命令
// 对 SDK 的调用。fake 的 func 字段按 endpoint 分发并回放 canned 结果。
func setTestSDKCommandFactory(t *testing.T, fake *sdkCommandFake) {
	t.Helper()
	old := newCLIRunResources
	ports := wireCLIPorts(t, fake)
	newCLIRunResources = func() (*runResources, error) {
		resources := newTestResources(t)
		resources.sdk = pixivSDKPorts{open: ports.Open, pooled: ports.Pooled, jsonOut: ports.JSONOut}
		return resources, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })
}

// setTestSDKCommandFactoryObserve 额外观察 open 收到的 pixivdeps.Request。
func setTestSDKCommandFactoryObserve(t *testing.T, fake *sdkCommandFake, observe func(pixivdeps.Request)) {
	t.Helper()
	old := newCLIRunResources
	client := openCLIWireClient(t, fake)
	open := func(request pixivdeps.Request) (*pixiv.Client, error) {
		if observe != nil {
			observe(request)
		}
		return client, nil
	}
	newCLIRunResources = func() (*runResources, error) {
		resources := newTestResources(t)
		resources.sdk = pixivSDKPorts{
			open: open,
			pooled: func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				if _, err := open(request); err != nil {
					return err
				}
				_, err := attempt(ctx, client)
				return err
			},
			jsonOut: func(override *bool) (bool, error) {
				if override != nil {
					return *override, nil
				}
				snapshot, err := config.DefaultStore().Current()
				if err != nil {
					return false, err
				}
				runtime, err := snapshot.Runtime()
				if err != nil {
					return false, err
				}
				return runtime.OutputJSON, nil
			},
		}
		return resources, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })
}
