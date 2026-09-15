// Package timeline_illust_latest 实现 timeline_illust_latest tool。
package timeline_illust_latest

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 timeline_illust_latest。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "timeline_illust_latest", Description: "Browse latest illustrations or manga through the App API.", InputSchema: latestInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleIllustNew(ctx, app, input)
	})
}

type In struct {
	ContentType  pixiv.SearchContentType `json:"content_type" jsonschema:"required: illust or manga"`
	IllustFilter *filters.IllustFilter   `json:"illust_filter,omitempty"`
	runtime.PageLimitIn
}

func latestInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"content_type"},
		"properties": map[string]any{
			"content_type":  map[string]any{"type": "string", "enum": []string{"illust", "manga"}},
			"illust_filter": filters.IllustFilterSchema(),
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func handleIllustNew(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	if in.ContentType != pixiv.SearchContentTypeIllust && in.ContentType != pixiv.SearchContentTypeManga {
		return outputs.Error(errors.New("content_type must be one of: illust, manga"))
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: in.ContentType, Cursor: cursor})
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
	out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}
