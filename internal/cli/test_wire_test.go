package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// 本文件把 CLI 根测试的 typed SDK fake 转为真实 public SDK client + wire
// responder：runResources.sdk.open 返回经 pixiv.OpenWith 构造的 client，其
// HTTP transport 按 App API path 分发并回放 canned wire JSON。

// cliWireTransport 是 sdkCommandFake 驱动的 App API wire responder。
type cliWireTransport struct {
	t    *testing.T
	fake *sdkCommandFake

	// cursors 保存从 fake page.Next 派生的 offset→cursor 映射，使继续请求能
	// 还原同一个 cursor 值传给 fake（与 MCP wire responder 的语义一致）。
	cursors map[int64]sdk.Cursor
}

func (tr *cliWireTransport) nextPageURL(cursor sdk.Cursor) *string {
	if cursor.IsZero() {
		return nil
	}
	if tr.cursors == nil {
		tr.cursors = make(map[int64]sdk.Cursor)
	}
	text := cursor.String()
	sum := 0
	for _, r := range text {
		sum = (sum*31 + int(r)) % 100000
	}
	if sum < 0 {
		sum = -sum
	}
	offset := int64(1 + sum)
	tr.cursors[offset] = cursor
	value := "https://app-api.pixiv.net/v1/continuation?offset=" + strconv.Itoa(1+sum)
	return &value
}

func (tr *cliWireTransport) cursorFromQuery(query url.Values) sdk.Cursor {
	offset, _ := strconv.ParseInt(query.Get("offset"), 10, 64)
	if offset <= 0 || tr.cursors == nil {
		return sdk.Cursor{}
	}
	return tr.cursors[offset]
}

func (tr *cliWireTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	fake := tr.fake
	if fake == nil {
		fake = &sdkCommandFake{}
	}
	if request.URL.Path == "/auth/token" {
		userID := int64(42)
		if fake.currentUserID != nil {
			if current, err := fake.currentUserID(request.Context()); err == nil && current > 0 {
				userID = current
			}
		}
		body, _ := json.Marshal(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
			"user":          map[string]any{"id": userID, "name": "artist"},
		})
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	switch request.URL.Path {
	case "/v1/search/illust":
		req := parseCLISearchArtworks(request.URL.Query())
		req.Cursor = tr.cursorFromQuery(request.URL.Query())
		if fake.search != nil {
			page, err := fake.search(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/search/novel":
		req := parseCLISearchNovels(request.URL.Query())
		req.Cursor = tr.cursorFromQuery(request.URL.Query())
		if fake.searchNovel != nil {
			page, err := fake.searchNovel(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/search/user":
		req := pixivsdk.SearchUsersRequest{Word: request.URL.Query().Get("word")}
		if fake.searchUser != nil {
			page, err := fake.searchUser(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIUserPreviewPage(page)
		}
		return tr.wireCLIUserPreviewPage(sdk.Page[pixivsdk.UserPreview]{})
	case "/v1/illust/detail":
		id := cliQueryInt64(request.URL.Query(), "illust_id")
		if fake.detail != nil {
			artwork, err := fake.detail(request.Context(), id)
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLIArtwork(artwork)
		}
		return wireCLIArtwork(pixivsdk.Artwork{})
	case "/v1/novel/detail":
		id := cliQueryInt64(request.URL.Query(), "novel_id")
		if fake.novelDetail != nil {
			novel, err := fake.novelDetail(request.Context(), pixivsdk.NovelRequest{NovelID: id})
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLINovel(novel)
		}
		return wireCLINovel(pixivsdk.Novel{})
	case "/v1/novel/content":
		if fake.novelContent != nil {
			content, err := fake.novelContent(request.Context(), pixivsdk.NovelContentRequest{NovelID: cliQueryInt64(request.URL.Query(), "novel_id")})
			if err != nil {
				return wireCLIError(err)
			}
			_ = content
		}
		return wireCLIError(sdk.NewError("pixiv", "NovelContent", sdk.ContentUnavailable))
	case "/v1/illust/series":
		req := pixivsdk.ArtworkSeriesRequest{SeriesID: cliQueryInt64(request.URL.Query(), "illust_series_id")}
		if fake.artworkSeries != nil {
			page, err := fake.artworkSeries(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLIArtworkSeries(page)
		}
		return wireCLIArtworkSeries(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/novel/series":
		req := pixivsdk.NovelSeriesRequest{SeriesID: cliQueryInt64(request.URL.Query(), "series_id")}
		if fake.novelSeries != nil {
			result, err := fake.novelSeries(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLINovelSeries(result)
		}
		return wireCLINovelSeries(pixivsdk.NovelSeriesResult{})
	case "/v2/illust/comments":
		if fake.artworkComments != nil {
			page, err := fake.artworkComments(request.Context(), pixivsdk.ArtworkCommentsRequest{ArtworkID: cliQueryInt64(request.URL.Query(), "illust_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLICommentPage(page)
		}
		return wireCLICommentPage(pixivsdk.CommentPage{})
	case "/v2/novel/comments":
		if fake.novelComments != nil {
			page, err := fake.novelComments(request.Context(), pixivsdk.NovelCommentsRequest{NovelID: cliQueryInt64(request.URL.Query(), "novel_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLICommentPage(page)
		}
		return wireCLICommentPage(pixivsdk.CommentPage{})
	case "/v1/user/bookmark-tags/illust":
		if fake.bookmarkTags != nil {
			req := pixivsdk.UserArtworkBookmarkTagsRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
			if req.Restrict == "" {
				req.Restrict = pixivsdk.RestrictPublic
			}
			page, err := fake.bookmarkTags(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLIBookmarkTags(page)
		}
		return wireCLIBookmarkTags(sdk.Page[pixivsdk.BookmarkTag]{})
	case "/v1/illust/bookmark/detail":
		if fake.bookmarkDetail != nil {
			detail, err := fake.bookmarkDetail(request.Context(), pixivsdk.ArtworkBookmarkRequest{ArtworkID: cliQueryInt64(request.URL.Query(), "illust_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLIBookmarkDetail(detail)
		}
		return wireCLIBookmarkDetail(pixivsdk.ArtworkBookmarkDetail{})
	case "/v1/trending-tags/illust":
		if fake.trendingTags != nil {
			tags, err := fake.trendingTags(request.Context(), pixivsdk.TrendingArtworkTagsRequest{})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLITrendingTags(tags)
		}
		return tr.wireCLITrendingTags(nil)
	case "/v1/illust/ranking":
		req := pixivsdk.ArtworkRankingRequest{Mode: pixivsdk.RankingMode(request.URL.Query().Get("mode")), Date: request.URL.Query().Get("date")}
		if fake.ranking != nil {
			page, err := fake.ranking(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/illust/recommended":
		if fake.recommended != nil {
			req := pixivsdk.RecommendedArtworksRequest{Cursor: tr.cursorFromQuery(request.URL.Query())}
			page, err := fake.recommended(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/novel/recommended":
		if fake.novelRecommended != nil {
			req := pixivsdk.RecommendedNovelsRequest{Cursor: tr.cursorFromQuery(request.URL.Query())}
			page, err := fake.novelRecommended(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/user/recommended":
		if fake.userRecommended != nil {
			req := pixivsdk.RecommendedUsersRequest{Cursor: tr.cursorFromQuery(request.URL.Query())}
			page, err := fake.userRecommended(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIUserPreviewPage(page)
		}
		return tr.wireCLIUserPreviewPage(sdk.Page[pixivsdk.UserPreview]{})
	case "/v1/user/detail":
		if fake.userDetail != nil {
			user, err := fake.userDetail(request.Context(), pixivsdk.UserRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return wireCLIUserDetail(user)
		}
		return wireCLIUserDetail(pixivsdk.UserDetail{})
	case "/v1/user/illusts":
		if fake.artworks != nil {
			req := pixivsdk.UserArtworksRequest{
				UserID: cliQueryInt64(request.URL.Query(), "user_id"),
				Kind:   pixivsdk.ArtworkKind(request.URL.Query().Get("type")),
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.artworks(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/user/bookmarks/illust":
		if fake.bookmarks != nil {
			req := pixivsdk.UserArtworkBookmarksRequest{
				UserID:   cliQueryInt64(request.URL.Query(), "user_id"),
				Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict")),
				Tag:      request.URL.Query().Get("tag"),
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.bookmarks(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/user/following":
		if fake.following != nil {
			req := pixivsdk.UserFollowingRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
			if req.Restrict == "" {
				req.Restrict = pixivsdk.RestrictPublic
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.following(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIUserPreviewPage(page)
		}
		return tr.wireCLIUserPreviewPage(sdk.Page[pixivsdk.UserPreview]{})
	case "/v1/user/follower":
		if fake.userFollowers != nil {
			req := pixivsdk.UserFollowersRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
			if req.Restrict == "" {
				req.Restrict = pixivsdk.RestrictPublic
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.userFollowers(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIUserPreviewPage(page)
		}
		return tr.wireCLIUserPreviewPage(sdk.Page[pixivsdk.UserPreview]{})
	case "/v2/illust/follow":
		if fake.followingArtworks != nil {
			req := pixivsdk.FollowingArtworksRequest{Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
			if req.Restrict == "" {
				req.Restrict = pixivsdk.RestrictPublic
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.followingArtworks(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/novel/follow":
		if fake.followingNovels != nil {
			req := pixivsdk.FollowingNovelsRequest{Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
			if req.Restrict == "" {
				req.Restrict = pixivsdk.RestrictPublic
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.followingNovels(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/illust/new":
		if fake.latestArtworks != nil {
			req := pixivsdk.LatestArtworksRequest{ContentType: pixivsdk.SearchContentType(request.URL.Query().Get("content_type"))}
			if req.ContentType == "" {
				req.ContentType = pixivsdk.SearchContentTypeIllust
			}
			req.Cursor = tr.cursorFromQuery(request.URL.Query())
			page, err := fake.latestArtworks(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/novel/new":
		if fake.latestNovels != nil {
			req := pixivsdk.LatestNovelsRequest{Cursor: tr.cursorFromQuery(request.URL.Query())}
			page, err := fake.latestNovels(request.Context(), req)
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/user/mypixiv":
		if fake.myPixivUsers != nil {
			page, err := fake.myPixivUsers(request.Context(), pixivsdk.MyPixivUsersRequest{})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIUserPreviewPage(page)
		}
		return tr.wireCLIUserPreviewPage(sdk.Page[pixivsdk.UserPreview]{})
	case "/v2/illust/mypixiv":
		if fake.myPixivArtworks != nil {
			page, err := fake.myPixivArtworks(request.Context(), pixivsdk.MyPixivArtworksRequest{})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLIArtworkPage(page)
		}
		return tr.wireCLIArtworkPage(sdk.Page[pixivsdk.Artwork]{})
	case "/v1/novel/mypixiv":
		if fake.myPixivNovels != nil {
			page, err := fake.myPixivNovels(request.Context(), pixivsdk.MyPixivNovelsRequest{})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/user/novels":
		if fake.userNovels != nil {
			page, err := fake.userNovels(request.Context(), pixivsdk.UserNovelsRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/user/bookmarks/novel":
		if fake.novelBookmarks != nil {
			page, err := fake.novelBookmarks(request.Context(), pixivsdk.UserNovelBookmarksRequest{UserID: cliQueryInt64(request.URL.Query(), "user_id")})
			if err != nil {
				return wireCLIError(err)
			}
			return tr.wireCLINovelPage(page)
		}
		return tr.wireCLINovelPage(sdk.Page[pixivsdk.Novel]{})
	case "/v1/ugoira/metadata":
		return wireCLIError(sdk.NewError("pixiv", "UgoiraMetadata", sdk.ContentUnavailable))
	case "/v1/illust/bookmark/add", "/v2/illust/bookmark/add":
		_ = request.ParseForm()
		req := pixivsdk.AddBookmarkRequest{
			ArtworkID: cliFormInt64(request, "illust_id"),
			Restrict:  pixivsdk.Restrict(cliFormValue(request, "restrict")),
		}
		if req.Restrict == "" {
			req.Restrict = pixivsdk.RestrictPublic
		}
		if tags := request.PostForm["tags[]"]; len(tags) > 0 {
			req.Tags = tags
		}
		if fake.addBookmark != nil {
			if err := fake.addBookmark(request.Context(), req); err != nil {
				return wireCLIError(err)
			}
		}
		return wireCLIEmptyOK()
	case "/v1/illust/bookmark/delete":
		_ = request.ParseForm()
		req := pixivsdk.RemoveBookmarkRequest{ArtworkID: cliFormInt64(request, "illust_id")}
		if fake.removeBookmark != nil {
			if err := fake.removeBookmark(request.Context(), req); err != nil {
				return wireCLIError(err)
			}
		}
		return wireCLIEmptyOK()
	case "/v1/user/follow/add":
		_ = request.ParseForm()
		req := pixivsdk.FollowUserRequest{
			UserID:   cliFormInt64(request, "user_id"),
			Restrict: pixivsdk.Restrict(cliFormValue(request, "restrict")),
		}
		if req.Restrict == "" {
			req.Restrict = pixivsdk.RestrictPublic
		}
		if fake.follow != nil {
			if err := fake.follow(request.Context(), req); err != nil {
				return wireCLIError(err)
			}
		}
		return wireCLIEmptyOK()
	case "/v1/user/follow/delete":
		_ = request.ParseForm()
		req := pixivsdk.UnfollowUserRequest{UserID: cliFormInt64(request, "user_id")}
		if fake.unfollow != nil {
			if err := fake.unfollow(request.Context(), req); err != nil {
				return wireCLIError(err)
			}
		}
		return wireCLIEmptyOK()
	}
	tr.t.Fatalf("CLI wire transport has no handler for %s", request.URL.Path)
	return nil, nil
}

// openCLIWireClient 打开一个 wire-responder 的 public SDK client。
func openCLIWireClient(t *testing.T, fake *sdkCommandFake) *pixivsdk.Client {
	t.Helper()
	client, _, err := pixivsdk.OpenWith(context.Background(), "test-refresh-token", pixivsdk.Options{
		HTTPClient: &http.Client{Transport: &cliWireTransport{t: t, fake: fake}},
	})
	if err != nil {
		t.Fatalf("open test pixiv client: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

// wireCLIPorts 构造 CLI 测试用的窄 SDK 端口；fake 的 userID 决定 OAuth 身份。
func wireCLIPorts(t *testing.T, fake *sdkCommandFake) pixivdeps.Data {
	client := openCLIWireClient(t, fake)
	return pixivdeps.Data{
		Open: func(pixivdeps.Request) (*pixivsdk.Client, error) { return client, nil },
		Pooled: func(ctx context.Context, _ pixivdeps.Request, attempt func(context.Context, *pixivsdk.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
		JSONOut: func(override *bool) (bool, error) {
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
}

// --- wire helpers ---

func wireCLIError(cause error) (*http.Response, error) {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return nil, &url.Error{Op: "Get", URL: "https://app-api.pixiv.net", Err: cause}
	}
	status, code := http.StatusInternalServerError, "error"
	var typed *sdk.Error
	if ok := cliErrorAs(cause, &typed); ok && typed != nil {
		switch typed.Reason {
		case sdk.Unauthorized:
			status, code = http.StatusUnauthorized, "unauthorized"
		case sdk.Forbidden:
			status, code = http.StatusForbidden, "forbidden"
		case sdk.NotFound:
			status, code = http.StatusNotFound, "not_found"
		case sdk.InvalidArgument:
			status, code = http.StatusBadRequest, "invalid_argument"
		case sdk.RateLimited:
			status, code = http.StatusTooManyRequests, "rate_limited"
		case sdk.MalformedUpstreamResponse:
			body := []byte(`{}`)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"user_message": cause.Error(), "message": cause.Error(), "reason": code}})
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func cliErrorAs(err error, target **sdk.Error) bool {
	for current := err; current != nil; {
		if typed, ok := current.(*sdk.Error); ok {
			*target = typed
			return true
		}
		next, ok := current.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		current = next.Unwrap()
	}
	return false
}

func cliQueryInt64(query url.Values, key string) int64 {
	value, _ := strconv.ParseInt(query.Get(key), 10, 64)
	return value
}

func parseCLIResolution(query url.Values) pixivsdk.SearchResolution {
	minWidth := cliQueryInt64(query, "width_min")
	maxWidth := cliQueryInt64(query, "width_max")
	minHeight := cliQueryInt64(query, "height_min")
	maxHeight := cliQueryInt64(query, "height_max")
	if minWidth == 0 && maxWidth == 0 && minHeight == 0 && maxHeight == 0 {
		return pixivsdk.SearchResolutionAll
	}
	switch {
	case minWidth >= 3000 && minHeight >= 3000:
		return pixivsdk.SearchResolutionHigh
	case maxWidth >= 1000 && maxWidth <= 2999 && maxHeight >= 1000 && maxHeight <= 2999:
		return pixivsdk.SearchResolutionMedium
	case maxWidth <= 999 && maxHeight <= 999:
		return pixivsdk.SearchResolutionLow
	}
	return pixivsdk.SearchResolutionAll
}

func cliFormValue(request *http.Request, key string) string {
	_ = request.ParseForm()
	return request.PostForm.Get(key)
}

func cliFormInt64(request *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(cliFormValue(request, key), 10, 64)
	return value
}

func wireCLIEmptyOK() (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func parseCLISearchArtworks(query url.Values) pixivsdk.SearchArtworksRequest {
	req := pixivsdk.SearchArtworksRequest{
		Word:      query.Get("word"),
		Sort:      pixivsdk.SortMode(query.Get("sort")),
		StartDate: query.Get("start_date"),
		EndDate:   query.Get("end_date"),
	}
	req.Target = pixivsdk.SearchTarget(query.Get("search_target"))
	req.Duration = pixivsdk.DurationFilter(query.Get("duration"))
	if req.Target == "" {
		req.Target = pixivsdk.SearchTargetPartialMatchForTags
	}
	if req.Sort == "" {
		req.Sort = pixivsdk.SortModeDateDesc
	}
	req.ContentType = pixivsdk.SearchContentType(strings.ReplaceAll(query.Get("content_type"), "_", "-"))
	req.AIMode = pixivsdk.SearchAIMode(query.Get("ai_mode"))
	req.AspectRatio = pixivsdk.SearchAspectRatio(query.Get("ratio_pattern"))
	req.Resolution = parseCLIResolution(query)
	req.Tool = query.Get("tool")
	if req.ContentType == "" {
		req.ContentType = pixivsdk.SearchContentTypeAll
	}
	if req.AIMode == "" {
		req.AIMode = pixivsdk.SearchAIModeAll
	}
	if req.AspectRatio == "" {
		req.AspectRatio = pixivsdk.SearchAspectRatioAll
	}
	if req.Resolution == "" {
		req.Resolution = pixivsdk.SearchResolutionAll
	}
	if minValue, err := strconv.ParseInt(query.Get("bookmark_num_min"), 10, 64); err == nil {
		minInt := int(minValue)
		req.BookmarkMin = &minInt
	}
	if maxValue, err := strconv.ParseInt(query.Get("bookmark_num_max"), 10, 64); err == nil {
		maxInt := int(maxValue)
		req.BookmarkMax = &maxInt
	}
	return req
}

func parseCLISearchNovels(query url.Values) pixivsdk.SearchNovelsRequest {
	return pixivsdk.SearchNovelsRequest{
		Word:     query.Get("word"),
		Sort:     pixivsdk.SortMode(query.Get("sort")),
		Target:   pixivsdk.SearchTarget(query.Get("search_target")),
		Duration: pixivsdk.DurationFilter(query.Get("duration")),
	}
}

type cliWireImageURLs struct {
	SquareMedium string `json:"square_medium,omitempty"`
	Medium       string `json:"medium,omitempty"`
	Large        string `json:"large,omitempty"`
	Original     string `json:"original,omitempty"`
}

type cliWireUser struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
}

func cliWireArtworkValue(a pixivsdk.Artwork) map[string]any {
	kind := "illust"
	switch a.Kind {
	case pixivsdk.ArtworkKindManga:
		kind = "manga"
	case pixivsdk.ArtworkKindUgoira:
		kind = "ugoira"
	}
	createDate := "2024-05-01T01:00:00Z"
	if !a.PublishedAt.IsZero() {
		createDate = a.PublishedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	var imageURLs cliWireImageURLs
	if a.Cover.Resource.URL != "" {
		imageURLs.Original = a.Cover.Resource.URL
	}
	user := cliWireUser{ID: a.User.ID, Name: a.User.Name, Account: a.User.Account}
	if user.ID == 0 {
		user.ID = 1
	}
	value := map[string]any{
		"id":              a.ID,
		"title":           a.Title,
		"caption":         a.Caption,
		"type":            kind,
		"user":            user,
		"tags":            wireCLITags(a.Tags),
		"image_urls":      imageURLs,
		"create_date":     createDate,
		"page_count":      a.PageCount,
		"total_bookmarks": a.TotalBookmarks,
		"total_view":      a.TotalViews,
		"width":           a.Width,
		"height":          a.Height,
		"x_restrict":      a.XRestrict,
		"ai_type":         a.AIType,
		"tools":           a.Tools,
	}
	if len(a.Pages) > 0 {
		pages := make([]map[string]any, 0, len(a.Pages))
		for _, page := range a.Pages {
			pages = append(pages, map[string]any{
				"image_urls": map[string]any{"original": fmt.Sprintf("https://i.pximg.net/img/%d/p%d.png", a.ID, page.PageIndex+1)},
				"width":      page.Width,
				"height":     page.Height,
			})
		}
		value["meta_pages"] = pages
	} else {
		value["meta_single_page"] = map[string]any{"original_image_url": a.Cover.Resource.URL}
	}
	return value
}

func wireCLITags(tags []pixivsdk.Tag) []map[string]any {
	out := make([]map[string]any, 0, len(tags))
	for _, tag := range tags {
		item := map[string]any{"name": tag.Name}
		if tag.TranslatedName != "" {
			item["translated_name"] = tag.TranslatedName
		}
		out = append(out, item)
	}
	return out
}

func cliWireNovelValue(n pixivsdk.Novel) map[string]any {
	createDate := "2024-05-01T01:00:00Z"
	if !n.PublishedAt.IsZero() {
		createDate = n.PublishedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	var imageURLs cliWireImageURLs
	if n.Cover.Resource.URL != "" {
		imageURLs.Original = n.Cover.Resource.URL
	}
	user := cliWireUser{ID: n.User.ID, Name: n.User.Name, Account: n.User.Account}
	if user.ID == 0 {
		user.ID = 1
	}
	return map[string]any{
		"id":          n.ID,
		"title":       n.Title,
		"caption":     n.Caption,
		"user":        user,
		"tags":        wireCLITags(n.Tags),
		"image_urls":  imageURLs,
		"create_date": createDate,
		"text_length": n.TextLength,
		"x_restrict":  n.XRestrict,
		"is_original": n.IsOriginal,
	}
}

func cliWireUserPreviewValue(p pixivsdk.UserPreview) map[string]any {
	value := map[string]any{
		"user":        cliWireUser{ID: p.User.ID, Name: p.User.Name, Account: p.User.Account},
		"is_followed": p.User.IsFollowed,
	}
	if len(p.Illusts) > 0 {
		illusts := make([]map[string]any, 0, len(p.Illusts))
		for _, item := range p.Illusts {
			illusts = append(illusts, cliWireArtworkValue(item))
		}
		value["illusts"] = illusts
	}
	if len(p.Novels) > 0 {
		novels := make([]map[string]any, 0, len(p.Novels))
		for _, item := range p.Novels {
			novels = append(novels, cliWireNovelValue(item))
		}
		value["novels"] = novels
	}
	return value
}

func (tr *cliWireTransport) wireCLIArtworkPage(page sdk.Page[pixivsdk.Artwork]) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, cliWireArtworkValue(item))
	}
	body, _ := json.Marshal(struct {
		Illusts []map[string]any `json:"illusts"`
		NextURL *string          `json:"next_url"`
	}{items, tr.nextPageURL(page.Next)})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func (tr *cliWireTransport) wireCLINovelPage(page sdk.Page[pixivsdk.Novel]) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, cliWireNovelValue(item))
	}
	body, _ := json.Marshal(struct {
		Novels  []map[string]any `json:"novels"`
		NextURL *string          `json:"next_url"`
	}{items, tr.nextPageURL(page.Next)})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func (tr *cliWireTransport) wireCLIUserPreviewPage(page sdk.Page[pixivsdk.UserPreview]) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, cliWireUserPreviewValue(item))
	}
	body, _ := json.Marshal(struct {
		UserPreviews []map[string]any `json:"user_previews"`
		NextURL      *string          `json:"next_url"`
	}{items, tr.nextPageURL(page.Next)})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLIArtwork(artwork pixivsdk.Artwork) (*http.Response, error) {
	body, _ := json.Marshal(struct {
		Illust map[string]any `json:"illust"`
	}{cliWireArtworkValue(artwork)})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLINovel(novel pixivsdk.Novel) (*http.Response, error) {
	body, _ := json.Marshal(struct {
		Novel map[string]any `json:"novel"`
	}{cliWireNovelValue(novel)})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLIUserDetail(user pixivsdk.UserDetail) (*http.Response, error) {
	profile := map[string]any{
		"webpage":              user.Profile.Webpage,
		"gender":               user.Profile.Gender,
		"birth_day":            user.Profile.BirthDay,
		"birth_year":           user.Profile.BirthYear,
		"region":               user.Profile.Region,
		"country_code":         user.Profile.CountryCode,
		"job":                  user.Profile.Job,
		"total_follow_users":   user.Profile.TotalFollowUsers,
		"total_mypixiv_users":  user.Profile.TotalMyPixivUsers,
		"total_illusts":        user.Profile.TotalIllusts,
		"total_manga":          user.Profile.TotalManga,
		"total_novels":         user.Profile.TotalNovels,
		"twitter_account":      user.Profile.TwitterAccount,
		"background_image_url": user.Profile.BackgroundImageURL,
		"twitter_url":          user.Profile.TwitterURL,
		"pawoo_url":            user.Profile.PawooURL,
	}
	publicity := map[string]any{
		"gender":     wireCLIPublicity(user.ProfilePublicity.Gender),
		"region":     wireCLIPublicity(user.ProfilePublicity.Region),
		"birth_day":  wireCLIPublicity(user.ProfilePublicity.BirthDay),
		"birth_year": wireCLIPublicity(user.ProfilePublicity.BirthYear),
		"job":        wireCLIPublicity(user.ProfilePublicity.Job),
		"pawoo":      wireCLIPublicity(user.ProfilePublicity.Pawoo),
	}
	workspace := map[string]any{
		"pc":      user.Workspace.PC,
		"monitor": user.Workspace.Monitor,
		"tool":    user.Workspace.Tool,
		"scanner": user.Workspace.Scanner,
		"tablet":  user.Workspace.Tablet,
		"mouse":   user.Workspace.Mouse,
		"printer": user.Workspace.Printer,
		"desktop": user.Workspace.Desktop,
		"music":   user.Workspace.Music,
	}
	body, _ := json.Marshal(struct {
		User             map[string]any `json:"user"`
		Profile          map[string]any `json:"profile"`
		ProfilePublicity map[string]any `json:"profile_publicity"`
		Workspace        map[string]any `json:"workspace"`
	}{
		map[string]any{"id": user.User.ID, "name": user.User.Name, "account": user.User.Account, "comment": user.User.Comment, "is_followed": user.User.IsFollowed},
		profile,
		publicity,
		workspace,
	})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLIString(value string) string {
	return value
}

func wireCLIPublicity(public bool) any {
	if public {
		return "public"
	}
	return "private"
}

func wireCLIArtworkSeries(page sdk.Page[pixivsdk.Artwork]) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Items))
	userID := int64(1)
	for _, item := range page.Items {
		items = append(items, cliWireArtworkValue(item))
		if item.User.ID > 0 {
			userID = item.User.ID
		}
	}
	body, _ := json.Marshal(struct {
		SeriesDetail map[string]any   `json:"illust_series_detail"`
		Illusts      []map[string]any `json:"illusts"`
	}{map[string]any{"user": cliWireUser{ID: userID}}, items})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLINovelSeries(result pixivsdk.NovelSeriesResult) (*http.Response, error) {
	items := make([]map[string]any, 0, len(result.Novels.Items))
	for _, item := range result.Novels.Items {
		items = append(items, cliWireNovelValue(item))
	}
	body, _ := json.Marshal(struct {
		Detail map[string]any   `json:"novel_series_detail"`
		Novels []map[string]any `json:"novels"`
	}{map[string]any{
		"id":           result.Series.ID,
		"title":        result.Series.Title,
		"caption":      result.Series.Caption,
		"user":         cliWireUser{ID: result.Series.User.ID, Name: result.Series.User.Name},
		"is_concluded": result.Series.IsConcluded,
	}, items})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLICommentPage(page pixivsdk.CommentPage) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Page.Items))
	for _, item := range page.Page.Items {
		createdAt := "2024-05-01T01:00:00Z"
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, map[string]any{
			"id":         item.ID,
			"comment":    item.Comment,
			"user":       cliWireUser{ID: item.User.ID, Name: item.User.Name},
			"created_at": createdAt,
		})
	}
	var total *int64
	if page.Total != nil {
		value := *page.Total
		total = &value
	}
	body, _ := json.Marshal(struct {
		Comments      []map[string]any `json:"comments"`
		TotalComments *int64           `json:"total_comments"`
	}{items, total})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLIBookmarkTags(page sdk.Page[pixivsdk.BookmarkTag]) (*http.Response, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, map[string]any{"name": item.Name, "count": item.Count})
	}
	body, _ := json.Marshal(struct {
		BookmarkTags []map[string]any `json:"bookmark_tags"`
	}{items})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func wireCLIBookmarkDetail(detail pixivsdk.ArtworkBookmarkDetail) (*http.Response, error) {
	restrict := ""
	switch detail.Restrict {
	case pixivsdk.RestrictPublic:
		restrict = "public"
	case pixivsdk.RestrictPrivate:
		restrict = "private"
	}
	body, _ := json.Marshal(struct {
		BookmarkDetail map[string]any `json:"bookmark_detail"`
	}{map[string]any{"is_bookmarked": true, "restrict": restrict, "tags": detail.Tags}})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func (tr *cliWireTransport) wireCLITrendingTags(tags []pixivsdk.TrendingTag) (*http.Response, error) {
	items := make([]map[string]any, 0, len(tags))
	for index, item := range tags {
		artwork := item.Artwork
		if artwork.ID <= 0 {
			artwork = pixivsdk.Artwork{ID: int64(1000 + index), Title: item.Tag}
		}
		items = append(items, map[string]any{
			"tag":             item.Tag,
			"translated_name": item.TranslatedName,
			"illust":          cliWireArtworkValue(artwork),
		})
	}
	body, _ := json.Marshal(struct {
		TrendTags []map[string]any `json:"trend_tags"`
	}{items})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}
