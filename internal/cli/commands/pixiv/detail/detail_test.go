package detail

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

func TestCommandRejectsUnsupportedURLBeforeOpeningClient(t *testing.T) {
	opened := false
	data := Dependencies{
		Input:        strings.NewReader(""),
		Output:       &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"https://www.pixiv.net/users/7?secret=must-not-echo"})
	err := cmd.Execute()

	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected unsupported URL error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before entity input validation")
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatal("unsupported URL query leaked into the error")
	}
}

func TestCommandRejectsNovelURLBeforeOpeningClient(t *testing.T) {
	opened := false
	data := Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			return Request{}, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--type", "novel", "https://www.pixiv.net/novel/show.php?id=7"})
	err := cmd.Execute()

	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected novel URL validation error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before novel URL validation")
	}
}

func TestCommandNovelContentReturnsUnavailableBeforeOpeningClient(t *testing.T) {
	built := false
	opened := false
	data := Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			built = true
			return Request{}, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--type", "novel", "--content", "42"})
	err := cmd.Execute()

	if sdk.ReasonOf(err) != sdk.ContentUnavailable {
		t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ContentUnavailable, err)
	}
	if built {
		t.Fatal("request was built before reporting unavailable novel content")
	}
	if opened {
		t.Fatal("client was opened before reporting unavailable novel content")
	}
}

func TestCommandRoutesTypedIDsThroughPublicSDK(t *testing.T) {
	tests := []struct {
		name      string
		typ       string
		id        string
		path      string
		queryName string
		body      string
	}{
		{
			name:      "artwork",
			typ:       "artwork",
			id:        "101",
			path:      "/v1/illust/detail",
			queryName: "illust_id",
			body:      `{"illust":{"id":101,"title":"artwork","type":"illust","create_date":"2024-01-01T00:00:00Z","page_count":1,"image_urls":{"original":"https://i.pximg.net/img/101.png"},"user":{"id":9,"name":"artist"},"tags":[]}}`,
		},
		{
			name:      "novel",
			typ:       "novel",
			id:        "201",
			path:      "/v2/novel/detail",
			queryName: "novel_id",
			body:      `{"novel":{"id":201,"title":"novel","create_date":"2024-01-01T00:00:00Z","user":{"id":9,"name":"writer"}}}`,
		},
		{
			name:      "user",
			typ:       "user",
			id:        "301",
			path:      "/v1/user/detail",
			queryName: "user_id",
			body:      `{"user":{"id":301,"name":"user"},"profile":{},"profile_publicity":{"gender":false,"region":false,"birth_day":false,"birth_year":false,"job":false,"pawoo":false},"workspace":{}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotPath string
			var gotID string
			cmd := New(Dependencies{
				Input:      strings.NewReader(""),
				Output:     &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				BuildRequest: func(*cobra.Command, Options) (Request, error) {
					return Request{}, nil
				},
				Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: detailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						gotPath = req.URL.Path
						gotID = req.URL.Query().Get(test.queryName)
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": {"application/json"}},
							Body:       io.NopCloser(strings.NewReader(test.body)),
							Request:    req,
						}, nil
					})}})
					if err != nil {
						return err
					}
					_, err = attempt(ctx, client)
					return err
				},
				JSONOut: func(*bool) (bool, error) { return false, nil },
			})
			cmd.SetArgs([]string{"--type", test.typ, test.id})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if gotPath != test.path || gotID != test.id {
				t.Fatalf("request = %s?%s, want %s?%s", gotPath, gotID, test.path, test.queryName+"="+test.id)
			}
		})
	}
}

type detailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f detailRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCommandRejectsURLTypeConflictBeforeBuildingRequest(t *testing.T) {
	built := false
	opened := false
	cmd := New(Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			built = true
			return Request{}, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	})
	cmd.SetArgs([]string{"--type", "novel", "https://www.pixiv.net/artworks/42?secret=must-not-echo"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if built || opened {
		t.Fatal("URL/type conflict reached request construction or client execution")
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatal("URL query leaked into the conflict error")
	}
}

func TestCommandRejectsContentForNonNovelBeforeOpeningClient(t *testing.T) {
	opened := false
	data := Dependencies{
		Input:        strings.NewReader(""),
		Output:       &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"42", "--content"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--content is only supported") {
		t.Fatalf("expected content validation error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before option validation")
	}
}
