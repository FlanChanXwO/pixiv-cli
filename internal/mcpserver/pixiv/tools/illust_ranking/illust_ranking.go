// Package illust_ranking 实现 illust_ranking tool。
package illust_ranking

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 illust_ranking。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "illust_ranking", Description: "Browse Pixiv rankings.", InputSchema: rankingInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input rankingIn) (*mcp.CallToolResult, outputs.Records, error) {
		return handleIllustRanking(ctx, app, input)
	})
}

type rankingIn struct {
	Mode         string                `json:"mode,omitempty"`
	Date         string                `json:"date,omitempty"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	runtime.PageLimitIn
}

var rankingModes = []string{
	"day", "day_male", "day_female", "week", "week_original", "week_rookie", "month",
	"day_manga", "week_manga", "month_manga", "week_rookie_manga", "day_r18", "day_male_r18",
	"day_female_r18", "week_r18", "week_r18g",
}

func rankingInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"mode":          map[string]any{"type": "string", "enum": rankingModes, "description": "Pixiv ranking mode; omitted defaults to day."},
			"date":          map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", "description": "Ranking date in YYYY-MM-DD."},
			"illust_filter": filters.IllustFilterSchema(),
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func isRankingMode(value string) bool {
	return slices.Contains(rankingModes, value)
}

func handleIllustRanking(ctx context.Context, app *runtime.App, in rankingIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.Mode == "" {
		in.Mode = string(pixiv.RankingModeDay)
	}
	if !isRankingMode(in.Mode) {
		return outputs.Error(errors.New("mode must be a supported Pixiv ranking mode"))
	}
	if in.Date != "" {
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			return outputs.Error(errors.New("date must be a valid YYYY-MM-DD calendar date"))
		}
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
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(in.Mode), Date: in.Date, Cursor: cursor})
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
