package appapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArtworkCommentsMapsEnvelopeAndContinuation(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/illust/comments" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("illust_id") != "123" || r.URL.Query().Get("offset") != "5" {
			t.Fatalf("query = %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{
			"comments":[{"id":101,"comment":"first","created_at":"2026-01-01T00:00:00+09:00","user":{"id":7,"name":"u7"},"parent_comment":{"id":100,"caption":"parent","created_at":"2025-12-31T00:00:00+09:00","user":{"id":8,"name":"u8"}}}],
			"next_url":"https://app-api.pixiv.net/v2/illust/comments?illust_id=123&offset=10",
			"total_comments":25,
			"access_control":{"can_comment":true,"is_locked":false}
		}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).ArtworkComments(context.Background(), 123, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 1 || got.Comments[0].ID != 101 || got.Comments[0].Comment != "first" || got.Comments[0].User.ID != 7 {
		t.Fatalf("comments = %#v", got.Comments)
	}
	parent := got.Comments[0].ParentComment
	if parent == nil || parent.ID != 100 || parent.Comment != "parent" {
		t.Fatalf("parent = %#v", parent)
	}
	if got.NextOffset != 10 || !got.ContinuationExists {
		t.Fatalf("continuation = offset %d exists %v", got.NextOffset, got.ContinuationExists)
	}
	if got.Total == nil || *got.Total != 25 {
		t.Fatalf("total = %#v", got.Total)
	}
	if got.AccessControl == nil || !got.AccessControl.CanComment || got.AccessControl.IsLocked {
		t.Fatalf("access control = %#v", got.AccessControl)
	}
}

func TestNovelCommentsQueryAndOptionalMetadataAbsent(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/novel/comments" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("novel_id") != "9" {
			t.Fatalf("query = %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"comments":[],"next_url":null}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).NovelComments(context.Background(), 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Comments == nil || len(got.Comments) != 0 || got.Total != nil || got.AccessControl != nil || got.ContinuationExists {
		t.Fatalf("got = %#v", got)
	}
}

func TestCommentsRejectMalformedWire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{"zero comment id", `{"comments":[{"id":0,"comment":"x"}]}`},
		{"zero parent id", `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`},
		{"empty next url", `{"comments":[],"next_url":""}`},
		{"bad next url offset", `{"comments":[],"next_url":"https://app-api.pixiv.net/v2/illust/comments?illust_id=1&offset=x"}`},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer api.Close()
			_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).ArtworkComments(context.Background(), 1, 0)
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestArtworkBookmarkDetailMapsTagsAndNotBookmarked(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		body      string
		wantTags  int
		wantRestr string
	}{
		{name: "bookmarked", body: `{"bookmark_detail":{"restrict":"public","tags":["a","b"]}}`, wantTags: 2, wantRestr: "public"},
		{name: "not bookmarked", body: `{"bookmark_detail":null}`, wantTags: 0, wantRestr: ""},
		{name: "missing detail", body: `{}`, wantTags: 0, wantRestr: ""},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/illust/bookmark/detail" || r.URL.Query().Get("illust_id") != "55" {
					t.Fatalf("path=%q query=%v", r.URL.Path, r.URL.Query())
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer api.Close()
			got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).ArtworkBookmarkDetail(context.Background(), 55)
			if err != nil {
				t.Fatal(err)
			}
			if got.Restrict != test.wantRestr || len(got.Tags) != test.wantTags {
				t.Fatalf("got = %#v", got)
			}
		})
	}
}

func TestUserArtworkBookmarkTagsMapsAndValidates(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/bookmark-tags/illust" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("user_id") != "7" || q.Get("restrict") != "private" || q.Get("offset") != "3" {
			t.Fatalf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{"bookmark_tags":[{"name":"cat","count":12}],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?user_id=7&restrict=private&offset=6"}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).UserArtworkBookmarkTags(context.Background(), 7, "private", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "cat" || got.Tags[0].Count != 12 {
		t.Fatalf("tags = %#v", got.Tags)
	}
	if got.NextOffset != 6 || !got.ContinuationExists {
		t.Fatalf("continuation = %#v", got)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"bookmark_tags":[{"name":"","count":1}]}`))
	}))
	defer bad.Close()
	if _, err := New(WithBaseURL(bad.URL), WithHTTPClient(bad.Client()), WithAccessToken("access")).UserArtworkBookmarkTags(context.Background(), 7, "public", 0); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestNovelDetailMapsSeriesReferences(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/novel/detail" || r.URL.Query().Get("novel_id") != "42" {
			t.Fatalf("path=%q query=%v", r.URL.Path, r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"novel":{"id":42,"title":"t","user":{"id":7}},"series_next":{"id":43,"title":"Series"},"series_prev":{"id":41,"title":"Series"}}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).NovelDetail(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.Novel.ID != 42 || got.SeriesNextID != 43 || got.SeriesPrevID != 41 || got.SeriesTitle != "Series" {
		t.Fatalf("got = %#v", got)
	}

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer missing.Close()
	if _, err := New(WithBaseURL(missing.URL), WithHTTPClient(missing.Client()), WithAccessToken("access")).NovelDetail(context.Background(), 42); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestNovelSeriesMapsDetailNovelsAndLastOrderContinuation(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/novel/series" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("series_id") != "3" || q.Get("last_order") != "2" {
			t.Fatalf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{
			"novel_series_detail":{"id":3,"title":"S","caption":"c","is_concluded":true,"user":{"id":9,"name":"u9"}},
			"novels":[{"id":1,"title":"n1","user":{"id":9}}],
			"next_url":"https://app-api.pixiv.net/v1/novel/series?series_id=3&last_order=5"
		}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).NovelSeries(context.Background(), 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Series.ID != 3 || got.Series.Title != "S" || !got.Series.IsConcluded || got.Series.User.ID != 9 {
		t.Fatalf("series = %#v", got.Series)
	}
	if len(got.Novels) != 1 || got.Novels[0].ID != 1 {
		t.Fatalf("novels = %#v", got.Novels)
	}
	if got.NextValue != 5 || !got.ContinuationExists {
		t.Fatalf("continuation = %#v", got)
	}

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"novels":[]}`)) }))
	defer missing.Close()
	if _, err := New(WithBaseURL(missing.URL), WithHTTPClient(missing.Client()), WithAccessToken("access")).NovelSeries(context.Background(), 3, 0); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestNovelContentReturnsRawBodyAndUserIDHeader(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/novel/content" || r.URL.Query().Get("novel_id") != "9" {
			t.Fatalf("path=%q query=%v", r.URL.Path, r.URL.Query())
		}
		if r.Header.Get("X-User-Id") != "42" {
			t.Fatalf("X-User-Id = %q", r.Header.Get("X-User-Id"))
		}
		_, _ = w.Write([]byte("<p>hello</p>"))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access"), WithUserID(42)).NovelContent(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<p>hello</p>" {
		t.Fatalf("body = %q", got)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("   ")) }))
	defer empty.Close()
	if _, err := New(WithBaseURL(empty.URL), WithHTTPClient(empty.Client()), WithAccessToken("access")).NovelContent(context.Background(), 9); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestUserNovelBookmarksMapsMaxBookmarkIDContinuation(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/bookmarks/novel" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("user_id") != "7" || q.Get("restrict") != "public" || q.Get("tag") != "cat" || q.Get("max_bookmark_id") != "99" {
			t.Fatalf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{"novels":[{"id":1,"user":{"id":7}}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?user_id=7&restrict=public&max_bookmark_id=88"}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).UserNovelBookmarks(context.Background(), 7, "public", "cat", 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Novels) != 1 || got.Novels[0].ID != 1 {
		t.Fatalf("novels = %#v", got.Novels)
	}
	if got.NextMaxBookmarkID != 88 || !got.ContinuationExists {
		t.Fatalf("continuation = %#v", got)
	}
}

func TestUserRelatedFollowerBlockedShareEnvelope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, path, queryKey, queryValue string
		call                             func(*Client) error
	}{
		{"related", "/v1/user/related", "seed_user_id", "11", func(c *Client) error { _, e := c.RelatedUsers(context.Background(), 11, 4); return e }},
		{"followers", "/v1/user/follower", "user_id", "11", func(c *Client) error { _, e := c.UserFollowers(context.Background(), 11, "public", 4); return e }},
		{"blocked", "/v1/user/list", "user_id", "11", func(c *Client) error { _, e := c.UserBlockedUsers(context.Background(), 11, 4); return e }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q", r.URL.Path)
				}
				q := r.URL.Query()
				if q.Get(tt.queryKey) != tt.queryValue || q.Get("offset") != "4" {
					t.Fatalf("query = %v", q)
				}
				_, _ = w.Write([]byte(`{"user_previews":[{"user":{"id":5}}],"next_url":"https://app-api.pixiv.net/v1/user/related?seed_user_id=11&offset=8"}`))
			}))
			defer api.Close()
			client := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access"))
			if err := tt.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSetAIArtworkVisibilityPostsAIShowForm(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		show bool
		want string
	}{
		{show: true, want: "1"},
		{show: false, want: "0"},
	} {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/user/edit-ai-show-settings" {
				t.Fatalf("method=%s path=%q", r.Method, r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("ai_show") != test.want {
				t.Fatalf("ai_show = %q, want %q; form=%v", r.Form.Get("ai_show"), test.want, r.Form)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).SetAIArtworkVisibility(context.Background(), test.show)
		api.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCurrentUserMapsUserDetailEnvelope(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/me" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if len(r.URL.Query()) != 0 {
			t.Fatalf("query = %v, want none", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"user":{"id":1,"name":"me"},"profile":{},"profile_publicity":{"gender":false,"region":false,"birth_day":false,"birth_year":false,"job":false,"pawoo":false},"workspace":{}}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.User.ID != 1 || got.User.Name != "me" {
		t.Fatalf("got = %#v", got)
	}
}

func TestCommentsValidateCaptionFallback(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"comments":[{"id":1,"caption":"fallback","created_at":"2026-01-01T00:00:00Z","user":{"id":2}}]}`))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).ArtworkComments(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Comments[0].Comment != "fallback" {
		t.Fatalf("comment = %q", got.Comments[0].Comment)
	}
}

func TestNovelContentRetriesRateLimitAndSurfacesHTTPStatus(t *testing.T) {
	t.Parallel()
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer api.Close()

	got, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).NovelContent(context.Background(), 1)
	if err != nil || string(got) != "ok" || requests != 2 {
		t.Fatalf("body=%q err=%v requests=%d", got, err, requests)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	if _, err := New(WithBaseURL(bad.URL), WithHTTPClient(bad.Client()), WithAccessToken("access")).NovelContent(context.Background(), 1); err == nil || strings.Contains(err.Error(), "malformed") {
		t.Fatalf("err = %v", err)
	}
}
