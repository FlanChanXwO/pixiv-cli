package tools

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type vectorSearchInput struct {
	Query  *string             `json:"query,omitempty"`
	Source *string             `json:"source,omitempty"`
	Image  *reverseSearchImage `json:"image,omitempty"`
	toolrun.ListIn
}

type vectorSearchOutput struct {
	vectorindex.SearchResponse
	Error string `json:"error,omitempty"`
}

func (t pixivTools) registerVectorSearch(server *mcp.Server) {
	toolrun.ProtectInput(server, "pixiv_vector_search", func() *mcp.CallToolResult {
		result, out := vectorSearchFailure("invalid_request")
		result.StructuredContent = out
		return result
	})
	toolhost.AddTool(t.app, server, &mcp.Tool{
		Name: "pixiv_vector_search", Meta: mcp.Meta{"openai/fileParams": []string{"image"}},
		Description: "Search the local artwork index with exactly one text query, server-side image source or host image. Catch-up saves local state and cloud models upload images/queries with possible usage charges. Run pixiv vector setup and pixiv vector sync in a terminal first. Results can include private/R18 snapshots and are shared across accounts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: vectorSearchInputSchema(), OutputSchema: vectorSearchOutputSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input vectorSearchInput) (*mcp.CallToolResult, vectorSearchOutput, error) {
		return t.vectorSearchHandler(ctx, input)
	})
}

func vectorSearchInputSchema() map[string]any {
	properties := reverseSearchInputSchema()["properties"].(map[string]any)
	delete(properties, "provider")
	properties["query"] = map[string]any{"type": "string", "minLength": 1, "description": "Text description of the artwork to find; local models accept at most 64 tokens including EOS."}
	schema := schemas.List(properties)
	schema["oneOf"] = []map[string]any{{"required": []string{"query"}}, {"required": []string{"source"}}, {"required": []string{"image"}}}
	return schema
}

func vectorSearchOutputSchema() map[string]any {
	return schemas.ClosedObject(map[string]any{
		"records":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true}},
		"pagination": map[string]any{"type": "object", "additionalProperties": true},
		"index": map[string]any{"type": "object", "required": []string{"model", "pages", "pending", "catch_up", "bookmarks_read"}, "properties": map[string]any{
			"model": map[string]any{"type": "string"}, "pages": map[string]any{"type": "integer"}, "pending": map[string]any{"type": "integer"}, "catch_up": map[string]any{"type": "string"}, "bookmarks_read": map[string]any{"type": "boolean"}, "error": map[string]any{"type": "string"},
		}, "additionalProperties": false},
		"error": map[string]any{"type": "string", "enum": []string{"not_set_up", "initial_sync_required", "invalid_request", "invalid_source", "source_not_regular_file", "source_read_failed", "source_http_status", "snapshot_failed", "text_too_long", "unsupported_image", "model_unavailable", "index_unavailable", "sync_failed", "canceled", "unknown"}},
	}, []string{"records", "pagination", "index"})
}

func (t pixivTools) vectorSearchHandler(ctx context.Context, input vectorSearchInput) (*mcp.CallToolResult, vectorSearchOutput, error) {
	fail := func(code string) (*mcp.CallToolResult, vectorSearchOutput, error) {
		r, out := vectorSearchFailure(code)
		return r, out, nil
	}
	choices := 0
	for _, v := range []bool{input.Query != nil, input.Source != nil, input.Image != nil} {
		if v {
			choices++
		}
	}
	if choices != 1 {
		return fail("invalid_request")
	}
	if _, err := toolrun.ParseListPlan(input.ListIn); err != nil {
		return fail("invalid_request")
	}
	request := vectorindex.SearchInput{Page: input.Page, Limit: input.Limit}
	source := ""
	if input.Query != nil {
		request.Query = *input.Query
		if strings.TrimSpace(request.Query) == "" {
			return fail("invalid_request")
		}
	} else {
		var code reversesearch.ErrorCode
		source, code = imageSource(input.Source, input.Image)
		if code != "" {
			return fail(string(code))
		}
	}
	port := t.app.SDKPorts().VectorSearch
	if port == nil {
		return fail("not_set_up")
	}
	response, err := port(ctx, request, source, t.app.VectorAccount)
	if err != nil {
		return fail(vectorSearchErrorCode(ctx, err))
	}
	out := vectorSearchOutput{SearchResponse: response}
	message := fmt.Sprintf("Retrieved %d vector-search results.", len(out.Records))
	if out.Index.Pending > 0 {
		message += fmt.Sprintf(" %d pages await embedding; run pixiv vector sync.", out.Index.Pending)
	}
	if out.Index.Error != "" {
		message += " Catch-up stopped: " + out.Index.Error + "."
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}}, out, nil
}

func vectorSearchFailure(code string) (*mcp.CallToolResult, vectorSearchOutput) {
	out := vectorSearchOutput{SearchResponse: vectorindex.SearchResponse{Records: []record.Record{}}, Error: code}
	message := "Vector search failed (" + code + ")."
	if code == "initial_sync_required" {
		message += " The index is empty and the initial bookmark scan is incomplete; run pixiv vector sync."
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}, out
}

func vectorSearchErrorCode(ctx context.Context, err error) string {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	if errors.Is(err, vectormodel.ErrTextTooLong) {
		return "text_too_long"
	}
	if errors.Is(err, image.ErrFormat) {
		return "unsupported_image"
	}
	switch code := reversesearch.CodeOf(err); code {
	case reversesearch.CodeInvalidSource, reversesearch.CodeSourceNotRegularFile, reversesearch.CodeSourceReadFailed, reversesearch.CodeSourceHTTPStatus, reversesearch.CodeSnapshotFailed:
		return string(code)
	}
	var classified *vectorindex.SearchError
	if errors.As(err, &classified) {
		return classified.Code
	}
	return "unknown"
}
