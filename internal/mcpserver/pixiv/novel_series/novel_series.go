// Package novel_series 实现 novel_series tool。
package novel_series

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 novel_series。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "novel_series", Description: "Browse novels in a Pixiv series.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.NovelSeries, error) {
		return handleNovelSeries(ctx, app, input)
	})
}

type In struct {
	SeriesID int64 `json:"series_id" jsonschema:"positive Pixiv series ID"`
	runtime.PageLimitIn
}

func validateSeriesID(id int64) error {
	if id <= 0 {
		return errors.New("series_id must be a positive integer")
	}
	return nil
}

func handleNovelSeries(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.NovelSeries, error) {
	if err := validateSeriesID(in.SeriesID); err != nil {
		return outputs.NovelSeriesError(err)
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.NovelSeriesError(err)
	}
	var series pixiv.NovelSeries
	items, more, err := runtime.CollectPooledPages(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
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
	recordItems, err := records.FromNovels(items)
	if err != nil {
		return outputs.NovelSeriesError(err)
	}
	out := outputs.NovelSeries{Series: pixiv.ToNovelSeriesDTO(series), Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.NovelSeriesResult(out), out, nil
}
