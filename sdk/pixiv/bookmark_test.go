package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// TestArtworkBookmarkMutations 覆盖 add/remove 共用的 form 与默认 restrict。
func TestArtworkBookmarkMutations(t *testing.T) {
	client, recorder := newWireClient(t, `{}`, `{}`, `{}`)
	if err := client.AddArtworkBookmark(context.Background(), AddArtworkBookmarkRequest{
		ArtworkID: 12,
		Restrict:  RestrictPrivate,
		Tags:      []string{"cat", "favorite"},
	}); err != nil {
		t.Fatalf("AddArtworkBookmark: %v", err)
	}
	if err := client.AddArtworkBookmark(context.Background(), AddArtworkBookmarkRequest{ArtworkID: 13, Tags: []string{"tag"}}); err != nil {
		t.Fatalf("AddArtworkBookmark without restrict: %v", err)
	}
	if err := client.RemoveArtworkBookmark(context.Background(), RemoveArtworkBookmarkRequest{ArtworkID: 12}); err != nil {
		t.Fatalf("RemoveArtworkBookmark: %v", err)
	}
	if recorder.calls() != 3 {
		t.Fatalf("request count = %d, want 3", recorder.calls())
	}
	checks := []struct {
		path, restrict string
		tags           []string
	}{
		{path: "/v2/illust/bookmark/add", restrict: "private", tags: []string{"cat", "favorite"}},
		{path: "/v2/illust/bookmark/add", restrict: "public", tags: []string{"tag"}},
		{path: "/v1/illust/bookmark/delete"},
	}
	for index, check := range checks {
		request := recorder.request(t, index)
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if request.URL.Path != check.path || request.PostForm.Get("restrict") != check.restrict {
			t.Fatalf("request %d = %s form=%v", index, request.URL.Path, request.PostForm)
		}
		if check.tags != nil && len(request.PostForm["tags[]"]) != len(check.tags) {
			t.Fatalf("request %d tags = %v, want %v", index, request.PostForm["tags[]"], check.tags)
		}
	}
}

func TestNovelBookmarkDetailStates(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		status int
		want   NovelBookmarkDetail
	}{
		"registered tags only": {
			body: `{"bookmark_detail":{"is_bookmarked":true,"restrict":"public","tags":[{"name":"content-tag","is_registered":false},{"name":"saved-tag","is_registered":true}]}}`,
			want: NovelBookmarkDetail{Restrict: RestrictPublic, Tags: []string{"saved-tag"}},
		},
		"explicitly not bookmarked": {body: `{"bookmark_detail":{"is_bookmarked":false,"restrict":"private","tags":[{"name":"cat","is_registered":false}]}}`, want: NovelBookmarkDetail{Tags: []string{}}},
		"null detail":               {body: `{"bookmark_detail":null}`, want: NovelBookmarkDetail{Tags: []string{}}},
		"not found":                 {status: http.StatusNotFound, want: NovelBookmarkDetail{Tags: []string{}}},
	} {
		t.Run(name, func(t *testing.T) {
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
				if test.status != 0 {
					return &http.Response{StatusCode: test.status, Header: http.Header{}, Body: http.NoBody}, nil
				}
				return jsonResponse(test.body), nil
			})
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			detail, err := client.NovelBookmark(context.Background(), NovelBookmarkRequest{NovelID: 42})
			if err != nil {
				t.Fatalf("NovelBookmark: %v", err)
			}
			if detail.Restrict != test.want.Restrict || detail.Tags == nil || len(detail.Tags) != len(test.want.Tags) {
				t.Fatalf("detail = %#v, want %#v", detail, test.want)
			}
			if len(test.want.Tags) > 0 && detail.Tags[0] != test.want.Tags[0] {
				t.Fatalf("detail = %#v, want %#v", detail, test.want)
			}
		})
	}
}

func TestUserNovelBookmarkTags(t *testing.T) {
	client, recorder := newWireClient(t, `{"bookmark_tags":[{"name":"story","count":4}],"next_url":null}`)
	tags, err := client.UserNovelBookmarkTags(context.Background(), UserNovelBookmarkTagsRequest{UserID: 8, Restrict: RestrictPrivate})
	if err != nil {
		t.Fatalf("UserNovelBookmarkTags: %v", err)
	}
	if len(tags.Items) != 1 || tags.Items[0] != (BookmarkTag{Name: "story", Count: 4}) || !tags.Next.IsZero() {
		t.Fatalf("tags page = %#v", tags)
	}
	if query := recorder.request(t, 0).URL.Query(); query.Get("user_id") != "8" || query.Get("restrict") != "private" {
		t.Fatalf("query = %v", query)
	}

	for name, body := range map[string]string{
		"missing tags":    `{}`,
		"null tags":       `{"bookmark_tags":null}`,
		"empty name":      `{"bookmark_tags":[{"name":"","count":1}]}`,
		"unexpected next": `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/novel?offset=2"}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, body)
			if _, err := client.UserNovelBookmarkTags(context.Background(), UserNovelBookmarkTagsRequest{UserID: 8}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
		})
	}
}

func TestBookmarkOperationsRejectInvalidInputBeforeNetwork(t *testing.T) {
	client, recorder := newWireClient(t)
	tests := []struct {
		name   string
		reason sdk.Reason
		call   func() error
	}{
		{name: "artwork bookmarks restrict", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "artwork bookmark tags restrict", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "novel bookmarks restrict", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "add artwork bookmark restrict", reason: sdk.InvalidArgument, call: func() error {
			return client.AddArtworkBookmark(context.Background(), AddArtworkBookmarkRequest{ArtworkID: 7, Restrict: Restrict("unknown")})
		}},
		{name: "novel bookmark tags cursor", reason: sdk.InvalidCursor, call: func() error {
			cursor, err := sdk.NewCursor("pixiv", "UserNovelBookmarkTags", 1, "digest", []byte("payload"))
			if err != nil {
				return err
			}
			_, err = client.UserNovelBookmarkTags(context.Background(), UserNovelBookmarkTagsRequest{UserID: 8, Cursor: cursor})
			return err
		}},
		{name: "novel bookmark tags user id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserNovelBookmarkTags(context.Background(), UserNovelBookmarkTagsRequest{Restrict: RestrictPublic})
			return err
		}},
		{name: "novel bookmark id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.NovelBookmark(context.Background(), NovelBookmarkRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != test.reason {
				t.Fatalf("ReasonOf = %q, want %q", reason, test.reason)
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid input reached upstream %d time(s)", recorder.calls())
	}
}
