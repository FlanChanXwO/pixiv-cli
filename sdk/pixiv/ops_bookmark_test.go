package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestExplicitArtworkBookmarkMutationsKeepLegacyWrappers(t *testing.T) {
	var requests []*http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		requests = append(requests, req)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	if err := client.AddArtworkBookmark(context.Background(), pixiv.AddArtworkBookmarkRequest{
		ArtworkID: 12,
		Restrict:  pixiv.RestrictPrivate,
		Tags:      []string{"cat", "favorite"},
	}); err != nil {
		t.Fatalf("AddArtworkBookmark: %v", err)
	}
	if err := client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{ArtworkID: 13, Tags: []string{"legacy"}}); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if err := client.RemoveArtworkBookmark(context.Background(), pixiv.RemoveArtworkBookmarkRequest{ArtworkID: 12}); err != nil {
		t.Fatalf("RemoveArtworkBookmark: %v", err)
	}
	if err := client.RemoveBookmark(context.Background(), pixiv.RemoveBookmarkRequest{ArtworkID: 13}); err != nil {
		t.Fatalf("RemoveBookmark: %v", err)
	}

	if len(requests) != 4 {
		t.Fatalf("request count = %d, want 4", len(requests))
	}
	if requests[0].URL.Path != "/v2/illust/bookmark/add" || requests[0].PostForm.Get("illust_id") != "12" || requests[0].PostForm.Get("restrict") != "private" || len(requests[0].PostForm["tags[]"]) != 2 {
		t.Fatalf("explicit add request = %s form=%v", requests[0].URL.Path, requests[0].PostForm)
	}
	if requests[1].URL.Path != "/v2/illust/bookmark/add" || requests[1].PostForm.Get("illust_id") != "13" || requests[1].PostForm.Get("restrict") != "public" || requests[1].PostForm.Get("tags[]") != "legacy" {
		t.Fatalf("legacy add request = %s form=%v", requests[1].URL.Path, requests[1].PostForm)
	}
	if requests[2].URL.Path != "/v1/illust/bookmark/delete" || requests[2].PostForm.Get("illust_id") != "12" {
		t.Fatalf("explicit remove request = %s form=%v", requests[2].URL.Path, requests[2].PostForm)
	}
	if requests[3].URL.Path != "/v1/illust/bookmark/delete" || requests[3].PostForm.Get("illust_id") != "13" {
		t.Fatalf("legacy remove request = %s form=%v", requests[3].URL.Path, requests[3].PostForm)
	}
}

func TestExplicitNovelBookmarkReadSDK(t *testing.T) {
	calls := 0
	detailCalls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		switch req.URL.Path {
		case "/v1/user/bookmark-tags/novel":
			if req.URL.Query().Get("user_id") != "8" || req.URL.Query().Get("restrict") != "private" {
				t.Errorf("tags query = %v", req.URL.Query())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"bookmark_tags":[{"name":"story","count":4}],"next_url":null}`)),
			}, nil
		case "/v2/novel/bookmark/detail":
			if req.URL.Query().Get("novel_id") != "42" {
				t.Errorf("detail query = %v", req.URL.Query())
			}
			detailCalls++
			if detailCalls == 2 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"bookmark_detail":null}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"story","is_registered":true}]}}`)),
			}, nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	tags, err := client.UserNovelBookmarkTags(context.Background(), pixiv.UserNovelBookmarkTagsRequest{UserID: 8, Restrict: pixiv.RestrictPrivate})
	if err != nil {
		t.Fatalf("UserNovelBookmarkTags: %v", err)
	}
	if tags.Items == nil || len(tags.Items) != 1 || tags.Items[0] != (pixiv.BookmarkTag{Name: "story", Count: 4}) || !tags.Next.IsZero() {
		t.Fatalf("tags page = %#v", tags)
	}

	detail, err := client.NovelBookmark(context.Background(), pixiv.NovelBookmarkRequest{NovelID: 42})
	if err != nil {
		t.Fatalf("NovelBookmark: %v", err)
	}
	if detail.Restrict != pixiv.RestrictPrivate || len(detail.Tags) != 1 || detail.Tags[0] != "story" {
		t.Fatalf("novel bookmark detail = %#v", detail)
	}

	absent, err := client.NovelBookmark(context.Background(), pixiv.NovelBookmarkRequest{NovelID: 42})
	if err != nil {
		t.Fatalf("NovelBookmark absent: %v", err)
	}
	if absent.Restrict != "" || absent.Tags == nil || len(absent.Tags) != 0 {
		t.Fatalf("absent novel bookmark detail = %#v", absent)
	}
	if calls != 3 {
		t.Fatalf("request count = %d, want 3", calls)
	}
}

func TestExplicitNovelBookmarkSDKRejectsUnsupportedInputsBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	cursor, err := sdk.NewCursor("pixiv", "UserNovelBookmarkTags", 1, "digest", []byte("payload"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}

	tests := []struct {
		name       string
		wantReason sdk.Reason
		call       func() error
	}{
		{name: "tags user id", wantReason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserNovelBookmarkTags(context.Background(), pixiv.UserNovelBookmarkTagsRequest{Restrict: pixiv.RestrictPublic})
			return err
		}},
		{name: "tags restrict", wantReason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserNovelBookmarkTags(context.Background(), pixiv.UserNovelBookmarkTagsRequest{UserID: 8, Restrict: pixiv.Restrict("friends")})
			return err
		}},
		{name: "tags continuation", wantReason: sdk.InvalidCursor, call: func() error {
			_, err := client.UserNovelBookmarkTags(context.Background(), pixiv.UserNovelBookmarkTagsRequest{UserID: 8, Restrict: pixiv.RestrictPublic, Cursor: cursor})
			return err
		}},
		{name: "detail novel id", wantReason: sdk.InvalidArgument, call: func() error {
			_, err := client.NovelBookmark(context.Background(), pixiv.NovelBookmarkRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != test.wantReason {
				t.Fatalf("ReasonOf = %q, want %q", reason, test.wantReason)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid request reached upstream %d time(s)", calls)
	}
}

// TestBookmarkListingCoverResolvesWithoutArtworkDetail 证明 Task 13 依赖的核心契约：
// bookmark listing 给出的 cover 引用可以在同一 client 上直接取回，且过程不触发
// artwork detail。若这里出现 /v1/illust/detail，就说明“不做 N+1”已被破坏。
func TestBookmarkListingCoverResolvesWithoutArtworkDetail(t *testing.T) {
	const mediaURL = "https://i.pximg.net/img/101_large.jpg?signature=sentinel"
	var paths []string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Host+req.URL.Path)
		switch req.URL.Host {
		case "app-api.pixiv.net":
			if req.URL.Path != "/v1/user/bookmarks/illust" {
				return nil, errors.New("unexpected app path: " + req.URL.Path)
			}
			return jsonResponse(`{"illusts":[{"id":101,"title":"multi","page_count":3,"user":{"id":7,"name":"artist"},"image_urls":{"large":"` + mediaURL + `"},"create_date":"2024-01-02T03:04:05+00:00"}]}`), nil
		case "i.pximg.net":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: io.NopCloser(strings.NewReader("IMAGE"))}, nil
		default:
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.UserArtworkBookmarks(context.Background(), pixiv.UserArtworkBookmarksRequest{UserID: 7, Restrict: pixiv.RestrictPublic})
	if err != nil {
		t.Fatalf("UserArtworkBookmarks: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d", len(page.Items))
	}
	// listing 必须只给 cover，且不伪装成多页结果。
	if len(page.Items[0].Pages) != 0 {
		t.Fatalf("listing must not expose pages: %+v", page.Items[0].Pages)
	}
	if page.Items[0].Cover.Resource.Ref.IsZero() {
		t.Fatal("listing must expose a cover reference")
	}
	destination := filepath.Join(t.TempDir(), "cover.jpg")
	if _, err := client.SaveResource(context.Background(), page.Items[0].Cover.Resource.Ref, sdk.SaveOptions{Path: destination}); err != nil {
		t.Fatalf("SaveResource: %v", err)
	}
	if body, err := os.ReadFile(destination); err != nil || string(body) != "IMAGE" {
		t.Fatalf("saved cover = %q err=%v", body, err)
	}
	for _, path := range paths {
		if strings.Contains(path, "/v1/illust/detail") {
			t.Fatalf("cover resolution issued artwork detail (N+1): %v", paths)
		}
	}
}
