package user

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

func TestUserDetailRejectsInvalidIDBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"detail", "not-an-id"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "user_id") {
		t.Fatalf("expected user ID validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating user ID")
	}
}

func TestUserArtworksRejectsUnsupportedTypeBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"artworks", "123", "--type", "all"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "type") {
		t.Fatalf("expected unsupported artwork type error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating artwork type")
	}
}

func TestUserDetailClassifiesURLInputAsInvalidArgument(t *testing.T) {
	opened := false
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"detail", "https://www.pixiv.net/users/123"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if opened {
		t.Fatal("opened SDK client before validating strict user ID input")
	}
}

func TestUserFollowingRejectsUnsupportedRestrictBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"following", "123", "--restrict", "all"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "restrict") {
		t.Fatalf("expected unsupported restrict error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating relationship restrict")
	}
}

func TestUserArtworksClassifiesURLInputAsInvalidArgument(t *testing.T) {
	opened := false
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"artworks", "https://www.pixiv.net/users/123"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if opened {
		t.Fatal("opened SDK client before validating strict user ID input")
	}
}

func TestUserArtworksRequiresKnownCurrentUserForOmittedID(t *testing.T) {
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			client, err := pixiv.NewWith("token", pixiv.Options{})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"artworks"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.Unauthorized {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.Unauthorized, err)
	}
}

// 每条命令只验证 CLI 自己的 flag → 请求字段映射与 JSON 外形；上游路由与响应字段归 SDK。
func TestUserReadCommandsUseTypedSDKRoutes(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		query   map[string]string
		body    string
		jsonKey string
	}{
		{
			name:    "detail",
			args:    []string{"detail", "123", "--json"},
			body:    `{"user":{"id":123,"name":"artist"},"profile":{},"profile_publicity":{"gender":false,"region":false,"birth_day":false,"birth_year":false,"job":false,"pawoo":false},"workspace":{}}`,
			jsonKey: "user",
		},
		{
			name:    "artworks type flag",
			args:    []string{"artworks", "123", "--type", "illust", "--json", "--limit", "1"},
			query:   map[string]string{"type": "illust"},
			body:    `{"illusts":[{"id":7101,"title":"artwork","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":123,"name":"artist"},"tags":[]}],"next_url":null}`,
			jsonKey: "illusts",
		},
		{
			name:    "following restrict flag",
			args:    []string{"following", "123", "--json", "--limit", "1"},
			query:   map[string]string{"restrict": "public"},
			body:    `{"user_previews":[{"user":{"id":7301,"name":"followed"}}],"next_url":null}`,
			jsonKey: "user_previews",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			transport := userSearchTransport(func(request *http.Request) (*http.Response, error) {
				for key, want := range test.query {
					if got := request.URL.Query().Get(key); got != want {
						t.Fatalf("query %s = %q, want %q", key, got, want)
					}
				}
				return jsonHTTPResponse(request, test.body), nil
			})
			cmd := newTestUserCommand(testData{
				Input:      strings.NewReader(""),
				Output:     output,
				UsageError: func(err error) error { return err },
				JSONOut: func(override *bool) (bool, error) {
					if override != nil {
						return *override, nil
					}
					return false, nil
				},
				Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
					if err != nil {
						return err
					}
					_, err = attempt(ctx, client)
					return err
				},
			})
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode output: %v; output=%q", err, output.String())
			}
			if _, ok := payload[test.jsonKey]; !ok {
				t.Fatalf("missing %q in output: %q", test.jsonKey, output.String())
			}
		})
	}
}

func TestUserRelationshipTextOutputEscapesControlBytes(t *testing.T) {
	output := &bytes.Buffer{}
	transport := userSearchTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/user/following" {
			t.Fatalf("path = %q, want /v1/user/following", request.URL.Path)
		}
		return jsonHTTPResponse(request, `{"user_previews":[{"user":{"id":7701,"name":"line\ninjected"}}],"next_url":null}`), nil
	})
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader(""),
		Output:     output,
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"following", "123", "--limit", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "users followed by 123\n7701 line\\ninjected\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

// 一条 artwork 列表 + 一条 user-preview 列表足以覆盖 CLI 的跨页 --limit；其余端点共用同一分页机制。
func TestUserListCommandsContinueAcrossUpstreamPages(t *testing.T) {
	for _, test := range []struct {
		name       string
		args       []string
		firstBody  string
		secondBody string
		jsonKey    string
	}{
		{
			name:       "artworks",
			args:       []string{"artworks", "123", "--type", "illust", "--json", "--limit", "2"},
			firstBody:  `{"illusts":[{"id":7101,"title":"first","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":123,"name":"artist"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/user/illusts?user_id=123&type=illust&offset=20"}`,
			secondBody: `{"illusts":[{"id":7102,"title":"second","type":"illust","create_date":"2026-01-02T00:00:00Z","user":{"id":123,"name":"artist"},"tags":[]}],"next_url":null}`,
			jsonKey:    "illusts",
		},
		{
			name:       "following",
			args:       []string{"following", "123", "--json", "--limit", "2"},
			firstBody:  `{"user_previews":[{"user":{"id":7301,"name":"first"}}],"next_url":"https://app-api.pixiv.net/v1/user/following?user_id=123&restrict=public&offset=20"}`,
			secondBody: `{"user_previews":[{"user":{"id":7302,"name":"second"}}],"next_url":null}`,
			jsonKey:    "user_previews",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			requests := 0
			transport := userSearchTransport(func(request *http.Request) (*http.Response, error) {
				requests++
				if got := request.URL.Query().Get("offset"); got != map[bool]string{true: "20"}[requests == 2] {
					t.Fatalf("request %d offset = %q", requests, got)
				}
				body := test.firstBody
				if requests == 2 {
					body = test.secondBody
				}
				return jsonHTTPResponse(request, body), nil
			})
			cmd := newTestUserCommand(testData{
				Input:      strings.NewReader(""),
				Output:     output,
				UsageError: func(err error) error { return err },
				JSONOut: func(override *bool) (bool, error) {
					if override != nil {
						return *override, nil
					}
					return false, nil
				},
				Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
					if err != nil {
						return err
					}
					_, err = attempt(ctx, client)
					return err
				},
			})
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if requests != 2 {
				t.Fatalf("HTTP requests = %d, want 2", requests)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode output: %v; output=%q", err, output.String())
			}
			var items []json.RawMessage
			if err := json.Unmarshal(payload[test.jsonKey], &items); err != nil {
				t.Fatalf("decode %s: %v", test.jsonKey, err)
			}
			if len(items) != 2 {
				t.Fatalf("%s item count = %d, want 2", test.name, len(items))
			}
		})
	}
}

func TestLegacyUserSearchReadsWordFromStdinAndEmitsNDJSONAcrossBatches(t *testing.T) {
	output := &bytes.Buffer{}
	requests := 0
	transport := userSearchTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Path != "/v1/search/user" {
			t.Fatalf("path = %q, want /v1/search/user", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("word") != "artist" {
			t.Fatalf("word = %q, want artist", query.Get("word"))
		}
		wantOffset := ""
		body := `{"user_previews":[{"user":{"id":3001,"name":"artist","account":"artist","comment":"first"}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`
		if requests == 2 {
			wantOffset = "20"
			body = `{"user_previews":[{"user":{"id":3002,"name":"artist two","account":"artist2","comment":"second"}}],"next_url":null}`
		}
		if query.Get("offset") != wantOffset {
			t.Fatalf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		return jsonHTTPResponse(request, body), nil
	})
	cmd := newTestUserCommand(testData{
		Input:      strings.NewReader("artist\n"),
		Output:     output,
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"search", "--limit", "2", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute legacy user search: %v", err)
	}
	if requests != 2 {
		t.Fatalf("HTTP requests = %d, want 2", requests)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2; output=%q", len(lines), output.String())
	}
	for index, wantID := range []string{"3001", "3002"} {
		var record map[string]any
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode NDJSON line %d: %v; output=%q", index, err, output.String())
		}
		if record["id"] != wantID || record["type"] != "user" {
			t.Fatalf("record %d = %#v, want user %s", index, record, wantID)
		}
	}
}

type userSearchTransport func(*http.Request) (*http.Response, error)

func (f userSearchTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

type testData struct {
	Input      io.Reader
	Output     io.Writer
	UsageError func(error) error
	JSONOut    func(*bool) (bool, error)
	Pooled     func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	Follow     func() *cobra.Command
}

func newTestUserCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return New(env, pixivcmd.New(d.Output, pixivcmd.Executor(d.Pooled)), d.Follow)
}
