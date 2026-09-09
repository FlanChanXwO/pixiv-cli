package timeline

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestFollowingArtworkContentTypeFiltersAcrossPages(t *testing.T) {
	output := &bytes.Buffer{}
	var requests []*http.Request
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request)
		if request.URL.Path != "/v2/illust/follow" {
			return nil, io.ErrUnexpectedEOF
		}
		if request.URL.Query().Get("restrict") != "public" {
			t.Fatalf("restrict = %q, want public", request.URL.Query().Get("restrict"))
		}
		if _, present := request.URL.Query()["content_type"]; present {
			t.Fatalf("following artwork request must not send content_type: %v", request.URL.Query())
		}
		var body string
		switch request.URL.Query().Get("offset") {
		case "":
			body = `{"illusts":[` +
				`{"id":8101,"title":"illustration one","type":"illust","create_date":"2026-01-05T00:00:00Z","user":{"id":21,"name":"artist"}},` +
				`{"id":8102,"title":"manga one","type":"manga","create_date":"2026-01-06T00:00:00Z","user":{"id":22,"name":"mangaka"}}` +
				`],"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=public&offset=30"}`
		case "30":
			body = `{"illusts":[` +
				`{"id":8103,"title":"manga two","type":"manga","create_date":"2026-01-07T00:00:00Z","user":{"id":23,"name":"mangaka two"}},` +
				`{"id":8104,"title":"illustration two","type":"illust","create_date":"2026-01-08T00:00:00Z","user":{"id":24,"name":"artist two"}}` +
				`],"next_url":null}`
		default:
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := New(deps.Data{
		Input:      strings.NewReader(""),
		Output:     output,
		UsageError: func(err error) error { return err },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"following", "--type", "artwork", "--content-type", "manga", "--limit", "2", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("HTTP requests = %d, want 2; output=%q", len(requests), output.String())
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2; output=%q", len(lines), output.String())
	}
	for index, wantID := range []string{"8102", "8103"} {
		var record map[string]any
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode record %d: %v; output=%q", index, err, output.String())
		}
		if record["id"] != wantID || record["type"] != "manga" {
			t.Fatalf("record %d = %#v, want manga %s", index, record, wantID)
		}
	}
}

func TestFollowingNovelRejectsContentTypeBeforeOpeningClient(t *testing.T) {
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
	cmd.SetArgs([]string{"following", "--type", "novel", "--content-type", "manga", "--limit", "1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--content-type is only supported when --type artwork") {
		t.Fatalf("expected novel content-type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating novel content-type")
	}
}

func TestLatestNovelRejectsContentTypeBeforeOpeningClient(t *testing.T) {
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
	cmd.SetArgs([]string{"latest", "--type", "novel", "--content-type", "manga", "--limit", "1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--content-type is only supported when --type artwork") {
		t.Fatalf("expected novel content-type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating novel content-type")
	}
}

func TestFollowingArtworkRejectsUnsupportedContentTypeBeforeOpeningClient(t *testing.T) {
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
	cmd.SetArgs([]string{"following", "--type", "artwork", "--content-type", "unknown", "--limit", "1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "content-type must be one of") {
		t.Fatalf("expected artwork content-type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating following artwork content-type")
	}
}
