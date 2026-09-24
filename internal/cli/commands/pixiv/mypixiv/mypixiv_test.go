package mypixiv

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewRegistersMyPixivLeaves(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	seen := map[string]bool{}
	for _, child := range cmd.Commands() {
		seen[child.Name()] = true
	}
	for _, name := range []string{"users", "works"} {
		if !seen[name] {
			t.Fatalf("mypixiv command missing %q", name)
		}
	}
}

func TestWorksAcceptsDocumentedArtworkEntityForCurrentUser(t *testing.T) {
	var path string
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		path = request.URL.Path
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"illusts":[],"next_url":null}`)),
		}, nil
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"works", "--type", "artwork", "--limit", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "/v2/illust/mypixiv" {
		t.Fatalf("request path = %q, want /v2/illust/mypixiv", path)
	}
}

func TestWorksUsesTypedSDKRoutes(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		path  string
		query map[string]string
		body  string
	}{
		{
			name: "current artwork",
			args: []string{"works", "--type", "artwork", "--json", "--limit", "1"},
			path: "/v2/illust/mypixiv",
			body: `{"illusts":[],"next_url":null}`,
		},
		{
			name: "current novel",
			args: []string{"works", "--type", "novel", "--json", "--limit", "1"},
			path: "/v1/novel/mypixiv",
			body: `{"novels":[],"next_url":null}`,
		},
		{
			name:  "user manga",
			args:  []string{"works", "123", "--type", "manga", "--json", "--limit", "1"},
			path:  "/v1/user/illusts",
			query: map[string]string{"user_id": "123", "type": "manga"},
			body:  `{"illusts":[],"next_url":null}`,
		},
		{
			name:  "user novel",
			args:  []string{"works", "123", "--type", "novel", "--json", "--limit", "1"},
			path:  "/v1/user/novels",
			query: map[string]string{"user_id": "123", "filter": "for_android"},
			body:  `{"novels":[],"next_url":null}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "app-api.pixiv.net" {
					t.Fatalf("request host = %q, want app-api.pixiv.net", request.URL.Host)
				}
				if request.URL.Path != test.path {
					t.Fatalf("request path = %q, want %q", request.URL.Path, test.path)
				}
				for key, want := range test.query {
					if got := request.URL.Query().Get(key); got != want {
						t.Fatalf("query %s = %q, want %q", key, got, want)
					}
				}
				if got := request.URL.Query().Get("offset"); got != "" {
					t.Fatalf("initial request offset = %q, want empty", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(test.body)),
				}, nil
			})
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}

			cmd := New(deps.Data{
				Input:      strings.NewReader(""),
				Output:     &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				JSONOut:    func(*bool) (bool, error) { return false, nil },
				Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					_, err := attempt(ctx, client)
					return err
				},
			})
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	}
}

func TestUsersUsesVerifiedIdentityAndFixedFilter(t *testing.T) {
	appRequests := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "oauth.secure.pixiv.net" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"access","refresh_token":"refresh","expires_in":3600,"user":{"id":42,"name":"tester"}}`)),
			}, nil
		}
		if request.URL.Host != "app-api.pixiv.net" {
			t.Fatalf("request host = %q, want app-api.pixiv.net", request.URL.Host)
		}
		if request.URL.Path != "/v1/user/mypixiv" {
			t.Fatalf("request path = %q, want /v1/user/mypixiv", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("user_id") != "42" {
			t.Fatalf("user_id = %q, want verified identity 42", query.Get("user_id"))
		}
		if query.Get("filter") != "for_android" {
			t.Fatalf("filter = %q, want for_android", query.Get("filter"))
		}
		if query.Get("offset") != "" {
			t.Fatalf("initial request offset = %q, want empty", query.Get("offset"))
		}
		appRequests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"user_previews":[],"next_url":null}`)),
		}, nil
	})
	client, _, err := pixiv.OpenWith(context.Background(), "refresh-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}

	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"users", "--json", "--limit", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if appRequests != 1 {
		t.Fatalf("MyPixiv app requests = %d, want 1", appRequests)
	}
}

func TestWorksRejectsUnsupportedTypeBeforeOpeningClient(t *testing.T) {
	pooledCalls := 0
	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		Pooled: func(context.Context, deps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			pooledCalls++
			return nil
		},
	})
	cmd.SetArgs([]string{"works", "--type", "user"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if pooledCalls != 0 {
		t.Fatalf("Pooled calls = %d, want 0", pooledCalls)
	}
}

func TestWorksRejectsInvalidUserIDBeforeOpeningClient(t *testing.T) {
	pooledCalls := 0
	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		Pooled: func(context.Context, deps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			pooledCalls++
			return nil
		},
	})
	cmd.SetArgs([]string{"works", "https://www.pixiv.net/users/123?secret=must-not-echo", "--type", "artwork"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatalf("error leaked user URL: %v", err)
	}
	if pooledCalls != 0 {
		t.Fatalf("Pooled calls = %d, want 0", pooledCalls)
	}
}

func TestUsersRequiresKnownCurrentUserIdentity(t *testing.T) {
	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"users", "--limit", "1"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.Unauthorized {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.Unauthorized, err)
	}
}
