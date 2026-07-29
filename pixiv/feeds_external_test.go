package pixiv_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestAuthenticatedReadFeedsUseConfirmedAppAPIEndpoints(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		query map[string]string
		body  string
		call  func(*pixiv.Client) error
	}{
		{
			name: "following novels", path: "/v1/novel/follow", query: map[string]string{"restrict": "private"},
			body: `{"novels":[{"id":1,"user":{"id":10}}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.FollowingNovels(context.Background(), pixiv.FollowingNovelsRequest{Restrict: pixiv.RestrictPrivate})
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 1 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "latest manga", path: "/v1/illust/new", query: map[string]string{"content_type": "manga", "filter": "for_android"},
			body: `{"illusts":[{"id":2}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.LatestIllusts(context.Background(), pixiv.LatestIllustsRequest{Type: pixiv.IllustTypeManga})
				if err != nil {
					return err
				}
				if len(result.Illusts) != 1 || result.Illusts[0].ID != 2 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "latest novels", path: "/v1/novel/new", query: map[string]string{"filter": "for_android"},
			body: `{"novels":[{"id":3,"user":{"id":30}}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.LatestNovels(context.Background(), pixiv.LatestNovelsRequest{})
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 3 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv users", path: "/v1/user/mypixiv", query: map[string]string{"user_id": "44", "filter": "for_android"},
			body: `{"user_previews":[{"user":{"id":4}}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.MyPixivUsers(context.Background(), pixiv.MyPixivUsersRequest{UserID: 44})
				if err != nil {
					return err
				}
				if len(result.UserPreviews) != 1 || result.UserPreviews[0].User.ID != 4 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv illusts", path: "/v2/illust/mypixiv", query: map[string]string{},
			body: `{"illusts":[{"id":5}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.MyPixivIllusts(context.Background(), pixiv.MyPixivIllustsRequest{})
				if err != nil {
					return err
				}
				if len(result.Illusts) != 1 || result.Illusts[0].ID != 5 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv novels", path: "/v1/novel/mypixiv", query: map[string]string{},
			body: `{"novels":[{"id":6,"user":{"id":60}}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.MyPixivNovels(context.Background(), pixiv.MyPixivNovelsRequest{})
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 6 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "user novels", path: "/v1/user/novels", query: map[string]string{"user_id": "77", "filter": "for_android"},
			body: `{"novels":[{"id":7,"user":{"id":77}}]}`,
			call: func(client *pixiv.Client) error {
				result, err := client.UserNovels(context.Background(), pixiv.UserNovelsRequest{UserID: 77})
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 7 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, test.path)
				}
				if r.Header.Get("Authorization") != "Bearer access" {
					t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
				}
				for key, want := range test.query {
					if got := r.URL.Query().Get(key); got != want {
						t.Fatalf("%s = %q, want %q; query=%v", key, got, want, r.URL.Query())
					}
				}
				if len(r.URL.Query()) != len(test.query) {
					t.Fatalf("query = %v, want exactly %v", r.URL.Query(), test.query)
				}
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
			if err != nil {
				t.Fatal(err)
			}
			if err := test.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadFeedsValidateArgumentsAndAuthenticationBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	invalid := []func() error{
		func() error {
			_, err := client.LatestIllusts(context.Background(), pixiv.LatestIllustsRequest{Type: pixiv.IllustTypeUgoira})
			return err
		},
		func() error {
			_, err := client.FollowingNovels(context.Background(), pixiv.FollowingNovelsRequest{Restrict: "invalid"})
			return err
		},
		func() error {
			_, err := client.MyPixivUsers(context.Background(), pixiv.MyPixivUsersRequest{})
			return err
		},
		func() error { _, err := client.UserNovels(context.Background(), pixiv.UserNovelsRequest{}); return err },
	}
	for _, call := range invalid {
		if err := call(); !errors.Is(err, pixiv.ErrInvalidArgument) {
			t.Errorf("invalid argument error = %v", err)
		}
	}

	authRequired := []func() error{
		func() error {
			_, err := client.FollowingNovels(context.Background(), pixiv.FollowingNovelsRequest{})
			return err
		},
		func() error {
			_, err := client.LatestIllusts(context.Background(), pixiv.LatestIllustsRequest{Type: pixiv.IllustTypeIllust})
			return err
		},
		func() error {
			_, err := client.LatestNovels(context.Background(), pixiv.LatestNovelsRequest{})
			return err
		},
		func() error {
			_, err := client.MyPixivUsers(context.Background(), pixiv.MyPixivUsersRequest{UserID: 1})
			return err
		},
		func() error {
			_, err := client.MyPixivIllusts(context.Background(), pixiv.MyPixivIllustsRequest{})
			return err
		},
		func() error {
			_, err := client.MyPixivNovels(context.Background(), pixiv.MyPixivNovelsRequest{})
			return err
		},
		func() error {
			_, err := client.UserNovels(context.Background(), pixiv.UserNovelsRequest{UserID: 1})
			return err
		},
	}
	for _, call := range authRequired {
		if err := call(); !errors.Is(err, pixiv.ErrUnauthorized) {
			t.Errorf("authentication error = %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestReadFeedCursorCannotCrossOperations(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/illust/new" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"illusts":[{"id":8}],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=illust&filter=for_android&offset=30&token=feed-next-secret"}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.LatestIllusts(context.Background(), pixiv.LatestIllustsRequest{Type: pixiv.IllustTypeIllust})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(first.NextCursor))
	if err != nil || strings.Contains(string(first.NextCursor), "feed-next-secret") || strings.Contains(string(decoded), "feed-next-secret") || strings.Contains(string(decoded), "next_url") {
		t.Fatalf("cursor leaked upstream continuation: %q", first.NextCursor)
	}
	_, err = client.LatestIllusts(context.Background(), pixiv.LatestIllustsRequest{Type: pixiv.IllustTypeManga, Cursor: first.NextCursor})
	if !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("changed-query error = %v", err)
	}
	_, err = client.MyPixivIllusts(context.Background(), pixiv.MyPixivIllustsRequest{Cursor: first.NextCursor})
	if !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("cross-operation error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("cross-operation cursor used network: %d", requests.Load())
	}
}

func TestReadFeedAppErrorsExposeOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "feed-secret", http.StatusBadGateway)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.LatestNovels(context.Background(), pixiv.LatestNovelsRequest{})
	if result != nil || !errors.Is(err, pixiv.ErrUpstreamError) {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Backend != pixiv.BackendAppAPI || typed.Operation != pixiv.OperationLatestNovels || typed.UpstreamStatus != http.StatusBadGateway {
		t.Fatalf("metadata = %#v", typed)
	}
}
