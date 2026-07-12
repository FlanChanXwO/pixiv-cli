package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

func TestAuthenticatedMutationsUseExpectedAppForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		call func(*pixiv.Client) error
		want url.Values
	}{
		{
			name: "add bookmark defaults restrict and preserves tags",
			path: "/v2/illust/bookmark/add",
			call: func(client *pixiv.Client) error {
				return client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: 731, Tags: []string{"landscape", "landscape", "夜"}})
			},
			want: url.Values{"illust_id": {"731"}, "restrict": {"public"}, "tags[]": {"landscape", "landscape", "夜"}},
		},
		{
			name: "remove bookmark",
			path: "/v1/illust/bookmark/delete",
			call: func(client *pixiv.Client) error {
				return client.RemoveBookmark(context.Background(), pixiv.RemoveBookmarkRequest{IllustID: 731})
			},
			want: url.Values{"illust_id": {"731"}},
		},
		{
			name: "follow accepts private restrict",
			path: "/v1/user/follow/add",
			call: func(client *pixiv.Client) error {
				return client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 419, Restrict: pixiv.RestrictPrivate})
			},
			want: url.Values{"user_id": {"419"}, "restrict": {"private"}},
		},
		{
			name: "follow defaults restrict",
			path: "/v1/user/follow/add",
			call: func(client *pixiv.Client) error {
				return client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 420})
			},
			want: url.Values{"user_id": {"420"}, "restrict": {"public"}},
		},
		{
			name: "unfollow user",
			path: "/v1/user/follow/delete",
			call: func(client *pixiv.Client) error {
				return client.UnfollowUser(context.Background(), pixiv.UnfollowUserRequest{UserID: 419})
			},
			want: url.Values{"user_id": {"419"}},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost {
					t.Fatalf("method=%s", request.Method)
				}
				if request.URL.Path != test.path {
					t.Fatalf("path=%s", request.URL.Path)
				}
				if request.Header.Get("Authorization") != "Bearer access-secret" {
					t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
				}
				if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
					t.Fatalf("content-type=%q", request.Header.Get("Content-Type"))
				}
				if err := request.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if got := request.PostForm; !equalValues(got, test.want) {
					t.Fatalf("form=%v want=%v", got, test.want)
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access-secret"})
			if err != nil {
				t.Fatal(err)
			}
			if err := test.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMutationsRejectInvalidInputBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access-secret"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		call func() error
		op   pixiv.Operation
		id   int64
		user bool
	}{
		{"bookmark id", func() error { return client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{}) }, pixiv.OperationAddBookmark, 0, false},
		{"bookmark restrict", func() error {
			return client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: 1, Restrict: "friends"})
		}, pixiv.OperationAddBookmark, 1, false},
		{"bookmark empty tag", func() error {
			return client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: 1, Tags: []string{" "}})
		}, pixiv.OperationAddBookmark, 1, false},
		{"remove bookmark id", func() error { return client.RemoveBookmark(context.Background(), pixiv.RemoveBookmarkRequest{}) }, pixiv.OperationRemoveBookmark, 0, false},
		{"follow id", func() error { return client.FollowUser(context.Background(), pixiv.FollowUserRequest{}) }, pixiv.OperationFollowUser, 0, true},
		{"follow restrict", func() error {
			return client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 1, Restrict: "friends"})
		}, pixiv.OperationFollowUser, 1, true},
		{"unfollow id", func() error { return client.UnfollowUser(context.Background(), pixiv.UnfollowUserRequest{}) }, pixiv.OperationUnfollowUser, 0, true},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			var typed *pixiv.Error
			if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != test.op || typed.Backend != "" {
				t.Fatalf("err=%v typed=%+v", err, typed)
			}
			if test.user && typed.UserID != test.id {
				t.Fatalf("user_id=%d", typed.UserID)
			}
			if !test.user && typed.IllustID != test.id {
				t.Fatalf("illust_id=%d", typed.IllustID)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests=%d", got)
	}
}

func TestMutationsRequireAuthenticatedAppWithoutWebFallback(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { return client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: 1}) },
		func() error {
			return client.RemoveBookmark(context.Background(), pixiv.RemoveBookmarkRequest{IllustID: 1})
		},
		func() error { return client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 1}) },
		func() error { return client.UnfollowUser(context.Background(), pixiv.UnfollowUserRequest{UserID: 1}) },
	} {
		if err := call(); !errors.Is(err, pixiv.ErrUnauthorized) {
			t.Fatalf("err=%v", err)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests=%d", got)
	}
}

func TestMutationUpstreamFailureIsTypedAndSecretSafe(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/v1/user/follow/delete" {
			t.Fatalf("path=%s", request.URL.Path)
		}
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("upstream-body-secret unauthorized access-secret"))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access-secret"})
	if err != nil {
		t.Fatal(err)
	}
	err = client.UnfollowUser(context.Background(), pixiv.UnfollowUserRequest{UserID: 419})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeUpstreamError || typed.Operation != pixiv.OperationUnfollowUser || typed.Backend != pixiv.BackendAppAPI || typed.UserID != 419 || typed.UpstreamStatus != http.StatusBadGateway || !typed.Retryable {
		t.Fatalf("err=%v typed=%+v", err, typed)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests=%d", got)
	}
	if rendered := err.Error() + " " + fmt.Sprint(errors.Unwrap(err)); strings.Contains(rendered, "upstream-body-secret") || strings.Contains(rendered, "access-secret") {
		t.Fatalf("secret leaked in %q", rendered)
	}
}

func equalValues(got, want url.Values) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValues := range want {
		gotValues, ok := got[key]
		if !ok || len(gotValues) != len(wantValues) {
			return false
		}
		for index := range wantValues {
			if gotValues[index] != wantValues[index] {
				return false
			}
		}
	}
	return true
}
