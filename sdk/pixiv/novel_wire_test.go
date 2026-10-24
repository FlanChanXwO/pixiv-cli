package pixiv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const oneNovel = `{"id":11,"title":"story","caption":"body","x_restrict":0,"text_length":42,"is_original":true,"create_date":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"writer"},"tags":[{"name":"fantasy","translated_name":"幻想"}],"image_urls":{"original":"https://i.pximg.net/novel.jpg"}}`

func TestNovelListFamilyRoutes(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		invoke  func(*Client) ([]int64, bool, error)
		path    string
		query   map[string]string
		absent  []string
		cursor  bool
		itemIDs []int64
	}{
		{
			name: "search",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=20"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"}))
			},
			path:    "/v1/search/novel",
			query:   map[string]string{"word": "story", "search_target": "partial_match_for_tags", "sort": "date_desc"},
			absent:  []string{"offset", "duration"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "ranking default",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=30"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.NovelRanking(context.Background(), NovelRankingRequest{}))
			},
			path:    "/v1/novel/ranking",
			query:   map[string]string{"filter": "for_android", "mode": "day"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "ranking explicit mode",
			body: `{"novels":[` + oneNovel + `]}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.NovelRanking(context.Background(), NovelRankingRequest{Mode: RankingModeWeek}))
			},
			path:    "/v1/novel/ranking",
			query:   map[string]string{"filter": "for_android", "mode": "week"},
			itemIDs: []int64{11},
		},
		{
			name: "recommended",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.RecommendedNovels(context.Background(), RecommendedNovelsRequest{}))
			},
			path:    "/v1/novel/recommended",
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "following",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/follow?restrict=public&offset=30"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.FollowingNovels(context.Background(), FollowingNovelsRequest{}))
			},
			path:    "/v1/novel/follow",
			query:   map[string]string{"restrict": "public"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "latest",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/new?filter=for_android&max_novel_id=987654"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.LatestNovels(context.Background(), LatestNovelsRequest{}))
			},
			path:    "/v1/novel/new",
			query:   map[string]string{"filter": "for_android"},
			absent:  []string{"max_novel_id", "offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "mypixiv",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/mypixiv?offset=40"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{}))
			},
			path:    "/v1/novel/mypixiv",
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "user novels",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/user/novels?user_id=77&filter=for_android&offset=50"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.UserNovels(context.Background(), UserNovelsRequest{UserID: 77}))
			},
			path:    "/v1/user/novels",
			query:   map[string]string{"user_id": "77", "filter": "for_android"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{11},
		},
		{
			name: "user novel bookmarks",
			body: `{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?user_id=77&restrict=public&tag=cat&max_bookmark_id=88"}`,
			invoke: func(c *Client) ([]int64, bool, error) {
				return novelPageIDs(c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 77, Tag: "cat"}))
			},
			path:    "/v1/user/bookmarks/novel",
			query:   map[string]string{"user_id": "77", "tag": "cat"},
			absent:  []string{"max_bookmark_id"},
			cursor:  true,
			itemIDs: []int64{11},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			ids, hasNext, err := test.invoke(client)
			if err != nil {
				t.Fatalf("invoke: %v", err)
			}
			if len(ids) != len(test.itemIDs) {
				t.Fatalf("item IDs = %v, want %v", ids, test.itemIDs)
			}
			for index, want := range test.itemIDs {
				if ids[index] != want {
					t.Fatalf("item IDs = %v, want %v", ids, test.itemIDs)
				}
			}
			if hasNext != test.cursor {
				t.Fatalf("continuation = %t, want %t", hasNext, test.cursor)
			}
			request := recorder.request(t, 0)
			if request.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", request.URL.Path, test.path)
			}
			for key, want := range test.query {
				if got := request.URL.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q; query=%v", key, got, want, request.URL.Query())
				}
			}
			for _, key := range test.absent {
				if _, present := request.URL.Query()[key]; present {
					t.Errorf("initial query unexpectedly contains %s: %v", key, request.URL.Query())
				}
			}
		})
	}
}

func novelPageIDs(page sdk.Page[Novel], err error) ([]int64, bool, error) {
	ids := make([]int64, len(page.Items))
	for index, item := range page.Items {
		ids[index] = item.ID
	}
	return ids, !page.Next.IsZero(), err
}

func TestNovelListFamilyContinuation(t *testing.T) {
	tests := []struct {
		name     string
		bodies   []string
		invoke   func(*Client, sdk.Cursor) error
		mutate   func(*Client) (sdk.Cursor, error)
		secondQR map[string]string
	}{
		{
			name: "search offset",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=20"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story", Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"offset": "20", "word": "story"},
		},
		{
			name: "ranking offset",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=30"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.NovelRanking(context.Background(), NovelRankingRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"offset": "30", "mode": "day"},
		},
		{
			name: "latest max novel id",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/new?filter=for_android&max_novel_id=987654"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"max_novel_id": "987654", "filter": "for_android"},
		},
		{
			name: "following offset",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/follow?restrict=public&offset=30"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.FollowingNovels(context.Background(), FollowingNovelsRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.FollowingNovels(context.Background(), FollowingNovelsRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"offset": "30", "restrict": "public"},
		},
		{
			name: "mypixiv offset",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/novel/mypixiv?offset=40"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"offset": "40"},
		},
		{
			name: "bookmarks max bookmark id",
			bodies: []string{
				`{"novels":[` + oneNovel + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?user_id=77&restrict=public&tag=cat&max_bookmark_id=88"}`,
				`{"novels":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 77, Tag: "cat"})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 77, Tag: "cat", Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"user_id": "77", "max_bookmark_id": "88", "tag": "cat"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.bodies...)
			cursor, err := test.mutate(client)
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if cursor.IsZero() {
				t.Fatal("expected a continuation cursor")
			}
			if err := test.invoke(client, cursor); err != nil {
				t.Fatalf("continuation: %v", err)
			}
			query := recorder.request(t, 1).URL.Query()
			for key, want := range test.secondQR {
				if got := query.Get(key); got != want {
					t.Errorf("%s = %q, want %q; query=%v", key, got, want, query)
				}
			}
		})
	}
}

func TestRecommendedNovelContinuationReplaysLiveParams(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"novels":[`+oneNovel+`],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?include_ranking_novels=false&include_privacy_policy=false&offset=15&already_recommended=28977240,29004295&max_bookmark_id_for_recommend=38756871711"}`,
		`{"novels":[],"next_url":null}`,
	)
	page, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
	if err != nil {
		t.Fatalf("RecommendedNovels: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("multi-param next_url must produce a continuation")
	}
	if _, present := recorder.request(t, 0).URL.Query()["offset"]; present {
		t.Fatalf("initial request must not include offset: %v", recorder.request(t, 0).URL.Query())
	}
	if _, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{Cursor: page.Next}); err != nil {
		t.Fatalf("continuation: %v", err)
	}
	query := recorder.request(t, 1).URL.Query()
	for key, want := range map[string]string{
		"offset":                        "15",
		"already_recommended":           "28977240,29004295",
		"max_bookmark_id_for_recommend": "38756871711",
		"include_ranking_novels":        "false",
		"include_privacy_policy":        "false",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestNovelSeriesResponses(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"novel_series_detail":{"id":21,"title":"series","caption":"description","is_concluded":true,"user":{"id":7}},"novels":[`+oneNovel+`],"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=21&last_order=9"}`,
		`{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":null}`,
	)
	result, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
	if err != nil {
		t.Fatalf("NovelSeries: %v", err)
	}
	request := recorder.request(t, 0)
	if request.URL.Path != "/v2/novel/series" || request.URL.Query().Get("series_id") != "21" {
		t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
	}
	if _, present := request.URL.Query()["last_order"]; present {
		t.Fatalf("initial request must not include last_order: %v", request.URL.Query())
	}
	if result.Series.ID != 21 || result.Series.Caption != "description" || !result.Series.IsConcluded ||
		len(result.Novels.Items) != 1 || result.Novels.Items[0].ID != 11 || result.Novels.Next.IsZero() {
		t.Fatalf("result = %#v", result)
	}
	second, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21, Cursor: result.Novels.Next})
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if got := recorder.request(t, 1).URL.Query().Get("last_order"); got != "9" {
		t.Fatalf("last_order = %q, want 9", got)
	}
	if second.Novels.Items == nil || len(second.Novels.Items) != 0 || !second.Novels.Next.IsZero() {
		t.Fatalf("continuation result = %#v", second)
	}
}

func TestNovelDetailResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"novel":null}`, `{"novel":{"id":12,"user":{"id":0}}}`, `{"novel":{"id":12,"user":{"id":7}},"series_next":{"id":0}}`} {
		client, _ := newWireClient(t, body)
		if _, err := client.Novel(context.Background(), NovelRequest{NovelID: 12}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
			t.Fatalf("body %s error = %v, want malformed response", body, err)
		}
	}

	client, recorder := newWireClient(t, `{"novel":`+oneNovel+`}`)
	novel, err := client.Novel(context.Background(), NovelRequest{NovelID: 11})
	if err != nil {
		t.Fatalf("Novel: %v", err)
	}
	request := recorder.request(t, 0)
	if request.URL.Path != "/v2/novel/detail" || request.URL.Query().Get("novel_id") != "11" {
		t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
	}
	if novel.ID != 11 || novel.Title != "story" || novel.Caption != "body" || novel.User.ID != 7 ||
		novel.TextLength != 42 || !novel.IsOriginal || novel.XRestrict != 0 || novel.TotalViews != 0 {
		t.Fatalf("novel = %#v", novel)
	}
	if novel.Tags == nil || len(novel.Tags) != 1 || novel.Tags[0] != (Tag{Name: "fantasy", TranslatedName: "幻想"}) {
		t.Fatalf("tags = %#v", novel.Tags)
	}
	want := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	if !novel.PublishedAt.Equal(want) {
		t.Fatalf("published = %s, want %s", novel.PublishedAt, want)
	}
	if novel.Cover.Resource.Ref.IsZero() {
		t.Fatal("novel cover resource is missing")
	}
}

func TestNovelCommentsResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		check   func(*testing.T, CommentPage)
		wantErr bool
	}{
		{
			name: "parent chain with object access control",
			body: `{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7},"parent_comment":{"id":6,"caption":"parent","created_at":"2024-01-01T03:04:05+00:00","user":{"id":5}}}],"total_comments":2,"access_control":{"can_comment":false,"is_locked":true},"next_url":"https://app-api.pixiv.net/v2/novel/comments?novel_id=123&offset=20"}`,
			check: func(t *testing.T, page CommentPage) {
				if len(page.Page.Items) != 1 || page.Page.Items[0].Comment != "child" ||
					page.Page.Items[0].ParentComment == nil || page.Page.Items[0].ParentComment.Comment != "parent" ||
					page.Total == nil || *page.Total != 2 ||
					page.AccessControl == nil || page.AccessControl.CanComment || !page.AccessControl.IsLocked ||
					page.Page.Next.IsZero() {
					t.Fatalf("page = %#v", page)
				}
			},
		},
		{
			name: "current date field",
			body: `{"comments":[{"id":9,"comment":"current wire","date":"2026-01-02T03:04:05+00:00","user":{"id":7}}]}`,
			check: func(t *testing.T, page CommentPage) {
				want := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
				if len(page.Page.Items) != 1 || !page.Page.Items[0].CreatedAt.Equal(want) {
					t.Fatalf("page = %#v", page)
				}
			},
		},
		{
			name: "numeric access control",
			body: `{"comments":[{"id":9,"comment":"x","date":"2026-01-02T03:04:05+00:00","user":{"id":7}}],"comment_access_control":1}`,
			check: func(t *testing.T, page CommentPage) {
				if page.AccessControl == nil || page.AccessControl.NumericValue == nil || *page.AccessControl.NumericValue != 1 {
					t.Fatalf("numeric access control = %#v", page.AccessControl)
				}
				if page.AccessControl.CanComment || page.AccessControl.IsLocked {
					t.Fatalf("scalar access control was assigned object semantics: %#v", page.AccessControl)
				}
			},
		},
		{name: "invalid parent id", body: `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`, wantErr: true},
		{name: "missing comments", body: `{}`, wantErr: true},
		{name: "null comments", body: `{"comments":null}`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			page, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 123})
			if test.wantErr {
				if err == nil {
					t.Fatal("malformed comments response unexpectedly succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("NovelComments: %v", err)
			}
			request := recorder.request(t, 0)
			if request.URL.Path != "/v2/novel/comments" || request.URL.Query().Get("novel_id") != "123" {
				t.Fatalf("request = %q %v", request.URL.Path, request.URL.Query())
			}
			test.check(t, page)
		})
	}

	client, _ := newWireClient(t, `{"comments":[]}`)
	page, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 1})
	if err != nil || page.Page.Items == nil || len(page.Page.Items) != 0 || !page.Page.Next.IsZero() {
		t.Fatalf("empty page = %#v err=%v, want non-nil empty terminal page", page, err)
	}
}

func TestNovelCommentsContinuation(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7}}],"next_url":"https://app-api.pixiv.net/v2/novel/comments?novel_id=123&offset=20"}`,
		`{"comments":[],"next_url":null}`,
	)
	first, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 123})
	if err != nil {
		t.Fatalf("NovelComments: %v", err)
	}
	if _, present := recorder.request(t, 0).URL.Query()["offset"]; present {
		t.Fatalf("initial request must not include offset: %v", recorder.request(t, 0).URL.Query())
	}
	if _, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 123, Cursor: first.Page.Next}); err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if got := recorder.request(t, 1).URL.Query().Get("offset"); got != "20" {
		t.Fatalf("offset = %q, want 20", got)
	}
}

func TestNovelCommentMutations(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
		path   string
		form   map[string]string
		body   string
	}{
		{
			name: "post",
			invoke: func(c *Client) error {
				_, err := c.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 123, Comment: "hello"})
				return err
			},
			path: "/v1/novel/comment/add",
			form: map[string]string{"novel_id": "123", "comment": "hello"},
			body: `{"comment_id":73}`,
		},
		{
			name: "reply",
			invoke: func(c *Client) error {
				_, err := c.ReplyNovelComment(context.Background(), ReplyNovelCommentRequest{NovelID: 123, Comment: "reply", ParentCommentID: 6})
				return err
			},
			path: "/v1/novel/comment/add",
			form: map[string]string{"novel_id": "123", "comment": "reply", "parent_comment_id": "6"},
			body: `{"comment_id":74}`,
		},
		{
			name: "stamp",
			invoke: func(c *Client) error {
				_, err := c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 123, Comment: "stamp", StampID: 9})
				return err
			},
			path: "/v1/novel/comment/add",
			form: map[string]string{"novel_id": "123", "comment": "stamp", "stamp_id": "9"},
			body: `{"comment_id":75}`,
		},
		{
			name: "stamp without comment",
			invoke: func(c *Client) error {
				_, err := c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 123, StampID: 9})
				return err
			},
			path: "/v1/novel/comment/add",
			form: map[string]string{"novel_id": "123", "comment": "", "stamp_id": "9"},
			body: `{"comment":{"id":75}}`,
		},
		{
			name: "delete",
			invoke: func(c *Client) error {
				return c.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{CommentID: 76})
			},
			path: "/v1/novel/comment/delete",
			form: map[string]string{"comment_id": "76"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			request := recorder.request(t, 0)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if request.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", request.URL.Path, test.path)
			}
			if len(request.PostForm) != len(test.form) {
				t.Fatalf("form = %#v, want %d fields", request.PostForm, len(test.form))
			}
			for key, want := range test.form {
				if got := request.PostForm.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestNovelCommentMutationResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"comment_id":null}`, `{"comment_id":0}`, `{"comment_id":-1}`} {
		t.Run(body, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			_, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1, Comment: "body"})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
	client, _ := newWireClient(t, `{"comment":{"id":77}}`)
	result, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1, Comment: "body"})
	if err != nil || result.CommentID != 77 {
		t.Fatalf("result = %#v err=%v", result, err)
	}

	client, recorder := newWireClient(t)
	recorder.err = errors.New("transport canary")
	if _, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
		t.Fatalf("create error = %v, want UpstreamUnavailable", err)
	}
	if err := client.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{CommentID: 1}); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
		t.Fatalf("delete error = %v, want UpstreamUnavailable", err)
	}
}

func TestNovelCommentMutationsRejectInvalidInputBeforeNetwork(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "post non-positive novel", invoke: func(c *Client) error {
			_, err := c.PostNovelComment(context.Background(), PostNovelCommentRequest{Comment: "body"})
			return err
		}},
		{name: "post empty body", invoke: func(c *Client) error {
			_, err := c.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1})
			return err
		}},
		{name: "reply non-positive parent", invoke: func(c *Client) error {
			_, err := c.ReplyNovelComment(context.Background(), ReplyNovelCommentRequest{NovelID: 1, Comment: "body"})
			return err
		}},
		{name: "stamp non-positive stamp", invoke: func(c *Client) error {
			_, err := c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 1, Comment: "body"})
			return err
		}},
		{name: "delete non-positive comment", invoke: func(c *Client) error {
			return c.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{})
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

func TestNovelListRejectsMalformedEnvelopes(t *testing.T) {
	tests := map[string]string{
		"missing novels":   `{}`,
		"null novels":      `{"novels":null}`,
		"zero id":          `{"novels":[{"id":0,"user":{"id":1}}]}`,
		"zero owner":       `{"novels":[{"id":1,"user":{"id":0}}]}`,
		"search missing x": `{"novels":[{"id":1,"user":{"id":1}}]}`,
	}
	invokes := map[string]func(*Client) error{
		"missing novels": func(c *Client) error {
			_, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
			return err
		},
		"null novels": func(c *Client) error {
			_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
			return err
		},
		"zero id": func(c *Client) error {
			_, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
			return err
		},
		"zero owner": func(c *Client) error {
			_, err := c.UserNovels(context.Background(), UserNovelsRequest{UserID: 7})
			return err
		},
		"search missing x": func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
			return err
		},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if err := invokes[name](client); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}

func TestNovelContinuationAllowlists(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		invoke func(*Client) error
	}{
		{
			name: "search unknown key",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=30&start_date=2024-01-01"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
				return err
			},
		},
		{
			name: "search wrong path",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
				return err
			},
		},
		{
			name: "ranking non-positive offset",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=0"}`,
			invoke: func(c *Client) error {
				_, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
				return err
			},
		},
		{
			name: "latest offset instead of max novel id",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
				return err
			},
		},
		{
			name: "latest mixed keys",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?max_novel_id=987654&offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
				return err
			},
		},
		{
			name: "mypixiv carries following query",
			body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/follow?offset=20&restrict=public"}`,
			invoke: func(c *Client) error {
				_, err := c.MyPixivNovels(context.Background(), MyPixivNovelsRequest{})
				return err
			},
		},
		{
			name: "series non-positive last order",
			body: `{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=21&last_order=0"}`,
			invoke: func(c *Client) error {
				_, err := c.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
		{name: "search unsupported sort", invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "x", Sort: SortMode("unknown")})
			return err
		}},
		{name: "search unsupported duration", invoke: func(c *Client) error {
			_, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "x", Duration: DurationFilter("unknown")})
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
		{
			name: "search", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.SearchNovels(context.Background(), SearchNovelsRequest{Word: "empty"})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "ranking", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "recommended", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "following", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.FollowingNovels(context.Background(), FollowingNovelsRequest{})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "latest", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.LatestNovels(context.Background(), LatestNovelsRequest{})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "user novels", body: `{"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.UserNovels(context.Background(), UserNovelsRequest{UserID: 77})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "series", body: `{"novel_series_detail":{"id":21,"user":{"id":7}},"novels":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 21})
				return len(page.Novels.Items), page.Novels.Items == nil, errOrNext(page.Novels.Next, err)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			count, nilItems, next := test.invoke(client)
			if next != nil {
				t.Fatalf("unexpected error: %v", next)
			}
			if nilItems || count != 0 {
				t.Fatalf("empty page = %d items, nil=%t; want non-nil empty list", count, nilItems)
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
		{name: "ranking", invoke: func(c *Client) error {
			_, err := c.NovelRanking(context.Background(), NovelRankingRequest{})
			return err
		}},
		{name: "recommended", invoke: func(c *Client) error {
			_, err := c.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
			return err
		}},
		{name: "comments", invoke: func(c *Client) error {
			_, err := c.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 1})
			return err
		}},
		{name: "bookmarks", invoke: func(c *Client) error {
			_, err := c.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7})
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
func TestNovelSeriesRejectsMalformedEnvelopes(t *testing.T) {
	for name, body := range map[string]string{
		"missing detail":   `{"novels":[]}`,
		"null detail":      `{"novel_series_detail":null,"novels":[]}`,
		"zero detail user": `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":null}`,
		"null novels":      `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":null}`,
		"zero novel id":    `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":[{"id":0,"user":{"id":1}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			result, err := client.NovelSeries(context.Background(), NovelSeriesRequest{SeriesID: 1})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
			if result.Novels.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", result.Novels.Items)
			}
		})
	}
}

func TestNovelListFamilyMapsFields(t *testing.T) {
	client, _ := newWireClient(t, `{"novels":[`+oneNovel+`],"next_url":null}`)
	page, err := client.SearchNovels(context.Background(), SearchNovelsRequest{Word: "story"})
	if err != nil {
		t.Fatalf("SearchNovels: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %#v", page.Items)
	}
	novel := page.Items[0]
	if novel.TextLength != 42 || !novel.IsOriginal || novel.XRestrict != 0 ||
		novel.User.ID != 7 || novel.User.Name != "writer" ||
		novel.Tags == nil || len(novel.Tags) != 1 || novel.Tags[0].TranslatedName != "幻想" {
		t.Fatalf("novel = %#v", novel)
	}
	if novel.Cover.Resource.Ref.IsZero() || novel.Cover.Variant != "original" {
		t.Fatalf("cover = %#v", novel.Cover)
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

func TestRecommendedNovelsRejectsMalformedContinuationParams(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"novels":[`+oneNovel+`],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`,
	)
	first, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	cursor := cursorWithPayload(t, first.Next, `{"p":{}}`)
	if _, err := client.RecommendedNovels(context.Background(), RecommendedNovelsRequest{Cursor: cursor}); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("ReasonOf = %q, want InvalidCursor (err=%v)", sdk.ReasonOf(err), err)
	}
	if recorder.calls() != 1 {
		t.Fatalf("invalid cursor reached upstream %d time(s)", recorder.calls()-1)
	}
}
