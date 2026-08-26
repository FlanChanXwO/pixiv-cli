// Package reverse_search 实现 reverse_search MCP tool。
package reverse_search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 reverse_search。source 只在当前请求中传给 Facade，输出永远只
// 暴露 source kind/hash 摘要；provider、pixiv-only、代理和凭据来自启动快照。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:         "reverse_search",
		Description:  "Search an image source with SauceNAO or ascii2d and return Pixiv matches.",
		InputSchema:  reverseSearchInputSchema(),
		OutputSchema: reverseSearchOutputSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input reverseSearchInput) (*mcp.CallToolResult, reverseSearchOutput, error) {
		return handleReverseSearch(ctx, app, input)
	})
}

type reverseSearchInput struct {
	Source   string `json:"source"`
	Provider string `json:"provider,omitempty"`
}

// reverseSearchOutput 是 MCP 的完整 wire envelope。它刻意不把原始 source、
// 临时路径、请求头、cookie 或 API key 放进 structured content。
type reverseSearchOutput struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	Results        []reversesearch.Result          `json:"results"`
	Records        []record.Record                 `json:"records"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

func reverseSearchInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"source"},
		"properties": map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "Readable regular file path or HTTP(S) image URL. The source is fetched/uploaded by the trusted local MCP server and is never returned.",
			},
			"provider": map[string]any{
				"type":        "string",
				"enum":        []string{"saucenao", "ascii2d-color", "ascii2d-bovw", "all"},
				"description": "Optional provider override; omit to use the server startup configuration.",
			},
		},
	}
}

func reverseSearchOutputSchema() map[string]any {
	recordIdentity := map[string]any{
		"type":                 "object",
		"required":             []string{"type", "id", "url"},
		"properties":           map[string]any{"type": map[string]any{"type": "string"}, "id": map[string]any{"type": "string"}, "url": map[string]any{"type": "string"}},
		"additionalProperties": map[string]any{},
	}
	pixivIdentity := map[string]any{
		"type":                 "object",
		"required":             []string{"type", "id"},
		"properties":           map[string]any{"type": map[string]any{"type": "string"}, "id": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	quota := map[string]any{
		"type":                 "object",
		"required":             []string{"short_remaining", "long_remaining", "short_limit", "long_limit"},
		"properties":           map[string]any{"short_remaining": map[string]any{"type": "integer"}, "long_remaining": map[string]any{"type": "integer"}, "short_limit": map[string]any{"type": "integer"}, "long_limit": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	providerSummary := map[string]any{
		"type":                 "object",
		"required":             []string{"name", "status", "result_count"},
		"properties":           map[string]any{"name": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}, "result_count": map[string]any{"type": "integer"}, "quota": quota},
		"additionalProperties": false,
	}
	evidence := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider":      map[string]any{"type": "string"},
			"rank":          map[string]any{"type": "integer"},
			"similarity":    map[string]any{"type": "number"},
			"index_id":      map[string]any{"type": "integer"},
			"index_name":    map[string]any{"type": "string"},
			"title":         map[string]any{"type": "string"},
			"author":        map[string]any{"type": "string"},
			"external_urls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"provider", "rank", "similarity", "index_id", "index_name"},
		"additionalProperties": false,
	}
	result := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pixiv":    pixivIdentity,
			"title":    map[string]any{"type": "string"},
			"author":   map[string]any{"type": "string"},
			"evidence": map[string]any{"type": "array", "items": evidence},
		},
		"required":             []string{"evidence"},
		"additionalProperties": false,
	}
	providerError := map[string]any{
		"type":                 "object",
		"required":             []string{"provider", "code", "message"},
		"properties":           map[string]any{"provider": map[string]any{"type": "string"}, "code": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}},
		"additionalProperties": false,
	}
	input := map[string]any{
		"type":                 "object",
		"required":             []string{"kind", "sha256"},
		"properties":           map[string]any{"kind": map[string]any{"type": "string"}, "sha256": map[string]any{"type": "string"}},
		"additionalProperties": false,
	}
	return map[string]any{
		"type":     "object",
		"required": []string{"input", "providers", "results", "records", "provider_errors", "partial"},
		"properties": map[string]any{
			"input": input,
			"providers": map[string]any{
				"type":  "array",
				"items": providerSummary,
			},
			"results": map[string]any{
				"type":  "array",
				"items": result,
			},
			"records": map[string]any{
				"type":  "array",
				"items": recordIdentity,
			},
			"provider_errors": map[string]any{
				"type":  "array",
				"items": providerError,
			},
			"partial": map[string]any{"type": "boolean"},
		},
		"additionalProperties": false,
	}
}

func handleReverseSearch(ctx context.Context, app *runtime.App, input reverseSearchInput) (*mcp.CallToolResult, reverseSearchOutput, error) {
	out := emptyReverseSearchOutput()
	if strings.TrimSpace(input.Source) == "" {
		return reverseSearchError(out, reversesearch.CodeInvalidRequest), out, nil
	}
	ports := app.ReverseSearchPorts()
	if ports.Searcher == nil {
		return reverseSearchError(out, reversesearch.CodeProviderNotConfigured), out, nil
	}
	provider := ports.Provider
	if input.Provider != "" {
		provider = reversesearch.Provider(input.Provider)
	}
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	response, err := ports.Searcher.Search(ctx, reversesearch.Request{
		Source: input.Source, Provider: provider, PixivOnly: ports.PixivOnly,
	})
	out = outputFromResponse(response)
	// 即使 Search 返回错误，response 也可能包含已完成的 Pixiv 结果（例如 Snapshot.Close 失败）。
	// 先填充 records，使 isError=true 的结构化结果保留与 CLI 一致的完整 envelope。
	records, recordsErr := reverseSearchRecords(response.Results)
	if recordsErr == nil {
		out.Records = records
	}
	if err != nil {
		return reverseSearchError(out, safeErrorCode(ctx, err)), out, nil
	}
	if recordsErr != nil {
		return reverseSearchError(out, reversesearch.CodeUnknown), out, nil
	}
	return reverseSearchResult(out), out, nil
}

func emptyReverseSearchOutput() reverseSearchOutput {
	return reverseSearchOutput{
		Providers:      []reversesearch.ProviderSummary{},
		Results:        []reversesearch.Result{},
		Records:        []record.Record{},
		ProviderErrors: []reversesearch.ProviderError{},
	}
}

func outputFromResponse(response reversesearch.Response) reverseSearchOutput {
	out := emptyReverseSearchOutput()
	out.Input = response.Input
	if response.Providers != nil {
		out.Providers = response.Providers
	}
	if response.Results != nil {
		out.Results = make([]reversesearch.Result, 0, len(response.Results))
		for _, result := range response.Results {
			if result.Evidence == nil {
				result.Evidence = []reversesearch.Evidence{}
			}
			out.Results = append(out.Results, result)
		}
	}
	if response.ProviderErrors != nil {
		out.ProviderErrors = response.ProviderErrors
	}
	out.Partial = response.Partial
	return out
}

func reverseSearchRecords(results []reversesearch.Result) ([]record.Record, error) {
	items := make([]record.Record, 0, len(results))
	for _, result := range results {
		if result.Pixiv == nil {
			continue
		}
		var recordType, path string
		switch result.Pixiv.Type {
		case reversesearch.PixivRefArtwork:
			recordType, path = "artwork", fmt.Sprintf("https://www.pixiv.net/artworks/%d", result.Pixiv.ID)
		case reversesearch.PixivRefUser:
			recordType, path = "user", fmt.Sprintf("https://www.pixiv.net/users/%d", result.Pixiv.ID)
		default:
			return nil, errors.New("invalid reverse search identity")
		}
		identity, err := record.NewIdentityRecord(result.Pixiv.ID, recordType, path)
		if err != nil {
			return nil, err
		}
		items = append(items, identity)
	}
	return items, nil
}

func reverseSearchResult(out reverseSearchOutput) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d reverse-search results.", len(out.Results))}},
	}
}

func reverseSearchError(out reverseSearchOutput, code reversesearch.ErrorCode) *mcp.CallToolResult {
	message := "Error: reverse search failed"
	if code != "" && code != reversesearch.CodeUnknown {
		message += " (" + string(code) + ")"
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func safeErrorCode(ctx context.Context, err error) reversesearch.ErrorCode {
	if errors.Is(err, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)) {
		return reversesearch.CodeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return reversesearch.CodeUnknown
	}
	return reversesearch.CodeOf(err)
}
