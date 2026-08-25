package timeline

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewRegistersTimelineLeaves(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	seen := map[string]bool{}
	for _, child := range cmd.Commands() {
		seen[child.Name()] = true
	}
	for _, name := range []string{"following", "latest"} {
		if !seen[name] {
			t.Fatalf("timeline command missing %q", name)
		}
	}
}

func TestLatestArtworkDefaultsToSupportedIllustContentType(t *testing.T) {
	var contentType string
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		contentType = request.URL.Query().Get("content_type")
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
	cmd.SetArgs([]string{"latest", "--type", "artwork", "--limit", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if contentType != "illust" {
		t.Fatalf("content_type = %q, want illust", contentType)
	}
}

func TestLatestArtworkRejectsUnsupportedContentTypeBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, deps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"latest", "--type", "artwork", "--content-type", "all", "--limit", "1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "content-type must be one of: illust, manga") {
		t.Fatalf("expected content-type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating latest artwork content type")
	}
}
