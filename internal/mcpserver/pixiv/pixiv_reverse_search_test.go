package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
)

type reverseSearchOutputFixture struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	Results        []reversesearch.Result          `json:"results"`
	Records        []record.Record                 `json:"records"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

type reverseSearcherFunc func(context.Context, reversesearch.Request) (reversesearch.Response, error)

func (f reverseSearcherFunc) Search(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
	return f(ctx, request)
}

func TestReverseSearchPublishesClosedInputAndEnvelopeSchemas(t *testing.T) {
	tools := connectAndListTools(t)
	var reverseToolName string
	var inputSchema, outputSchema any
	for _, tool := range tools {
		if tool.Name != "pixiv_reverse_search" {
			continue
		}
		reverseToolName = tool.Name
		require.JSONEq(t, `["image"]`, string(mustJSON(t, tool.Meta["openai/fileParams"])))
		if err := json.Unmarshal(mustJSON(t, tool.InputSchema), &inputSchema); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(mustJSON(t, tool.OutputSchema), &outputSchema); err != nil {
			t.Fatal(err)
		}
	}
	if reverseToolName == "" {
		t.Fatal("reverse_search tool is not registered")
	}

	input, ok := inputSchema.(map[string]any)
	if !ok || input["type"] != "object" || input["additionalProperties"] != false {
		t.Fatalf("reverse_search input schema = %#v, want closed object", inputSchema)
	}
	require.NotContains(t, string(mustJSON(t, input["required"])), `"source"`)
	properties, ok := input["properties"].(map[string]any)
	if !ok || properties["source"] == nil || properties["provider"] == nil {
		t.Fatalf("reverse_search input schema properties = %#v", input["properties"])
	}
	require.Len(t, properties, 3)
	image, ok := properties["image"].(map[string]any)
	require.True(t, ok, "image must be an object schema")
	require.Equal(t, "object", image["type"])
	require.Equal(t, false, image["additionalProperties"])
	require.JSONEq(t, `["download_url", "file_id"]`, string(mustJSON(t, image["required"])))
	imageFields, ok := image["properties"].(map[string]any)
	require.True(t, ok)
	require.Len(t, imageFields, 4)
	for _, name := range []string{"download_url", "file_id", "mime_type", "file_name"} {
		field, ok := imageFields[name].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "string", field["type"])
	}
	if _, ok := properties["pixiv_only"]; ok {
		t.Fatalf("reverse_search input schema exposes server config: %#v", properties)
	}
	provider, ok := properties["provider"].(map[string]any)
	if !ok || !strings.Contains(string(mustJSON(t, provider["enum"])), `"all"`) {
		t.Fatalf("reverse_search provider schema = %#v", properties["provider"])
	}

	output, ok := outputSchema.(map[string]any)
	if !ok || output["type"] != "object" || output["additionalProperties"] != false {
		t.Fatalf("reverse_search output schema = %#v, want closed object", outputSchema)
	}
	for _, field := range []string{"input", "providers", "results", "records", "provider_errors", "partial"} {
		if !strings.Contains(string(mustJSON(t, output["required"])), `"`+field+`"`) {
			t.Fatalf("reverse_search output schema missing required %q: %#v", field, output)
		}
	}
}

func TestReverseSearchReturnsStructuredEnvelopeAndRecord(t *testing.T) {
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{
					Input:     reversesearch.Input{Kind: reversesearch.SourceKindFile, SHA256: "hash-42"},
					Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1}},
					Results:   []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 42}, Evidence: []reversesearch.Evidence{{Provider: reversesearch.ProviderASCII2DColor, Rank: 1}}}},
				}, nil
			}),
			Provider:  reversesearch.ProviderSauceNAO,
			PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{
		"source": "/private/source-secret.png", "provider": "ascii2d-color",
	})
	if result.IsError {
		t.Fatalf("reverse_search returned MCP error: %+v", result)
	}
	if got.Source != "/private/source-secret.png" || got.Provider != reversesearch.ProviderASCII2DColor || !got.PixivOnly {
		t.Fatalf("reverse search request = %+v", got)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if out.Input.Kind != reversesearch.SourceKindFile || out.Input.SHA256 != "hash-42" || len(out.Providers) != 1 || len(out.Results) != 1 || len(out.Records) != 1 {
		t.Fatalf("reverse search structured output = %+v", out)
	}
	if out.Records[0].Type() != "artwork" || out.Records[0].ID() != "42" || out.Records[0].URL() != "https://www.pixiv.net/artworks/42" {
		t.Fatalf("reverse search record = %+v", out.Records[0])
	}
	raw := string(mustJSON(t, result.StructuredContent))
	for _, secret := range []string{"/private/source-secret.png", "source-secret.png"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("reverse search structured output leaked %q: %s", secret, raw)
		}
	}
}

func TestReverseSearchUsesStartupProviderWhenInputOmitsOverride(t *testing.T) {
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{}, nil
			}),
			Provider:  reversesearch.ProviderASCII2DBOVW,
			PixivOnly: false,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "https://image.example.test/picture.png"})
	if result.IsError {
		t.Fatalf("reverse_search returned MCP error: %+v", result)
	}
	if got.Provider != reversesearch.ProviderASCII2DBOVW || got.PixivOnly {
		t.Fatalf("reverse search startup defaults = %+v", got)
	}
}

func TestReverseSearchPartialResultIsNotMCPError(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
				return reversesearch.Response{
					Providers: []reversesearch.ProviderSummary{
						{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusError},
						{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
					},
					Results:        []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 7}}},
					ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderSauceNAO, Code: reversesearch.CodeMissingCredential, Message: "SauceNAO API key is required"}},
					Partial:        true,
				}, nil
			}),
			PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "https://image.example.test/picture.png", "provider": "all"})
	if result.IsError {
		t.Fatalf("partial reverse search must not be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if !out.Partial || len(out.ProviderErrors) != 1 || len(out.Records) != 1 {
		t.Fatalf("partial reverse search output = %+v", out)
	}
}

func TestReverseSearchStableSolverErrorKeepsStructuredErrorEnvelope(t *testing.T) {
	const secret = "solver-secret source-secret csrf-secret"
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
				return reversesearch.Response{
					Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusError}},
					Results:   []reversesearch.Result{},
					ProviderErrors: []reversesearch.ProviderError{{
						Provider: reversesearch.ProviderASCII2DColor,
						Code:     reversesearch.CodeSolverFailed,
						Message:  "ascii2d challenge solver failed",
					}},
				}, reversesearch.NewError(reversesearch.CodeSolverFailed, "ascii2d challenge solver failed", errors.New(secret))
			}),
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{
		"source": "https://source-secret.example.test/image?token=solver-secret", "provider": "ascii2d-color",
	})
	if !result.IsError {
		t.Fatalf("solver failure must be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if len(out.Providers) != 1 || len(out.Results) != 0 || len(out.ProviderErrors) != 1 {
		t.Fatalf("solver failure envelope is not preserved: %+v", out)
	}
	if out.ProviderErrors[0].Code != reversesearch.CodeSolverFailed {
		t.Fatalf("provider error code = %q, want %q", out.ProviderErrors[0].Code, reversesearch.CodeSolverFailed)
	}
	raw := string(mustJSON(t, result))
	if !strings.Contains(raw, string(reversesearch.CodeSolverFailed)) {
		t.Fatalf("MCP error did not expose stable code: %s", raw)
	}
	if strings.Contains(raw, secret) {
		t.Fatalf("MCP solver failure leaked private cause: %s", raw)
	}
}

func TestReverseSearchFailurePreservesSafeStructuredEnvelope(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
				return reversesearch.Response{
					Providers:      []reversesearch.ProviderSummary{{Name: reversesearch.ProviderAll, Status: reversesearch.ProviderStatusError}},
					ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderAll, Code: reversesearch.CodeAllProvidersFailed, Message: "all reverse-search providers failed"}},
				}, reversesearch.NewError(reversesearch.CodeAllProvidersFailed, "all reverse-search providers failed", errors.New("api-key-secret source-secret upstream-body-secret csrf-secret location-secret"))
			}),
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{
		"source": "https://source-secret.example.test/image?token=api-key-secret", "provider": "all",
	})
	if !result.IsError {
		t.Fatalf("all-provider failure must be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if len(out.Providers) != 1 || len(out.ProviderErrors) != 1 || out.Records == nil || out.Results == nil {
		t.Fatalf("failure envelope is not preserved: %+v", out)
	}
	raw := string(mustJSON(t, result))
	for _, secret := range []string{"source-secret.example.test", "api-key-secret", "upstream-body-secret", "csrf-secret", "location-secret"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("MCP failure leaked %q: %s", secret, raw)
		}
	}
}

func TestReverseSearchConfigurationFailureKeepsEmptyEnvelope(t *testing.T) {
	session, closeSession := newTestSession(t)
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "/private/source-secret.png"})
	if !result.IsError {
		t.Fatalf("unconfigured reverse search must be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if out.Providers == nil || out.Results == nil || out.Records == nil || out.ProviderErrors == nil {
		t.Fatalf("configuration failure envelope has nil collections: %+v", out)
	}
	if strings.Contains(string(mustJSON(t, result)), "source-secret") {
		t.Fatalf("configuration failure leaked source: %s", mustJSON(t, result))
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return data
}

func TestReverseSearchImageUsesDownloadURLAndStartupSettings(t *testing.T) {
	const source = "https://image.example.test/photo?signature=private-signature"
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{Input: reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "safe-hash"}}, nil
			}),
			Provider: reversesearch.ProviderASCII2DBOVW, PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{
		"download_url": source, "file_id": "private-file-id", "mime_type": "image/png", "file_name": "private-name.png",
	}})
	require.False(t, result.IsError, "%s", mustJSON(t, result))
	require.Equal(t, source, got.Source)
	require.Equal(t, reversesearch.ProviderASCII2DBOVW, got.Provider)
	require.True(t, got.PixivOnly)
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	require.Equal(t, "safe-hash", out.Input.SHA256)
	for _, secret := range []string{source, "private-signature", "private-file-id", "private-name.png"} {
		require.NotContains(t, string(mustJSON(t, result)), secret)
	}
}

func TestReverseSearchRejectsInvalidInputsWithoutLeakingOrSearching(t *testing.T) {
	image := func(source string) map[string]any {
		return map[string]any{"download_url": source, "file_id": "private-file-id"}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"neither", map[string]any{}},
		{"remote file authority", map[string]any{"source": "file://other-host/private/source-secret"}},
		{"file userinfo", map[string]any{"source": "file://source-secret@localhost/private/photo"}},
		{"file query", map[string]any{"source": "file:///private/source-secret?private-download"}},
		{"empty file query", map[string]any{"source": "file:///private/source-secret?"}},
		{"file fragment", map[string]any{"source": "file:///private/source-secret#private-download"}},
		{"empty file fragment", map[string]any{"source": "file:///private/source-secret#"}},
		{"file UNC path", map[string]any{"source": "file:////private/source-secret"}},
		{"empty file path", map[string]any{"source": "file://localhost"}},
		{"malformed file escape", map[string]any{"source": "file:///private/%source-secret"}},
		{"file NUL", map[string]any{"source": "file:///private/source-secret%00"}},
		{"both", map[string]any{"source": "/private/source-secret", "image": image("https://example.test/private-download")}},
		{"empty source with image", map[string]any{"source": "", "image": image("https://example.test/private-download")}},
		{"empty source", map[string]any{"source": "  "}},
		{"null source", map[string]any{"source": nil}},
		{"null image", map[string]any{"image": nil}},
		{"wrong source type", map[string]any{"source": []string{"source-secret"}}},
		{"wrong image type", map[string]any{"image": "private-download"}},
		{"missing file id", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download"}}},
		{"missing download url", map[string]any{"image": map[string]any{"file_id": "private-file-id"}}},
		{"empty file id", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download", "file_id": "  "}}},
		{"unknown image field", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download", "file_id": "private-file-id", "extra": "source-secret"}}},
		{"local image path", map[string]any{"image": image("/private/source-secret")}},
		{"file image URI", map[string]any{"image": image("file:///private/source-secret")}},
		{"ftp image URL", map[string]any{"image": image("ftp://example.test/private-download")}},
		{"empty image URL", map[string]any{"image": image("")}},
		{"missing URL host", map[string]any{"image": image("https:///private-download")}},
		{"URL userinfo", map[string]any{"image": image("https://source-secret:private-password@example.test/private-download")}},
		{"malformed URL", map[string]any{"image": image("https://example.test/%private-download")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{
				Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
					called = true
					return reversesearch.Response{}, nil
				}),
			}}, pixivmcpserver.Account{})
			defer closeSession()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pixiv_reverse_search", Arguments: tc.args})
			require.True(t, err != nil || (result != nil && result.IsError), "invalid input accepted: %s", mustJSON(t, result))
			require.False(t, called)
			raw := string(mustJSON(t, result))
			if err != nil {
				raw += err.Error()
			}
			for _, secret := range []string{"source-secret", "private-download", "private-file-id", "private-password"} {
				require.NotContains(t, raw, secret)
			}
		})
	}
}

type reversePayloadFunc func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error)

func (f reversePayloadFunc) Preflight(context.Context, reversesearch.PayloadQuery) error { return nil }
func (f reversePayloadFunc) SearchPayload(ctx context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
	return f(ctx, request)
}

func TestReverseSearchInputsSharePrivateSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private 空格#%.png")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0600))
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	fileURL := (&url.URL{Scheme: "file", Path: uriPath}).String()
	localURL := (&url.URL{Scheme: "file", Host: "localhost", Path: uriPath}).String()
	var fetched atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetched.Add(1); _, _ = io.WriteString(w, "hello\n") }))
	defer upstream.Close()
	for _, tc := range []struct {
		name    string
		args    map[string]any
		kind    reversesearch.SourceKind
		fetches int
	}{
		{"path", map[string]any{"source": path}, reversesearch.SourceKindFile, 0},
		{"file URI", map[string]any{"source": fileURL}, reversesearch.SourceKindFile, 0},
		{"localhost file URI", map[string]any{"source": localURL}, reversesearch.SourceKindFile, 0},
		{"HTTP source", map[string]any{"source": upstream.URL + "/private-download"}, reversesearch.SourceKindURL, 1},
		{"host image", map[string]any{"image": map[string]any{"download_url": upstream.URL + "/private-download?sig=private-signature", "file_id": "private-file-id"}}, reversesearch.SourceKindURL, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temp := t.TempDir()
			before := fetched.Load()
			var snapshot *reversesearch.Snapshot
			facade := reversesearch.NewFacade(reversesearch.Dependencies{
				Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: upstream.Client()}),
				Payloads: reversePayloadFunc(func(_ context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
					snapshot = request.Snapshot
					require.Equal(t, reversesearch.ProviderASCII2DBOVW, request.Provider)
					require.True(t, request.PixivOnly)
					reader, err := snapshot.Open()
					require.NoError(t, err)
					defer reader.Close()
					data, err := io.ReadAll(reader)
					require.NoError(t, err)
					require.Equal(t, "hello\n", string(data))
					entries, err := os.ReadDir(temp)
					require.NoError(t, err)
					require.Len(t, entries, 1)
					info, err := entries[0].Info()
					require.NoError(t, err)
					if os.PathSeparator != '\\' {
						require.Equal(t, os.FileMode(0600), info.Mode().Perm())
					}
					return reversesearch.Response{}, nil
				}),
			})
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: facade, Provider: reversesearch.ProviderASCII2DBOVW, PixivOnly: true}}, pixivmcpserver.Account{})
			defer closeSession()
			result := callTool(t, session, "pixiv_reverse_search", tc.args)
			require.False(t, result.IsError, "%s", mustJSON(t, result))
			require.NotNil(t, snapshot)
			var out reverseSearchOutputFixture
			decodeStructured(t, result, &out)
			require.Equal(t, tc.kind, out.Input.Kind)
			require.Equal(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", out.Input.SHA256)
			require.Equal(t, int32(tc.fetches), fetched.Load()-before)
			_, err := snapshot.Open()
			require.Error(t, err)
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries)
			for _, secret := range []string{path, fileURL, "private-download", "private-signature", "private-file-id"} {
				require.NotContains(t, string(mustJSON(t, result)), secret)
			}
		})
	}
}

func TestReverseSearchExpiredHostImageIsSafeAndNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var fetched atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fetched.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private-response-body")
			}))
			defer upstream.Close()
			temp := t.TempDir()
			var searched atomic.Bool
			facade := reversesearch.NewFacade(reversesearch.Dependencies{
				Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: upstream.Client()}),
				Payloads: reversePayloadFunc(func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error) {
					searched.Store(true)
					return reversesearch.Response{}, nil
				}),
			})
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: facade}}, pixivmcpserver.Account{})
			defer closeSession()
			result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{"download_url": upstream.URL + "/private-download?signature=private-signature", "file_id": "private-file-id", "file_name": "private-name.png"}})
			require.True(t, result.IsError)
			require.True(t, resultHasText(result, "source_http_status"))
			require.True(t, resultHasText(result, "Reattach"), "expired image must explain how to recover: %s", mustJSON(t, result))
			require.Equal(t, int32(1), fetched.Load())
			require.False(t, searched.Load())
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries)
			for _, secret := range []string{"private-download", "private-signature", "private-file-id", "private-name.png", "private-response-body"} {
				require.NotContains(t, string(mustJSON(t, result)), secret)
			}
		})
	}
}

func TestReverseSearchHostImageProviderFailureCleansSnapshotAndKeepsResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "hello\n") }))
	defer upstream.Close()
	temp := t.TempDir()
	var snapshot *reversesearch.Snapshot
	facade := reversesearch.NewFacade(reversesearch.Dependencies{
		Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: upstream.Client()}),
		Payloads: reversePayloadFunc(func(_ context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
			snapshot = request.Snapshot
			return reversesearch.Response{Partial: true, Results: []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 42}}}, ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderSauceNAO, Code: reversesearch.CodeProviderFailed, Message: "provider failed"}}}, reversesearch.NewError(reversesearch.CodeProviderFailed, "provider failed", errors.New("private-download private-file-id private-name.png"))
		}),
	})
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: facade}}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{"download_url": upstream.URL + "/private-download", "file_id": "private-file-id", "file_name": "private-name.png"}})
	require.True(t, result.IsError)
	require.NotNil(t, snapshot)
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	require.True(t, out.Partial)
	require.Len(t, out.Results, 1)
	require.Len(t, out.Records, 1)
	require.Len(t, out.ProviderErrors, 1)
	require.Equal(t, "42", out.Records[0].ID())
	_, err := snapshot.Open()
	require.Error(t, err)
	entries, err := os.ReadDir(temp)
	require.NoError(t, err)
	require.Empty(t, entries)
	for _, secret := range []string{"private-download", "private-file-id", "private-name.png"} {
		require.NotContains(t, string(mustJSON(t, result)), secret)
	}
}

type reverseImageTransport func(*http.Request) (*http.Response, error)

func (f reverseImageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type reverseCancelBody struct {
	cancel context.CancelFunc
	closed bool
}

func (b *reverseCancelBody) Read(p []byte) (int, error) {
	// 首次读取发生在私有 snapshot 创建之后；取消必须删除这个未完成文件。
	n := copy(p, "partial image")
	b.cancel()
	return n, nil
}
func (b *reverseCancelBody) Close() error { b.closed = true; return nil }

func TestReverseSearchHostImageCancellationCleansPartialSnapshot(t *testing.T) {
	temp := t.TempDir()
	var body *reverseCancelBody
	searched := false
	searcher := reverseSearcherFunc(func(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		body = &reverseCancelBody{cancel: cancel}
		client := &http.Client{Transport: reverseImageTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Request: r, Body: body, Header: make(http.Header)}, nil
		})}
		facade := reversesearch.NewFacade(reversesearch.Dependencies{
			Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: client}),
			Payloads: reversePayloadFunc(func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error) {
				searched = true
				return reversesearch.Response{}, nil
			}),
		})
		return facade.Search(ctx, request)
	})
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: searcher}}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{"download_url": "http://127.0.0.1/private-download", "file_id": "private-file-id"}})
	require.True(t, result.IsError)
	require.False(t, searched)
	require.NotNil(t, body)
	require.True(t, body.closed)
	entries, err := os.ReadDir(temp)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NotContains(t, string(mustJSON(t, result)), "private-download")
	require.NotContains(t, string(mustJSON(t, result)), "private-file-id")
}
