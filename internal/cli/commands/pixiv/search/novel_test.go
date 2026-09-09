package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

func TestNovelSearchRejectsTagTitleCaptionForCanonicalAndLegacyRoutes(t *testing.T) {
	tests := []struct {
		name string
		new  func(Dependencies) *cobra.Command
		args []string
	}{
		{
			name: "canonical",
			new: func(data Dependencies) *cobra.Command {
				return New(data)
			},
			args: []string{"cat", "--type", "novel", "--search-by", searchTargetTagTitleCaption},
		},
		{
			name: "legacy",
			new: func(data Dependencies) *cobra.Command {
				return NewNovel(data)
			},
			args: []string{"search", "cat", "--search-by", searchTargetTagTitleCaption},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			cmd := tt.new(Dependencies{
				Input:      strings.NewReader(""),
				Output:     &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				JSONOut:    func(*bool) (bool, error) { return false, nil },
				Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					opened = true
					return nil
				},
			})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "search-by must be one of tag-partial, tag-exact, title-caption") {
				t.Fatalf("expected unsupported novel search target error, got %v", err)
			}
			if opened {
				t.Fatal("opened SDK client before validating novel search target")
			}
		})
	}
}

func TestCanonicalNovelSearchUsesRoutePeriodAndLogicalPaginationJSON(t *testing.T) {
	output := &bytes.Buffer{}
	requests := 0
	transport := searchFixtureTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Path != "/v1/search/novel" {
			t.Fatalf("path = %q, want /v1/search/novel", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("word") != "cat" {
			t.Fatalf("word = %q, want cat", query.Get("word"))
		}
		if query.Get("search_target") != string(pixiv.SearchTargetPartialMatchForTags) {
			t.Fatalf("search_target = %q, want %q", query.Get("search_target"), pixiv.SearchTargetPartialMatchForTags)
		}
		if query.Get("sort") != string(pixiv.SortModeDateDesc) {
			t.Fatalf("sort = %q, want %q", query.Get("sort"), pixiv.SortModeDateDesc)
		}
		if query.Get("duration") != string(pixiv.DurationLastWeek) {
			t.Fatalf("duration = %q, want %q", query.Get("duration"), pixiv.DurationLastWeek)
		}
		wantOffset := ""
		if requests == 2 {
			wantOffset = "30"
		}
		if query.Get("offset") != wantOffset {
			t.Fatalf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		nextURL := ""
		if requests == 1 {
			nextURL = "https://app-api.pixiv.net/v1/search/novel?offset=30"
		}
		return novelSearchResponse(2000+requests, fmt.Sprintf("novel %d", requests), nextURL)
	})
	cmd := New(Dependencies{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled:      novelSearchPooled(transport),
	})
	cmd.SetArgs([]string{"cat", "--type", "novel", "--period", "week", "--limit", "2", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute canonical novel search: %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	var envelope struct {
		Novels []struct {
			ID int64 `json:"id"`
		} `json:"novels"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if len(envelope.Novels) != 2 || envelope.Novels[0].ID != 2001 || envelope.Novels[1].ID != 2002 {
		t.Fatalf("novels = %#v, want IDs 2001 and 2002", envelope.Novels)
	}
}

func TestLegacyNovelSearchReadsStdinAndEmitsNDJSON(t *testing.T) {
	output := &bytes.Buffer{}
	var requestQuery map[string]string
	transport := searchFixtureTransport(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		requestQuery = map[string]string{
			"word":          query.Get("word"),
			"search_target": query.Get("search_target"),
			"sort":          query.Get("sort"),
			"duration":      query.Get("duration"),
			"offset":        query.Get("offset"),
		}
		return novelSearchResponse(2003, "stdin novel", "")
	})
	cmd := NewNovel(Dependencies{
		Input:       strings.NewReader("stdin novel\n"),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut: func(*bool) (bool, error) {
			t.Fatal("resolved JSON output for --ndjson")
			return false, nil
		},
		Pooled: novelSearchPooled(transport),
	})
	cmd.SetArgs([]string{"search", "--search-by", "tag-exact", "--sort", "date_asc", "--period", "month", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute legacy novel search: %v", err)
	}
	wantQuery := map[string]string{
		"word":          "stdin novel",
		"search_target": string(pixiv.SearchTargetExactMatchForTags),
		"sort":          string(pixiv.SortModeDateAsc),
		"duration":      string(pixiv.DurationLastMonth),
		"offset":        "",
	}
	for key, want := range wantQuery {
		if requestQuery[key] != want {
			t.Fatalf("query[%q] = %q, want %q; query=%v", key, requestQuery[key], want, requestQuery)
		}
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode NDJSON output: %v; output=%q", err, output.String())
	}
	if record["id"] != "2003" || record["type"] != "novel" || record["title"] != "stdin novel" {
		t.Fatalf("record = %#v, want novel 2003", record)
	}
}

func TestNovelSearchRejectsUnsupportedPeriodsBeforeOpeningClient(t *testing.T) {
	tests := []struct {
		name string
		new  func(Dependencies) *cobra.Command
		args []string
	}{
		{
			name: "canonical half-year",
			new: func(data Dependencies) *cobra.Command {
				return New(data)
			},
			args: []string{"cat", "--type", "novel", "--period", "half-year"},
		},
		{
			name: "legacy year",
			new: func(data Dependencies) *cobra.Command {
				return NewNovel(data)
			},
			args: []string{"search", "cat", "--period", "year"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			cmd := tt.new(Dependencies{
				Input:      strings.NewReader(""),
				Output:     &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					opened = true
					return nil
				},
			})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil || err.Error() != "novel period must be one of day, week, month" {
				t.Fatalf("expected unsupported novel period error, got %v", err)
			}
			if opened {
				t.Fatal("opened SDK client before validating novel period")
			}
		})
	}
}

func TestCanonicalNovelSearchRejectsDateFlagsBeforeOpeningClient(t *testing.T) {
	for _, flag := range []string{"--start-date", "--end-date"} {
		t.Run(flag, func(t *testing.T) {
			opened := false
			cmd := New(Dependencies{
				Input:      strings.NewReader(""),
				Output:     &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					opened = true
					return nil
				},
			})
			cmd.SetArgs([]string{"cat", "--type", "novel", flag, "2026-01-01"})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), flag+" is not supported when --type novel") {
				t.Fatalf("expected unsupported date flag error, got %v", err)
			}
			if opened {
				t.Fatal("opened SDK client before validating novel date flag")
			}
		})
	}
}

func novelSearchPooled(transport http.RoundTripper) func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
	return func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
		if err != nil {
			return err
		}
		_, err = attempt(ctx, client)
		return err
	}
}

func novelSearchResponse(id int, title, nextURL string) (*http.Response, error) {
	next := "null"
	if nextURL != "" {
		next = fmt.Sprintf("%q", nextURL)
	}
	body := fmt.Sprintf(`{"novels":[{"id":%d,"title":%q,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"writer"},"x_restrict":0,"text_length":12,"is_original":true,"tags":[]}],"next_url":%s}`, id, title, next)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}
