package series

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

// artwork series URL 走公开 SDK；--limit 跨页时 NDJSON 按上游顺序逐条输出。
func TestSeriesArtworkURLUsesPublicSDK(t *testing.T) {
	output := &bytes.Buffer{}
	calls := 0
	transport := seriesRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body := `{"illust_series_detail":{"user":{"id":7,"name":"artist"}},"illusts":[{"id":5002,"title":"chapter","type":"manga","create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=5001&last_order=8"}`
		if calls == 2 {
			body = `{"illust_series_detail":{"user":{"id":7,"name":"artist"}},"illusts":[{"id":5003,"title":"next chapter","type":"illust","create_date":"2026-01-02T00:00:00Z","user":{"id":7,"name":"artist"}}],"next_url":null}`
		}
		return seriesJSONResponse(request, body), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"--type", "artwork", "https://www.pixiv.net/user/7/series/5001?ref=tracking", "--limit", "2", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute series URL: %v", err)
	}
	if calls != 2 {
		t.Fatalf("HTTP requests = %d, want 2", calls)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2; output=%q", len(lines), output.String())
	}
	for index, want := range []struct{ id, typ string }{{"5002", "manga"}, {"5003", "illust"}} {
		var record map[string]any
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode record %d: %v; output=%q", index, err, output.String())
		}
		if record["id"] != want.id || record["type"] != want.typ {
			t.Fatalf("record %d = %#v, want %s %s", index, record, want.typ, want.id)
		}
	}
}

func TestSeriesNovelURLContinuesAcrossPages(t *testing.T) {
	output := &bytes.Buffer{}
	calls := 0
	transport := seriesRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path != "/v2/novel/series" {
			t.Fatalf("path = %q, want /v2/novel/series", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("series_id") != "6001" {
			t.Fatalf("series_id = %q, want 6001", query.Get("series_id"))
		}
		wantLastOrder := ""
		if calls == 2 {
			wantLastOrder = "9"
		}
		if query.Get("last_order") != wantLastOrder {
			t.Fatalf("last_order = %q, want %q", query.Get("last_order"), wantLastOrder)
		}
		body := `{"novel_series_detail":{"id":6001,"title":"series","caption":"caption","is_concluded":true,"user":{"id":8,"name":"writer"}},"novels":[{"id":6002,"title":"first chapter","create_date":"2026-01-01T00:00:00Z","user":{"id":8,"name":"writer"}}],"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=6001&last_order=9"}`
		if calls == 2 {
			body = `{"novel_series_detail":{"id":6001,"title":"series","user":{"id":8,"name":"writer"}},"novels":[{"id":6003,"title":"second chapter","create_date":"2026-01-02T00:00:00Z","user":{"id":8,"name":"writer"}}],"next_url":null}`
		}
		return seriesJSONResponse(request, body), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"--type", "novel", "https://www.pixiv.net/novel/series/6001", "--limit", "2", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute novel series URL: %v", err)
	}
	if calls != 2 {
		t.Fatalf("HTTP requests = %d, want 2", calls)
	}
	var envelope struct {
		Series struct {
			ID int64 `json:"id"`
		} `json:"series"`
		Novels []struct {
			ID int64 `json:"id"`
		} `json:"novels"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if envelope.Series.ID != 6001 || len(envelope.Novels) != 2 || envelope.Novels[0].ID != 6002 || envelope.Novels[1].ID != 6003 {
		t.Fatalf("series envelope = %#v, want series 6001 with novels 6002 and 6003", envelope)
	}
}

func TestSeriesRejectsUnsupportedTypeBeforeParsingInput(t *testing.T) {
	for _, typ := range []string{"all", "bogus"} {
		t.Run(typ, func(t *testing.T) {
			opened := false
			cmd := newTestCommand(testData{
				Input:       strings.NewReader(""),
				Output:      &bytes.Buffer{},
				ErrorOutput: &bytes.Buffer{},
				UsageError:  func(err error) error { return err },
				Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					opened = true
					return nil
				},
			})
			cmd.SetArgs([]string{"--type", typ, "not-a-series"})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "type must be one of artwork, novel") {
				t.Fatalf("expected unsupported series type error, got %v", err)
			}
			if opened {
				t.Fatal("opened SDK client before validating series type")
			}
		})
	}
}

func TestSeriesRejectsURLTypeMismatchBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"--type", "novel", "https://www.pixiv.net/user/7/series/5001"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected invalid URL/type combination, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating series URL/type combination")
	}
}

type seriesRoundTripFunc func(*http.Request) (*http.Response, error)

func (f seriesRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func seriesJSONResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
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
