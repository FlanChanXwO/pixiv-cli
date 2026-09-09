package ranking

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

func TestNewDeclaresRankingInputAndOutputFlags(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "ranking" {
		t.Fatalf("unexpected ranking use: %q", cmd.Use)
	}
	for _, name := range []string{"type", "mode", "date", "limit", "page", "ndjson"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("ranking command missing flag %q", name)
		}
	}
}

func TestNovelRankingUsesTypedRouteAndContinuesAcrossPages(t *testing.T) {
	output := &bytes.Buffer{}
	var requests []string
	transport := rankingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/novel/ranking" {
			t.Fatalf("ranking path = %q, want novel ranking", request.URL.Path)
		}
		if request.URL.Query().Get("filter") != "for_android" || request.URL.Query().Get("mode") != "week" {
			t.Fatalf("novel ranking query = %v", request.URL.Query())
		}
		requests = append(requests, request.URL.Query().Get("offset"))
		switch request.URL.Query().Get("offset") {
		case "":
			return rankingJSONResponse(request, `{"novels":[{"id":9301,"title":"first novel","create_date":"2026-09-01T00:00:00Z","user":{"id":31,"name":"writer one"}}],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=week&offset=30"}`), nil
		case "30":
			return rankingJSONResponse(request, `{"novels":[{"id":9302,"title":"second novel","create_date":"2026-09-02T00:00:00Z","user":{"id":32,"name":"writer two"}}],"next_url":null}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := New(deps.Data{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"--type", "novel", "--mode", "week", "--limit", "2", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := requests, []string{"", "30"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("novel ranking offsets = %v, want %v", got, want)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2; output=%q", len(lines), output.String())
	}
	for index, wantID := range []string{"9301", "9302"} {
		var record map[string]any
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode record %d: %v; output=%q", index, err, output.String())
		}
		if record["id"] != wantID || record["type"] != "novel" || record["url"] != "https://www.pixiv.net/novel/show.php?id="+wantID {
			t.Fatalf("record %d = %#v, want novel %s", index, record, wantID)
		}
	}
}

func TestArtworkRankingDefaultPreservesLegacyRoute(t *testing.T) {
	output := &bytes.Buffer{}
	transport := rankingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/illust/ranking" {
			t.Fatalf("ranking path = %q, want artwork ranking", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("mode") != "week" || query.Get("date") != "2026-09-01" || query.Get("offset") != "" {
			t.Fatalf("artwork ranking query = %v", query)
		}
		return rankingJSONResponse(request, `{"illusts":[{"id":9401,"title":"ranked artwork","type":"illust","create_date":"2026-09-01T00:00:00Z","user":{"id":41,"name":"artist"}}],"next_url":null}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := New(deps.Data{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"--mode", "week", "--date", "2026-09-01", "--limit", "1", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &record); err != nil {
		t.Fatalf("decode artwork record: %v; output=%q", err, output.String())
	}
	if record["id"] != "9401" || record["type"] != "illustration" {
		t.Fatalf("artwork record = %#v, want illustration 9401", record)
	}
}

func TestNovelRankingRejectsDateBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := New(deps.Data{
		Input:       strings.NewReader(""),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, deps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"--type", "novel", "--date", "2026-09-01", "--limit", "1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--date is only supported when --type artwork") {
		t.Fatalf("expected novel date validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating novel ranking date")
	}
}

type rankingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f rankingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func rankingJSONResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
