package pixiv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const oneNovel = `{"id":11,"title":"story","caption":"body","create_date":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"writer"},"x_restrict":0,"text_length":42,"is_original":true,"tags":[{"name":"fantasy","translated_name":"幻想"}],"image_urls":{"original":"https://i.pximg.net/img/11.png"}}`

type novelFamilyCase struct {
	name      string
	body      string
	invoke    func(*Client) (sdk.Page[Novel], error)
	resume    func(*Client, sdk.Cursor) (sdk.Page[Novel], error)
	path      string
	query     map[string]string
	nextQuery map[string]string
}

func TestNovelListFamily(t *testing.T) {
	cases := []novelFamilyCase{
		{
			name: "search",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=20"}`,
			path: "/v1/search/novel",
			query: map[string]string{
				"word": "story", "search_target": "partial_match_for_tags", "sort": "date_desc",
			},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story", Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "20", "word": "story"},
		},
		{
			name:  "ranking",
			body:  `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=30"}`,
			path:  "/v1/novel/ranking",
			query: map[string]string{"filter": "for_android", "mode": "day"},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.NovelRanking(context.Background(), NovelRankingRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.NovelRanking(context.Background(), NovelRankingRequest{Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "30", "mode": "day"},
		},
		{
			name: "recommended",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`,
			path: "/v1/novel/recommended",
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.RecommendedNovels(context.Background(), RecommendedNovelsRequest{Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "0"},
		},
		{
			name:  "following",
			body:  `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/follow?restrict=public&offset=30"}`,
			path:  "/v1/novel/follow",
			query: map[string]string{"restrict": "public"},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.FollowingNovels(context.Background(), FollowingNovelsRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.FollowingNovels(context.Background(), FollowingNovelsRequest{Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "30", "restrict": "public"},
		},
		{
			name:  "latest",
			body:  `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/new?filter=for_android&max_novel_id=987654"}`,
			path:  "/v1/novel/new",
			query: map[string]string{"filter": "for_android"},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.LatestNovels(context.Background(), LatestNovelsRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.LatestNovels(context.Background(), LatestNovelsRequest{Cursor: cursor})
			},
			nextQuery: map[string]string{"max_novel_id": "987654", "filter": "for_android"},
		},
		{
			name: "mypixiv",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/mypixiv?offset=40"}`,
			path: "/v1/novel/mypixiv",
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "40"},
		},
		{
			name:  "user novels",
			body:  `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/user/novels?user_id=77&filter=for_android&offset=50"}`,
			path:  "/v1/user/novels",
			query: map[string]string{"user_id": "77", "filter": "for_android"},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.UserNovels(context.Background(), UserNovelsRequest{UserID: 77})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.UserNovels(context.Background(), UserNovelsRequest{UserID: 77, Cursor: cursor})
			},
			nextQuery: map[string]string{"offset": "50", "user_id": "77", "filter": "for_android"},
		},
		{
			name:  "user novel bookmarks",
			body:  `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?user_id=77&restrict=public&tag=cat&max_bookmark_id=88"}`,
			path:  "/v1/user/bookmarks/novel",
			query: map[string]string{"user_id": "77", "tag": "cat"},
			invoke: func(c *Client) (sdk.Page[Novel], error) {
				return c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 77, Tag: "cat"})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 77, Tag: "cat", Cursor: cursor})
			},
			nextQuery: map[string]string{"user_id": "77", "max_bookmark_id": "88", "tag": "cat"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body, `{"novels":[],"next_url":null}`)
			page, err := test.invoke(client)
			if err != nil || page.Next.IsZero() || len(page.Items) != 1 || page.Items[0].ID != 11 {
				t.Fatalf("first page = %#v, err = %v", page, err)
			}
			if _, err := test.resume(client, page.Next); err != nil {
				t.Fatalf("continuation: %v", err)
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
			query := recorder.request(t, 1).URL.Query()
			for key, want := range test.nextQuery {
				if got := query.Get(key); got != want {
					t.Errorf("continuation %s = %q, want %q; query=%v", key, got, want, query)
				}
			}
		})
	}
}

func TestNovelListFamilyMapsAndContinues(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		mutate  func(*Client) (sdk.Page[Novel], error)
		resume  func(*Client, sdk.Cursor) (sdk.Page[Novel], error)
		nextKey string
	}{
		{
			name: "search",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=20"}`,
			mutate: func(c *Client) (sdk.Page[Novel], error) {
				return c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story", Cursor: cursor})
			},
			nextKey: "20",
		},
		{
			name: "ranking",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=30"}`,
			mutate: func(c *Client) (sdk.Page[Novel], error) {
				return c.NovelRanking(context.Background(), NovelRankingRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.NovelRanking(context.Background(), NovelRankingRequest{Cursor: cursor})
			},
			nextKey: "30",
		},
		{
			name: "latest",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/new?filter=for_android&max_novel_id=987654"}`,
			mutate: func(c *Client) (sdk.Page[Novel], error) {
				return c.LatestNovels(context.Background(), LatestNovelsRequest{})
			},
			resume: func(c *Client, cursor sdk.Cursor) (sdk.Page[Novel], error) {
				return c.LatestNovels(context.Background(), LatestNovelsRequest{Cursor: cursor})
			},
			nextKey: "987654",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body, `{"novels":[],"next_url":null}`)
			page, err := test.mutate(client)
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if len(page.Items) != 1 || page.Items[0].ID != 11 || page.Items[0].TextLength != 42 ||
				!page.Items[0].IsOriginal || page.Items[0].Tags == nil || page.Items[0].Tags[0].TranslatedName != "幻想" ||
				page.Items[0].Cover.Resource.Ref.IsZero() || page.Next.IsZero() {
				t.Fatalf("page = %#v", page)
			}
			second, err := test.resume(client, page.Next)
			if err != nil {
				t.Fatalf("continuation: %v", err)
			}
			if second.Items == nil || len(second.Items) != 0 || !second.Next.IsZero() {
				t.Fatalf("terminal page = %#v", second)
			}
			if got := recorder.request(t, 1).URL.Query().Get("offset"); test.name == "search" && got != test.nextKey {
				t.Fatalf("offset = %q, want %q", got, test.nextKey)
			}
		})
	}
}

func TestNovelEmptyTagsStayEmptyArray(t *testing.T) {
	client, _ := newWireClient(t, `{"novels":[{"id":11,"user":{"id":7},"create_date":"2024-01-02T03:04:05+00:00","tags":[]}]}`)
	page, err := client.NovelRanking(context.Background(), NovelRankingRequest{})
	if err != nil {
		t.Fatalf("NovelRanking: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Tags == nil || len(page.Items[0].Tags) != 0 {
		t.Fatalf("tags = %#v, want non-nil empty", page.Items)
	}
}

func TestNovelListRejectsMalformedEnvelopes(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		invoke func(*Client) error
	}{
		"missing novels": {body: `{}`, invoke: func(c *Client) error {
			_, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
			return err
		}},
		"null novels": {body: `{"novels":null}`, invoke: func(c *Client) error {
			_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
			return err
		}},
		"zero id": {body: `{"novels":[{"id":0,"user":{"id":1}}]}`, invoke: func(c *Client) error {
			_, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
			return err
		}},
		"zero owner": {body: `{"novels":[{"id":1,"user":{"id":0}}]}`, invoke: func(c *Client) error {
			_, err := c.UserNovels(context.Background(), UserNovelsRequest{UserID: 7})
			return err
		}},
		"search missing fields": {body: `{"novels":[{"id":1,"user":{"id":1}}]}`, invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
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

func TestNovelContinuationRejectsUnexpectedNextURLs(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		invoke func(*Client) error
	}{
		"search unknown key": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=30&start_date=2024-01-01"}`, invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
			return err
		}},
		"search wrong path": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30"}`, invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
			return err
		}},
		"ranking non-positive offset": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=0"}`, invoke: func(c *Client) error {
			_, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
			return err
		}},
		"latest offset": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?offset=30"}`, invoke: func(c *Client) error {
			_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
			return err
		}},
		"latest mixed keys": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?max_novel_id=987654&offset=30"}`, invoke: func(c *Client) error {
			_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
			return err
		}},
		"mypixiv wrong path": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/follow?offset=20&restrict=public"}`, invoke: func(c *Client) error {
			_, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
			return err
		}},
		"series non-positive last order": {body: `{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=21&last_order=0"}`, invoke: func(c *Client) error {
			_, err := c.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
			return err
		}},
		"bookmarks zero cursor": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=0"}`, invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7})
			return err
		}},
		"bookmarks duplicate key": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=2&max_bookmark_id=3"}`, invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7})
			return err
		}},
		"bookmarks wrong path": {body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=2"}`, invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7})
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

func TestNovelListValidationBeforeNetwork(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "search missing word", invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{})
			return err
		}},
		{name: "search unsupported target", invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "x", Target: SearchTarget("unknown")})
			return err
		}},
		{name: "ranking unsupported mode", invoke: func(c *Client) error {
			_, err := c.NovelRanking(context.Background(), NovelRankingRequest{Mode: RankingMode("unknown")})
			return err
		}},
		{name: "following unsupported restrict", invoke: func(c *Client) error {
			_, err := c.FollowingNovels(context.Background(), FollowingNovelsRequest{Restrict: Restrict("friends")})
			return err
		}},
		{name: "series non-positive id", invoke: func(c *Client) error {
			_, err := c.NovelSeries(context.Background(), NovelSeriesRequest{})
			return err
		}},
		{name: "detail non-positive id", invoke: func(c *Client) error {
			_, err := c.Novel(context.Background(), NovelRequest{})
			return err
		}},
		{name: "content non-positive id", invoke: func(c *Client) error {
			_, err := c.NovelContent(context.Background(), NovelContentRequest{})
			return err
		}},
		{name: "comments non-positive id", invoke: func(c *Client) error {
			_, err := c.NovelComments(context.Background(), NovelCommentsRequest{})
			return err
		}},
		{name: "bookmarks missing user", invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{})
			return err
		}},
		{name: "bookmark tags unsupported restrict", invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarkTags(context.Background(), UserNovelBookmarkTagsRequest{UserID: 7, Restrict: Restrict("friends")})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t)
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want InvalidArgument (err=%v)", sdk.ReasonOf(err), err)
			}
			if recorder.calls() != 0 {
				t.Fatalf("invalid request reached upstream %d time(s)", recorder.calls())
			}
		})
	}
}

func TestNovelListFamilyPreservesEmptyTerminalPage(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		invoke func(*Client) (int, bool, error)
	}{
		{name: "search", body: `{"novels":[],"next_url":null}`, invoke: func(c *Client) (int, bool, error) {
			page, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "empty"})
			return len(page.Items), page.Items == nil, err
		}},
		{name: "series", body: `{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":null}`, invoke: func(c *Client) (int, bool, error) {
			page, err := c.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
			return len(page.Novels.Items), page.Novels.Items == nil, err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			count, nilItems, err := test.invoke(client)
			if err != nil || nilItems || count != 0 {
				t.Fatalf("empty page = %d items, nil=%t, err=%v", count, nilItems, err)
			}
		})
	}
}

func TestNovelReadTransportErrorsPropagate(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "detail", invoke: func(c *Client) error {
			_, err := c.Novel(context.Background(), NovelRequest{NovelID: 1})
			return err
		}},
		{name: "series", invoke: func(c *Client) error {
			_, err := c.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 1})
			return err
		}},
		{name: "comments", invoke: func(c *Client) error {
			_, err := c.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 1})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t)
			recorder.err = errors.New("novel transport failed")
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
				t.Fatalf("error = %v, want a classified transport failure", err)
			}
		})
	}
}

func TestNovelSeriesAndDetail(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"novel_series_detail":{"id":21,"title":"series","caption":"description","is_concluded":true,"user":{"id":7}},"novels":[`+oneNovel+`],"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=21&last_order=9"}`,
		`{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":null}`,
	)
	result, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
	if err != nil {
		t.Fatalf("NovelSeries: %v", err)
	}
	if request := recorder.request(t, 0); request.URL.Path != "/v2/novel/series" || request.URL.Query().Get("series_id") != "21" || request.URL.Query().Get("last_order") != "" {
		t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
	}
	if result.Series.ID != 21 || result.Series.Caption != "description" || !result.Series.IsConcluded || len(result.Novels.Items) != 1 || result.Novels.Next.IsZero() {
		t.Fatalf("result = %#v", result)
	}
	if _, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21, Cursor: result.Novels.Next}); err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if got := recorder.request(t, 1).URL.Query().Get("last_order"); got != "9" {
		t.Fatalf("last_order = %q, want 9", got)
	}

	client, recorder = newWireClient(t, `{"novel":`+oneNovel+`}`)
	novel, err := client.Novel(context.Background(), NovelRequest{NovelID: 11})
	if err != nil {
		t.Fatalf("Novel: %v", err)
	}
	if request := recorder.request(t, 0); request.URL.Path != "/v2/novel/detail" || request.URL.Query().Get("novel_id") != "11" {
		t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
	}
	want := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	if novel.ID != 11 || novel.Title != "story" || novel.Caption != "body" || novel.User.ID != 7 || !novel.PublishedAt.Equal(want) || novel.Cover.Resource.Ref.IsZero() {
		t.Fatalf("novel = %#v", novel)
	}
}

func TestNovelSeriesAndDetailRejectMalformedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"missing detail":   `{"novels":[]}`,
		"null detail":      `{"novel_series_detail":null,"novels":[]}`,
		"zero detail user": `{"novel_series_detail":{"id":1,"user":{"id":0}},"novels":[]}`,
		"null novels":      `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":null}`,
		"zero novel id":    `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":[{"id":0,"user":{"id":1}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			result, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 1})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse || result.Novels.Items != nil {
				t.Fatalf("result = %#v, err = %v", result, err)
			}
		})
	}
	for _, body := range []string{`{}`, `{"novel":null}`, `{"novel":{"id":12,"user":{"id":0}}}`, `{"novel":{"id":12,"user":{"id":7}},"series_next":{"id":0}}`} {
		t.Run("detail/"+body, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if _, err := client.Novel(context.Background(), NovelRequest{NovelID: 12}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}

func TestNovelRecommendedContinuationReplaysLiveParams(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"novels":[`+oneNovel+`],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?include_ranking_novels=false&include_privacy_policy=false&offset=15&already_recommended=28977240,29004295&max_bookmark_id_for_recommend=38756871711"}`,
		`{"novels":[],"next_url":null}`,
	)
	page, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
	if err != nil || page.Next.IsZero() {
		t.Fatalf("page = %#v err=%v", page, err)
	}
	if _, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{Cursor: page.Next}); err != nil {
		t.Fatalf("continuation: %v", err)
	}
	query := recorder.request(t, 1).URL.Query()
	for key, want := range map[string]string{
		"offset": "15", "already_recommended": "28977240,29004295",
		"max_bookmark_id_for_recommend": "38756871711",
		"include_ranking_novels":        "false", "include_privacy_policy": "false",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
