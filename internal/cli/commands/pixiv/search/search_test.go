package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestCommandRejectsInvalidFilterBeforeOpeningClient(t *testing.T) {
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
	cmd.SetArgs([]string{"miku", "--resolution", "impossible"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "resolution must be one of") {
		t.Fatalf("expected resolution validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating search filters")
	}
}

func TestCommandUsesReverseSearchForHTTPSourceWithoutOpeningPixivSDK(t *testing.T) {
	opened := false
	reverseCalls := 0
	const source = "https://example.test/image.jpg"
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		JSONOut: func(*bool) (bool, error) {
			opened = true
			return false, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		ReverseSearch: func(_ context.Context, request ReverseSearchRequest) (reversesearch.Response, error) {
			reverseCalls++
			if request.Source != source {
				t.Fatalf("reverse search source = %q, want %q", request.Source, source)
			}
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{source, "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	if reverseCalls != 1 {
		t.Fatalf("reverse search calls = %d, want 1", reverseCalls)
	}
	if opened {
		t.Fatal("opened Pixiv SDK or JSON resolver for image search")
	}
}

func TestCommandPassesReverseSearchProviderAndProxyOverride(t *testing.T) {
	var captured ReverseSearchRequest
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			t.Fatal("opened Pixiv SDK for image search")
			return nil
		},
		ReverseSearch: func(_ context.Context, request ReverseSearchRequest) (reversesearch.Response, error) {
			captured = request
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{
		"https://example.test/image.jpg",
		"--provider", "ascii2d-color",
		"--proxy", "http://127.0.0.1:7890",
		"--ndjson",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	if captured.Provider != reversesearch.ProviderASCII2DColor {
		t.Fatalf("provider = %q, want %q", captured.Provider, reversesearch.ProviderASCII2DColor)
	}
	if captured.HTTPSProxyOverride == nil || *captured.HTTPSProxyOverride != "http://127.0.0.1:7890" {
		t.Fatalf("proxy override = %v, want configured proxy", captured.HTTPSProxyOverride)
	}
}

func TestCommandUsesReverseSearchForRegularFile(t *testing.T) {
	path := t.TempDir() + "/image.jpg"
	if err := os.WriteFile(path, []byte("not-an-image-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			t.Fatal("opened Pixiv SDK for regular-file image search")
			return nil
		},
		ReverseSearch: func(_ context.Context, request ReverseSearchRequest) (reversesearch.Response, error) {
			called = true
			if request.Source != path {
				t.Fatalf("reverse search source = %q, want %q", request.Source, path)
			}
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{path, "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	if !called {
		t.Fatal("did not use reverse search for a regular file")
	}
}

func TestCommandDoesNotFallbackInvalidHTTPSourceToKeywordSearch(t *testing.T) {
	called := false
	opened := false
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		ReverseSearch: func(_ context.Context, request ReverseSearchRequest) (reversesearch.Response, error) {
			called = true
			if request.Source != "https://[invalid" {
				t.Fatalf("reverse search source = %q", request.Source)
			}
			return reversesearch.Response{}, reversesearch.NewError(reversesearch.CodeInvalidSource, "reverse search source is invalid", nil)
		},
	})
	cmd.SetArgs([]string{"https://[invalid", "--ndjson"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "reverse search source is invalid") {
		t.Fatalf("expected invalid source error, got %v", err)
	}
	if !called {
		t.Fatal("did not send invalid HTTP source to reverse search")
	}
	if opened {
		t.Fatal("fell back to Pixiv keyword search for invalid HTTP source")
	}
}

func TestCommandJSONOutputPreservesStableSolverErrorAndSafeCause(t *testing.T) {
	const secret = "solver-secret source-secret csrf-secret"
	output := &bytes.Buffer{}
	response := reversesearch.Response{
		Input:     reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"},
		Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusError}},
		Results:   []reversesearch.Result{},
		ProviderErrors: []reversesearch.ProviderError{{
			Provider: reversesearch.ProviderASCII2DColor,
			Code:     reversesearch.CodeSolverUnavailable,
			Message:  "ascii2d challenge solver is unavailable",
		}},
	}
	cmd := New(Dependencies{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			return response, reversesearch.NewError(reversesearch.CodeSolverUnavailable, "ascii2d challenge solver is unavailable", errors.New(secret))
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--json"})

	err := cmd.Execute()
	if err == nil || err.Error() != "ascii2d challenge solver is unavailable" {
		t.Fatalf("execute reverse search error = %v", err)
	}
	var envelope struct {
		Providers      []reversesearch.ProviderSummary `json:"providers"`
		Results        []reversesearch.Result          `json:"results"`
		ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	}
	if decodeErr := json.Unmarshal(output.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode JSON output: %v; output=%q", decodeErr, output.String())
	}
	if len(envelope.Providers) != 1 || len(envelope.Results) != 0 || len(envelope.ProviderErrors) != 1 {
		t.Fatalf("unexpected solver error envelope: %+v", envelope)
	}
	if envelope.ProviderErrors[0].Code != reversesearch.CodeSolverUnavailable {
		t.Fatalf("provider error code = %q, want %q", envelope.ProviderErrors[0].Code, reversesearch.CodeSolverUnavailable)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(err.Error(), secret) {
		t.Fatalf("solver error leaked private cause: output=%q err=%q", output.String(), err)
	}
}

func TestCommandJSONOutputContainsReverseSearchEnvelopeAndRecords(t *testing.T) {
	output := &bytes.Buffer{}
	response := reversesearch.Response{
		Input: reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"},
		Providers: []reversesearch.ProviderSummary{{
			Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1,
		}},
		Results: []reversesearch.Result{
			{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 42}, Title: "Miku", Author: "artist"},
			{Title: "external only", Author: "other"},
		},
		ProviderErrors: []reversesearch.ProviderError{{
			Provider: reversesearch.ProviderASCII2DColor, Code: reversesearch.CodeProviderFailed, Message: "provider failed",
		}},
		Partial: true,
	}
	cmd := New(Dependencies{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		JSONOut: func(*bool) (bool, error) {
			return true, nil
		},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			return response, nil
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	var envelope struct {
		Input          reversesearch.Input             `json:"input"`
		Providers      []reversesearch.ProviderSummary `json:"providers"`
		Results        []reversesearch.Result          `json:"results"`
		Records        []map[string]any                `json:"records"`
		ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
		Partial        bool                            `json:"partial"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if envelope.Input != response.Input || len(envelope.Providers) != 1 || len(envelope.Results) != 2 || !envelope.Partial {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if len(envelope.Records) != 1 || envelope.Records[0]["id"] != "42" || envelope.Records[0]["type"] != "artwork" || envelope.Records[0]["url"] != "https://www.pixiv.net/artworks/42" {
		t.Fatalf("unexpected records: %+v", envelope.Records)
	}
	if len(envelope.ProviderErrors) != 1 || envelope.ProviderErrors[0].Provider != reversesearch.ProviderASCII2DColor {
		t.Fatalf("unexpected provider errors: %+v", envelope.ProviderErrors)
	}
}

func TestCommandNDJSONOutputsOnlyRecordsAndWarnsOnPartial(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	response := reversesearch.Response{
		Input: reversesearch.Input{Kind: reversesearch.SourceKindFile, SHA256: "cafebabe"},
		Providers: []reversesearch.ProviderSummary{
			{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
			{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusError},
		},
		Results:        []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 7}, Title: "User"}},
		ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderASCII2DColor, Code: reversesearch.CodeProviderFailed, Message: "provider failed"}},
		Partial:        true,
	}
	cmd := New(Dependencies{
		Input:         strings.NewReader(""),
		Output:        output,
		ErrorOutput:   errorOutput,
		JSONOut:       func(*bool) (bool, error) { t.Fatal("resolved JSON output for explicit NDJSON"); return false, nil },
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) { return response, nil },
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode NDJSON output: %v; output=%q", err, output.String())
	}
	if record["id"] != "7" || record["type"] != "user" || record["url"] != "https://www.pixiv.net/users/7" {
		t.Fatalf("unexpected NDJSON record: %+v", record)
	}
	if !strings.Contains(errorOutput.String(), "ascii2d-color") {
		t.Fatalf("partial warning = %q", errorOutput.String())
	}
}

func TestCommandHumanOutputUsesFilteredSafeSummary(t *testing.T) {
	output := &bytes.Buffer{}
	response := reversesearch.Response{
		Results: []reversesearch.Result{
			{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 9}, Title: "title\nnext", Author: "artist"},
			{Title: "external\tresult", Author: "other"},
		},
	}
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: output,
		JSONOut: func(*bool) (bool, error) {
			return false, nil
		},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			return response, nil
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "https://www.pixiv.net/artworks/9") || !strings.Contains(text, `title\nnext`) || !strings.Contains(text, `external\tresult`) {
		t.Fatalf("human output = %q", text)
	}
}

func TestCommandRejectsSearchFlagsForImageSourceBeforeCallingService(t *testing.T) {
	for _, args := range [][]string{
		{"https://example.test/image.jpg", "--type", "artwork"},
		{"https://example.test/image.jpg", "--limit", "1"},
		{"https://example.test/image.jpg", "--search-by", "tag-exact"},
		{"https://example.test/image.jpg", "--trending-tags"},
	} {
		args := args
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			called := false
			cmd := New(Dependencies{
				Input:  strings.NewReader(""),
				Output: &bytes.Buffer{},
				ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
					called = true
					return reversesearch.Response{}, nil
				},
			})
			cmd.SetArgs(append(args, "--ndjson"))

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected image flag usage error, got %v", err)
			}
			if args[1] != "--trending-tags" && !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("expected image flag usage error, got %v", err)
			}
			if called {
				t.Fatal("called reverse search after rejecting image flag")
			}
		})
	}
}

func TestCommandRejectsJSONAndNDJSONConflictBeforeCallingService(t *testing.T) {
	called := false
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			called = true
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--json", "--ndjson"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--ndjson cannot be used with --json") {
		t.Fatalf("expected JSON/NDJSON conflict, got %v", err)
	}
	if called {
		t.Fatal("called reverse search after rejecting output conflict")
	}
}

func TestCommandRejectsInvalidReverseSearchProviderBeforeCallingService(t *testing.T) {
	called := false
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			called = true
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--provider", "unknown", "--ndjson"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "provider must be one of") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
	if called {
		t.Fatal("called reverse search with invalid provider")
	}
}

func TestCommandKeepsNonFileInputOnKeywordSearchPath(t *testing.T) {
	pooled := false
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: &bytes.Buffer{},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			return nil
		},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			t.Fatal("used reverse search for a keyword")
			return reversesearch.Response{}, nil
		},
	})
	cmd.SetArgs([]string{"ordinary-keyword", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute keyword search: %v", err)
	}
	if !pooled {
		t.Fatal("did not use Pixiv keyword search for non-file input")
	}
}

func TestCommandLocallyFiltersArtworkRatingAndCanonicalizesSubtype(t *testing.T) {
	output := &bytes.Buffer{}
	var requests []*http.Request
	pooled := false
	cmd := New(Dependencies{
		Input:   strings.NewReader(""),
		Output:  output,
		JSONOut: func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: searchFixtureTransport(func(request *http.Request) (*http.Response, error) {
				requests = append(requests, request)
				body := `{"illusts":[{"id":1,"title":"safe","type":"illust","x_restrict":0,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"safe"},"tags":[]},{"id":2,"title":"r18","type":"illust","x_restrict":1,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"r18"},"tags":[]}]}`
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})}})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"cat", "--rating", "r18", "--content-type", "illustration", "--limit", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute filtered artwork search: %v", err)
	}
	if !pooled {
		t.Fatal("did not open the pooled Pixiv client")
	}
	if len(requests) != 1 {
		t.Fatalf("HTTP requests = %d, want 1", len(requests))
	}
	query := requests[0].URL.Query()
	if query.Get("word") != "cat" {
		t.Fatalf("word = %q, want cat", query.Get("word"))
	}
	if query.Get("content_type") != "illust" {
		t.Fatalf("content_type = %q, want canonical illust", query.Get("content_type"))
	}
	if query.Get("rating") != "" || query.Get("x_restrict") != "" {
		t.Fatalf("rating leaked to upstream query: %v", query)
	}
	var envelope struct {
		Illusts []struct {
			ID int64 `json:"id"`
		} `json:"illusts"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if len(envelope.Illusts) != 1 || envelope.Illusts[0].ID != 2 {
		t.Fatalf("filtered output = %+v, want only r18 artwork 2", envelope.Illusts)
	}
}

func TestCommandSearchReadsWordFromStdinAndEmitsFilteredNDJSONAcrossBatches(t *testing.T) {
	output := &bytes.Buffer{}
	var requests []*http.Request
	cmd := New(Dependencies{
		Input:  strings.NewReader("cat\n"),
		Output: output,
		Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: searchFixtureTransport(func(request *http.Request) (*http.Response, error) {
				requests = append(requests, request)
				offset := request.URL.Query().Get("offset")
				body := `{"illusts":[{"id":1,"title":"safe","type":"illust","x_restrict":0,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"safe"},"tags":[]}]}`
				if offset == "" {
					body = `{"illusts":[{"id":1,"title":"safe","type":"illust","x_restrict":0,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"safe"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30"}`
				} else if offset == "30" {
					body = `{"illusts":[{"id":2,"title":"r18","type":"illust","x_restrict":1,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"r18"},"tags":[]}]}`
				} else {
					t.Fatalf("unexpected offset %q", offset)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})}})
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"--rating", "r18", "--limit", "1", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute stdin filtered search: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("HTTP requests = %d, want 2", len(requests))
	}
	for _, request := range requests {
		if request.URL.Query().Get("word") != "cat" {
			t.Fatalf("word = %q, want cat", request.URL.Query().Get("word"))
		}
		if request.URL.Query().Get("rating") != "" || request.URL.Query().Get("x_restrict") != "" {
			t.Fatalf("rating leaked to upstream query: %v", request.URL.Query())
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("NDJSON lines = %d, want 1; output=%q", len(lines), output.String())
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &item); err != nil {
		t.Fatalf("decode NDJSON record: %v; output=%q", err, output.String())
	}
	if item["id"] != "2" {
		t.Fatalf("NDJSON id = %v, want 2", item["id"])
	}
}

type searchFixtureTransport func(*http.Request) (*http.Response, error)

func (f searchFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCommandOutputsEnvelopeBeforeSingleProviderFailure(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	response := reversesearch.Response{
		Input:     reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"},
		Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusError}},
		ProviderErrors: []reversesearch.ProviderError{{
			Provider: reversesearch.ProviderSauceNAO, Code: reversesearch.CodeMissingCredential, Message: "SauceNAO API key is required",
		}},
	}
	cmd := New(Dependencies{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: errorOutput,
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			return response, reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg", "--json"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "SauceNAO API key is required") {
		t.Fatalf("expected provider failure, got %v", err)
	}
	var envelope map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode failure envelope: %v; output=%q", decodeErr, output.String())
	}
	if _, ok := envelope["provider_errors"]; !ok {
		t.Fatalf("failure envelope omitted provider_errors: %+v", envelope)
	}
	if strings.Contains(output.String()+errorOutput.String(), "https://example.test/image.jpg") {
		t.Fatal("leaked source URL in provider failure output")
	}
}

type fdBuffer struct {
	bytes.Buffer
	fd uintptr
}

func (w *fdBuffer) Fd() uintptr { return w.fd }

func TestCommandAutoUsesNDJSONForNonTerminalOutput(t *testing.T) {
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipeReader.Close()
	defer pipeWriter.Close()
	output := &fdBuffer{fd: pipeReader.Fd()}
	cmd := New(Dependencies{
		Input:  strings.NewReader(""),
		Output: output,
		JSONOut: func(*bool) (bool, error) {
			return false, nil
		},
		ReverseSearch: func(context.Context, ReverseSearchRequest) (reversesearch.Response, error) {
			return reversesearch.Response{
				Results: []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 12}}},
			}, nil
		},
	})
	cmd.SetArgs([]string{"https://example.test/image.jpg"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reverse search: %v", err)
	}
	var item map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &item); err != nil {
		t.Fatalf("expected auto-NDJSON record: %v; output=%q", err, output.String())
	}
	if item["url"] != "https://www.pixiv.net/artworks/12" {
		t.Fatalf("unexpected auto-NDJSON record: %+v", item)
	}
}
