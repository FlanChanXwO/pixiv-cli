package pixiv_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// TestFollowMutations 覆盖 follow/unfollow 共用 form/post 的契约，
// 以及不确定失败不得重放。
func TestFollowMutations(t *testing.T) {
	client, recorder := newWireClient(t, `{}`, `{}`)
	if err := client.FollowUser(context.Background(), FollowUserRequest{UserID: 7}); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if err := client.UnfollowUser(context.Background(), UnfollowUserRequest{UserID: 7}); err != nil {
		t.Fatalf("UnfollowUser: %v", err)
	}
	for index, want := range []struct {
		path     string
		restrict string
	}{
		{path: "/v1/user/follow/add", restrict: "public"},
		{path: "/v1/user/follow/delete"},
	} {
		request := recorder.request(t, index)
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if request.URL.Path != want.path || request.PostForm.Get("user_id") != "7" || request.PostForm.Get("restrict") != want.restrict {
			t.Fatalf("request %d = %q form=%v", index, request.URL.Path, request.PostForm)
		}
	}

	for _, test := range []struct {
		name string
		call func(*Client) error
	}{
		{name: "follow", call: func(c *Client) error {
			return c.FollowUser(context.Background(), FollowUserRequest{UserID: 7, Restrict: RestrictPublic})
		}},
		{name: "unfollow", call: func(c *Client) error {
			return c.UnfollowUser(context.Background(), UnfollowUserRequest{UserID: 7})
		}},
	} {
		t.Run("uncertain/"+test.name, func(t *testing.T) {
			calls := 0
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: http.NoBody}, nil
			})
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if err := test.call(client); err == nil || calls != 1 {
				t.Fatalf("err = %v calls = %d, want one uncertain failure", err, calls)
			}
		})
	}

	client, recorder = newWireClient(t)
	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "follow user id", call: func() error {
			return client.FollowUser(context.Background(), FollowUserRequest{Restrict: RestrictPublic})
		}},
		{name: "follow restrict", call: func() error {
			return client.FollowUser(context.Background(), FollowUserRequest{UserID: 7, Restrict: Restrict("friends")})
		}},
		{name: "unfollow user id", call: func() error {
			return client.UnfollowUser(context.Background(), UnfollowUserRequest{})
		}},
	} {
		t.Run("invalid/"+test.name, func(t *testing.T) {
			if err := test.call(); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q", sdk.ReasonOf(err))
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid input reached upstream %d time(s)", recorder.calls())
	}
}

// TestNovelBookmarkMutations 覆盖 add/remove 的 candidate path、form 与默认 restrict。
func TestNovelBookmarkMutations(t *testing.T) {
	client, recorder := newWireClient(t, `{}`, `{}`, `{}`)
	if err := client.AddNovelBookmark(context.Background(), AddNovelBookmarkRequest{
		NovelID: 42, Restrict: RestrictPrivate, Tags: []string{"story", "favorite"},
	}); err != nil {
		t.Fatalf("AddNovelBookmark: %v", err)
	}
	if err := client.AddNovelBookmark(context.Background(), AddNovelBookmarkRequest{NovelID: 43}); err != nil {
		t.Fatalf("AddNovelBookmark default: %v", err)
	}
	if err := client.RemoveNovelBookmark(context.Background(), RemoveNovelBookmarkRequest{NovelID: 42}); err != nil {
		t.Fatalf("RemoveNovelBookmark: %v", err)
	}
	for index, want := range []struct {
		path     string
		restrict string
		tags     []string
	}{
		{path: "/v2/novel/bookmark/add", restrict: "private", tags: []string{"story", "favorite"}},
		{path: "/v2/novel/bookmark/add", restrict: "public"},
		{path: "/v1/novel/bookmark/delete"},
	} {
		request := recorder.request(t, index)
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if request.URL.Path != want.path || request.PostForm.Get("restrict") != want.restrict {
			t.Fatalf("request %d = %q form=%v", index, request.URL.Path, request.PostForm)
		}
		if want.tags != nil && !slices.Equal(request.PostForm["tags[]"], want.tags) {
			t.Fatalf("request %d tags = %v, want %v", index, request.PostForm["tags[]"], want.tags)
		}
	}

	client, recorder = newWireClient(t)
	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "add novel id", call: func() error {
			return client.AddNovelBookmark(context.Background(), AddNovelBookmarkRequest{Restrict: RestrictPublic})
		}},
		{name: "add restrict", call: func() error {
			return client.AddNovelBookmark(context.Background(), AddNovelBookmarkRequest{NovelID: 1, Restrict: Restrict("friends")})
		}},
		{name: "remove novel id", call: func() error {
			return client.RemoveNovelBookmark(context.Background(), RemoveNovelBookmarkRequest{})
		}},
	} {
		t.Run("invalid/"+test.name, func(t *testing.T) {
			if err := test.call(); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q", sdk.ReasonOf(err))
			}
		})
	}
	for _, test := range []struct {
		name string
		call func(*Client) error
	}{
		{name: "add", call: func(c *Client) error {
			return c.AddNovelBookmark(context.Background(), AddNovelBookmarkRequest{NovelID: 1})
		}},
		{name: "remove", call: func(c *Client) error {
			return c.RemoveNovelBookmark(context.Background(), RemoveNovelBookmarkRequest{NovelID: 1})
		}},
	} {
		t.Run("uncertain/"+test.name, func(t *testing.T) {
			failing, failingRecorder := newWireClient(t)
			failingRecorder.err = errors.New("uncertain transport")
			if err := test.call(failing); err == nil || failingRecorder.calls() != 1 {
				t.Fatalf("err = %v calls = %d", err, failingRecorder.calls())
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid input reached upstream %d time(s)", recorder.calls())
	}
}

// TestBookmarkMutationReadBackOrchestrationOffline 证明写后回读与删除后恢复
// 可以按 public read/write 组合，2xx 本身不代表状态已改变。
func TestBookmarkMutationReadBackOrchestrationOffline(t *testing.T) {
	var sequence []string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sequence = append(sequence, req.Method+" "+req.URL.Path)
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v2/illust/bookmark/add":
			return jsonResponse("{}"), nil
		case req.Method == http.MethodGet && req.URL.Path == "/v2/illust/bookmark/detail":
			body := `{"bookmark_detail":{"is_bookmarked":true,"restrict":"public","tags":[{"name":"cat","is_registered":true}]}}`
			if len(sequence) == 4 {
				body = `{"bookmark_detail":{"is_bookmarked":false,"restrict":"","tags":[]}}`
			}
			return jsonResponse(body), nil
		case req.Method == http.MethodPost && req.URL.Path == "/v1/illust/bookmark/delete":
			return jsonResponse("{}"), nil
		default:
			t.Fatalf("unexpected read-back wire call: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	ctx := context.Background()
	if err := client.AddArtworkBookmark(ctx, AddArtworkBookmarkRequest{ArtworkID: 77, Tags: []string{"cat"}}); err != nil {
		t.Fatalf("AddArtworkBookmark: %v", err)
	}
	detail, err := client.ArtworkBookmark(ctx, ArtworkBookmarkRequest{ArtworkID: 77})
	if err != nil || len(detail.Tags) != 1 || detail.Tags[0] != "cat" {
		t.Fatalf("read-back after add = %#v err=%v", detail, err)
	}
	if err := client.RemoveArtworkBookmark(ctx, RemoveArtworkBookmarkRequest{ArtworkID: 77}); err != nil {
		t.Fatalf("RemoveArtworkBookmark: %v", err)
	}
	detail, err = client.ArtworkBookmark(ctx, ArtworkBookmarkRequest{ArtworkID: 77})
	if err != nil || len(detail.Tags) != 0 {
		t.Fatalf("read-back after remove = %#v err=%v", detail, err)
	}
	want := []string{
		http.MethodPost + " /v2/illust/bookmark/add",
		http.MethodGet + " /v2/illust/bookmark/detail",
		http.MethodPost + " /v1/illust/bookmark/delete",
		http.MethodGet + " /v2/illust/bookmark/detail",
	}
	if !slices.Equal(sequence, want) {
		t.Fatalf("wire sequence = %v, want %v", sequence, want)
	}
}
