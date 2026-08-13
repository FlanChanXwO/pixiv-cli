package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestSearchArtworksWiresOperation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "app-api.pixiv.net" {
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
		if req.URL.Path != "/v1/search/illust" {
			t.Errorf("path = %s", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("word") != "test" {
			t.Errorf("word = %q, want test", query.Get("word"))
		}
		if query.Get("search_target") != string(SearchTargetPartialMatchForTags) {
			t.Errorf("search_target = %q, want %q", query.Get("search_target"), SearchTargetPartialMatchForTags)
		}
		if query.Get("sort") != string(SortModeDateDesc) {
			t.Errorf("sort = %q, want %q", query.Get("sort"), SortModeDateDesc)
		}
		offset := query.Get("offset")
		if calls == 1 && offset != "" {
			t.Errorf("first call should omit offset, got %q", offset)
		}
		if calls == 2 && offset != "30" {
			t.Errorf("second call should pass offset=30, got %q", offset)
		}
		body := `{"illusts":[{"id":9001,"title":"art","type":"illust","create_date":"2024-05-01T10:00:00+09:00","image_urls":{"original":"https://i.pximg.net/img/9001.png"},"user":{"id":7,"name":"n","account":"a"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&search_target=partial_match_for_tags&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})

	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 9001 {
		t.Fatalf("page items = %+v", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected a continuation cursor")
	}
	// Continue pagination with the cursor; the query digest must match.
	page2, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags, Cursor: page.Next})
	if err != nil {
		t.Fatalf("continuation SearchArtworks: %v", err)
	}
	if len(page2.Items) != 1 || calls != 2 {
		t.Fatalf("page2 items=%d calls=%d", len(page2.Items), calls)
	}
}

func TestSearchArtworksRejectsChangedQuery(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected cursor")
	}
	// Reusing the cursor with a different query must fail closed.
	if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "different", Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("expected InvalidCursor for changed query, got %v", err)
	}
}

func TestArtworkWiresDetail(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" {
			t.Errorf("path = %s", req.URL.Path)
		}
		body := `{"illust":{"id":5,"title":"one","type":"manga","create_date":"2024-01-01T00:00:00Z","page_count":2,"image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_pages":[{"page_index":0,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p0.png"}},{"page_index":1,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p1.png"}}],"user":{"id":9,"name":"u","account":"u"},"tags":[]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.ID != 5 || len(artwork.Pages) != 2 {
		t.Fatalf("artwork = %+v", artwork)
	}
}

func TestUgoiraMetadataWires(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/ugoira/metadata" {
			t.Errorf("path = %s", req.URL.Path)
		}
		body := `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/zip/m.zip","original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100}]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	meta, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 77})
	if err != nil {
		t.Fatalf("UgoiraMetadata: %v", err)
	}
	if meta.ArtworkID != 77 || len(meta.Archives) != 2 || len(meta.Frames) != 1 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestNoWebFallbackOnUnauthorized(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.pixiv.net" {
			t.Fatal("must never fall back to the Web API")
		}
		if req.URL.Host == "app-api.pixiv.net" {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
	if sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
	}
}

func TestPixivFamiliesNoWebFallbackTokenMatrix(t *testing.T) {
	t.Run("empty access token", func(t *testing.T) {
		calls := 0
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("empty-token request unexpectedly reached transport")
		})
		_, err := NewWith("", Options{HTTPClient: &http.Client{Transport: rt}})
		if sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("NewWith reason = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
		}
		if calls != 0 {
			t.Fatalf("empty token made %d network calls, want 0", calls)
		}
	})

	families := []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{name: "search", call: func(ctx context.Context, client *Client) error {
			_, err := client.SearchArtworks(ctx, SearchArtworksRequest{Word: "matrix"})
			return err
		}},
		{name: "detail", call: func(ctx context.Context, client *Client) error {
			_, err := client.Artwork(ctx, ArtworkRequest{ArtworkID: 1})
			return err
		}},
		{name: "pages", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkPages(ctx, ArtworkPagesRequest{ArtworkID: 1})
			return err
		}},
		{name: "ugoira", call: func(ctx context.Context, client *Client) error {
			_, err := client.UgoiraMetadata(ctx, UgoiraMetadataRequest{ArtworkID: 1})
			return err
		}},
		{name: "related", call: func(ctx context.Context, client *Client) error {
			_, err := client.RelatedArtworks(ctx, RelatedArtworksRequest{ArtworkID: 1})
			return err
		}},
		{name: "series", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkSeries(ctx, ArtworkSeriesRequest{SeriesID: 1})
			return err
		}},
		{name: "ranking", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkRanking(ctx, ArtworkRankingRequest{Mode: RankingModeDay})
			return err
		}},
		{name: "recommended", call: func(ctx context.Context, client *Client) error {
			_, err := client.RecommendedArtworks(ctx, RecommendedArtworksRequest{})
			return err
		}},
		{name: "following", call: func(ctx context.Context, client *Client) error {
			_, err := client.FollowingArtworks(ctx, FollowingArtworksRequest{Restrict: RestrictPrivate})
			return err
		}},
		{name: "latest", call: func(ctx context.Context, client *Client) error {
			_, err := client.LatestArtworks(ctx, LatestArtworksRequest{ContentType: SearchContentTypeIllust})
			return err
		}},
		{name: "user artworks", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserArtworks(ctx, UserArtworksRequest{UserID: 1, Kind: ArtworkKindIllustration})
			return err
		}},
		{name: "mypixiv", call: func(ctx context.Context, client *Client) error {
			_, err := client.MyPixivArtworks(ctx, MyPixivArtworksRequest{})
			return err
		}},
		{name: "trending", call: func(ctx context.Context, client *Client) error {
			_, err := client.TrendingArtworkTags(ctx, TrendingArtworkTagsRequest{})
			return err
		}},
		{name: "comments", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkComments(ctx, ArtworkCommentsRequest{ArtworkID: 1})
			return err
		}},
		{name: "bookmark list", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserArtworkBookmarks(ctx, UserArtworkBookmarksRequest{UserID: 1, Restrict: RestrictPrivate})
			return err
		}},
		{name: "bookmark tags", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserArtworkBookmarkTags(ctx, UserArtworkBookmarkTagsRequest{UserID: 1, Restrict: RestrictPrivate})
			return err
		}},
		{name: "bookmark detail", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkBookmark(ctx, ArtworkBookmarkRequest{ArtworkID: 1})
			return err
		}},
		{name: "bookmark add", call: func(ctx context.Context, client *Client) error {
			return client.AddBookmark(ctx, AddBookmarkRequest{ArtworkID: 1})
		}},
		{name: "bookmark remove", call: func(ctx context.Context, client *Client) error {
			return client.RemoveBookmark(ctx, RemoveBookmarkRequest{ArtworkID: 1})
		}},
		{name: "novel search", call: func(ctx context.Context, client *Client) error {
			_, err := client.SearchNovels(ctx, SearchNovelsRequest{Word: "matrix"})
			return err
		}},
		{name: "novel detail", call: func(ctx context.Context, client *Client) error {
			_, err := client.Novel(ctx, NovelRequest{NovelID: 1})
			return err
		}},
		{name: "novel content", call: func(ctx context.Context, client *Client) error {
			_, err := client.NovelContent(ctx, NovelContentRequest{NovelID: 1})
			return err
		}},
		{name: "novel series", call: func(ctx context.Context, client *Client) error {
			_, err := client.NovelSeries(ctx, NovelSeriesRequest{SeriesID: 1})
			return err
		}},
		{name: "novel comments", call: func(ctx context.Context, client *Client) error {
			_, err := client.NovelComments(ctx, NovelCommentsRequest{NovelID: 1})
			return err
		}},
		{name: "novel recommended", call: func(ctx context.Context, client *Client) error {
			_, err := client.RecommendedNovels(ctx, RecommendedNovelsRequest{})
			return err
		}},
		{name: "novel following", call: func(ctx context.Context, client *Client) error {
			_, err := client.FollowingNovels(ctx, FollowingNovelsRequest{Restrict: RestrictPrivate})
			return err
		}},
		{name: "novel latest", call: func(ctx context.Context, client *Client) error {
			_, err := client.LatestNovels(ctx, LatestNovelsRequest{})
			return err
		}},
		{name: "user novels", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserNovels(ctx, UserNovelsRequest{UserID: 1})
			return err
		}},
		{name: "user novel bookmarks", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserNovelBookmarks(ctx, UserNovelBookmarksRequest{UserID: 1, Restrict: RestrictPrivate})
			return err
		}},
		{name: "user search", call: func(ctx context.Context, client *Client) error {
			_, err := client.SearchUsers(ctx, SearchUsersRequest{Word: "matrix"})
			return err
		}},
		{name: "user detail", call: func(ctx context.Context, client *Client) error {
			_, err := client.User(ctx, UserRequest{UserID: 1})
			return err
		}},
		{name: "user recommended", call: func(ctx context.Context, client *Client) error {
			_, err := client.RecommendedUsers(ctx, RecommendedUsersRequest{})
			return err
		}},
		{name: "user related", call: func(ctx context.Context, client *Client) error {
			_, err := client.RelatedUsers(ctx, RelatedUsersRequest{UserID: 1})
			return err
		}},
		{name: "user following", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserFollowing(ctx, UserFollowingRequest{UserID: 1, Restrict: RestrictPrivate})
			return err
		}},
		{name: "user followers", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserFollowers(ctx, UserFollowersRequest{UserID: 1, Restrict: RestrictPrivate})
			return err
		}},
		{name: "user blocked", call: func(ctx context.Context, client *Client) error {
			_, err := client.UserBlockedUsers(ctx, UserBlockedUsersRequest{UserID: 1})
			return err
		}},
		{name: "follow add", call: func(ctx context.Context, client *Client) error {
			return client.FollowUser(ctx, FollowUserRequest{UserID: 1})
		}},
		{name: "follow delete", call: func(ctx context.Context, client *Client) error {
			return client.UnfollowUser(ctx, UnfollowUserRequest{UserID: 1})
		}},
		{name: "AI visibility", call: func(ctx context.Context, client *Client) error {
			return client.SetAIArtworkVisibility(ctx, SetAIArtworkVisibilityRequest{Visible: true})
		}},
	}
	for _, family := range families {
		family := family
		t.Run(family.name, func(t *testing.T) {
			calls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host != "app-api.pixiv.net" {
					return nil, errors.New("unexpected non-App API host")
				}
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			client, err := NewWith("access-token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if err := family.call(context.Background(), client); sdk.ReasonOf(err) != sdk.CredentialsExpired {
				t.Fatalf("error reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.CredentialsExpired, err)
			}
			if calls != 1 {
				t.Fatalf("family made %d App API calls, want 1", calls)
			}
		})
	}
}

func TestAddBookmarkWiresMutation(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v2/illust/bookmark/add" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
		}
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err := client.AddBookmark(context.Background(), AddBookmarkRequest{ArtworkID: 12, Tags: []string{"tag"}}); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
}

func TestSearchArtworksEmptyPageUsesNonNilItems(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "{\"illusts\":[],\"next_url\":null}"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "empty"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if page.Items == nil {
		t.Fatal("successful empty page must contain a non-nil Items slice")
	}
	if !page.Next.IsZero() {
		t.Fatal("empty terminal page must not contain a cursor")
	}
}

func TestSearchArtworksMapsHTTPStatusErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		reason sdk.Reason
	}{
		{name: "forbidden", status: http.StatusForbidden, reason: sdk.Forbidden},
		{name: "upstream", status: http.StatusBadGateway, reason: sdk.UpstreamError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			_, err = client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "status"})
			if sdk.ReasonOf(err) != test.reason {
				t.Fatalf("ReasonOf = %q, want %q", sdk.ReasonOf(err), test.reason)
			}
			var typed *sdk.Error
			if !errors.As(err, &typed) || typed.HTTPStatus != test.status {
				t.Fatalf("error = %#v, want HTTP status %d", err, test.status)
			}
		})
	}
}

func TestSearchArtworksRejectsInvalidBookmarkRange(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	minimum, maximum := 10, 5
	tests := []struct {
		name string
		min  *int
		max  *int
	}{
		{name: "negative minimum", min: pointer(-1)},
		{name: "negative maximum", max: pointer(-1)},
		{name: "minimum exceeds maximum", min: &minimum, max: &maximum},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{
				Word:        "range",
				BookmarkMin: test.min,
				BookmarkMax: test.max,
			})
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
			}
		})
	}
}

func pointer(value int) *int {
	return &value
}

func TestSearchArtworksRejectsUnknownQueryValues(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name    string
		request SearchArtworksRequest
	}{
		{name: "whitespace word", request: SearchArtworksRequest{Word: " \t"}},
		{name: "target", request: SearchArtworksRequest{Word: "query", Target: SearchTarget("unknown")}},
		{name: "sort", request: SearchArtworksRequest{Word: "query", Sort: SortMode("unknown")}},
		{name: "duration", request: SearchArtworksRequest{Word: "query", Duration: DurationFilter("unknown")}},
		{name: "content type", request: SearchArtworksRequest{Word: "query", ContentType: SearchContentType("unknown")}},
		{name: "ai mode", request: SearchArtworksRequest{Word: "query", AIMode: SearchAIMode("unknown")}},
		{name: "aspect ratio", request: SearchArtworksRequest{Word: "query", AspectRatio: SearchAspectRatio("unknown")}},
		{name: "resolution", request: SearchArtworksRequest{Word: "query", Resolution: SearchResolution("unknown")}},
		{name: "invalid start date", request: SearchArtworksRequest{Word: "query", StartDate: "2026-02-30"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SearchArtworks(context.Background(), test.request)
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
			}
		})
	}
}

func TestSearchArtworksAIModeOnlyFiltersBatchAndBindsCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		query := req.URL.Query()
		if query.Get("search_ai_type") != "0" {
			t.Errorf("search_ai_type = %q, want 0 for local only filtering", query.Get("search_ai_type"))
		}
		body := `{"illusts":[
			{"id":901,"title":"ai","type":"illust","ai_type":2,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}},
			{"id":902,"title":"human","type":"illust","ai_type":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}
		],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=tag&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := SearchArtworksRequest{Word: "tag", AIMode: SearchAIModeOnly}
	page, err := client.SearchArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 901 || page.Items[0].AIType != 2 {
		t.Fatalf("filtered items = %#v, want only AI artwork", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation cursor")
	}

	_, err = client.SearchArtworks(context.Background(), SearchArtworksRequest{
		Word: "tag", AIMode: SearchAIModeExclude, Cursor: page.Next,
	})
	if sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("ReasonOf = %q, want %q (calls=%d err=%v)", sdk.ReasonOf(err), sdk.InvalidCursor, calls, err)
	}
	if calls != 1 {
		t.Fatalf("changed AI mode reached upstream; calls = %d, want 1", calls)
	}
}

func TestSearchNovelsRejectsUnknownQueryValues(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name    string
		request SearchNovelsRequest
	}{
		{name: "whitespace word", request: SearchNovelsRequest{Word: " \t"}},
		{name: "target", request: SearchNovelsRequest{Word: "query", Target: SearchTarget("unknown")}},
		{name: "sort", request: SearchNovelsRequest{Word: "query", Sort: SortMode("unknown")}},
		{name: "duration", request: SearchNovelsRequest{Word: "query", Duration: DurationFilter("unknown")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SearchNovels(context.Background(), test.request)
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
			}
		})
	}
}

func TestArtworkRankingDefaultsAndValidatesMode(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingMode("unknown")}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("invalid mode ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
	}
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingModeDay, Date: "2026-02-30"}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("invalid date ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
	}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("mode"); got != string(RankingModeDay) {
			t.Errorf("mode = %q, want %q", got, RankingModeDay)
		}
		body := `{"illusts":[],"next_url":null}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err = NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
	if err != nil {
		t.Fatalf("ArtworkRanking: %v", err)
	}
	if page.Items == nil {
		t.Fatal("successful empty ranking must contain a non-nil Items slice")
	}
}

func TestSearchUsersRejectsWhitespaceWord(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.SearchUsers(context.Background(), SearchUsersRequest{Word: " \t"})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
}
