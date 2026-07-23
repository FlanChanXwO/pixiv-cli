package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errLegacyThumbnailUnavailable = errors.New("legacy thumbnail is unavailable")

type searchIllustIn struct {
	Word         string `json:"word"`
	SearchTarget string `json:"search_target,omitempty"`
	Sort         string `json:"sort,omitempty"`
	Duration     string `json:"duration,omitempty"`
	Page         *int   `json:"page,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
	Rating       string `json:"rating,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	AIMode       string `json:"ai_mode,omitempty"`
	AspectRatio  string `json:"aspect_ratio,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	Tool         string `json:"tool,omitempty"`
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
			"word":          stringProperty("Illustration search keyword."),
			"search_target": stringProperty("Pixiv search target."),
			"sort":          stringProperty("Pixiv result order."),
			"duration":      stringProperty("Pixiv search duration."),
			"page":          map[string]any{"type": "integer", "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "description": "Maximum logical results; 0 returns all; omit for one logical batch."},
			"rating":        enumProperty("Artwork age rating filter.", "all", "sfw", "r18", "r18g", "mature"),
			"content_type":  enumProperty("Artwork content type filter.", "all", "illust-and-ugoira", "illust", "manga", "ugoira"),
			"ai_mode":       enumProperty("AI-generated artwork filter.", "all", "exclude", "only"),
			"aspect_ratio":  enumProperty("Artwork aspect ratio filter.", "all", "landscape", "portrait", "square"),
			"resolution":    enumProperty("Artwork resolution tier filter.", "all", "high", "medium", "low"),
			"tool":          stringProperty("Exact Pixiv drawing tool name from search_illust_options."),
		},
	}
}

type searchIllustOptionsIn struct {
	Word string `json:"word" jsonschema:"required illustration search keyword"`
}

type searchNovelIn struct {
	Word          string `json:"word"`
	SearchTarget  string `json:"search_target,omitempty"`
	Sort          string `json:"sort,omitempty"`
	Duration      string `json:"duration,omitempty"`
	Rating        string `json:"rating,omitempty"`
	MinTextLength int    `json:"min_text_length,omitempty"`
	MaxTextLength int    `json:"max_text_length,omitempty"`
	OriginalOnly  bool   `json:"original_only,omitempty"`
	pageLimitIn
}

// searchNovelInputSchema 只声明 App API 的稳定检索语义与可由返回字段验证的筛选。
// 正文长度和原创筛选由 public SDK 在连续批次中验证，因此没有伪造上游 query 参数。
func searchNovelInputSchema() map[string]any {
	stringProperty := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	rating := stringProperty("Novel age rating filter.")
	rating["enum"] = []string{"all", "sfw", "r18", "r18g", "mature"}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"word"},
		"properties": map[string]any{
			"word":            stringProperty("Novel search keyword."),
			"search_target":   stringProperty("Pixiv novel search target."),
			"sort":            stringProperty("Pixiv novel result order."),
			"duration":        stringProperty("Pixiv novel search duration."),
			"rating":          rating,
			"min_text_length": map[string]any{"type": "integer", "minimum": 0, "description": "Minimum novel text length; 0 disables the bound."},
			"max_text_length": map[string]any{"type": "integer", "minimum": 0, "description": "Maximum novel text length; 0 disables the bound."},
			"original_only":   map[string]any{"type": "boolean", "description": "Return only original novels."},
			"page":            map[string]any{"type": "integer", "description": "1-based logical page; requires a positive limit."},
			"limit":           map[string]any{"type": "integer", "description": "Maximum logical results; 0 returns all; omit for one logical batch."},
		},
	}
}

type novelSearchOut struct {
	Novels     []sdk.Novel   `json:"novels"`
	Pagination paginationOut `json:"pagination"`
	Text       string        `json:"text"`
}

func novelSearchResult(out novelSearchOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) searchNovel(ctx context.Context, _ *mcp.CallToolRequest, in searchNovelIn) (*mcp.CallToolResult, novelSearchOut, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(sdk.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(sdk.SortModeDateDesc)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.searchNovelError(ctx, err)
	}
	filters := sdk.NovelSearchFilters{
		Rating:        sdk.SearchRating(in.Rating),
		MinTextLength: in.MinTextLength,
		MaxTextLength: in.MaxTextLength,
		OriginalOnly:  in.OriginalOnly,
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.searchNovelError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		return nextNonEmptySearchBatch(ctx, cursor, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
			result, err := client.SearchNovel(ctx, sdk.SearchNovelRequest{
				Word: in.Word, Target: sdk.SearchTarget(in.SearchTarget), Sort: sdk.SortMode(in.Sort), Duration: in.Duration, Cursor: cursor, Filters: filters,
			})
			if err != nil {
				return nil, "", err
			}
			if result == nil {
				return nil, "", errors.New("pixiv sdk returned an empty novel search result")
			}
			return result.Novels, result.NextCursor, nil
		})
	})
	if err != nil {
		return a.searchNovelError(ctx, err)
	}
	if items == nil {
		items = []sdk.Novel{}
	}
	text := fmt.Sprintf("Found %d novels for %q:\n\n%s", len(items), in.Word, formatNovels(items))
	if len(items) == 0 {
		text = fmt.Sprintf("No novels found for %q.", in.Word)
	}
	out := novelSearchOut{Novels: normalizeNovels(items), Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return novelSearchResult(out), out, nil
}

func (a *App) searchNovelError(ctx context.Context, err error) (*mcp.CallToolResult, novelSearchOut, error) {
	recordToolError(ctx, err)
	text := "Error: " + err.Error()
	out := novelSearchOut{Novels: []sdk.Novel{}, Pagination: paginationOut{Page: 1}, Text: text}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
}

type searchIllustOptionsOut struct {
	Tools []string `json:"tools"`
	Text  string   `json:"text"`
}

// searchIllustOptions 只把 MCP 输入转交给公开 SDK；工具名保持上游顺序和原值，
// 使调用方不需要跟随 MCP 发版才能识别 Pixiv 新增的绘图工具。
func (a *App) searchIllustOptions(ctx context.Context, _ *mcp.CallToolRequest, in searchIllustOptionsIn) (*mcp.CallToolResult, searchIllustOptionsOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.searchIllustOptionsError(ctx, err)
	}
	defer release()
	result, err := client.SearchIllustOptions(ctx, sdk.SearchIllustOptionsRequest{Word: in.Word})
	if err != nil {
		return a.searchIllustOptionsError(ctx, err)
	}
	if result == nil {
		return a.searchIllustOptionsError(ctx, errors.New("pixiv sdk returned an empty search options result"))
	}
	tools := append([]string(nil), result.Tools...)
	if tools == nil {
		tools = []string{}
	}
	text := searchIllustOptionsText(tools)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, searchIllustOptionsOut{Tools: tools, Text: text}, nil
}

// searchIllustOptionsText 保留全部工具名和上游顺序，兼容只读取 Content 文本、
// 不读取 StructuredContent 的旧 MCP 客户端。
func searchIllustOptionsText(tools []string) string {
	if len(tools) == 0 {
		return "No drawing tools are available."
	}
	quoted := make([]string, len(tools))
	for index, tool := range tools {
		quoted[index] = strconv.Quote(tool)
	}
	return fmt.Sprintf("Available drawing tools (%d):\n- %s", len(tools), strings.Join(quoted, "\n- "))
}

func (a *App) searchIllustOptionsError(ctx context.Context, err error) (*mcp.CallToolResult, searchIllustOptionsOut, error) {
	recordToolError(ctx, err)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}}}, searchIllustOptionsOut{Tools: []string{}, Text: "Error: " + err.Error()}, nil
}

func (a *App) searchIllust(ctx context.Context, _ *mcp.CallToolRequest, in searchIllustIn) (*mcp.CallToolResult, textOut, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(sdk.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(sdk.SortModeDateDesc)
	}
	word := in.Word
	filters := sdk.SearchIllustFilters{
		Rating:      sdk.SearchRating(in.Rating),
		ContentType: sdk.SearchContentType(in.ContentType),
		AIMode:      sdk.SearchAIMode(in.AIMode),
		AspectRatio: sdk.SearchAspectRatio(in.AspectRatio),
		Resolution:  sdk.SearchResolution(in.Resolution),
		Tool:        in.Tool,
	}
	plan, err := searchIllustListPlan(in)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	// 本地筛选（rating/AI）可能产生空上游批次；nextNonEmpty + CollectPages 共同保证
	// 默认逻辑批、limit 填满与 page 逻辑分页都跳过连续空批。
	items, _, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		return nextNonEmptySearchBatch(ctx, cursor, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{Word: word, Target: sdk.SearchTarget(in.SearchTarget), Sort: sdk.SortMode(in.Sort), Duration: in.Duration, Cursor: cursor, Filters: filters})
			if err != nil {
				return nil, "", err
			}
			return result.Illusts, result.NextCursor, nil
		})
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("No illustrations found for %q.", word))
	}
	displayOffset := plan.skip
	return toolText(fmt.Sprintf("Found %d illustrations for %q:\n\n%s", len(items), word, formatIllusts(items, displayOffset, false)))
}

// searchIllustListPlan 将 search_illust 的 page/limit 归一为 mcpListPlan。
// 省略 limit 时保持单逻辑批次（含空批补拉）。
func searchIllustListPlan(in searchIllustIn) (mcpListPlan, error) {
	return parseMCPListPlan(pageLimitIn{Page: in.Page, Limit: in.Limit})
}

// nextNonEmptySearchBatch 把 SDK 本地过滤产生的连续空上游批次折叠为一个逻辑
// 批次。它只在真正结束或首次出现结果时停止，不设置任意页数上限。
func nextNonEmptySearchBatch[T any](ctx context.Context, cursor sdk.Cursor, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, sdk.Cursor, error) {
	seen := make(map[sdk.Cursor]struct{})
	for {
		if _, exists := seen[cursor]; exists {
			return nil, "", fmt.Errorf("pagination cursor repeated: %q", cursor)
		}
		seen[cursor] = struct{}{}
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, "", err
		}
		if len(items) > 0 || next == "" {
			return items, next, nil
		}
		cursor = next
	}
}

type illustIDIn struct {
	IllustID int64 `json:"illust_id"`
}

func (a *App) illustDetail(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	result, err := client.IllustDetail(ctx, in.IllustID)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	body, _ := json.MarshalIndent(result.Illust, "", "  ")
	return toolText(string(body))
}

type relatedIn struct {
	IllustID int64 `json:"illust_id"`
	pageLimitIn
}

func (a *App) illustRelated(ctx context.Context, _ *mcp.CallToolRequest, in relatedIn) (*mcp.CallToolResult, textOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRelated(ctx, sdk.IllustRelatedRequest{IllustID: in.IllustID, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("No related illustrations found for artwork %d.", in.IllustID))
	}
	return toolText(fmt.Sprintf("Found %d related illustrations:\n\n%s", len(items), formatIllusts(items, plan.skip, false)))
}

type rankingIn struct {
	Mode string `json:"mode,omitempty"`
	Date string `json:"date,omitempty"`
	pageLimitIn
}

var rankingLabels = map[sdk.RankingMode]string{
	sdk.RankingModeDay:             "Daily ranking",
	sdk.RankingModeDayMale:         "Daily ranking (male)",
	sdk.RankingModeDayFemale:       "Daily ranking (female)",
	sdk.RankingModeWeek:            "Weekly ranking",
	sdk.RankingModeWeekOriginal:    "Weekly original ranking",
	sdk.RankingModeWeekRookie:      "Weekly rookie ranking",
	sdk.RankingModeMonth:           "Monthly ranking",
	sdk.RankingModeDayManga:        "Daily manga ranking",
	sdk.RankingModeWeekManga:       "Weekly manga ranking",
	sdk.RankingModeMonthManga:      "Monthly manga ranking",
	sdk.RankingModeWeekRookieManga: "Weekly rookie manga ranking",
	sdk.RankingModeDayR18:          "Daily R-18 ranking",
	sdk.RankingModeDayMaleR18:      "Daily male R-18 ranking",
	sdk.RankingModeDayFemaleR18:    "Daily female R-18 ranking",
	sdk.RankingModeWeekR18:         "Weekly R-18 ranking",
	sdk.RankingModeWeekR18G:        "Weekly R-18G ranking",
}

func rankingLabel(mode string) string {
	if label, ok := rankingLabels[sdk.RankingMode(mode)]; ok {
		return label
	}
	return mode + " ranking"
}

func (a *App) illustRanking(ctx context.Context, _ *mcp.CallToolRequest, in rankingIn) (*mcp.CallToolResult, textOut, error) {
	if in.Mode == "" {
		in.Mode = string(sdk.RankingModeDay)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRanking(ctx, sdk.IllustRankingRequest{Mode: sdk.RankingMode(in.Mode), Date: in.Date, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("No ranking results found for mode %q.", in.Mode))
	}
	return toolText(fmt.Sprintf("%s:\n\n%s", rankingLabel(in.Mode), formatIllusts(items, plan.skip, true)))
}

type searchUserIn struct {
	Word string `json:"word"`
	pageLimitIn
}

type userSearchOut struct {
	Source       sdk.UserSearchSource `json:"source"`
	UserPreviews []sdk.UserPreview    `json:"user_previews"`
	Pagination   paginationOut        `json:"pagination"`
	Text         string               `json:"text"`
}

func userSearchResult(out userSearchOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) searchUser(ctx context.Context, _ *mcp.CallToolRequest, in searchUserIn) (*mcp.CallToolResult, userSearchOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.searchUserError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.searchUserError(ctx, err)
	}
	defer release()
	var source sdk.UserSearchSource
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
		result, err := client.SearchUser(ctx, sdk.SearchUserRequest{Word: in.Word, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		if result == nil {
			return nil, "", errors.New("pixiv sdk returned an empty user search result")
		}
		if !validUserSearchSource(result.Source) {
			return nil, "", fmt.Errorf("pixiv sdk returned an unknown user search source %q", result.Source)
		}
		if source == "" {
			source = result.Source
		} else if source != result.Source {
			return nil, "", fmt.Errorf("pixiv sdk changed user search source from %q to %q across pages", source, result.Source)
		}
		return result.UserPreviews, result.NextCursor, nil
	})
	if err != nil {
		return a.searchUserError(ctx, err)
	}
	if items == nil {
		items = []sdk.UserPreview{}
	}
	text := userSearchText(in.Word, source, items)
	out := userSearchOut{Source: source, UserPreviews: items, Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return userSearchResult(out), out, nil
}

func validUserSearchSource(source sdk.UserSearchSource) bool {
	return source == sdk.UserSearchSourceApp || source == sdk.UserSearchSourceRelatedIllustAuthors
}

func userSearchText(word string, source sdk.UserSearchSource, items []sdk.UserPreview) string {
	if len(items) == 0 {
		if source == sdk.UserSearchSourceRelatedIllustAuthors {
			return fmt.Sprintf("No related illustration authors found for %q (source: %s; not a username search).", word, source)
		}
		return fmt.Sprintf("No users found for %q (source: %s).", word, source)
	}
	if source == sdk.UserSearchSourceRelatedIllustAuthors {
		return fmt.Sprintf("Found %d related illustration authors for %q (source: %s; not a username search):\n\n%s", len(items), word, source, formatUsers(items))
	}
	return fmt.Sprintf("Found %d users for %q (source: %s):\n\n%s", len(items), word, source, formatUsers(items))
}

// searchUserError 保留既有 search_user 的非 isError wire 语义，同时新增的 structured
// output 给调用方空数组与明确文本，避免把执行失败伪装为空搜索结果。
func (a *App) searchUserError(ctx context.Context, err error) (*mcp.CallToolResult, userSearchOut, error) {
	recordToolError(ctx, err)
	text := err.Error()
	out := userSearchOut{UserPreviews: []sdk.UserPreview{}, Pagination: paginationOut{Page: 1}, Text: text}
	return userSearchResult(out), out, nil
}

type recommendedLegacyIn struct {
	pageLimitIn
}

func (a *App) illustRecommended(ctx context.Context, _ *mcp.CallToolRequest, in recommendedLegacyIn) (*mcp.CallToolResult, textOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText("No recommendations are available.")
	}
	return toolText(fmt.Sprintf("Recommended %d illustrations:\n\n%s", len(items), formatIllusts(items, plan.skip, false)))
}

func (a *App) trendingTags(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	result, err := client.TrendingTagsIllust(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(result.TrendTags) == 0 {
		return toolText("Could not retrieve trending tags.")
	}
	lines := make([]string, 0, len(result.TrendTags))
	for _, tag := range result.TrendTags {
		translated := tag.TranslatedName
		if translated == "" {
			translated = "none"
		}
		lines = append(lines, fmt.Sprintf("- %s (translation: %s)", tag.Tag, translated))
	}
	return toolText("Trending tags:\n" + strings.Join(lines, "\n"))
}

type followIn struct {
	Restrict string `json:"restrict,omitempty"`
	pageLimitIn
}

func (a *App) illustFollow(ctx context.Context, _ *mcp.CallToolRequest, in followIn) (*mcp.CallToolResult, textOut, error) {
	if in.Restrict == "" {
		in.Restrict = string(sdk.RestrictPublic)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.FollowingIllusts(ctx, sdk.FollowingIllustsRequest{Restrict: sdk.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText("No new artworks from followed users.")
	}
	return toolText(fmt.Sprintf("Found %d new artworks from followed users:\n\n%s", len(items), formatIllusts(items, plan.skip, false)))
}
