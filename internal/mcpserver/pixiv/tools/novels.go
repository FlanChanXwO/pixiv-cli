// pixiv novels tools.

package tools

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerNovelBookmarkDetail 注册 pixiv_novel_bookmark_detail。
func (t pixivTools) registerNovelBookmarkDetail(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_novel_bookmark_detail", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get the current user's bookmark state for one novel.", OutputSchema: schemas.BookmarkDetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input novelBookmarkDetailIn) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
		return t.novelBookmarkDetailHandler(ctx, input)
	})
}

type novelBookmarkDetailIn struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive novel ID"`
}

func (t pixivTools) novelBookmarkDetailHandler(ctx context.Context, in novelBookmarkDetailIn) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
	if in.NovelID <= 0 {
		return outputs.BookmarkDetailError(errors.New("novel_id must be a positive integer"))
	}
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelBookmarkDetail, error) {
		return client.NovelBookmark(ctx, pixiv.NovelBookmarkRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.BookmarkDetailError(err)
	}
	out := outputs.BookmarkDetail{Bookmarked: result.Restrict != "", Restrict: string(result.Restrict), Tags: append([]string{}, result.Tags...)}
	return outputs.NovelBookmarkDetailResult(out, in.NovelID), out, nil
}

// registerNovelContent 注册 pixiv_novel_content。
func (t pixivTools) registerNovelContent(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_novel_content", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read the complete structured content of one Pixiv novel.", OutputSchema: schemas.NovelContentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input novelContentIn) (*mcp.CallToolResult, outputs.NovelContent, error) {
		return t.novelContentHandler(ctx, input)
	})
}

type novelContentIn struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive Pixiv novel ID"`
}

func (t pixivTools) novelContentHandler(ctx context.Context, in novelContentIn) (*mcp.CallToolResult, outputs.NovelContent, error) {
	if in.NovelID <= 0 {
		return outputs.NovelContentError(errors.New("novel_id must be a positive integer"))
	}
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
		return client.NovelContent(ctx, pixiv.NovelContentRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.NovelContentError(err)
	}
	out := outputs.NovelContent{Content: result}
	return outputs.NovelContentResult(result.NovelID), out, nil
}

// registerNovelDetail 注册 pixiv_novel_detail。
func (t pixivTools) registerNovelDetail(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_novel_detail", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get detailed information for one Pixiv novel.", OutputSchema: schemas.DetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input novelDetailIn) (*mcp.CallToolResult, outputs.Detail, error) {
		return t.novelDetailHandler(ctx, input)
	})
}

type novelDetailIn struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive Pixiv novel ID"`
}

func (t pixivTools) novelDetailHandler(ctx context.Context, in novelDetailIn) (*mcp.CallToolResult, outputs.Detail, error) {
	if in.NovelID <= 0 {
		return outputs.DetailError(errors.New("novel_id must be a positive integer"))
	}
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.Novel, error) {
		return client.Novel(ctx, pixiv.NovelRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.DetailError(err)
	}
	novelRecord, err := record.RecordFromNovel(result)
	if err != nil {
		return outputs.DetailError(err)
	}
	out := outputs.Detail{Records: []record.Record{novelRecord}}
	return outputs.DetailResult(out), out, nil
}

// registerNovelSeries 注册 pixiv_novel_series。
func (t pixivTools) registerNovelSeries(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_novel_series", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Browse novels in a Pixiv series.", OutputSchema: schemas.NovelSeriesOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input novelSeriesIn) (*mcp.CallToolResult, outputs.NovelSeries, error) {
		return t.novelSeriesHandler(ctx, input)
	})
}

type novelSeriesIn struct {
	SeriesID int64 `json:"series_id" jsonschema:"positive Pixiv series ID"`
	toolrun.ListIn
}

func (t pixivTools) novelSeriesHandler(ctx context.Context, in novelSeriesIn) (*mcp.CallToolResult, outputs.NovelSeries, error) {
	if in.SeriesID <= 0 {
		return outputs.NovelSeriesError(errors.New("series_id must be a positive integer"))
	}
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.NovelSeriesError(err)
	}
	var series pixiv.NovelSeries
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.NovelSeries(ctx, pixiv.NovelSeriesRequest{SeriesID: in.SeriesID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		if cursor.IsZero() {
			series = result.Series
		}
		return result.Novels.Items, result.Novels.Next, nil
	})
	if err != nil {
		return outputs.NovelSeriesError(err)
	}
	recordItems, err := record.FromNovels(items)
	if err != nil {
		return outputs.NovelSeriesError(err)
	}
	out := outputs.NovelSeries{Series: series, Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.NovelSeriesResult(out), out, nil
}
