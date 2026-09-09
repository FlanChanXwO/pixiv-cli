package user

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestUserDetailRejectsInvalidIDBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := New(Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
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
	cmd := New(Dependencies{
		Input:      strings.NewReader("artist\n"),
		Output:     output,
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
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
