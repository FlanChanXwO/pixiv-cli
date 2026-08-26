package pixiv_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type testSDKTransport struct {
	t    *testing.T
	fake *fakeSDKClient
}

func (tr *testSDKTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if tr.fake == nil {
		tr.t.Fatalf("wire transport has no fake fixture")
	}
	tr.fake.mu.Lock()
	defer tr.fake.mu.Unlock()
	if request.URL.Path == "/auth/token" {
		userID := tr.fake.userID
		if userID <= 0 {
			userID = 42
		}
		body, _ := json.Marshal(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
			"user":          map[string]any{"id": userID, "name": "artist"},
		})
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	var status int
	var body []byte
	var err error
	switch request.URL.Path {
	case "/v1/search/illust":
		req := parseSearchArtworks(request.URL.Query())
		tr.fake.searchIllustRequest = req
		page, callErr := callIllustPage(tr.fake.searchIllust, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/search/novel":
		req := parseSearchNovels(request.URL.Query())
		tr.fake.searchNovelRequest = req
		page, callErr := callNovelPage(tr.fake.searchNovel, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/search/user":
		req := parseSearchUsers(request.URL.Query())
		tr.fake.searchUserRequest = req
		page, callErr := callUserPage(tr.fake.searchUser, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/illust/detail":
		id := queryInt64(request.URL.Query(), "illust_id")
		tr.fake.illustDetailRequest = id
		artwork, callErr := callIllustDetail(tr.fake.illustDetail, id)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = wireArtwork(artwork)
	case "/v2/illust/related":
		req := pixivsdk.RelatedArtworksRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id")}
		tr.fake.relatedArtworksRequest = req
		page, callErr := callIllustPageFunc(tr.fake.relatedArtworks, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/illust/series":
		req := pixivsdk.ArtworkSeriesRequest{SeriesID: queryInt64(request.URL.Query(), "illust_series_id")}
		tr.fake.artworkSeriesRequest = req
		page, callErr := callIllustPageFunc(tr.fake.artworkSeries, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.artworkSeries == nil && len(tr.fake.artworkSeriesPage.Items) > 0 {
			page = tr.fake.artworkSeriesPage
		}
		status, body, err = tr.wireArtworkSeriesPage(page)
	case "/v1/illust/ranking":
		req := parseArtworkRanking(request.URL.Query())
		tr.fake.illustRankingRequest = req
		page, callErr := callIllustPageFunc(tr.fake.illustRanking, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/illust/recommended":
		tr.fake.recommendedArtworksCalls++
		req := pixivsdk.RecommendedArtworksRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.recommendedArtworksRequest = req
		page, callErr := callRecommendedArtworks(tr.fake.recommendedArtworks, req, tr.fake.recommendedArtworksCalls)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v2/illust/follow":
		req := pixivsdk.FollowingArtworksRequest{}
		tr.fake.followingIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.followingIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/follow":
		req := pixivsdk.FollowingNovelsRequest{Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict"))}
		tr.fake.followingNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.followingNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/illust/new":
		req := pixivsdk.LatestArtworksRequest{ContentType: pixivsdk.SearchContentType(request.URL.Query().Get("content_type"))}
		tr.fake.latestIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.latestIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/new":
		req := pixivsdk.LatestNovelsRequest{}
		tr.fake.latestNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.latestNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/mypixiv":
		req := pixivsdk.MyPixivUsersRequest{}
		tr.fake.myPixivUsersRequest = req
		page, callErr := callUserPageFunc(tr.fake.myPixivUsers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v2/illust/mypixiv":
		req := pixivsdk.MyPixivArtworksRequest{}
		tr.fake.myPixivIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.myPixivIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/mypixiv":
		req := pixivsdk.MyPixivNovelsRequest{}
		tr.fake.myPixivNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.myPixivNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/novel/recommended":
		req := pixivsdk.RecommendedNovelsRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.novelRecommendedRequest = req
		page, callErr := callNovelPageFunc(tr.fake.novelRecommended, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/recommended":
		req := pixivsdk.RecommendedUsersRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.userRecommendedRequest = req
		page, callErr := callUserPageFunc(tr.fake.userRecommended, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/detail":
		req := pixivsdk.UserRequest{UserID: queryInt64(request.URL.Query(), "user_id")}
		tr.fake.userDetailRequest = req
		if tr.fake.userDetailErr != nil {
			return wireErrorResponse(tr.fake.userDetailErr)
		}
		status, body, err = wireUserDetail(tr.fake.userDetailResult)
	case "/v1/user/illusts":
		req := pixivsdk.UserArtworksRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Kind: kindFromType(request.URL.Query().Get("type")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.artworksRequest = req
		tr.fake.artworksRequests = append(tr.fake.artworksRequests, req)
		tr.fake.userArtworksCalls++
		page, callErr := callUserArtworks(tr.fake.userArtworksFunc, tr.fake.artworks, req, tr.fake.userArtworksCalls)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/user/novels":
		req := pixivsdk.UserNovelsRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.userNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.userNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/bookmarks/illust":
		req := pixivsdk.UserArtworkBookmarksRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict")), Tag: request.URL.Query().Get("tag"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.bookmarksRequest = req
		status, body, err = tr.wireArtworkPage(sdk.Page[pixivsdk.Artwork]{Items: tr.fake.bookmarks})
	case "/v1/user/following":
		req := pixivsdk.UserFollowingRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.followingRequest = req
		page, callErr := callUserPageFunc(tr.fake.userFollowing, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.userFollowing == nil && len(tr.fake.following) > 0 {
			page = sdk.Page[pixivsdk.UserPreview]{Items: tr.fake.following}
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/related":
		req := pixivsdk.RelatedUsersRequest{UserID: queryInt64(request.URL.Query(), "seed_user_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.relatedUsersRequest = req
		tr.fake.relatedRequest = req
		page, callErr := callUserPageFunc(tr.fake.relatedUsers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.relatedUsers == nil && len(tr.fake.relatedPage.Items) > 0 {
			page = tr.fake.relatedPage
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/bookmark-tags/illust":
		req := pixivsdk.UserArtworkBookmarkTagsRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixivsdk.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.bookmarkTagsRequest = req
		status, body, err = wireBookmarkTags(tr.fake.bookmarkTagsPage.Items)
	case "/v1/illust/bookmark/detail":
		req := pixivsdk.ArtworkBookmarkRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id")}
		tr.fake.artworkBookmarkRequest = req
		tr.fake.bookmarkDetailRequest = req
		status, body, err = wireBookmarkDetail(tr.fake.bookmarkDetailResult)
	case "/v1/novel/detail":
		id := queryInt64(request.URL.Query(), "novel_id")
		tr.fake.novelDetailRequest = id
		tr.fake.novelRequest = pixivsdk.NovelRequest{NovelID: id}
		novel, callErr := callNovelDetail(tr.fake.novelDetail, id)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.novelDetail == nil && tr.fake.novelDetailResult.ID > 0 {
			novel = tr.fake.novelDetailResult
		}
		status, body, err = wireNovel(novel)
	case "/v1/novel/content":
		id := queryInt64(request.URL.Query(), "novel_id")
		tr.fake.novelContentRequest = pixivsdk.NovelContentRequest{NovelID: id}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(tr.fake.novelContentHTML))}, nil
	case "/v2/illust/comments":
		req := pixivsdk.ArtworkCommentsRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id")}
		tr.fake.illustCommentsRequest = req
		tr.fake.artworkCommentsRequest = req
		status, body, err = wireCommentPageResult(tr.fake.artworkCommentsResult)
	case "/v1/trending-tags/illust":
		status, body, err = wireTrendingTags(tr.fake.trendingTags)
	case "/v2/illust/bookmark/add":
		req := pixivsdk.AddBookmarkRequest{ArtworkID: formInt64(request, "illust_id"), Restrict: pixivsdk.Restrict(formValue(request, "restrict"))}
		if tags := formValues(request, "tags[]"); len(tags) > 0 {
			req.Tags = tags
		}
		tr.fake.addBookmarkRequest = req
		if tr.fake.addBookmarkErr != nil {
			return wireErrorResponse(tr.fake.addBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/illust/bookmark/delete":
		req := pixivsdk.RemoveBookmarkRequest{ArtworkID: formInt64(request, "illust_id")}
		tr.fake.removeBookmarkRequest = req
		if tr.fake.removeBookmarkErr != nil {
			return wireErrorResponse(tr.fake.removeBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/user/follow/add":
		req := pixivsdk.FollowUserRequest{UserID: formInt64(request, "user_id"), Restrict: pixivsdk.Restrict(formValue(request, "restrict"))}
		tr.fake.followUserRequest = req
		if tr.fake.followUserErr != nil {
			return wireErrorResponse(tr.fake.followUserErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/user/follow/delete":
		req := pixivsdk.UnfollowUserRequest{UserID: formInt64(request, "user_id")}
		tr.fake.unfollowUserRequest = req
		if tr.fake.unfollowUserErr != nil {
			return wireErrorResponse(tr.fake.unfollowUserErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	default:
		tr.t.Fatalf("wire transport has no handler for %s", request.URL.Path)
		return nil, nil
	}
	if err != nil {
		tr.t.Fatalf("wire transport encode %s: %v", request.URL.Path, err)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

// wireErrorResponse 把 typed fake 错误编码为 App API error envelope，使
// classifyAppError 能还原 reason/detail。
func wireErrorResponse(cause error) (*http.Response, error) {
	status, code, message := http.StatusInternalServerError, "error", cause.Error()
	var typed *sdk.Error
	if ok := errorAs(cause, &typed); ok && typed != nil && typed.Reason == sdk.MalformedUpstreamResponse {
		body := []byte(`{}`)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	if ok := errorAs(cause, &typed); ok && typed != nil {
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
		}
		if typed.Detail != "" {
			message = typed.Detail
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"user_message": message, "message": message, "reason": code}})
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func errorAs(err error, target **sdk.Error) bool {
	for current := err; current != nil; {
		typed, ok := current.(*sdk.Error)
		if ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := current.(unwrapper)
		if !ok {
			return false
		}
		current = next.Unwrap()
	}
	return false
}

// testSDKPorts 构造使用 wire responder 的真实 public SDK client 端口。
func testSDKPorts(t *testing.T, fake *fakeSDKClient) pixivmcpserver.SDKPorts {
	t.Helper()
	ports, _ := newTestSDKPorts(t, fake)
	return ports
}

func newTestSDKPorts(t *testing.T, fake *fakeSDKClient) (pixivmcpserver.SDKPorts, pixivmcpserver.Account) {
	t.Helper()
	client := openWireClient(t, fake)
	return pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixivsdk.Client, error) { return client, nil },
		Execute: func(ctx context.Context, _ pixivmcpserver.Account, attempt func(context.Context, *pixivsdk.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	}, pixivmcpserver.Account{}
}

func callIllustPage(fn func(context.Context, pixivsdk.SearchArtworksRequest) (sdk.Page[pixivsdk.Artwork], error), req pixivsdk.SearchArtworksRequest) (sdk.Page[pixivsdk.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Artwork]{Items: []pixivsdk.Artwork{}}, nil
	}
	return fn(context.Background(), req)
}

func callIllustPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixivsdk.Artwork], error), req T) (sdk.Page[pixivsdk.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Artwork]{Items: []pixivsdk.Artwork{}}, nil
	}
	return fn(context.Background(), req)
}

func callNovelPage(fn func(context.Context, pixivsdk.SearchNovelsRequest) (sdk.Page[pixivsdk.Novel], error), req pixivsdk.SearchNovelsRequest) (sdk.Page[pixivsdk.Novel], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Novel]{Items: []pixivsdk.Novel{}}, nil
	}
	return fn(context.Background(), req)
}

func callNovelPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixivsdk.Novel], error), req T) (sdk.Page[pixivsdk.Novel], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Novel]{Items: []pixivsdk.Novel{}}, nil
	}
	return fn(context.Background(), req)
}

func callUserPage(fn func(context.Context, pixivsdk.SearchUsersRequest) (sdk.Page[pixivsdk.UserPreview], error), req pixivsdk.SearchUsersRequest) (sdk.Page[pixivsdk.UserPreview], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.UserPreview]{Items: []pixivsdk.UserPreview{}}, nil
	}
	return fn(context.Background(), req)
}

func callUserPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixivsdk.UserPreview], error), req T) (sdk.Page[pixivsdk.UserPreview], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.UserPreview]{Items: []pixivsdk.UserPreview{}}, nil
	}
	return fn(context.Background(), req)
}

func callIllustDetail(fn func(context.Context, int64) (pixivsdk.Artwork, error), id int64) (pixivsdk.Artwork, error) {
	if fn == nil {
		return pixivsdk.Artwork{}, nil
	}
	return fn(context.Background(), id)
}

func callNovelDetail(fn func(context.Context, int64) (pixivsdk.Novel, error), id int64) (pixivsdk.Novel, error) {
	if fn == nil {
		return pixivsdk.Novel{}, nil
	}
	return fn(context.Background(), id)
}

func callRecommendedArtworks(fn func(context.Context, pixivsdk.RecommendedArtworksRequest, int) (sdk.Page[pixivsdk.Artwork], error), req pixivsdk.RecommendedArtworksRequest, call int) (sdk.Page[pixivsdk.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Artwork]{Items: []pixivsdk.Artwork{}}, nil
	}
	return fn(context.Background(), req, call)
}

func callUserArtworks(fn func(pixivsdk.UserArtworksRequest, int) (sdk.Page[pixivsdk.Artwork], error), fallback []pixivsdk.Artwork, req pixivsdk.UserArtworksRequest, call int) (sdk.Page[pixivsdk.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixivsdk.Artwork]{Items: fallback}, nil
	}
	return fn(req, call)
}

func queryInt64(query url.Values, key string) int64 {
	value, _ := strconv.ParseInt(query.Get(key), 10, 64)
	return value
}

func queryInt(query url.Values, key string) int {
	value, _ := strconv.Atoi(query.Get(key))
	return value
}

func formInt64(request *http.Request, key string) int64 {
	_ = request.ParseForm()
	value, _ := strconv.ParseInt(request.PostForm.Get(key), 10, 64)
	return value
}

func formValue(request *http.Request, key string) string {
	_ = request.ParseForm()
	return request.PostForm.Get(key)
}

func formValues(request *http.Request, key string) []string {
	_ = request.ParseForm()
	return request.PostForm[key]
}

func parseSearchArtworks(query url.Values) pixivsdk.SearchArtworksRequest {
	req := pixivsdk.SearchArtworksRequest{
		Word:      query.Get("word"),
		Sort:      pixivsdk.SortMode(query.Get("sort")),
		StartDate: query.Get("start_date"),
		EndDate:   query.Get("end_date"),
		Cursor:    cursorFromOffset(queryInt(query, "offset")),
	}
	req.Target = pixivsdk.SearchTarget(query.Get("search_target"))
	req.Duration = pixivsdk.DurationFilter(query.Get("duration"))
	req.ContentType = pixivsdk.SearchContentType(query.Get("content_type"))
	req.AIMode = pixivsdk.SearchAIMode(query.Get("search_ai_type"))
	req.AspectRatio = pixivsdk.SearchAspectRatio(query.Get("ratio_pattern"))
	req.Resolution = pixivsdk.SearchResolution(query.Get("resolution"))
	req.Tool = query.Get("tool")
	if value := query.Get("bookmark_num_min"); value != "" {
		parsed, _ := strconv.Atoi(value)
		req.BookmarkMin = &parsed
	}
	if value := query.Get("bookmark_num_max"); value != "" {
		parsed, _ := strconv.Atoi(value)
		req.BookmarkMax = &parsed
	}
	return req
}

func parseSearchNovels(query url.Values) pixivsdk.SearchNovelsRequest {
	return pixivsdk.SearchNovelsRequest{
		Word:     query.Get("word"),
		Sort:     pixivsdk.SortMode(query.Get("sort")),
		Target:   pixivsdk.SearchTarget(query.Get("search_target")),
		Duration: pixivsdk.DurationFilter(query.Get("duration")),
		Cursor:   cursorFromOffset(queryInt(query, "offset")),
	}
}

func parseSearchUsers(query url.Values) pixivsdk.SearchUsersRequest {
	return pixivsdk.SearchUsersRequest{
		Word:   query.Get("word"),
		Cursor: cursorFromOffset(queryInt(query, "offset")),
	}
}

func parseArtworkRanking(query url.Values) pixivsdk.ArtworkRankingRequest {
	return pixivsdk.ArtworkRankingRequest{
		Mode:   pixivsdk.RankingMode(query.Get("mode")),
		Date:   query.Get("date"),
		Cursor: cursorFromOffset(queryInt(query, "offset")),
	}
}

// kindFromType 把 App API type 查询参数映射回 SDK ArtworkKind。
func kindFromType(value string) pixivsdk.ArtworkKind {
	switch value {
	case "manga":
		return pixivsdk.ArtworkKindManga
	case "ugoira":
		return pixivsdk.ArtworkKindUgoira
	default:
		return pixivsdk.ArtworkKindIllustration
	}
}

// cursorFromOffset 为 captured request 重建一个非零 cursor；offset=0 表示首页。
func cursorFromOffset(offset int) sdk.Cursor {
	if offset <= 0 {
		return sdk.Cursor{}
	}
	cursor, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "q", []byte(strconv.Itoa(offset)))
	if err != nil {
		panic(err)
	}
	return cursor
}

type wireImageURLs struct {
	SquareMedium string `json:"square_medium,omitempty"`
	Medium       string `json:"medium,omitempty"`
	Large        string `json:"large,omitempty"`
	Original     string `json:"original,omitempty"`
}

type wireUser struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
}

type wireTag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name,omitempty"`
}

func wireArtworkValue(a pixivsdk.Artwork) map[string]any {
	tags := make([]wireTag, 0, len(a.Tags))
	for _, tag := range a.Tags {
		tags = append(tags, wireTag{Name: tag.Name, TranslatedName: tag.TranslatedName})
	}
	var imageURLs wireImageURLs
	if a.Cover.Resource.URL != "" {
		imageURLs.Original = a.Cover.Resource.URL
	}
	kind := "illust"
	switch a.Kind {
	case pixivsdk.ArtworkKindManga:
		kind = "manga"
	case pixivsdk.ArtworkKindUgoira:
		kind = "ugoira"
	}
	createDate := "2024-05-01T01:00:00Z"
	if !a.PublishedAt.IsZero() {
		createDate = a.PublishedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"id":              a.ID,
		"title":           a.Title,
		"caption":         a.Caption,
		"type":            kind,
		"user":            wireUser{ID: a.User.ID, Name: a.User.Name, Account: a.User.Account},
		"tags":            tags,
		"image_urls":      imageURLs,
		"create_date":     createDate,
		"page_count":      a.PageCount,
		"total_bookmarks": a.TotalBookmarks,
		"total_view":      a.TotalViews,
		"width":           a.Width,
		"height":          a.Height,
		"x_restrict":      a.XRestrict,
		"tools":           a.Tools,
	}
}

func wireNovelValue(n pixivsdk.Novel) map[string]any {
	tags := make([]wireTag, 0, len(n.Tags))
	for _, tag := range n.Tags {
		tags = append(tags, wireTag{Name: tag.Name, TranslatedName: tag.TranslatedName})
	}
	var imageURLs wireImageURLs
	if n.Cover.Resource.URL != "" {
		imageURLs.Original = n.Cover.Resource.URL
	}
	createDate := "2024-05-01T01:00:00Z"
	if !n.PublishedAt.IsZero() {
		createDate = n.PublishedAt.UTC().Format(time.RFC3339)
	}
	value := map[string]any{
		"id":          n.ID,
		"title":       n.Title,
		"caption":     n.Caption,
		"user":        wireUser{ID: n.User.ID, Name: n.User.Name, Account: n.User.Account},
		"tags":        tags,
		"image_urls":  imageURLs,
		"create_date": createDate,
	}
	value["text_length"] = n.TextLength
	value["x_restrict"] = n.XRestrict
	value["is_original"] = n.IsOriginal
	return value
}

func wireUserPreviewValue(p pixivsdk.UserPreview) map[string]any {
	value := map[string]any{
		"user":        wireUser{ID: p.User.ID, Name: p.User.Name, Account: p.User.Account},
		"is_followed": p.User.IsFollowed,
	}
	if len(p.Illusts) > 0 {
		illusts := make([]map[string]any, 0, len(p.Illusts))
		for _, item := range p.Illusts {
			illusts = append(illusts, wireArtworkValue(item))
		}
		value["illusts"] = illusts
	}
	if len(p.Novels) > 0 {
		novels := make([]map[string]any, 0, len(p.Novels))
		for _, item := range p.Novels {
			novels = append(novels, wireNovelValue(item))
		}
		value["novels"] = novels
	}
	return value
}

func (tr *testSDKTransport) wireArtworkPage(page sdk.Page[pixivsdk.Artwork]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireArtworkValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		Illusts []map[string]any `json:"illusts"`
		NextURL *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

func (tr *testSDKTransport) wireNovelPage(page sdk.Page[pixivsdk.Novel]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireNovelValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		Novels  []map[string]any `json:"novels"`
		NextURL *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

func (tr *testSDKTransport) wireUserPreviewPage(page sdk.Page[pixivsdk.UserPreview]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireUserPreviewValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		UserPreviews []map[string]any `json:"user_previews"`
		NextURL      *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

// nextPageURL 为 wire page 编码 continuation。offset 由 cursor 文本稳定派生，
// 相同的 cursor 产生相同的 offset（供 pagination 周期检测），不同的 cursor 产生
// 不同 offset（避免把正常续页误判为重复）。
func (tr *testSDKTransport) nextPageURL(cursor sdk.Cursor) *string {
	if cursor.IsZero() {
		return nil
	}
	text := cursor.String()
	sum := 0
	for _, r := range text {
		sum = (sum*31 + int(r)) % 100000
	}
	if sum < 0 {
		sum = -sum
	}
	value := "https://app-api.pixiv.net/v1/continuation?offset=" + strconv.Itoa(1+sum)
	return &value
}

func wireArtwork(artwork pixivsdk.Artwork) (int, []byte, error) {
	body, err := json.Marshal(struct {
		Illust map[string]any `json:"illust"`
	}{wireArtworkValue(artwork)})
	return http.StatusOK, body, err
}

func wireNovel(novel pixivsdk.Novel) (int, []byte, error) {
	body, err := json.Marshal(struct {
		Novel map[string]any `json:"novel"`
	}{wireNovelValue(novel)})
	return http.StatusOK, body, err
}

func wireUserDetail(user pixivsdk.UserDetail) (int, []byte, error) {
	body, err := json.Marshal(struct {
		User             map[string]any `json:"user"`
		Profile          map[string]any `json:"profile"`
		ProfilePublicity map[string]any `json:"profile_publicity"`
		Workspace        map[string]any `json:"workspace"`
	}{
		map[string]any{"id": user.User.ID, "name": user.User.Name, "account": user.User.Account},
		wireUserProfile(user.Profile),
		wireUserProfilePublicity(user.ProfilePublicity),
		wireUserWorkspace(user.Workspace),
	})
	return http.StatusOK, body, err
}

func wireUserProfile(profile pixivsdk.UserProfile) map[string]any {
	return map[string]any{
		"webpage":            profile.Webpage,
		"region":             profile.Region,
		"country_code":       profile.CountryCode,
		"job":                profile.Job,
		"total_illusts":      profile.TotalIllusts,
		"total_manga":        profile.TotalManga,
		"total_novels":       profile.TotalNovels,
		"total_follow_users": profile.TotalFollowUsers,
	}
}

func wireUserProfilePublicity(publicity pixivsdk.UserProfilePublicity) map[string]any {
	return map[string]any{
		"gender":     publicity.Gender,
		"region":     publicity.Region,
		"birth_day":  publicity.BirthDay,
		"birth_year": publicity.BirthYear,
		"job":        publicity.Job,
		"pawoo":      publicity.Pawoo,
	}
}

func wireUserWorkspace(workspace pixivsdk.UserWorkspace) map[string]any {
	return map[string]any{
		"pc":                  workspace.PC,
		"tool":                workspace.Tool,
		"workspace_image_url": workspace.WorkspaceImageURL,
	}
}

func wireTrendingTags(tags []pixivsdk.TrendingTag) (int, []byte, error) {
	items := make([]map[string]any, 0, len(tags))
	for _, item := range tags {
		wireTags := make([]map[string]any, 0, len(item.Artwork.Tags))
		for _, tag := range item.Artwork.Tags {
			wireTags = append(wireTags, map[string]any{"tag": tag.Name, "translated_name": tag.TranslatedName})
		}
		tools := item.Artwork.Tools
		if tools == nil {
			tools = []string{}
		}
		illust := map[string]any{
			"id":              item.Artwork.ID,
			"title":           item.Artwork.Title,
			"caption":         item.Artwork.Caption,
			"user":            map[string]any{"id": item.Artwork.User.ID, "name": item.Artwork.User.Name},
			"tags":            wireTags,
			"create_date":     item.Artwork.PublishedAt.UTC().Format(time.RFC3339),
			"updated_at":      item.Artwork.PublishedAt.UTC().Format(time.RFC3339),
			"type":            string(item.Artwork.RawKind),
			"page_count":      item.Artwork.PageCount,
			"total_view":      item.Artwork.TotalViews,
			"total_bookmarks": item.Artwork.TotalBookmarks,
			"width":           item.Artwork.Width,
			"height":          item.Artwork.Height,
			"tools":           tools,
			"image_urls":      map[string]any{"large": "https://i.pximg.net/artworks/" + strconv.FormatInt(item.Artwork.ID, 10) + "/1.png"},
			"meta_pages":      []any{},
		}
		items = append(items, map[string]any{"tag": item.Tag, "translated_name": item.TranslatedName, "illust": illust})
	}
	body, err := json.Marshal(struct {
		TrendTags []map[string]any `json:"trend_tags"`
	}{items})
	return http.StatusOK, body, err
}

func wireBookmarkTags(tags []pixivsdk.BookmarkTag) (int, []byte, error) {
	items := make([]map[string]any, 0, len(tags))
	for _, item := range tags {
		items = append(items, map[string]any{"name": item.Name, "count": item.Count})
	}
	body, err := json.Marshal(struct {
		BookmarkTags []map[string]any `json:"bookmark_tags"`
	}{items})
	return http.StatusOK, body, err
}

func wireBookmarkDetail(detail pixivsdk.ArtworkBookmarkDetail) (int, []byte, error) {
	restrict := ""
	switch detail.Restrict {
	case pixivsdk.RestrictPublic:
		restrict = "public"
	case pixivsdk.RestrictPrivate:
		restrict = "private"
	}
	body, err := json.Marshal(struct {
		BookmarkDetail map[string]any `json:"bookmark_detail"`
	}{map[string]any{"is_bookmarked": true, "restrict": restrict, "tags": detail.Tags}})
	return http.StatusOK, body, err
}

func wireCommentPageResult(page pixivsdk.CommentPage) (int, []byte, error) {
	dtos := make([]pixivsdk.CommentDTO, 0, len(page.Page.Items))
	for _, item := range page.Page.Items {
		dtos = append(dtos, pixivsdk.ToCommentDTO(item))
	}
	var total *int64
	if page.Total != nil {
		value := *page.Total
		total = &value
	}
	var accessControl *pixivsdk.CommentAccessControlDTO
	if page.AccessControl != nil {
		converted := pixivsdk.ToCommentAccessControlDTO(*page.AccessControl)
		accessControl = &converted
	}
	body, err := json.Marshal(struct {
		Comments      []pixivsdk.CommentDTO             `json:"comments"`
		NextURL       *string                           `json:"next_url"`
		TotalComments *int64                            `json:"total_comments"`
		AccessControl *pixivsdk.CommentAccessControlDTO `json:"access_control"`
	}{dtos, nil, total, accessControl})
	return http.StatusOK, body, err
}

// openWireClient 直接打开一个 wire-responder 的 public SDK client（独立 transport）。
func openWireClient(t *testing.T, fake *fakeSDKClient) *pixivsdk.Client {
	t.Helper()
	if fake == nil {
		fake = &fakeSDKClient{userID: 42}
	}
	client, _, err := pixivsdk.OpenWith(context.Background(), "test-refresh-token", pixivsdk.Options{
		HTTPClient: &http.Client{Transport: &testSDKTransport{t: t, fake: fake}},
	})
	if err != nil {
		t.Fatalf("open test pixiv client: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func (tr *testSDKTransport) wireArtworkSeriesPage(page sdk.Page[pixivsdk.Artwork]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	userID := int64(0)
	for _, item := range page.Items {
		items = append(items, wireArtworkValue(item))
		if userID == 0 {
			userID = item.User.ID
		}
	}
	if userID == 0 {
		userID = 1
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		SeriesDetail map[string]any   `json:"illust_series_detail"`
		Illusts      []map[string]any `json:"illusts"`
		NextURL      *string          `json:"next_url"`
	}{
		SeriesDetail: map[string]any{"user": wireUser{ID: userID, Name: "series-author"}},
		Illusts:      items,
		NextURL:      next,
	})
	return http.StatusOK, body, err
}

// TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords 保留各时间线的专属
// App API 请求断言，并固定所有实体结果使用共享 records 契约。
