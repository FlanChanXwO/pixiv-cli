package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errLegacyThumbnailUnavailable = errors.New("legacy thumbnail is unavailable")

type searchIllustIn struct {
	Word             string `json:"word"`
	SearchTarget     string `json:"search_target,omitempty"`
	Sort             string `json:"sort,omitempty"`
	Duration         string `json:"duration,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	SearchR18        bool   `json:"search_r18,omitempty"`
	Rating           string `json:"rating,omitempty"`
	ContentType      string `json:"content_type,omitempty"`
	AIMode           string `json:"ai_mode,omitempty"`
	AspectRatio      string `json:"aspect_ratio,omitempty"`
	Resolution       string `json:"resolution,omitempty"`
	Tool             string `json:"tool,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

type searchIllustOptionsIn struct {
	Word string `json:"word" jsonschema:"required illustration search keyword"`
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
		return "当前没有可用的绘图工具。"
	}
	return fmt.Sprintf("可用绘图工具（%d 个）:\n- %s", len(tools), strings.Join(tools, "\n- "))
}

func (a *App) searchIllustOptionsError(ctx context.Context, err error) (*mcp.CallToolResult, searchIllustOptionsOut, error) {
	recordToolError(ctx, err)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "错误: " + err.Error()}}}, searchIllustOptionsOut{Tools: []string{}, Text: "错误: " + err.Error()}, nil
}

func (a *App) searchIllust(ctx context.Context, _ *mcp.CallToolRequest, in searchIllustIn) (*mcp.CallToolResult, textOut, error) {
	// search_r18 是既有 MCP wire 字段；它只映射稳定 rating，不再改写用户关键词。
	if in.SearchR18 && in.Rating != "" {
		err := fmt.Errorf("rating and search_r18 cannot be used together: %w", errLegacyValidation)
		return toolTextError(ctx, err, err.Error())
	}
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
	if in.SearchR18 {
		filters.Rating = sdk.SearchRatingR18
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{Word: word, Target: sdk.SearchTarget(in.SearchTarget), Sort: sdk.SortMode(in.Sort), Duration: in.Duration, Cursor: cursor, Filters: filters})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("抱歉，根据您提供的关键词 '%s'，未能找到相关的插画。", word))
	}
	return toolText(fmt.Sprintf("找到 %d 张关于 '%s' 的插画:\n\n%s", len(items), word, formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
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
	IllustID         int64 `json:"illust_id"`
	Offset           int   `json:"offset,omitempty"`
	IncludeThumbnail bool  `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRelated(ctx context.Context, _ *mcp.CallToolRequest, in relatedIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
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
		return toolText(fmt.Sprintf("找不到与插画 %d 相关的推荐。", in.IllustID))
	}
	return toolText(fmt.Sprintf("找到 %d 张相关推荐:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

type rankingIn struct {
	Mode             string `json:"mode,omitempty"`
	Date             string `json:"date,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

var rankingLabels = map[sdk.RankingMode]string{
	sdk.RankingModeDay:          "每日排行榜",
	sdk.RankingModeDayMale:      "男性向每日排行榜",
	sdk.RankingModeDayFemale:    "女性向每日排行榜",
	sdk.RankingModeWeek:         "每周排行榜",
	sdk.RankingModeWeekOriginal: "原创作品排行榜",
	sdk.RankingModeWeekRookie:   "新人排行榜",
	sdk.RankingModeMonth:        "每月排行榜",
}

func rankingLabel(mode string) string {
	if label, ok := rankingLabels[sdk.RankingMode(mode)]; ok {
		return label
	}
	return mode + " 排行榜"
}

func (a *App) illustRanking(ctx context.Context, _ *mcp.CallToolRequest, in rankingIn) (*mcp.CallToolResult, textOut, error) {
	if in.Mode == "" {
		in.Mode = string(sdk.RankingModeDay)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
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
		return toolText(fmt.Sprintf("找不到模式为 '%s' 的排行榜结果。", in.Mode))
	}
	return toolText(fmt.Sprintf("%s:\n\n%s", rankingLabel(in.Mode), formatIllusts(items, in.IncludeThumbnail, in.Offset, true)))
}

type searchUserIn struct {
	Word   string `json:"word"`
	Offset int    `json:"offset,omitempty"`
}

func (a *App) searchUser(ctx context.Context, _ *mcp.CallToolRequest, in searchUserIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
		result, err := client.SearchUser(ctx, sdk.SearchUserRequest{Word: in.Word, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	})
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("抱歉，未能找到名为 '%s' 的用户。", in.Word))
	}
	return toolText(fmt.Sprintf("找到 %d 位用户:\n\n%s", len(items), formatUsers(items)))
}

// offsetPlan 将 legacy offset 映射为 SDK cursor 的逻辑跳过。它保留旧工具“从
// offset 位置取一批”的可观察选择，不把已废弃上游 offset 泄露给公开 SDK。
func offsetPlan(offset int) mcpListPlan {
	if offset < 0 {
		offset = 0
	}
	return mcpListPlan{page: 1, limit: -1, oneBatch: true, skip: offset}
}

type offsetThumbnailIn struct {
	Offset           int  `json:"offset,omitempty"`
	IncludeThumbnail bool `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRecommended(ctx context.Context, _ *mcp.CallToolRequest, in offsetThumbnailIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
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
		return toolText("暂无推荐内容。")
	}
	return toolText(fmt.Sprintf("为您推荐 %d 张插画:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
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
		return toolText("无法获取热门标签。")
	}
	lines := make([]string, 0, len(result.TrendTags))
	for _, tag := range result.TrendTags {
		translated := tag.TranslatedName
		if translated == "" {
			translated = "无"
		}
		lines = append(lines, fmt.Sprintf("- %s (翻译: %s)", tag.Tag, translated))
	}
	return toolText("当前的热门标签:\n" + strings.Join(lines, "\n"))
}

type followIn struct {
	Restrict         string `json:"restrict,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

func (a *App) illustFollow(ctx context.Context, _ *mcp.CallToolRequest, in followIn) (*mcp.CallToolResult, textOut, error) {
	if in.Restrict == "" {
		in.Restrict = string(sdk.RestrictPublic)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
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
		return toolText("您的关注动态中暂时没有新作品。")
	}
	return toolText(fmt.Sprintf("找到 %d 篇关注动态:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

func (a *App) thumbnailBase64(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolTextError(ctx, err, "错误: 无法获取插画信息: "+err.Error())
	}
	defer release()
	result, err := client.IllustDetail(ctx, in.IllustID)
	if err != nil {
		return toolTextError(ctx, err, "错误: 无法获取插画信息: "+err.Error())
	}
	rawURL := thumbnailURL(result.Illust)
	if rawURL == "" {
		return toolTextError(ctx, errLegacyThumbnailUnavailable, "错误: 无法找到缩略图URL")
	}
	ref, err := client.ParseResourceRef(rawURL)
	if err != nil {
		return toolTextError(ctx, err, "错误: 获取缩略图失败: "+err.Error())
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref})
	if err != nil {
		return toolTextError(ctx, err, "错误: 获取缩略图失败: "+err.Error())
	}
	defer response.Body.Close()
	var buf strings.Builder
	writer := base64.NewEncoder(base64.StdEncoding, stringWriter{&buf})
	if _, err := io.Copy(writer, response.Body); err != nil {
		_ = writer.Close()
		return toolTextError(ctx, err, "错误: 获取缩略图失败: "+err.Error())
	}
	if err := writer.Close(); err != nil {
		return toolTextError(ctx, err, "错误: 获取缩略图失败: "+err.Error())
	}
	return toolText(fmt.Sprintf("缩略图数据 (插画ID: %d):\ndata:image/jpeg;base64,%s", in.IllustID, buf.String()))
}
