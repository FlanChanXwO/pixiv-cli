package pixiv_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestFollowUserRejectsUnknownRestrictBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	err = client.FollowUser(context.Background(), pixiv.FollowUserRequest{
		UserID:   7,
		Restrict: pixiv.Restrict("unknown"),
	})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if calls != 0 {
		t.Fatalf("invalid restrict reached upstream %d time(s)", calls)
	}
}

func TestFollowUserDefaultsEmptyRestrictToPublic(t *testing.T) {
	var captured *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	if err := client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 7}); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if captured == nil {
		t.Fatal("follow request was not captured")
	}
	if err := captured.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if captured.URL.Path != "/v1/user/follow/add" || captured.PostForm.Get("restrict") != "public" {
		t.Fatalf("request = %s form=%v", captured.URL.Path, captured.PostForm)
	}
}

func TestNovelBookmarkMutationsUseCandidatePathsAndForms(t *testing.T) {
	var requests []*http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	if err := client.AddNovelBookmark(context.Background(), pixiv.AddNovelBookmarkRequest{
		NovelID:  42,
		Restrict: pixiv.RestrictPrivate,
		Tags:     []string{"story", "favorite"},
	}); err != nil {
		t.Fatalf("AddNovelBookmark: %v", err)
	}
	if err := client.RemoveNovelBookmark(context.Background(), pixiv.RemoveNovelBookmarkRequest{NovelID: 42}); err != nil {
		t.Fatalf("RemoveNovelBookmark: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}

	if err := requests[0].ParseForm(); err != nil {
		t.Fatalf("ParseForm(add): %v", err)
	}
	if requests[0].URL.Path != "/v2/novel/bookmark/add" || requests[0].PostForm.Get("novel_id") != "42" || requests[0].PostForm.Get("restrict") != "private" || !slices.Equal(requests[0].PostForm["tags[]"], []string{"story", "favorite"}) {
		t.Fatalf("add request = %s form=%v", requests[0].URL.Path, requests[0].PostForm)
	}
	if err := requests[1].ParseForm(); err != nil {
		t.Fatalf("ParseForm(remove): %v", err)
	}
	if requests[1].URL.Path != "/v1/novel/bookmark/delete" || requests[1].PostForm.Get("novel_id") != "42" {
		t.Fatalf("remove request = %s form=%v", requests[1].URL.Path, requests[1].PostForm)
	}
}

func TestAddNovelBookmarkDefaultsEmptyRestrictToPublic(t *testing.T) {
	var captured *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	if err := client.AddNovelBookmark(context.Background(), pixiv.AddNovelBookmarkRequest{NovelID: 7}); err != nil {
		t.Fatalf("AddNovelBookmark: %v", err)
	}
	if captured == nil {
		t.Fatal("novel bookmark request was not captured")
	}
	if err := captured.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if captured.URL.Path != "/v2/novel/bookmark/add" || captured.PostForm.Get("restrict") != "public" {
		t.Fatalf("request = %s form=%v, want public restrict", captured.URL.Path, captured.PostForm)
	}
}

func TestNovelBookmarkMutationDoesNotReplayUncertainFailure(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"outcome unknown"}`)),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	err = client.AddNovelBookmark(context.Background(), pixiv.AddNovelBookmarkRequest{NovelID: 42, Restrict: pixiv.RestrictPublic})
	if err == nil {
		t.Fatal("AddNovelBookmark returned nil for an upstream 502")
	}
	if calls != 1 {
		t.Fatalf("upstream call count = %d, want 1 for uncertain mutation", calls)
	}
}

func TestNovelBookmarkMutationsRejectInvalidInputBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "add novel id", call: func() error {
			return client.AddNovelBookmark(context.Background(), pixiv.AddNovelBookmarkRequest{Restrict: pixiv.RestrictPublic})
		}},
		{name: "add restrict", call: func() error {
			return client.AddNovelBookmark(context.Background(), pixiv.AddNovelBookmarkRequest{NovelID: 42, Restrict: pixiv.Restrict("friends")})
		}},
		{name: "remove novel id", call: func() error {
			return client.RemoveNovelBookmark(context.Background(), pixiv.RemoveNovelBookmarkRequest{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidArgument)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid request reached upstream %d time(s)", calls)
	}
}
