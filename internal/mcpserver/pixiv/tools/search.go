// pixiv search tools.

package tools

import (
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"time"
)

// artworkSearchClient 是 bookmark 过滤所需的最小 public SDK capability。
type artworkSearchClient interface {
	SearchArtworks(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	CheckpointSearchArtworks(pixiv.SearchArtworksRequest, int) (sdk.Cursor, error)
}

type artworkSearchOutcome struct {
	Page   sdk.Page[pixiv.Artwork]
	Filter *bookmarkFilterOutcome
}

type bookmarkMembership string

const (
	bookmarkMembershipPremium    bookmarkMembership = "premium"
	bookmarkMembershipNonPremium bookmarkMembership = "non_premium"
	bookmarkMembershipUnknown    bookmarkMembership = "unknown"
)

type bookmarkFilterStrategy string

const (
	bookmarkFilterStrategyAuto       bookmarkFilterStrategy = "auto"
	bookmarkFilterStrategyServer     bookmarkFilterStrategy = "server"
	bookmarkFilterStrategyLocal      bookmarkFilterStrategy = "local"
	bookmarkFilterStrategyBestEffort bookmarkFilterStrategy = "best_effort"
)

type bookmarkFilterCompleteness string

const (
	bookmarkFilterCompletenessPartial           bookmarkFilterCompleteness = "partial"
	bookmarkFilterCompletenessCompleteForSource bookmarkFilterCompleteness = "complete_for_source"
)

type bookmarkFilterOutcome struct {
	Min          *int
	Max          *int
	Membership   bookmarkMembership
	Strategy     bookmarkFilterStrategy
	Completeness bookmarkFilterCompleteness
}

type artworkSearchRequest struct {
	Query      pixiv.SearchArtworksRequest
	Plan       pagination.PagePlan
	Membership bookmarkMembership
	Strategy   bookmarkFilterStrategy
	Include    func(pixiv.Artwork) (bool, error)
}

// searchArtworks 在 MCP adapter 内执行 bookmark 候选筛选。
// auto 只在当前已验证策略下解析为 local；server 不做未经证实的降级。
func searchArtworks[C artworkSearchClient](ctx context.Context, execute pagination.Execute[C], request artworkSearchRequest) (artworkSearchOutcome, error) {
	var outcome artworkSearchOutcome
	min, max, hasRange, err := validateArtworkBookmarkSearch(request)
	if err != nil {
		return outcome, err
	}
	membership, err := normalizeBookmarkMembership(request.Membership)
	if err != nil {
		return outcome, err
	}
	strategy, err := resolveBookmarkStrategy(request.Strategy, membership, hasRange)
	if err != nil {
		return outcome, err
	}
	if execute == nil {
		return outcome, sdk.NewError("pixiv", "SearchArtworks", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}

	err = execute(ctx, func(ctx context.Context, client C) (bool, error) {
		if !hasRange {
			page, err := searchArtworkPages(ctx, client, request.Query, request.Plan, request.Include)
			if err != nil {
				return false, err
			}
			outcome = artworkSearchOutcome{Page: page}
			if strategy == bookmarkFilterStrategyBestEffort {
				outcome.Filter = &bookmarkFilterOutcome{
					Membership: membership, Strategy: strategy, Completeness: bookmarkFilterCompletenessPartial,
				}
			}
			return false, nil
		}

		candidateQuery := request.Query
		candidateQuery.CursorContext = searchfilter.BookmarkContext(min, max, string(strategy))
		if strategy == bookmarkFilterStrategyLocal {
			// App API bounds 只是 candidate 条件；local 必须枚举正常候选流再本地复核。
			candidateQuery.BookmarkMin = nil
			candidateQuery.BookmarkMax = nil
		}
		items, next, pageResult, err := pagination.CollectFilteredPagesFrom(
			ctx,
			request.Plan,
			candidateQuery.Cursor,
			func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				candidateQuery.Cursor = cursor
				page, err := client.SearchArtworks(ctx, candidateQuery)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return page.Items, page.Next, nil
			},
			func(item pixiv.Artwork) (bool, error) {
				if item.TotalBookmarks < 0 {
					return false, sdk.NewError("pixiv", "SearchArtworks", sdk.MalformedUpstreamResponse,
						sdk.WithDetail("artwork bookmark count is negative"))
				}
				keep := (min == nil || item.TotalBookmarks >= *min) && (max == nil || item.TotalBookmarks <= *max)
				if !keep || request.Include == nil {
					return keep, nil
				}
				return request.Include(item)
			},
			func(cursor sdk.Cursor, consumed int) (sdk.Cursor, error) {
				checkpointQuery := candidateQuery
				checkpointQuery.Cursor = cursor
				return client.CheckpointSearchArtworks(checkpointQuery, consumed)
			},
		)
		if err != nil {
			return false, err
		}
		completeness := bookmarkFilterCompletenessCompleteForSource
		if strategy == bookmarkFilterStrategyBestEffort || pageResult.HasMore || !next.IsZero() {
			completeness = bookmarkFilterCompletenessPartial
		}
		outcome = artworkSearchOutcome{
			Page: sdk.Page[pixiv.Artwork]{Items: items, Next: next},
			Filter: &bookmarkFilterOutcome{
				Min:          cloneIntPointer(min),
				Max:          cloneIntPointer(max),
				Membership:   membership,
				Strategy:     strategy,
				Completeness: completeness,
			},
		}
		return false, nil
	})
	if err != nil {
		return artworkSearchOutcome{}, err
	}
	return outcome, nil
}

func bookmarkFilterFrom(value *bookmarkFilterOutcome) *outputs.BookmarkFilter {
	if value == nil {
		return nil
	}
	return &outputs.BookmarkFilter{
		Min:          value.Min,
		Max:          value.Max,
		Membership:   string(value.Membership),
		Strategy:     string(value.Strategy),
		Completeness: string(value.Completeness),
	}
}

func validateArtworkBookmarkSearch(request artworkSearchRequest) (min, max *int, hasRange bool, err error) {
	if request.Query.BookmarkMin != nil && *request.Query.BookmarkMin < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark minimum must be zero or positive"))
	}
	if request.Query.BookmarkMax != nil && *request.Query.BookmarkMax < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark maximum must be zero or positive"))
	}
	if request.Query.BookmarkMin != nil && request.Query.BookmarkMax != nil && *request.Query.BookmarkMin > *request.Query.BookmarkMax {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark minimum must not exceed maximum"))
	}
	if request.Plan.Skip < 0 || request.Plan.Limit < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("page skip and limit must be zero or positive"))
	}
	return request.Query.BookmarkMin, request.Query.BookmarkMax, request.Query.BookmarkMin != nil || request.Query.BookmarkMax != nil, nil
}

func normalizeBookmarkMembership(value bookmarkMembership) (bookmarkMembership, error) {
	switch value {
	case "", bookmarkMembershipUnknown:
		return bookmarkMembershipUnknown, nil
	case bookmarkMembershipPremium, bookmarkMembershipNonPremium:
		return value, nil
	default:
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("unknown bookmark membership"))
	}
}

func resolveBookmarkStrategy(requested bookmarkFilterStrategy, membership bookmarkMembership, hasRange bool) (bookmarkFilterStrategy, error) {
	if requested == "" {
		requested = bookmarkFilterStrategyAuto
	}
	switch requested {
	case bookmarkFilterStrategyAuto:
		if !hasRange {
			return bookmarkFilterStrategyAuto, nil
		}
		return bookmarkFilterStrategyLocal, nil
	case bookmarkFilterStrategyLocal, bookmarkFilterStrategyBestEffort:
		return requested, nil
	case bookmarkFilterStrategyServer:
		if membership == bookmarkMembershipNonPremium {
			return "", sdk.NewError("pixiv", "SearchArtworks", sdk.Forbidden,
				sdk.WithDetail("server bookmark strategy is not available for non-premium membership"))
		}
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.UpstreamUnavailable,
			sdk.WithDetail("server bookmark strategy requires verified premium membership and evidence"))
	default:
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("unknown bookmark filter strategy"))
	}
}

func searchArtworkPages[C artworkSearchClient](ctx context.Context, client C, query pixiv.SearchArtworksRequest, plan pagination.PagePlan, include func(pixiv.Artwork) (bool, error)) (sdk.Page[pixiv.Artwork], error) {
	if include == nil {
		include = func(pixiv.Artwork) (bool, error) { return true, nil }
	}
	items, next, _, err := pagination.CollectFilteredPagesFrom(ctx, plan, query.Cursor, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		fetchQuery := query
		fetchQuery.Cursor = cursor
		page, err := client.SearchArtworks(ctx, fetchQuery)
		return page.Items, page.Next, err
	}, include, func(cursor sdk.Cursor, consumed int) (sdk.Cursor, error) {
		checkpointQuery := query
		checkpointQuery.Cursor = cursor
		return client.CheckpointSearchArtworks(checkpointQuery, consumed)
	})
	if err != nil {
		return sdk.Page[pixiv.Artwork]{}, err
	}
	return sdk.Page[pixiv.Artwork]{Items: items, Next: next}, nil
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// registerSearchIllust 注册 pixiv_search_illust。
func (t pixivTools) registerSearchIllust(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_search_illust", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Search for illustrations using keywords with filters.", InputSchema: searchIllustInputSchema(), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input searchIllustIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.searchIllustHandler(ctx, input)
	})
}

type searchIllustIn struct {
	Cursor           *string               `json:"cursor,omitempty"`
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
			"cursor":            stringProperty("Opaque default-batch continuation; repeat original arguments and omit page and limit."),
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
			"bookmark_min":      map[string]any{"type": "integer", "minimum": 0, "description": "Inclusive minimum public bookmark count; ANDed with illust_filter before pagination; requires App OAuth."},
			"bookmark_max":      map[string]any{"type": "integer", "minimum": 0, "description": "Inclusive maximum public bookmark count; ANDed with illust_filter before pagination; requires App OAuth."},
			"bookmark_strategy": enumProperty("Bookmark count strategy; combines with illust_filter before pagination. best_effort always reports partial; server requires verified evidence and otherwise fails explicitly.", "auto", "local", "best_effort", "server"),
			"illust_filter":     filters.IllustFilterSchema(),
		},
	}
}

func (t pixivTools) searchIllustHandler(ctx context.Context, in searchIllustIn) (*mcp.CallToolResult, outputs.Records, error) {
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
	if in.Cursor != nil && !plan.OneBatch {
		return outputs.Error(errors.New("cursor requires omitted page and limit"))
	}
	bindingInput := in
	bindingInput.Cursor = nil
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
		binding, err := toolhost.BindCursor("search_illust", bindingInput)
		if err != nil {
			return outputs.Error(err)
		}
		initial, err := binding.Decode(in.Cursor)
		if err != nil {
			return outputs.Error(err)
		}
		query.Cursor = initial
		outcome, err := searchArtworks(ctx, t.app.ReadExecutor(), artworkSearchRequest{
			Query:      query,
			Plan:       pagination.PagePlan{Skip: plan.Skip, Limit: plan.Limit, OneBatch: plan.OneBatch},
			Membership: bookmarkMembershipUnknown,
			Strategy:   bookmarkFilterStrategy(in.BookmarkStrategy),
			Include: func(item pixiv.Artwork) (bool, error) {
				return filters.MatchesIllust(item, in.IllustFilter), nil
			},
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := record.FromArtworks(outcome.Page.Items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{
			Records:    recordItems,
			Pagination: toolhost.ListPagination(plan, in.Limit, len(outcome.Page.Items), !outcome.Page.Next.IsZero()),
			Filter:     bookmarkFilterFrom(outcome.Filter),
		}
		if plan.OneBatch {
			if !outcome.Page.Next.IsZero() && outcome.Page.Next == initial {
				return outputs.Error(errors.New("batch continuation cursor repeated"))
			}
			out.Pagination.NextCursor, err = binding.Encode(outcome.Page.Next)
			if err != nil {
				return outputs.Error(err)
			}
		}
		return outputs.Result(out, false), out, nil
	}
	items, page, err := toolhost.CollectCursorWith(ctx, t.app, "search_illust", bindingInput, in.Cursor, toolhost.PageLimitIn{Page: in.Page, Limit: in.Limit}, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
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
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: page}
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
		start = searchfilter.AddMonthsClamped(today, -6)
	case "within_year":
		start = searchfilter.AddMonthsClamped(today, -12)
	default:
		return "", "", false
	}
	return start.Format("2006-01-02"), today.Format("2006-01-02"), true
}

// searchIllustListPlan 将 search_illust 的 page/limit 归一为分页计划。
// 省略 limit 时保持单逻辑批次（含空批补拉）。
func searchIllustListPlan(in searchIllustIn) (toolhost.ListPlan, error) {
	return toolhost.ParseListPlan(toolhost.PageLimitIn{Page: in.Page, Limit: in.Limit})
}

// registerSearchNovel 注册 pixiv_search_novel。
func (t pixivTools) registerSearchNovel(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_search_novel", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Search for novels using keywords with supported filters.", InputSchema: searchNovelInputSchema(), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input searchNovelIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.searchNovelHandler(ctx, input)
	})
}

type searchNovelIn struct {
	Word         string               `json:"word"`
	SearchTarget string               `json:"search_target,omitempty"`
	Sort         string               `json:"sort,omitempty"`
	Duration     string               `json:"duration,omitempty"`
	NovelFilter  *filters.NovelFilter `json:"novel_filter,omitempty"`
	toolhost.PageLimitIn
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

func (t pixivTools) searchNovelHandler(ctx context.Context, in searchNovelIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(pixiv.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(pixiv.SortModeDateDesc)
	}
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
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
	recordItems, err := record.FromNovels(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}

// registerSearchUser 注册 pixiv_search_user。
func (t pixivTools) registerSearchUser(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_search_user", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Search for users/artists on Pixiv.", InputSchema: schemas.List(map[string]any{
		"word":        map[string]any{"type": "string", "minLength": 1, "description": "User search keyword."},
		"user_filter": filters.UserFilterSchema(),
	}, "word"), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input searchUserIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.searchUserHandler(ctx, input)
	})
}

type searchUserIn struct {
	Word       string              `json:"word"`
	UserFilter *filters.UserFilter `json:"user_filter,omitempty"`
	toolhost.PageLimitIn
}

func (t pixivTools) searchUserHandler(ctx context.Context, in searchUserIn) (*mcp.CallToolResult, outputs.Records, error) {
	if strings.TrimSpace(in.Word) == "" {
		return outputs.Error(sdk.NewError("pixiv", "SearchUsers", sdk.InvalidArgument,
			sdk.WithDetail("search word is required")))
	}
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithUserFilter(ctx, in.UserFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.SearchUsers(ctx, pixiv.SearchUsersRequest{Word: in.Word, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromUserPreviews(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}
