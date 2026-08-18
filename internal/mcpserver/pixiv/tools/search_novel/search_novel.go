// Package search_novel 实现 search_novel tool。
package search_novel

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

// Register 注册 search_novel。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "search_novel", Description: "Search for novels using keywords with supported filters.", InputSchema: searchNovelInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input searchNovelIn) (*mcp.CallToolResult, outputs.Records, error) {
		return handleSearchNovel(ctx, app, input)
	})
}

type searchNovelIn struct {
	Word         string               `json:"word"`
	SearchTarget string               `json:"search_target,omitempty"`
	Sort         string               `json:"sort,omitempty"`
	Duration     string               `json:"duration,omitempty"`
	NovelFilter  *filters.NovelFilter `json:"novel_filter,omitempty"`
	runtime.PageLimitIn
}

// searchNovelInputSchema 只声明 App API 的稳定检索语义与已接通的本地筛选。
// rating、正文长度和原创筛选在可靠接口证据与完整跨批次语义确定前不发布。
func searchNovelInputSchema() map[string]any {
	stringProperty := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"word"},
		"properties": map[string]any{
			"word":          stringProperty("Novel search keyword."),
			"search_target": stringProperty("Pixiv novel search target."),
			"sort":          stringProperty("Pixiv novel result order."),
			"duration":      stringProperty("Pixiv novel search duration."),
			"page":          map[string]any{"type": "integer", "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "description": "Maximum logical results; 0 returns all; omit for one logical batch."},
			"novel_filter":  filters.NovelFilterSchema(),
		},
	}
}

func handleSearchNovel(ctx context.Context, app *runtime.App, in searchNovelIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(pixiv.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(pixiv.SortModeDateDesc)
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.SearchNovels(ctx, pixiv.SearchNovelsRequest{
			Word: in.Word, Target: pixiv.SearchTarget(in.SearchTarget), Sort: pixiv.SortMode(in.Sort), Duration: pixiv.DurationFilter(in.Duration), Cursor: cursor,
		})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := records.FromNovels(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}
