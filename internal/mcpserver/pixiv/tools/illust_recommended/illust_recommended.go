// Package illust_recommended 实现 pixiv_illust_recommended tool。
package illust_recommended

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_illust_recommended。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_illust_recommended", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get personalized artwork recommendations.", InputSchema: recommendedArtworkInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input recommendedArtworkIn) (*mcp.CallToolResult, outputs.Records, error) {
		return handleIllustRecommended(ctx, app, input)
	})
}

type recommendedArtworkIn struct {
	Cursor       *string               `json:"cursor,omitempty" jsonschema:"opaque default-batch continuation; repeat original arguments and omit page and limit"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	runtime.PageLimitIn
}

func recommendedArtworkInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"illust_filter": filters.IllustFilterSchema(),
			"cursor":        map[string]any{"type": "string", "description": "Opaque default-batch continuation; repeat original arguments and omit page and limit."},
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func handleIllustRecommended(ctx context.Context, app *runtime.App, in recommendedArtworkIn) (*mcp.CallToolResult, outputs.Records, error) {
	ctx, err := filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	bindingInput := in
	bindingInput.Cursor = nil
	items, page, err := runtime.CollectCursorWith(ctx, app, "illust_recommended", bindingInput, in.Cursor, in.PageLimitIn, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := records.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: page}
	return outputs.Result(out, false), out, nil
}
