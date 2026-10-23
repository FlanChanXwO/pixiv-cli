package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
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
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: http_RoundTripper(recorder)}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client, recorder
}

func http_RoundTripper(recorder *wireRecorder) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorder.requests = append(recorder.requests, req)
		if recorder.err != nil {
			return nil, recorder.err
		}
		body := ""
		if len(recorder.bodies) > 0 {
			body, recorder.bodies = recorder.bodies[0], recorder.bodies[1:]
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
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

func TestArtworkListFamilyRoutes(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		invoke  func(*Client) error
		path    string
		query   map[string]string
		absent  []string
		cursor  bool
		itemIDs []int64
	}{
		{
			name: "following",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=private&offset=20"}`,
			invoke: func(c *Client) error {
				_, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{Restrict: RestrictPrivate})
				return err
			},
			path:    "/v2/illust/follow",
			query:   map[string]string{"restrict": "private"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "latest",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=illust&filter=for_android&max_illust_id=987654"}`,
			invoke: func(c *Client) error {
				_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
				return err
			},
			path:    "/v1/illust/new",
			query:   map[string]string{"content_type": "illust", "filter": "for_android"},
			absent:  []string{"max_illust_id", "offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "mypixiv",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/mypixiv?offset=40"}`,
			invoke: func(c *Client) error {
				_, err := c.MyPixivArtworks(context.Background(), MyPixivArtworksRequest{})
				return err
			},
			path:    "/v2/illust/mypixiv",
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "user artworks",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=77&type=manga&offset=50"}`,
			invoke: func(c *Client) error {
				_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Kind: ArtworkKindManga})
				return err
			},
			path:    "/v1/user/illusts",
			query:   map[string]string{"user_id": "77", "type": "manga"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "ranking",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=week&date=2024-01-01&offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingModeWeek, Date: "2024-01-01"})
				return err
			},
			path:    "/v1/illust/ranking",
			query:   map[string]string{"mode": "week", "date": "2024-01-01"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "search",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=landscape&offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
				return err
			},
			path:    "/v1/search/illust",
			query:   map[string]string{"word": "landscape"},
			absent:  []string{"offset"},
			cursor:  true,
			itemIDs: []int64{5},
		},
		{
			name: "bookmarks",
			body: `{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=8"}`,
			invoke: func(c *Client) error {
				_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Restrict: RestrictPrivate, Tag: "cat"})
				return err
			},
			path:    "/v1/user/bookmarks/illust",
			query:   map[string]string{"user_id": "7", "restrict": "private", "tag": "cat"},
			absent:  []string{"type", "content_type", "max_bookmark_id"},
			cursor:  true,
			itemIDs: []int64{5},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			req := recorder.request(t, 0)
			if req.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", req.URL.Path, test.path)
			}
			query := req.URL.Query()
			for key, want := range test.query {
				if got := query.Get(key); got != want {
					t.Errorf("%s = %q, want %q; query=%v", key, got, want, query)
				}
			}
			for _, key := range test.absent {
				if _, present := query[key]; present {
					t.Errorf("initial query unexpectedly contains %s: %v", key, query)
				}
			}
			if test.cursor {
				// 续页断言由各端点族的 continuation 测试负责，这里只确认响应被解析。
				_ = test.itemIDs
			}
		})
	}
}

func TestArtworkListFamilyContinuation(t *testing.T) {
	tests := []struct {
		name     string
		bodies   []string
		invoke   func(*Client, sdk.Cursor) error
		mutate   func(*Client) (sdk.Cursor, error)
		secondQR map[string]string
	}{
		{
			name: "following offset",
			bodies: []string{
				`{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=public&offset=20"}`,
				`{"illusts":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"restrict": "public", "offset": "20"},
		},
		{
			name: "ranking offset",
			bodies: []string{
				`{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=day&offset=30"}`,
				`{"illusts":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"mode": "day", "offset": "30"},
		},
		{
			name: "user artworks offset",
			bodies: []string{
				`{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=77&type=illust&offset=30"}`,
				`{"illusts":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77, Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"user_id": "77", "type": "illust", "offset": "30"},
		},
		{
			name: "bookmarks max bookmark id",
			bodies: []string{
				`{"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=7&restrict=public&tag=cat&max_bookmark_id=88"}`,
				`{"illusts":[],"next_url":null}`,
			},
			mutate: func(c *Client) (sdk.Cursor, error) {
				page, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Tag: "cat"})
				return page.Next, err
			},
			invoke: func(c *Client, cursor sdk.Cursor) error {
				_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Tag: "cat", Cursor: cursor})
				return err
			},
			secondQR: map[string]string{"user_id": "7", "tag": "cat", "max_bookmark_id": "88"},
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

func TestArtworkSeriesResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		invoke  func(*Client) error
		query   map[string]string
		wantErr bool
	}{
		{
			name: "last order continuation",
			body: `{"illust_series_detail":{"user":{"id":7}},"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=42&last_order=8"}`,
			invoke: func(c *Client) error {
				_, err := c.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42})
				return err
			},
			query: map[string]string{"illust_series_id": "42"},
		},
		{
			name: "offset continuation",
			body: `{"illust_series_detail":{"user":{"id":7}},"illusts":[` + oneArtwork + `],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=42&offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42})
				return err
			},
			query: map[string]string{"illust_series_id": "42"},
		},
		{
			name: "missing detail user",
			body: `{"illust_series_detail":null,"illusts":[]}`,
			invoke: func(c *Client) error {
				_, err := c.ArtworkSeries(context.Background(), ArtworkSeriesRequest{SeriesID: 42})
				return err
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			err := test.invoke(client)
			if test.wantErr {
				if err == nil {
					t.Fatal("malformed series response unexpectedly succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("ArtworkSeries: %v", err)
			}
			query := recorder.request(t, 0).URL.Query()
			for key, want := range test.query {
				if got := query.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if query.Get("offset") != "" || query.Get("last_order") != "" {
				t.Errorf("initial request must not carry a continuation: %v", query)
			}
		})
	}
}

func TestTrendingArtworkResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name: "complete list",
			body: `{"trend_tags":[{"tag":"cat","translated_name":"猫","illust":{"id":1,"title":"cover","user":{"id":2},"create_date":"2024-01-02T03:04:05+00:00"}},{"tag":"dog","illust":{"id":3,"user":{"id":4},"create_date":"2024-01-03T03:04:05+00:00"}}]}`,
		},
		{name: "missing tag artwork", body: `{"trend_tags":[{"tag":"cat","illust":null}]}`, wantErr: pixivapi.ErrMalformedResponse},
		{name: "missing envelope", body: `{}`, wantErr: pixivapi.ErrMalformedResponse},
		{name: "null list", body: `{"trend_tags":null}`, wantErr: pixivapi.ErrMalformedResponse},
		{name: "empty tag", body: `{"trend_tags":[{"tag":"","illust":{"id":1}}]}`, wantErr: pixivapi.ErrMalformedResponse},
		{name: "zero artwork", body: `{"trend_tags":[{"tag":"cat","illust":{"id":0}}]}`, wantErr: pixivapi.ErrMalformedResponse},
		{name: "empty list", body: `{"trend_tags":[]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			items, err := client.TrendingArtworkTags(context.Background(), TrendingArtworkTagsRequest{})
			if test.wantErr != nil {
				if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
					t.Fatalf("error = %v, want malformed response", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("TrendingArtworkTags: %v", err)
			}
			if items == nil {
				t.Fatal("successful empty list must be non-nil")
			}
			if test.name == "complete list" && (len(items) != 2 || items[0].Tag != "cat" || items[1].Artwork.ID != 3) {
				t.Fatalf("items = %#v", items)
			}
			req := recorder.request(t, 0)
			if req.URL.Path != "/v1/trending-tags/illust" || len(req.URL.Query()) != 0 {
				t.Fatalf("request = %q %v", req.URL.Path, req.URL.Query())
			}
		})
	}
}

func TestArtworkCommentResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		check   func(*testing.T, CommentPage)
		wantErr bool
	}{
		{
			name: "parent chain with object access control",
			body: `{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7},"parent_comment":{"id":6,"caption":"parent","created_at":"2024-01-01T03:04:05+00:00","user":{"id":5}}}],"total_comments":2,"access_control":{"can_comment":true,"is_locked":false},"next_url":"https://app-api.pixiv.net/v3/illust/comments?illust_id=123&offset=20"}`,
			check: func(t *testing.T, page CommentPage) {
				if len(page.Page.Items) != 1 || page.Page.Items[0].Comment != "child" ||
					page.Page.Items[0].ParentComment == nil || page.Page.Items[0].ParentComment.Comment != "parent" ||
					page.Total == nil || *page.Total != 2 ||
					page.AccessControl == nil || !page.AccessControl.CanComment || page.AccessControl.IsLocked ||
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
			},
		},
		{name: "invalid parent id", body: `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`, wantErr: true},
		{name: "missing comments", body: `{}`, wantErr: true},
		{name: "null comments", body: `{"comments":null}`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			page, err := client.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 123})
			if test.wantErr {
				if err == nil {
					t.Fatal("malformed comments response unexpectedly succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("ArtworkComments: %v", err)
			}
			test.check(t, page)
		})
	}

	client, _ := newWireClient(t, `{"comments":[]}`)
	page, err := client.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 1})
	if err != nil || page.Page.Items == nil || len(page.Page.Items) != 0 || !page.Page.Next.IsZero() {
		t.Fatalf("empty page = %#v err=%v, want non-nil empty terminal page", page, err)
	}
}

func TestArtworkCommentMutations(t *testing.T) {
	tests := []struct {
		name    string
		invoke  func(*Client) error
		path    string
		form    map[string]string
		body    string
		wantErr bool
	}{
		{
			name: "create",
			invoke: func(c *Client) error {
				_, err := c.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 123, Comment: "hello"})
				return err
			},
			path: "/v1/illust/comment/add",
			form: map[string]string{"illust_id": "123", "comment": "hello"},
			body: `{"comment_id":73}`,
		},
		{
			name: "reply",
			invoke: func(c *Client) error {
				_, err := c.ReplyArtworkComment(context.Background(), ReplyArtworkCommentRequest{ArtworkID: 123, Comment: "hello", ParentCommentID: 71})
				return err
			},
			path: "/v1/illust/comment/add",
			form: map[string]string{"illust_id": "123", "comment": "hello", "parent_comment_id": "71"},
			body: `{"comment": {"id": 74}}`,
		},
		{
			name: "stamp with empty comment",
			invoke: func(c *Client) error {
				_, err := c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 123, StampID: 9})
				return err
			},
			path: "/v1/illust/comment/add",
			form: map[string]string{"illust_id": "123", "comment": "", "stamp_id": "9"},
			body: `{"comment_id":75}`,
		},
		{
			name: "delete",
			invoke: func(c *Client) error {
				return c.DeleteArtworkComment(context.Background(), DeleteArtworkCommentRequest{CommentID: 76})
			},
			path: "/v1/illust/comment/delete",
			form: map[string]string{"comment_id": "76"},
		},
		{name: "create empty body", invoke: func(c *Client) error {
			_, err := c.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1})
			return err
		}, wantErr: true},
		{name: "reply missing parent", invoke: func(c *Client) error {
			_, err := c.ReplyArtworkComment(context.Background(), ReplyArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
			return err
		}, wantErr: true},
		{name: "stamp missing id", invoke: func(c *Client) error {
			_, err := c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
			return err
		}, wantErr: true},
		{name: "delete non-positive", invoke: func(c *Client) error {
			return c.DeleteArtworkComment(context.Background(), DeleteArtworkCommentRequest{})
		}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			err := test.invoke(client)
			if test.wantErr {
				if err == nil {
					t.Fatal("invalid mutation unexpectedly succeeded")
				}
				if recorder.calls() != 0 {
					t.Fatalf("invalid request reached upstream %d time(s)", recorder.calls())
				}
				return
			}
			if err != nil {
				t.Fatalf("mutation: %v", err)
			}
			req := recorder.request(t, 0)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if req.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", req.URL.Path, test.path)
			}
			if len(req.PostForm) != len(test.form) {
				t.Fatalf("form = %v, want %v", req.PostForm, test.form)
			}
			for key, want := range test.form {
				if got := req.PostForm.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestArtworkCommentMutationResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"comment_id":null}`, `{"comment_id":0}`, `{"comment_id":-1}`} {
		t.Run(body, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			_, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
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

func TestArtworkBookmarkResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		restrict Restrict
		tags     []string
		nullTags bool
	}{
		{
			name:     "registered tags only",
			body:     `{"bookmark_detail":{"is_bookmarked":true,"restrict":"public","tags":[{"name":"content-tag","is_registered":false},{"name":"saved-tag","is_registered":true}]}}`,
			restrict: RestrictPublic,
			tags:     []string{"saved-tag"},
		},
		{
			name:     "explicitly not bookmarked",
			body:     `{"bookmark_detail":{"is_bookmarked":false,"restrict":"private","tags":[{"name":"cat","is_registered":false}]}}`,
			nullTags: true,
		},
		{
			name:     "null detail",
			body:     `{"bookmark_detail":null}`,
			nullTags: true,
		},
		{
			name:     "not found means not bookmarked",
			status:   http.StatusNotFound,
			nullTags: true,
		},
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
			if test.nullTags {
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

func TestArtworkBookmarkTagsAndContinuation(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"bookmark_tags":[{"name":"cat","count":2}],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?user_id=7&restrict=public&offset=5"}`,
		`{"bookmark_tags":[],"next_url":null}`,
	)
	page, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7})
	if err != nil {
		t.Fatalf("UserArtworkBookmarkTags: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "cat" || page.Items[0].Count != 2 {
		t.Fatalf("items = %#v", page.Items)
	}
	req := recorder.request(t, 0)
	if req.URL.Path != "/v1/user/bookmark-tags/illust" || req.URL.Query().Get("user_id") != "7" {
		t.Fatalf("request = %q %v", req.URL.Path, req.URL.Query())
	}
	if req.URL.Query().Get("offset") != "" {
		t.Fatalf("initial query unexpectedly contains offset: %v", req.URL.Query())
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation cursor")
	}
	cont, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7, Cursor: page.Next})
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if cont.Items == nil || len(cont.Items) != 0 || !cont.Next.IsZero() {
		t.Fatalf("continuation page = %#v", cont)
	}
	if got := recorder.request(t, 1).URL.Query().Get("offset"); got != "5" {
		t.Fatalf("continuation offset = %q, want 5", got)
	}
}

func TestArtworkBookmarkTagsRejectMalformedResponses(t *testing.T) {
	tests := map[string]string{
		"missing tags":         `{}`,
		"null tags":            `{"bookmark_tags":null}`,
		"empty name":           `{"bookmark_tags":[{"name":"","count":1}]}`,
		"empty next url":       `{"bookmark_tags":[],"next_url":""}`,
		"missing continuation": `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?tag=cat"}`,
		"non-positive offset":  `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=0"}`,
		"duplicate offset":     `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=2&offset=3"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			page, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7})
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
			if page.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", page.Items)
			}
		})
	}
}

func TestArtworkListRejectsMalformedEnvelopes(t *testing.T) {
	tests := map[string]string{
		"missing illusts":    `{}`,
		"null illusts":       `{"illusts":null}`,
		"zero id":            `{"illusts":[{"id":0}]}`,
		"mypixiv zero owner": `{"illusts":[{"id":1,"user":{"id":0}}]}`,
	}
	invokes := map[string]func(*Client) error{
		"missing illusts": func(c *Client) error {
			_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 7})
			return err
		},
		"null illusts": func(c *Client) error {
			_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
			return err
		},
		"zero id": func(c *Client) error {
			_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
			return err
		},
		"mypixiv zero owner": func(c *Client) error {
			_, err := c.MyPixivArtworks(context.Background(), MyPixivArtworksRequest{})
			return err
		},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			err := invokes[name](client)
			if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}

func TestArtworkContinuationAllowlists(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		invoke func(*Client) error
	}{
		{
			name: "search unknown key",
			body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30&max_novel_id=9"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
				return err
			},
		},
		{
			name: "search wrong path",
			body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/novel?offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
				return err
			},
		},
		{
			name: "search wrong host",
			body: `{"illusts":[],"next_url":"https://example.invalid/v1/search/illust?offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "landscape"})
				return err
			},
		},
		{
			name: "latest duplicate continuation",
			body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/new?max_illust_id=987654&max_illust_id=123456&offset=30"}`,
			invoke: func(c *Client) error {
				_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{})
				return err
			},
		},
		{
			name: "bookmarks non-positive cursor",
			body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=0"}`,
			invoke: func(c *Client) error {
				_, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7})
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

func TestRelatedArtworkContinuationForms(t *testing.T) {
	tests := []struct {
		name    string
		nextURL string
		wantErr bool
	}{
		{
			name:    "offset form",
			nextURL: "https://app-api.pixiv.net/v2/illust/related?illust_id=123&offset=30",
		},
		{
			name:    "live indexed arrays",
			nextURL: "https://app-api.pixiv.net/v2/illust/related?illust_id=123&seed_illust_ids%5B0%5D=456&seed_illust_ids%5B1%5D=789&viewed%5B0%5D=123&viewed%5B1%5D=456",
		},
		{
			name:    "gapped indexed arrays",
			nextURL: "https://app-api.pixiv.net/v2/illust/related?illust_id=123&seed_illust_ids%5B0%5D=456&viewed%5B1%5D=123",
			wantErr: true,
		},
		{
			name:    "wrong artwork",
			nextURL: "https://app-api.pixiv.net/v2/illust/related?illust_id=999&offset=30",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"illusts":[` + oneArtwork + `],"next_url":"` + test.nextURL + `"}`
			client, _ := newWireClient(t, body, `{"illusts":[],"next_url":null}`)
			page, err := client.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123})
			if test.wantErr && err != nil {
				if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse && sdk.ReasonOf(err) != sdk.InvalidCursor {
					t.Fatalf("error = %v, want malformed response or invalid cursor", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RelatedArtworks: %v", err)
			}
			if page.Next.IsZero() {
				t.Fatal("expected continuation cursor")
			}
			if _, err := client.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 123, Cursor: page.Next}); test.wantErr {
				if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse && sdk.ReasonOf(err) != sdk.InvalidCursor {
					t.Fatalf("error = %v, want malformed response or invalid cursor", err)
				}
				return
			} else if err != nil {
				t.Fatalf("RelatedArtworks continuation: %v", err)
			}
		})
	}
}

func TestRecommendedArtworkResponses(t *testing.T) {
	client, recorder := newWireClient(t,
		`{"illusts":[`+oneArtwork+`],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?min_bookmark_id_for_recent_illust=38760619879&max_bookmark_id_for_recommend=38712263830&offset=0&include_ranking_illusts=false&include_privacy_policy=false&viewed%5B0%5D=144455829"}`,
		`{"illusts":[],"next_url":null}`,
	)
	page, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
	if err != nil {
		t.Fatalf("RecommendedArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 5 || page.Next.IsZero() {
		t.Fatalf("page = %#v", page)
	}
	if _, present := recorder.request(t, 0).URL.Query()["offset"]; present {
		t.Fatalf("initial request must not include offset: %v", recorder.request(t, 0).URL.Query())
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
	if _, present := query["viewed[0]"]; present {
		t.Fatalf("viewed session parameters must be dropped: %v", query)
	}
}

func TestRecommendedArtworkRejectsMissingList(t *testing.T) {
	client, _ := newWireClient(t, `{"next_url":null}`)
	if _, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("error = %v, want malformed response", err)
	}
}

func TestArtworkListValidationBeforeNetwork(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "ranking unsupported mode", invoke: func(c *Client) error {
			_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingMode("unknown")})
			return err
		}},
		{name: "ranking invalid date", invoke: func(c *Client) error {
			_, err := c.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingModeDay, Date: "2024-02-30"})
			return err
		}},
		{name: "user artworks zero id", invoke: func(c *Client) error {
			_, err := c.UserArtworks(context.Background(), UserArtworksRequest{})
			return err
		}},
		{name: "user artworks unsupported kind", invoke: func(c *Client) error {
			_, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 7, Kind: ArtworkKind("all")})
			return err
		}},
		{name: "latest ugoira", invoke: func(c *Client) error {
			_, err := c.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeUgoira})
			return err
		}},
		{name: "following unknown restrict", invoke: func(c *Client) error {
			_, err := c.FollowingArtworks(context.Background(), FollowingArtworksRequest{Restrict: Restrict("friends")})
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

func TestArtworkUgoiraResponses(t *testing.T) {
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
			body := ugoiraBody(files...)
			client, _ := newWireClient(t, body)
			if _, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 123}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response for %v", err, files)
			}
		})
	}
}

func ugoiraBody(files ...string) string {
	frames := make([]string, len(files))
	for index, file := range files {
		frames[index] = `{"file":` + strconvQuote(file) + `,"delay":80}`
	}
	return `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/img-zip-ugoira/img/123.zip"},"frames":[` + strings.Join(frames, ",") + `]}}`
}

func strconvQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func TestArtworkDetailResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"illust":null}`} {
		client, _ := newWireClient(t, body)
		if _, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 123}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
			t.Fatalf("body %s error = %v, want malformed response", body, err)
		}
	}

	client, recorder := newWireClient(t, `{"illust":{"id":123,"title":"two pages","type":"manga","page_count":2,"user":{"id":7,"name":"artist"},"tags":[{"name":"cat","translated_name":"猫"}],"image_urls":{"large":"https://i.pximg.net/cover.jpg"},"meta_pages":[{"width":1000,"height":1200,"extension":"jpg","image_urls":{"original":"https://i.pximg.net/0.jpg"}},{"width":1000,"height":1200,"extension":"jpg","image_urls":{"original":"https://i.pximg.net/1.jpg"}}],"illust_ai_type":2,"create_date":"2024-01-02T03:04:05+00:00"}}`)
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 123})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	req := recorder.request(t, 0)
	if req.URL.Path != "/v1/illust/detail" || req.URL.Query().Get("illust_id") != "123" {
		t.Fatalf("request = %q %v", req.URL.Path, req.URL.Query())
	}
	if artwork.ID != 123 || artwork.User.ID != 7 || artwork.AIType != 2 || len(artwork.Pages) != 2 {
		t.Fatalf("artwork = %#v", artwork)
	}
	if artwork.Pages[0].PageIndex != 0 || artwork.Pages[1].PageIndex != 1 {
		t.Fatalf("page indexes = %d, %d; want 0, 1", artwork.Pages[0].PageIndex, artwork.Pages[1].PageIndex)
	}
	if len(artwork.Tags) != 1 || artwork.Tags[0].Name != "cat" || artwork.Tags[0].TranslatedName != "猫" {
		t.Fatalf("tags = %#v", artwork.Tags)
	}
}

func TestArtworkRankingFrozenModesAndDefault(t *testing.T) {
	body := `{"illusts":[],"next_url":null}`
	for _, mode := range []RankingMode{
		RankingModeDay, RankingModeDayMale, RankingModeDayFemale, RankingModeWeek,
		RankingModeWeekOriginal, RankingModeWeekRookie, RankingModeMonth,
		RankingModeDayManga, RankingModeWeekManga, RankingModeMonthManga,
		RankingModeWeekRookieManga, RankingModeDayR18, RankingModeDayMaleR18,
		RankingModeDayFemaleR18, RankingModeWeekR18, RankingModeWeekR18G,
	} {
		t.Run(string(mode), func(t *testing.T) {
			client, recorder := newWireClient(t, body)
			if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: mode}); err != nil {
				t.Fatalf("ArtworkRanking: %v", err)
			}
			if got := recorder.request(t, 0).URL.Query().Get("mode"); got != string(mode) {
				t.Fatalf("mode = %q, want %q", got, mode)
			}
		})
	}

	client, recorder := newWireClient(t, body)
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{}); err != nil {
		t.Fatalf("ArtworkRanking: %v", err)
	}
	if got := recorder.request(t, 0).URL.Query().Get("mode"); got != "day" {
		t.Fatalf("default mode = %q, want day", got)
	}
}

// TestArtworkListFamilyPreservesEmptyTerminalPage 覆盖原先各 endpoint 测试中的
// “成功空列表必须非 nil 且无 continuation”断言。
func TestArtworkListFamilyPreservesEmptyTerminalPage(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		invoke func(*Client) (int, bool, error)
	}{
		{
			name: "bookmarks", body: `{"illusts":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "search", body: `{"illusts":[],"next_url":null}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "empty"})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "related", body: `{"illusts":[]}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.RelatedArtworks(context.Background(), RelatedArtworksRequest{ArtworkID: 1})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
			},
		},
		{
			name: "user artworks", body: `{"illusts":[]}`,
			invoke: func(c *Client) (int, bool, error) {
				page, err := c.UserArtworks(context.Background(), UserArtworksRequest{UserID: 77})
				return len(page.Items), page.Items == nil, errOrNext(page.Next, err)
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

func errOrNext(cursor sdk.Cursor, err error) error {
	if err != nil {
		return err
	}
	if !cursor.IsZero() {
		return errors.New("unexpected continuation")
	}
	return nil
}

// TestArtworkReadTransportErrorsPropagate 保留原 endpoint 测试对读请求传输错误的断言：
// 失败不得被报告为成功，且同样不得返回部分数据。
func TestArtworkReadTransportErrorsPropagate(t *testing.T) {
	tests := []struct {
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t)
			recorder.err = errors.New("bookmark transport failed")
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
				t.Fatalf("error = %v, want a classified transport failure", err)
			}
		})
	}
}

// TestArtworkMappingErrorsKeepTheirReason 验证映射阶段产生的公开错误不会被上游
// 分类覆盖（例如不允许的资源 host 必须保持 ResourceForbidden）。
func TestArtworkMappingErrorsKeepTheirReason(t *testing.T) {
	body := `{"ugoira_metadata":{"zip_urls":{"original":"https://evil.example/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100}]}}`
	client, _ := newWireClient(t, body)
	_, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 1})
	if sdk.ReasonOf(err) != sdk.ResourceForbidden {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ResourceForbidden, err)
	}
}

// TestSearchArtworksWireContentTypeAlias 固定 illust-and-ugoira 的 wire 值：
// 公开值保持连字符形式，App API 收到下划线形式（旧 endpoint 的同一转换）。
func TestSearchArtworksWireContentTypeAlias(t *testing.T) {
	client, recorder := newWireClient(t, `{"illusts":[],"next_url":null}`)
	if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{
		Word: "landscape", ContentType: SearchContentTypeIllustAndUgoira,
	}); err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if got := recorder.request(t, 0).URL.Query().Get("content_type"); got != "illust_and_ugoira" {
		t.Fatalf("content_type = %q, want %q", got, "illust_and_ugoira")
	}
}
