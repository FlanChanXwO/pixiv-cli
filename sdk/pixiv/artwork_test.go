package pixiv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// wireRecorder 记录 SDK 发出的 App API 请求，并按队列返回固定响应。
type wireRecorder struct {
	requests []*http.Request
	bodies   []string
	err      error
}

func newWireClient(t *testing.T, bodies ...string) (*Client, *wireRecorder) {
	t.Helper()
	recorder := &wireRecorder{bodies: bodies}
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorder.requests = append(recorder.requests, req)
		if recorder.err != nil {
			return nil, recorder.err
		}
		body := ""
		if len(recorder.bodies) > 0 {
			body, recorder.bodies = recorder.bodies[0], recorder.bodies[1:]
		}
		return jsonResponse(body), nil
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client, recorder
}

func (r *wireRecorder) request(t *testing.T, index int) *http.Request {
	t.Helper()
	if index >= len(r.requests) {
		t.Fatalf("request %d was not sent (%d recorded)", index, len(r.requests))
	}
	return r.requests[index]
}

func (r *wireRecorder) calls() int { return len(r.requests) }

const oneArtwork = `{"id":5,"title":"ranked","type":"illust","create_date":"2024-01-02T03:04:05+00:00","user":{"id":6,"name":"artist"},"tags":[]}`

type artworkFamilyCase struct {
	name      string
	body      string
	invoke    func(*Client) error
	path      string
	query     map[string]string
	nextQuery map[string]string
}

// TestArtworkListFamily 覆盖所有共用 artworkListWire/artworkPage 的列表端点：
// 路由、初始 query、续页 query 与响应映射。
func TestArtworkListFamily(t *testing.T) {
	cases := []artworkFamilyCase{
		{
			name:  "following",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=public&offset=20"}`,
			path:  "/v2/illust/follow",
			query: map[string]string{"restrict": "public"},
			invoke: func(c *Client) error {
				_, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{})
				return err
			},
			nextQuery: map[string]string{"restrict": "public", "offset": "20"},
		},
		{
			name:  "latest",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=illust&filter=for_android&max_illust_id=987654"}`,
			path:  "/v1/illust/new",
			query: map[string]string{"content_type": "illust", "filter": "for_android"},
			invoke: func(c *Client) error {
				_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
				return err
			},
			nextQuery: map[string]string{"max_illust_id": "987654", "filter": "for_android"},
		},
		{
			name: "mypixiv",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/mypixiv?offset=40"}`,
			path: "/v2/illust/mypixiv",
			invoke: func(c *Client) error {
				_, err := c.MyPixivArtworks(context.Background(), MyPixivArtworksRequest{})
				return err
			},
			nextQuery: map[string]string{"offset": "40"},
		},
		{
			name:  "user artworks",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=77&type=manga&offset=50"}`,
			path:  "/v1/user/illusts",
			query: map[string]string{"user_id": "77", "type": "manga"},
			invoke: func(c *Client) error {
				_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Kind: ArtworkKindManga})
				return err
			},
			nextQuery: map[string]string{"user_id": "77", "type": "manga", "offset": "50"},
		},
		{
			name:  "ranking",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=week&date=2024-01-01&offset=30"}`,
			path:  "/v1/illust/ranking",
			query: map[string]string{"mode": "week", "date": "2024-01-01"},
			invoke: func(c *Client) error {
				_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingModeWeek, Date: "2024-01-01"})
				return err
			},
			nextQuery: map[string]string{"mode": "week", "offset": "30"},
		},
		{
			name:  "search",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=landscape&offset=30"}`,
			path:  "/v1/search/illust",
			query: map[string]string{"word": "landscape"},
			invoke: func(c *Client) error {
				_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
				return err
			},
			nextQuery: map[string]string{"word": "landscape", "offset": "30"},
		},
		{
			name:  "bookmarks",
			body:  `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=7&restrict=private&tag=cat&max_bookmark_id=88"}`,
			path:  "/v1/user/bookmarks/illust",
			query: map[string]string{"user_id": "7", "restrict": "private", "tag": "cat"},
			invoke: func(c *Client) error {
				_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Restrict: RestrictPrivate, Tag: "cat"})
				return err
			},
			nextQuery: map[string]string{"user_id": "7", "tag": "cat", "max_bookmark_id": "88"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body, `{"illusts":[],"next_url":null}`)
			if err := test.invoke(client); err != nil {
				t.Fatalf("first page: %v", err)
			}
			first := recorder.request(t, 0)
			if first.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", first.URL.Path, test.path)
			}
			for key, want := range test.query {
				if got := first.URL.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q; query=%v", key, got, want, first.URL.Query())
				}
			}
			if _, present := first.URL.Query()["offset"]; present && test.name != "following" {
				t.Errorf("initial query unexpectedly contains offset: %v", first.URL.Query())
			}
		})
	}
}

func TestArtworkListFamilyMapsAndContinues(t *testing.T) {
	tests := []artworkFamilyCase{
		{
			name: "following", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=public&offset=20"}`,
			invoke: func(c *Client) error {
				page, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{})
				return continueArtwork(t, c, page, err, func(cursor sdk.Cursor) (sdk.Page[Artwork], error) {
					return c.FollowingArtworks(context.Background(), FollowingArtworksRequest{Cursor: cursor})
				})
			},
			nextQuery: map[string]string{"restrict": "public", "offset": "20"},
		},
		{
			name: "ranking", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=day&offset=30"}`,
			invoke: func(c *Client) error {
				page, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
				return continueArtwork(t, c, page, err, func(cursor sdk.Cursor) (sdk.Page[Artwork], error) {
					return c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Cursor: cursor})
				})
			},
			nextQuery: map[string]string{"mode": "day", "offset": "30"},
		},
		{
			name: "user artworks", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=77&type=illust&offset=30"}`,
			invoke: func(c *Client) error {
				page, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77})
				return continueArtwork(t, c, page, err, func(cursor sdk.Cursor) (sdk.Page[Artwork], error) {
					return c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Cursor: cursor})
				})
			},
			nextQuery: map[string]string{"user_id": "77", "type": "illust", "offset": "30"},
		},
		{
			name: "bookmarks", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=7&restrict=public&tag=cat&max_bookmark_id=88"}`,
			invoke: func(c *Client) error {
				page, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Tag: "cat"})
				return continueArtwork(t, c, page, err, func(cursor sdk.Cursor) (sdk.Page[Artwork], error) {
					return c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Tag: "cat", Cursor: cursor})
				})
			},
			nextQuery: map[string]string{"user_id": "7", "tag": "cat", "max_bookmark_id": "88"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body, `{"illusts":[],"next_url":null}`)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			query := recorder.request(t, 1).URL.Query()
			for key, want := range test.nextQuery {
				if got := query.Get(key); got != want {
					t.Errorf("%s = %q, want %q; query=%v", key, got, want, query)
				}
			}
		})
	}
}

func continueArtwork(t *testing.T, _ *Client, page sdk.Page[Artwork], err error, next func(sdk.Cursor) (sdk.Page[Artwork], error)) error {
	t.Helper()
	if err != nil {
		return err
	}
	if len(page.Items) != 1 || page.Items[0].ID != 5 || page.Items[0].Title != "ranked" {
		t.Fatalf("page = %#v", page)
	}
	if page.Next.IsZero() {
		t.Fatal("expected a continuation cursor")
	}
	second, err := next(page.Next)
	if err != nil {
		return err
	}
	if second.Items == nil || len(second.Items) != 0 || !second.Next.IsZero() {
		t.Fatalf("terminal page = %#v", second)
	}
	return nil
}

func TestArtworkDetailMapsPagesSeriesViewerAndResources(t *testing.T) {
	const rawURL = "https://i.pximg.net/img/5.png?signature=sentinel"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" || req.URL.Query().Get("illust_id") != "5" {
			t.Fatalf("request = %q %v", req.URL.Path, req.URL.Query())
		}
		return jsonResponse(`{"illust":{"id":5,"title":"one","type":"manga","create_date":"2024-01-01T00:00:00Z","page_count":2,"image_urls":{"original":"` + rawURL + `"},"meta_pages":[{"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p0.png"}},{"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p1.png"}}],"user":{"id":9,"name":"u","account":"u"},"tags":[{"name":"cat","translated_name":"猫"}],"is_bookmarked":true,"is_muted":false,"visible":true,"sanity_level":2,"restriction_attributes":["restricted_mode"],"series":{"id":123,"title":"chapter"},"total_comments":0}}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.Kind != ArtworkKindManga || artwork.ID != 5 || artwork.User.ID != 9 || len(artwork.Tags) != 1 || artwork.Tags[0].TranslatedName != "猫" {
		t.Fatalf("artwork = %+v", artwork)
	}
	if artwork.IsBookmarked == nil || !*artwork.IsBookmarked || artwork.IsMuted == nil || *artwork.IsMuted || artwork.SanityLevel == nil || *artwork.SanityLevel != 2 {
		t.Fatalf("viewer fields = %+v", artwork)
	}
	if artwork.Series == nil || artwork.Series.ID != 123 || artwork.TotalComments == nil || *artwork.TotalComments != 0 {
		t.Fatalf("series/comments = %+v/%v", artwork.Series, artwork.TotalComments)
	}
	if len(artwork.Pages) != 2 || artwork.Pages[0].PageIndex != 0 || artwork.Pages[1].PageIndex != 1 {
		t.Fatalf("pages = %+v", artwork.Pages)
	}
	if artwork.Cover.Resource.Ref.IsZero() || artwork.Pages[1].Image.Resource.Ref.IsZero() {
		t.Fatal("artwork resources lost opaque references")
	}
	payload, err := sdk.ResourceRefPayload(artwork.Cover.Resource.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if strings.Contains(string(payload), rawURL) || strings.Contains(string(payload), "signature") {
		t.Fatalf("resource reference payload exposes resolved URL: %q", payload)
	}

	pages, err := client.ArtworkPages(context.Background(), ArtworkPagesRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("ArtworkPages: %v", err)
	}
	if len(pages) != 2 || pages[1].PageIndex != 1 || pages[0].Image.Resource.Ref == pages[1].Image.Resource.Ref {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestArtworkDetailRejectsMalformedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"missing illust":       `{}`,
		"null illust":          `{"illust":null}`,
		"zero id":              `{"illust":{"id":0}}`,
		"missing publish time": `{"illust":{"id":1,"type":"illust","image_urls":{"original":"https://i.pximg.net/img/1.png"},"user":{"id":7}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if _, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}

func TestArtworkPagesDerivesSinglePageCover(t *testing.T) {
	client, _ := newWireClient(t, `{"illust":{"id":5,"type":"illust","create_date":"2024-01-01T00:00:00Z","image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img/5.png"},"user":{"id":7,"name":"artist"}}}`)
	pages, err := client.ArtworkPages(context.Background(), ArtworkPagesRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("ArtworkPages: %v", err)
	}
	if len(pages) != 1 || pages[0].PageIndex != 0 || !strings.Contains(pages[0].Image.Resource.URL, "5.png") {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestRelatedRecommendedSeriesAndTrending(t *testing.T) {
	t.Run("related indexed continuation", func(t *testing.T) {
		liveNextURL := "https://app-api.pixiv.net/v2/illust/related?illust_id=123&seed_illust_ids%5B0%5D=123&viewed%5B0%5D=456&viewed%5B1%5D=789"
		var queries []map[string][]string
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			queries = append(queries, req.URL.Query())
			if len(queries) == 2 {
				return jsonResponse(`{"illusts":[],"next_url":null}`), nil
			}
			return jsonResponse(`{"illusts":[` + oneArtwork + `],"next_url":"` + liveNextURL + `"}`), nil
		})
		client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		page, err := client.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if _, err := client.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123, Cursor: page.Next}); err != nil {
			t.Fatalf("continuation: %v", err)
		}
		if got := queries[1]["seed_illust_ids[]"]; len(got) != 1 || got[0] != "123" {
			t.Fatalf("seed_illust_ids[] = %v", got)
		}
		if got := queries[1]["viewed[]"]; len(got) != 2 {
			t.Fatalf("viewed[] = %v", got)
		}
		if _, present := queries[1]["offset"]; present {
			t.Fatalf("offset unexpectedly present: %v", queries[1])
		}
	})

	t.Run("recommended multi-param replay", func(t *testing.T) {
		liveNextURL := "https://app-api.pixiv.net/v1/illust/recommended?min_bookmark_id_for_recent_illust=38760619879&max_bookmark_id_for_recommend=38712263830&offset=0&viewed%5B0%5D=144455829"
		client, recorder := newWireClient(t, `{"illusts":[`+oneArtwork+`],"next_url":"`+liveNextURL+`"}`, `{"illusts":[],"next_url":null}`)
		page, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if strings.Contains(page.Next.String(), "http") {
			t.Fatalf("cursor leaks transport material: %s", page.Next.String())
		}
		if _, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{Cursor: page.Next}); err != nil {
			t.Fatalf("continuation: %v", err)
		}
		query := recorder.request(t, 1).URL.Query()
		if query.Get("offset") != "0" || query.Get("min_bookmark_id_for_recent_illust") != "38760619879" {
			t.Fatalf("continuation query = %v", query)
		}
		for key := range query {
			if strings.HasPrefix(key, "viewed[") {
				t.Fatalf("viewed params must be dropped: %v", query)
			}
		}
	})

	t.Run("series continuation and detail", func(t *testing.T) {
		client, recorder := newWireClient(t,
			`{"illust_series_detail":{"user":{"id":7}},"illusts":[`+oneArtwork+`],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=42&last_order=8"}`,
			`{"illust_series_detail":{"user":{"id":7}},"illusts":[],"next_url":null}`,
		)
		page, err := client.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if _, err := client.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42, Cursor: page.Next}); err != nil {
			t.Fatalf("continuation: %v", err)
		}
		query := recorder.request(t, 1).URL.Query()
		if query.Get("illust_series_id") != "42" || query.Get("last_order") != "8" || query.Get("offset") != "" {
			t.Fatalf("continuation query = %v", query)
		}
	})

	t.Run("trending tags", func(t *testing.T) {
		client, recorder := newWireClient(t, `{"trend_tags":[{"tag":"cat","translated_name":"猫","illust":{"id":1,"title":"cover","user":{"id":2},"create_date":"2024-01-02T03:04:05+00:00"}}]}`)
		items, err := client.TrendingArtworkTags(context.Background(), TrendingArtworkTagsRequest{})
		if err != nil || len(items) != 1 || items[0].Tag != "cat" || items[0].Artwork.ID != 1 {
			t.Fatalf("items = %#v err=%v", items, err)
		}
		if req := recorder.request(t, 0); req.URL.Path != "/v1/trending-tags/illust" || len(req.URL.Query()) != 0 {
			t.Fatalf("request = %q %v", req.URL.Path, req.URL.Query())
		}
	})
}

func TestArtworkContinuationRejectsForeignOrMismatchedNextURLs(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		invoke func(*Client) error
	}{
		{name: "related gapped", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/related?illust_id=123&seed_illust_ids%5B0%5D=456&viewed%5B1%5D=123"}`, invoke: func(c *Client) error {
			_, err := c.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123})
			return err
		}},
		{name: "related wrong artwork", body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/related?illust_id=999&offset=30"}`, invoke: func(c *Client) error {
			page, err := c.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123})
			if err != nil {
				return err
			}
			_, err = c.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123, Cursor: page.Next})
			return err
		}},
		{name: "recommended missing list", body: `{"next_url":null}`, invoke: func(c *Client) error {
			_, err := c.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
			return err
		}},
		{name: "search unknown key", body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30&max_novel_id=9"}`, invoke: func(c *Client) error {
			_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
			return err
		}},
		{name: "search wrong path", body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/novel?offset=30"}`, invoke: func(c *Client) error {
			_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
			return err
		}},
		{name: "search wrong host", body: `{"illusts":[],"next_url":"https://example.invalid/v1/search/illust?offset=30"}`, invoke: func(c *Client) error {
			_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
			return err
		}},
		{name: "latest duplicate", body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/new?max_illust_id=987654&max_illust_id=123456&offset=30"}`, invoke: func(c *Client) error {
			_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
			return err
		}},
		{name: "series missing detail", body: `{"illust_series_detail":null,"illusts":[]}`, invoke: func(c *Client) error {
			_, err := c.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse && sdk.ReasonOf(err) != sdk.InvalidCursor {
				t.Fatalf("error = %v, want malformed response or invalid cursor", err)
			}
		})
	}
}

func TestArtworkListValidationBeforeNetwork(t *testing.T) {
	client, recorder := newWireClient(t)
	recorder.err = errors.New("unexpected transport")
	tests := []struct {
		name   string
		invoke func() error
	}{
		{name: "search word", invoke: func() error {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{})
			return err
		}},
		{name: "negative offset", invoke: func() error {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "x", Offset: -1})
			return err
		}},
		{name: "bookmark range", invoke: func() error {
			minimum, maximum := 10, 5
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "x", BookmarkMin: &minimum, BookmarkMax: &maximum})
			return err
		}},
		{name: "unknown query value", invoke: func() error {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "x", Target: SearchTarget("unknown")})
			return err
		}},
		{name: "following unknown restrict", invoke: func() error {
			_, err := client.FollowingArtworks(context.Background(), FollowingArtworksRequest{Restrict: Restrict("friends")})
			return err
		}},
		{name: "latest ugoira", invoke: func() error {
			_, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeUgoira})
			return err
		}},
		{name: "ranking mode", invoke: func() error {
			_, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingMode("weekly")})
			return err
		}},
		{name: "user artworks id", invoke: func() error {
			_, err := client.UserArtworks(context.Background(), UserArtworksRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.invoke(); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want InvalidArgument (err=%v)", sdk.ReasonOf(err), err)
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid requests reached upstream %d time(s)", recorder.calls())
	}
}

func TestArtworkEmptyTerminalPageAndTransportFailures(t *testing.T) {
	empty := []struct {
		name   string
		body   string
		invoke func(*Client) error
	}{
		{name: "bookmarks", body: `{"illusts":[],"next_url":null}`, invoke: func(c *Client) error {
			_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7})
			return err
		}},
		{name: "search", body: `{"illusts":[],"next_url":null}`, invoke: func(c *Client) error {
			_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "empty"})
			return err
		}},
		{name: "related", body: `{"illusts":[]}`, invoke: func(c *Client) error {
			_, err := c.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 1})
			return err
		}},
		{name: "user artworks", body: `{"illusts":[]}`, invoke: func(c *Client) error {
			_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77})
			return err
		}},
	}
	for _, test := range empty {
		t.Run("empty/"+test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
		})
	}

	failures := []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "bookmarks", invoke: func(c *Client) error {
			_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7})
			return err
		}},
		{name: "bookmark tags", invoke: func(c *Client) error {
			_, err := c.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7})
			return err
		}},
		{name: "bookmark detail", invoke: func(c *Client) error {
			_, err := c.ArtworkBookmark(context.Background(), ArtworkBookmarkRequest{ArtworkID: 9})
			return err
		}},
	}
	for _, test := range failures {
		t.Run("transport/"+test.name, func(t *testing.T) {
			client, recorder := newWireClient(t)
			recorder.err = errors.New("transport failed")
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
				t.Fatalf("error = %v, want a classified transport failure", err)
			}
		})
	}
}

func TestArtworkRankingModes(t *testing.T) {
	for _, mode := range []RankingMode{
		RankingModeDay, RankingModeDayMale, RankingModeDayFemale, RankingModeWeek,
		RankingModeWeekOriginal, RankingModeWeekRookie, RankingModeMonth,
		RankingModeDayManga, RankingModeWeekManga, RankingModeMonthManga,
		RankingModeWeekRookieManga, RankingModeDayR18, RankingModeDayMaleR18,
		RankingModeDayFemaleR18, RankingModeWeekR18, RankingModeWeekR18G,
	} {
		t.Run(string(mode), func(t *testing.T) {
			client, recorder := newWireClient(t, `{"illusts":[],"next_url":null}`)
			if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: mode}); err != nil {
				t.Fatalf("ArtworkRanking: %v", err)
			}
			if got := recorder.request(t, 0).URL.Query().Get("mode"); got != string(mode) {
				t.Fatalf("mode = %q, want %q", got, mode)
			}
		})
	}
	client, recorder := newWireClient(t, `{"illusts":[],"next_url":null}`)
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{}); err != nil {
		t.Fatalf("ArtworkRanking: %v", err)
	}
	if got := recorder.request(t, 0).URL.Query().Get("mode"); got != "day" {
		t.Fatalf("default mode = %q, want day", got)
	}
}

func TestArtworkUgoiraMetadata(t *testing.T) {
	valid := `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/zip/m.zip","original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100},{"file":"1.jpg","delay":200}]}}`
	client, recorder := newWireClient(t, valid)
	metadata, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 777})
	if err != nil {
		t.Fatalf("UgoiraMetadata: %v", err)
	}
	if metadata.ArtworkID != 777 || len(metadata.Archives) != 2 || len(metadata.Frames) != 2 || metadata.Frames[1].DelayMilliseconds != 200 {
		t.Fatalf("metadata = %#v", metadata)
	}
	if got := recorder.request(t, 0).URL.Query().Get("illust_id"); got != "777" {
		t.Fatalf("illust_id = %q, want 777", got)
	}

	for _, files := range [][]string{
		{"../000000.jpg"},
		{"/tmp/000000.jpg"},
		{"frames/../000000.jpg"},
		{`frames\..\000000.jpg`},
		{"000000.jpg", "000000.jpg"},
	} {
		t.Run(strings.Join(files, ","), func(t *testing.T) {
			client, _ := newWireClient(t, ugoiraBody(files...))
			if _, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 123}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response for %v", err, files)
			}
		})
	}

	client, _ = newWireClient(t, `{"ugoira_metadata":{"zip_urls":{"original":"https://evil.example/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100}]}}`)
	if _, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 1}); sdk.ReasonOf(err) != sdk.ResourceForbidden {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ResourceForbidden, err)
	}
}

func ugoiraBody(files ...string) string {
	frames := make([]string, len(files))
	for index, file := range files {
		frames[index] = `{"file":` + strconv.Quote(file) + `,"delay":80}`
	}
	return `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/img-zip-ugoira/img/123.zip"},"frames":[` + strings.Join(frames, ",") + `]}}`
}

// TestArtworkCursorBindings 覆盖各列表 cursor 必须保留的原始请求条件：
// latest 的 content type、search 的 query、user artworks 的 canonical kind。
func TestArtworkCursorBindings(t *testing.T) {
	t.Run("latest binds content type", func(t *testing.T) {
		client, recorder := newWireClient(t,
			`{"illusts":[`+oneArtwork+`],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=illust&filter=for_android&max_illust_id=987654"}`,
			`{"illusts":[],"next_url":null}`,
		)
		page, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeIllust})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if _, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeManga, Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("changed content type = %v, want InvalidCursor", err)
		}
		if _, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeIllust, Cursor: page.Next}); err != nil {
			t.Fatalf("same content type continuation: %v", err)
		}
		if got := recorder.request(t, 1).URL.Query().Get("max_illust_id"); got != "987654" {
			t.Fatalf("max_illust_id = %q, want 987654", got)
		}
	})

	t.Run("search binds query", func(t *testing.T) {
		client, _ := newWireClient(t, `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=30"}`)
		page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "different", Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("changed query = %v, want InvalidCursor", err)
		}
	})

	t.Run("user artworks bind canonical kind", func(t *testing.T) {
		client, recorder := newWireClient(t,
			`{"illusts":[`+oneArtwork+`],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=77&type=illust&offset=30"}`,
			`{"illusts":[],"next_url":null}`,
		)
		page, err := client.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Kind: ArtworkKind("illust")})
		if err != nil || page.Next.IsZero() {
			t.Fatalf("page = %#v err=%v", page, err)
		}
		if _, err := client.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Kind: ArtworkKind("illust"), Cursor: page.Next}); err != nil {
			t.Fatalf("canonical continuation: %v", err)
		}
		if got := recorder.request(t, 1).URL.Query().Get("type"); got != "illust" {
			t.Fatalf("type = %q, want illust", got)
		}
		if _, err := client.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Kind: ArtworkKind("all")}); sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("unknown kind = %v, want InvalidArgument", err)
		}
		if recorder.calls() != 2 {
			t.Fatalf("invalid kind reached transport: %d calls", recorder.calls())
		}
	})
}

func TestSearchArtworkCursorBinding(t *testing.T) {
	t.Run("initial offset", func(t *testing.T) {
		calls := 0
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			want := "270"
			if calls == 2 {
				want = "300"
			}
			if got := req.URL.Query().Get("offset"); got != want {
				t.Errorf("request %d offset = %q, want %q", calls, got, want)
			}
			return jsonResponse(`{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=300"}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatal(err)
		}
		request := SearchArtworksRequest{Word: "test", Offset: 270}
		page, err := client.SearchArtworks(context.Background(), request)
		if err != nil || page.Next.IsZero() {
			t.Fatalf("first page = %+v, err = %v", page, err)
		}
		for _, offset := range []int{0, 300} {
			request.Offset = offset
			request.Cursor = page.Next
			if _, err := client.SearchArtworks(context.Background(), request); sdk.ReasonOf(err) != sdk.InvalidCursor {
				t.Fatalf("offset %d: want InvalidCursor, got %v", offset, err)
			}
		}
		if calls != 1 {
			t.Fatalf("mismatched cursors reached upstream: %d calls", calls)
		}
	})

	t.Run("legacy cursor and local context", func(t *testing.T) {
		client, _ := NewWith("token", Options{})
		legacy := buildLegacyRecommendedCursor(t, "offset", 0)
		if _, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{Cursor: legacy}); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidCursor, err)
		}
	})

	t.Run("local context not sent upstream", func(t *testing.T) {
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			query := req.URL.Query()
			if query.Get("rating") != "" || query.Get("x_restrict") != "" || query.Get("cursor_context") != "" {
				t.Fatalf("local filter leaked to upstream: %v", query)
			}
			return jsonResponse(`{"illusts":[],"next_url":null}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "cat", CursorContext: "local filter"}); err != nil {
			t.Fatalf("SearchArtworks: %v", err)
		}
	})
}

func buildLegacyRecommendedCursor(t *testing.T, key string, value int64) sdk.Cursor {
	t.Helper()
	client, _ := newWireClient(t, `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`)
	page, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
	if err != nil || page.Next.IsZero() {
		t.Fatalf("mint real cursor: page=%#v err=%v", page, err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(page.Next.String())
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	envelope["b"] = 1
	envelope["pl"] = []byte(`{"k":"` + key + `","v":` + strconv.FormatInt(value, 10) + `}`)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal legacy envelope: %v", err)
	}
	legacy, err := sdk.ParseCursor(base64.RawURLEncoding.EncodeToString(encoded))
	if err != nil {
		t.Fatalf("parse legacy cursor: %v", err)
	}
	return legacy
}

func TestSearchArtworkAIModeFiltersBatchLocally(t *testing.T) {
	client, recorder := newWireClient(t, `{"illusts":[{"id":901,"type":"illust","ai_type":2,"create_date":"2026-01-01T00:00:00Z","user":{"id":7}},{"id":902,"type":"illust","ai_type":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7}}]}`)
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "ai", AIMode: SearchAIModeOnly})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != 901 {
		t.Fatalf("page = %#v err=%v, want only the AI artwork", page, err)
	}
	if got := recorder.request(t, 0).URL.Query().Get("search_ai_type"); got != "0" {
		t.Fatalf("search_ai_type = %q, want local filtering", got)
	}
}

func TestSearchArtworkWireAliases(t *testing.T) {
	for name, test := range map[string]struct {
		request SearchArtworksRequest
		query   map[string]string
	}{
		"content type": {request: SearchArtworksRequest{Word: "x", ContentType: SearchContentTypeIllustAndUgoira}, query: map[string]string{"content_type": "illust_and_ugoira"}},
		"ai mode":      {request: SearchArtworksRequest{Word: "x", AIMode: SearchAIModeOnly}, query: map[string]string{"search_ai_type": "0"}},
	} {
		t.Run(name, func(t *testing.T) {
			client, recorder := newWireClient(t, `{"illusts":[],"next_url":null}`)
			if _, err := client.SearchArtworks(context.Background(), test.request); err != nil {
				t.Fatalf("SearchArtworks: %v", err)
			}
			for key, want := range test.query {
				if got := recorder.request(t, 0).URL.Query().Get(key); got != want {
					t.Fatalf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestArtworkCommentMutationForms(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
		path   string
		form   map[string]string
		body   string
	}{
		{name: "create", invoke: func(c *Client) error {
			_, err := c.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 123, Comment: "hello"})
			return err
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "123", "comment": "hello"}, body: `{"comment_id":73}`},
		{name: "reply", invoke: func(c *Client) error {
			_, err := c.ReplyArtworkComment(context.Background(), ReplyArtworkCommentRequest{ArtworkID: 123, Comment: "hello", ParentCommentID: 71})
			return err
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "123", "comment": "hello", "parent_comment_id": "71"}, body: `{"comment":{"id":74}}`},
		{name: "stamp", invoke: func(c *Client) error {
			_, err := c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 123, StampID: 9})
			return err
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "123", "comment": "", "stamp_id": "9"}, body: `{"comment_id":75}`},
		{name: "delete", invoke: func(c *Client) error {
			return c.DeleteArtworkComment(context.Background(), DeleteArtworkCommentRequest{CommentID: 76})
		}, path: "/v1/illust/comment/delete", form: map[string]string{"comment_id": "76"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			req := recorder.request(t, 0)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if req.URL.Path != test.path || len(req.PostForm) != len(test.form) {
				t.Fatalf("request = %q form=%v", req.URL.Path, req.PostForm)
			}
			for key, want := range test.form {
				if got := req.PostForm.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestArtworkCommentMutationFailures(t *testing.T) {
	for _, body := range []string{`{}`, `{"comment_id":null}`, `{"comment_id":0}`, `{"comment_id":-1}`} {
		t.Run("response/"+body, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if _, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
	client, recorder := newWireClient(t)
	recorder.err = errors.New("transport canary")
	if _, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
		t.Fatalf("create error = %v, want UpstreamUnavailable", err)
	}
}

// TestArtworkCommentFamily 覆盖 artwork 与 novel 评论读取共用 mapComment/
// commentPage 的同一族契约：parent chain、两种日期字段、两种 access control
// 形态、空终页与缺失列表。
func TestArtworkCommentFamily(t *testing.T) {
	for _, family := range []struct {
		name   string
		path   string
		idKey  string
		invoke func(*Client, sdk.Cursor) (CommentPage, error)
	}{
		{name: "artwork", path: "/v3/illust/comments", idKey: "illust_id", invoke: func(c *Client, cursor sdk.Cursor) (CommentPage, error) {
			return c.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 123, Cursor: cursor})
		}},
		{name: "novel", path: "/v2/novel/comments", idKey: "novel_id", invoke: func(c *Client, cursor sdk.Cursor) (CommentPage, error) {
			return c.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 123, Cursor: cursor})
		}},
	} {
		t.Run(family.name, func(t *testing.T) {
			nextURL := "https://app-api.pixiv.net" + family.path + "?" + family.idKey + "=123&offset=4"
			client, recorder := newWireClient(t,
				`{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7},"parent_comment":{"id":6,"caption":"parent","created_at":"2024-01-01T03:04:05+00:00","user":{"id":5}}}],"total_comments":2,"access_control":{"can_comment":true,"is_locked":false},"next_url":"`+nextURL+`"}`,
				`{"comments":[],"next_url":null}`,
			)
			page, err := family.invoke(client, sdk.Cursor{})
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if len(page.Page.Items) != 1 || page.Page.Items[0].Comment != "child" || page.Page.Items[0].ParentComment == nil ||
				page.Total == nil || *page.Total != 2 || page.AccessControl == nil || !page.AccessControl.CanComment || page.Page.Next.IsZero() {
				t.Fatalf("page = %#v", page)
			}
			if request := recorder.request(t, 0); request.URL.Path != family.path || request.URL.Query().Get(family.idKey) != "123" {
				t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
			}
			second, err := family.invoke(client, page.Page.Next)
			if err != nil {
				t.Fatalf("continuation: %v", err)
			}
			if second.Page.Items == nil || len(second.Page.Items) != 0 || !second.Page.Next.IsZero() {
				t.Fatalf("terminal page = %#v", second)
			}
			if got := recorder.request(t, 1).URL.Query().Get("offset"); got != "4" {
				t.Fatalf("offset = %q, want 4", got)
			}
		})
	}
}

func TestCommentDateAndNumericAccessControl(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"comments":[{"id":9,"comment":"current wire","date":"2026-01-02T03:04:05+00:00","user":{"id":7}}],"comment_access_control":0}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 1})
	if err != nil {
		t.Fatalf("ArtworkComments: %v", err)
	}
	want := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if !page.Page.Items[0].CreatedAt.Equal(want) {
		t.Fatalf("created_at = %s, want %s", page.Page.Items[0].CreatedAt, want)
	}
	if page.AccessControl == nil || page.AccessControl.NumericValue == nil || *page.AccessControl.NumericValue != 0 || page.AccessControl.CanComment || page.AccessControl.IsLocked {
		t.Fatalf("access control = %#v", page.AccessControl)
	}
}

func TestCommentFamilyRejectsMalformedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"missing comments": `{}`,
		"null comments":    `{"comments":null}`,
		"invalid parent":   `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if _, err := client.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 1}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
	client, _ := newWireClient(t, `{"comments":[]}`)
	page, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 1})
	if err != nil || page.Page.Items == nil || len(page.Page.Items) != 0 || !page.Page.Next.IsZero() {
		t.Fatalf("empty page = %#v err=%v", page, err)
	}
}

func TestArtworkBookmarkDetailStates(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		restrict Restrict
		tags     []string
	}{
		{name: "registered tags only", body: `{"bookmark_detail":{"is_bookmarked":true,"restrict":"public","tags":[{"name":"content-tag","is_registered":false},{"name":"saved-tag","is_registered":true}]}}`, restrict: RestrictPublic, tags: []string{"saved-tag"}},
		{name: "explicitly not bookmarked", body: `{"bookmark_detail":{"is_bookmarked":false,"restrict":"private","tags":[{"name":"cat","is_registered":false}]}}`},
		{name: "null detail", body: `{"bookmark_detail":null}`},
		{name: "not found", status: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			if test.status != 0 {
				recorder.err = pixivapi.HTTPStatus(test.status)
			}
			detail, err := client.ArtworkBookmark(context.Background(), ArtworkBookmarkRequest{ArtworkID: 9})
			if err != nil {
				t.Fatalf("ArtworkBookmark: %v", err)
			}
			if len(test.tags) == 0 {
				if detail.Restrict != "" || detail.Tags == nil || len(detail.Tags) != 0 {
					t.Fatalf("detail = %#v, want empty normalized state", detail)
				}
				return
			}
			if detail.Restrict != test.restrict || len(detail.Tags) != len(test.tags) || detail.Tags[0] != test.tags[0] {
				t.Fatalf("detail = %#v, want %v/%v", detail, test.restrict, test.tags)
			}
		})
	}
}

func TestArtworkBookmarkTags(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"bookmark_tags":[{"name":"cat","count":2}],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?user_id=7&restrict=public&offset=5"}`,
		`{"bookmark_tags":[],"next_url":null}`,
	)
	page, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7})
	if err != nil {
		t.Fatalf("UserArtworkBookmarkTags: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != (BookmarkTag{Name: "cat", Count: 2}) || page.Next.IsZero() {
		t.Fatalf("page = %#v", page)
	}
	if request := recorder.request(t, 0); request.URL.Path != "/v1/user/bookmark-tags/illust" || request.URL.Query().Get("user_id") != "7" || request.URL.Query().Get("offset") != "" {
		t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
	}
	cont, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7, Cursor: page.Next})
	if err != nil || cont.Items == nil || len(cont.Items) != 0 || !cont.Next.IsZero() {
		t.Fatalf("continuation = %#v err=%v", cont, err)
	}
	if got := recorder.request(t, 1).URL.Query().Get("offset"); got != "5" {
		t.Fatalf("continuation offset = %q, want 5", got)
	}

	for name, body := range map[string]string{
		"missing tags":         `{}`,
		"null tags":            `{"bookmark_tags":null}`,
		"empty name":           `{"bookmark_tags":[{"name":"","count":1}]}`,
		"empty next url":       `{"bookmark_tags":[],"next_url":""}`,
		"missing continuation": `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?tag=cat"}`,
		"non-positive offset":  `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=0"}`,
		"duplicate offset":     `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=2&offset=3"}`,
	} {
		t.Run("malformed/"+name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			page, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse || page.Items != nil {
				t.Fatalf("page = %#v err = %v, want malformed response", page, err)
			}
		})
	}
}

func TestArtworkBookmarkListRejectsMalformedEnvelopes(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		invoke func(*Client) error
	}{
		"missing illusts": {body: `{}`, invoke: func(c *Client) error {
			_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 7})
			return err
		}},
		"null illusts": {body: `{"illusts":null}`, invoke: func(c *Client) error {
			_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
			return err
		}},
		"zero id": {body: `{"illusts":[{"id":0}]}`, invoke: func(c *Client) error {
			_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
			return err
		}},
		"mypixiv zero owner": {body: `{"illusts":[{"id":1,"user":{"id":0}}]}`, invoke: func(c *Client) error {
			_, err := c.MyPixivArtworks(context.Background(), MyPixivArtworksRequest{})
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}
