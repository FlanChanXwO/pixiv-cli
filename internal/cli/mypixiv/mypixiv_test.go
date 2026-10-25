package mypixiv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// works 的 type 分派只在 CLI 层验证：四个 kind 组合都必须发起一次 SDK 请求并输出对应 JSON key。
func TestWorksUsesTypedSDKRoutes(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		body string
		key  string
	}{
		{name: "current artwork", args: []string{"works", "--type", "artwork", "--json", "--limit", "1"}, body: `{"illusts":[],"next_url":null}`, key: "illusts"},
		{name: "current novel", args: []string{"works", "--type", "novel", "--json", "--limit", "1"}, body: `{"novels":[],"next_url":null}`, key: "novels"},
		{name: "user manga", args: []string{"works", "123", "--type", "manga", "--json", "--limit", "1"}, body: `{"illusts":[],"next_url":null}`, key: "illusts"},
		{name: "user novel", args: []string{"works", "123", "--type", "novel", "--json", "--limit", "1"}, body: `{"novels":[],"next_url":null}`, key: "novels"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
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
			output := &bytes.Buffer{}
			cmd := newTestCommand(testData{
				Input: strings.NewReader(""), Output: output,
				UsageError: func(err error) error { return err },
				JSONOut:    func(*bool) (bool, error) { return true, nil },
				Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					_, err := attempt(ctx, client)
					return err
				},
			})
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want 1", calls)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode output: %v; output=%q", err, output.String())
			}
			if _, ok := payload[test.key]; !ok {
				t.Fatalf("missing %q in output: %q", test.key, output.String())
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

	cmd := newTestCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
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
	cmd := newTestCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
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
	cmd := newTestCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
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
	cmd := newTestCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
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

type testData struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	JSONOut     func(*bool) (bool, error)
	Pooled      func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return New(env, pixivcmd.Executor(d.Pooled))
}
