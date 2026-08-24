package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
		if tool.Name != "reverse_search" {
			continue
		}
		reverseToolName = tool.Name
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
	if !strings.Contains(string(mustJSON(t, input["required"])), `"source"`) {
		t.Fatalf("reverse_search input schema does not require source: %#v", input)
	}
	properties, ok := input["properties"].(map[string]any)
	if !ok || properties["source"] == nil || properties["provider"] == nil {
		t.Fatalf("reverse_search input schema properties = %#v", input["properties"])
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
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, pixivmcpserver.SDKPorts{
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

	result := callTool(t, session, "reverse_search", map[string]any{
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
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, pixivmcpserver.SDKPorts{
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

	result := callTool(t, session, "reverse_search", map[string]any{"source": "https://image.example.test/picture.png"})
	if result.IsError {
		t.Fatalf("reverse_search returned MCP error: %+v", result)
	}
	if got.Provider != reversesearch.ProviderASCII2DBOVW || got.PixivOnly {
		t.Fatalf("reverse search startup defaults = %+v", got)
	}
}

func TestReverseSearchPartialResultIsNotMCPError(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, pixivmcpserver.SDKPorts{
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

	result := callTool(t, session, "reverse_search", map[string]any{"source": "https://image.example.test/picture.png", "provider": "all"})
	if result.IsError {
		t.Fatalf("partial reverse search must not be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if !out.Partial || len(out.ProviderErrors) != 1 || len(out.Records) != 1 {
		t.Fatalf("partial reverse search output = %+v", out)
	}
}

func TestReverseSearchFailurePreservesSafeStructuredEnvelope(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
				return reversesearch.Response{
					Providers:      []reversesearch.ProviderSummary{{Name: reversesearch.ProviderAll, Status: reversesearch.ProviderStatusError}},
					ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderAll, Code: reversesearch.CodeAllProvidersFailed, Message: "all reverse-search providers failed"}},
				}, reversesearch.NewError(reversesearch.CodeAllProvidersFailed, "all reverse-search providers failed", errors.New("api-key-secret source-secret upstream-body-secret"))
			}),
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "reverse_search", map[string]any{
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
	for _, secret := range []string{"source-secret.example.test", "api-key-secret", "upstream-body-secret"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("MCP failure leaked %q: %s", secret, raw)
		}
	}
}

func TestReverseSearchConfigurationFailureKeepsEmptyEnvelope(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	result := callTool(t, session, "reverse_search", map[string]any{"source": "/private/source-secret.png"})
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
