package tools

import (
	"context"
	"errors"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	result, err := toolhost.ReadSDK(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelBookmarkDetail, error) {
		return client.Novel(in.NovelID).Bookmark(ctx)
	})
	if err != nil {
		return outputs.BookmarkDetailError(err)
	}
	out := outputs.BookmarkDetail{Bookmarked: result.Restrict != "", Restrict: string(result.Restrict), Tags: append([]string{}, result.Tags...)}
	return outputs.NovelBookmarkDetailResult(out, in.NovelID), out, nil
}
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
	result, err := toolhost.ReadSDK(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
		return client.Novel(in.NovelID).Content(ctx)
	})
	if sdk.ReasonOf(err) == sdk.ContentUnavailable {
		// 保留既有 MCP 正文不可用诊断，同时沿用新 SDK 的错误分类与原因链。
		err = &sdk.Error{Operation: "pixiv:NovelContent", Reason: sdk.ContentUnavailable, Message: "novel content is unsupported by the v1 App API", Err: err}
	}
	if err != nil {
		return outputs.NovelContentError(err)
	}
	out := outputs.NovelContent{Content: result}
	return outputs.NovelContentResult(result.NovelID), out, nil
}
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
	novelRecord, err := toolhost.ReadSDK(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (record.Record, error) {
		result, err := client.Novel(in.NovelID).Get(ctx)
		if err != nil {
			return record.Record{}, err
		}
		novel, err := record.NovelOutput(result)
		if err != nil {
			return record.Record{}, err
		}
		return record.RecordFromNovel(novel)
	})
	if err != nil {
		return outputs.DetailError(err)
	}
	out := outputs.Detail{Records: []record.Record{novelRecord}}
	return outputs.DetailResult(out), out, nil
}
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
	var series legacysdk.NovelSeries
	items, more, err := toolhost.CollectSDKWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
		result, err := client.NovelSeries(in.SeriesID).Get(ctx, pixiv.NovelSeriesOptions{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		if cursor.IsZero() {
			series, err = record.NovelSeriesOutput(result)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		novels := make([]legacysdk.Novel, len(result.Novels.Items))
		for i, novel := range result.Novels.Items {
			novels[i], err = record.NovelOutput(novel)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return novels, result.Novels.Cursor, nil
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
