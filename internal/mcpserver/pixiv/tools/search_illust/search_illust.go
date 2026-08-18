// Package search_illust 实现 search_illust tool。
package search_illust

import (
	"context"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	dateutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/date"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 search_illust。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "search_illust", Description: "Search for illustrations using keywords with filters.", InputSchema: searchIllustInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input searchIllustIn) (*mcp.CallToolResult, outputs.Records, error) {
		return handleSearchIllust(ctx, app, input)
	})
}

type searchIllustIn struct {
	Word             string                `json:"word"`
	SearchTarget     string                `json:"search_target,omitempty"`
	Sort             string                `json:"sort,omitempty"`
	Duration         string                `json:"duration,omitempty"`
	StartDate        string                `json:"start_date,omitempty"`
	EndDate          string                `json:"end_date,omitempty"`
	Page             *int                  `json:"page,omitempty"`
	Limit            *int                  `json:"limit,omitempty"`
	ContentType      string                `json:"content_type,omitempty"`
	AIMode           string                `json:"ai_mode,omitempty"`
	AspectRatio      string                `json:"aspect_ratio,omitempty"`
	Resolution       string                `json:"resolution,omitempty"`
	Tool             string                `json:"tool,omitempty"`
	BookmarkMin      *int                  `json:"bookmark_min,omitempty"`
	BookmarkMax      *int                  `json:"bookmark_max,omitempty"`
	BookmarkStrategy string                `json:"bookmark_strategy,omitempty"`
	IllustFilter     *filters.IllustFilter `json:"illust_filter,omitempty"`
}

// searchIllustInputSchema 显式发布稳定筛选枚举。go-sdk 会在解码 handler 输入前
// 校验该 schema，因此非法枚举不会打开 SDK snapshot 或发起网络请求。
func searchIllustInputSchema() map[string]any {
	stringProperty := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	enumProperty := func(description string, values ...string) map[string]any {
		property := stringProperty(description)
		property["enum"] = values
		return property
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"word"},
		"properties": map[string]any{
			"word":              stringProperty("Illustration search keyword."),
			"search_target":     enumProperty("Pixiv search target.", "partial_match_for_tags", "exact_match_for_tags", "title_and_caption", "keyword"),
			"sort":              stringProperty("Pixiv result order."),
			"duration":          enumProperty("Pixiv quick date range; cannot be combined with start_date or end_date.", "within_last_day", "within_last_week", "within_last_month", "within_half_year", "within_year"),
			"start_date":        map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", "description": "Inclusive start date in YYYY-MM-DD; may be used with end_date."},
			"end_date":          map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", "description": "Inclusive end date in YYYY-MM-DD; may be used with start_date."},
			"page":              map[string]any{"type": "integer", "description": "1-based logical page; requires a positive limit."},
			"limit":             map[string]any{"type": "integer", "description": "Maximum logical results; 0 returns all; omit for one logical batch."},
			"content_type":      enumProperty("Artwork content type filter.", "all", "illust-and-ugoira", "illust", "manga", "ugoira"),
			"ai_mode":           enumProperty("AI-generated artwork filter.", "all", "exclude", "only"),
			"aspect_ratio":      enumProperty("Artwork aspect ratio filter.", "all", "landscape", "portrait", "square"),
			"resolution":        enumProperty("Artwork resolution tier filter.", "all", "high", "medium", "low"),
			"tool":              stringProperty("Exact drawing tool name from the versioned pixiv-cli drawing-tool catalog."),
			"bookmark_min":      map[string]any{"type": "integer", "minimum": 0, "description": "Inclusive minimum public bookmark count; requires App OAuth."},
			"bookmark_max":      map[string]any{"type": "integer", "minimum": 0, "description": "Inclusive maximum public bookmark count; requires App OAuth."},
			"bookmark_strategy": enumProperty("Bookmark count strategy; server requires verified evidence and otherwise fails explicitly.", "auto", "local", "best_effort", "server"),
			"illust_filter":     filters.IllustFilterSchema(),
		},
	}
}

func handleSearchIllust(ctx context.Context, app *runtime.App, in searchIllustIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(pixiv.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(pixiv.SortModeDateDesc)
	}
	// 官方 App 以日期边界而非 duration 表达半年和一年，先在本地展开，
	// 使 MCP 与 CLI 的快捷日期语义保持一致。
	if in.StartDate == "" && in.EndDate == "" {
		if startDate, endDate, ok := quickDateRange(in.Duration, time.Now()); ok {
			in.Duration = ""
			in.StartDate = startDate
			in.EndDate = endDate
		}
	}
	word := in.Word
	if err := validateSearchIllustInput(in); err != nil {
		return outputs.Error(err)
	}
	plan, err := searchIllustListPlan(in)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	query := pixiv.SearchArtworksRequest{
		Word: word, Target: pixiv.SearchTarget(in.SearchTarget), Sort: pixiv.SortMode(in.Sort), Duration: pixiv.DurationFilter(in.Duration),
		StartDate: in.StartDate, EndDate: in.EndDate, ContentType: pixiv.SearchContentType(in.ContentType), AIMode: pixiv.SearchAIMode(in.AIMode),
		AspectRatio: pixiv.SearchAspectRatio(in.AspectRatio), Resolution: pixiv.SearchResolution(in.Resolution), Tool: in.Tool,
		BookmarkMin: in.BookmarkMin, BookmarkMax: in.BookmarkMax,
	}
	if in.BookmarkMin != nil || in.BookmarkMax != nil || in.BookmarkStrategy != "" {
		if (in.BookmarkMin != nil || in.BookmarkMax != nil) && in.IllustFilter != nil {
			return outputs.Error(errors.New("bookmark range cannot be combined with illust_filter"))
		}
		outcome, err := searchArtworks(ctx, app.Execute(), artworkSearchRequest{
			Query:      query,
			Plan:       pagination.PagePlan{Skip: plan.Skip, Limit: max(0, plan.Limit), OneBatch: plan.OneBatch},
			Membership: bookmarkMembershipUnknown,
			Strategy:   bookmarkFilterStrategy(in.BookmarkStrategy),
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := records.FromArtworks(outcome.Page.Items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{
			Records:    recordItems,
			Pagination: runtime.ListPagination(plan, in.Limit, len(outcome.Page.Items), !outcome.Page.Next.IsZero()),
			Filter:     bookmarkFilterFrom(outcome.Filter),
		}
		return outputs.Result(out, false), out, nil
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		query.Cursor = cursor
		result, err := client.SearchArtworks(ctx, query)
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

func validateSearchIllustInput(in searchIllustIn) error {
	if in.Duration != "" && (in.StartDate != "" || in.EndDate != "") {
		return errors.New("duration cannot be combined with start_date or end_date")
	}
	if in.StartDate != "" && !validMCPDate(in.StartDate) || in.EndDate != "" && !validMCPDate(in.EndDate) {
		return errors.New("start_date and end_date must use YYYY-MM-DD")
	}
	if in.StartDate != "" && in.EndDate != "" && in.StartDate > in.EndDate {
		return errors.New("start_date cannot be later than end_date")
	}
	if in.BookmarkMin != nil && in.BookmarkMax != nil && *in.BookmarkMin > *in.BookmarkMax {
		return errors.New("bookmark_min cannot be greater than bookmark_max")
	}
	return nil
}

func validMCPDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

const tokyoOffsetSeconds = 9 * 60 * 60

// quickDateRange 将 Pixiv 的长日期快捷项转换为包含边界的日本日期。
// App 搜索接口对半年和一年使用 start_date/end_date，而不是可靠的 duration 枚举。
func quickDateRange(value string, now time.Time) (startDate, endDate string, ok bool) {
	today := now.In(time.FixedZone("Asia/Tokyo", tokyoOffsetSeconds))
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	var start time.Time
	switch value {
	case "within_half_year":
		start = dateutil.AddMonthsClamped(today, -6)
	case "within_year":
		start = dateutil.AddMonthsClamped(today, -12)
	default:
		return "", "", false
	}
	return start.Format("2006-01-02"), today.Format("2006-01-02"), true
}

// searchIllustListPlan 将 search_illust 的 page/limit 归一为分页计划。
// 省略 limit 时保持单逻辑批次（含空批补拉）。
func searchIllustListPlan(in searchIllustIn) (runtime.ListPlan, error) {
	return runtime.ParseListPlan(runtime.PageLimitIn{Page: in.Page, Limit: in.Limit})
}
