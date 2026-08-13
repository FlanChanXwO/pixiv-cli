// Package outputs 持有 Pixiv MCP 多个 tool 共享的输出类型、schema 与 result
// helper。每个 tool 一个 package，共享输出契约集中在这里，避免 tool 之间交叉
// import 或复制记录协议。
package outputs

import (
	"context"
	"errors"
	"fmt"

	pipeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	searchpixiv "github.com/FlanChanXwO/pixiv-cli/internal/search/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Records 是实体记录列表的输出 envelope。
type Records struct {
	Records    []pipeline.Record     `json:"records"`
	Pagination runtime.PaginationOut `json:"pagination"`
	Filter     *BookmarkFilter       `json:"filter,omitempty"`
}

// BookmarkFilter 描述一次 bookmark 区间筛选的执行结果元数据。
type BookmarkFilter struct {
	Min          *int   `json:"min,omitempty"`
	Max          *int   `json:"max,omitempty"`
	Membership   string `json:"membership"`
	Strategy     string `json:"strategy"`
	Completeness string `json:"completeness"`
}

// BookmarkFilterFrom 把 search workflow 的结果转为输出元数据。
func BookmarkFilterFrom(value *searchpixiv.BookmarkFilterOutcome) *BookmarkFilter {
	if value == nil {
		return nil
	}
	return &BookmarkFilter{
		Min:          value.Min,
		Max:          value.Max,
		Membership:   string(value.Membership),
		Strategy:     string(value.Strategy),
		Completeness: string(value.Completeness),
	}
}

// Result 构造实体列表的 MCP 文本摘要。
func Result(out Records, isError bool) *mcp.CallToolResult {
	return records.Result(out.Records, isError, "")
}

// Error 构造实体列表的 MCP error 摘要。
func Error(err error) (*mcp.CallToolResult, Records, error) {
	out := Records{Records: []pipeline.Record{}, Pagination: runtime.PaginationOut{Page: 1}}
	return records.Result(out.Records, true, records.ErrorMessage(err)), out, nil
}

// UserDetail 是 user_detail tool 的输出 envelope。
type UserDetail struct {
	Records []pipeline.Record `json:"records"`
}

// UserDetailResult 构造 user detail 的 MCP 摘要。
func UserDetailResult(out UserDetail, isError bool) *mcp.CallToolResult {
	return records.Result(out.Records, isError, "")
}

// UserDetailError 构造 user detail 的 MCP error 摘要。
func UserDetailError(err error) (*mcp.CallToolResult, UserDetail, error) {
	out := UserDetail{Records: []pipeline.Record{}}
	return records.Result(out.Records, true, records.ErrorMessage(err)), out, nil
}

// RecommendedPagination 分别表达每条推荐流的逻辑分页；SDK opaque cursor 不离开适配层。
type RecommendedPagination struct {
	Illust *runtime.PaginationOut `json:"illust,omitempty"`
	Manga  *runtime.PaginationOut `json:"manga,omitempty"`
	Novel  *runtime.PaginationOut `json:"novel,omitempty"`
	User   *runtime.PaginationOut `json:"user,omitempty"`
}

// Recommended 是 recommended tool 的输出 envelope。
type Recommended struct {
	Records    []pipeline.Record     `json:"records"`
	Pagination RecommendedPagination `json:"pagination"`
}

// NewRecommended 构造空的 recommended 输出。
func NewRecommended() Recommended {
	return Recommended{Records: []pipeline.Record{}}
}

// RecommendedError 构造 recommended 的 MCP error 摘要。
func RecommendedError(err error) (*mcp.CallToolResult, Recommended, error) {
	out := NewRecommended()
	return records.Result(out.Records, true, records.ErrorMessage(err)), out, nil
}

// RecommendedPage 把单条推荐流的分页转为指针输出。
func RecommendedPage(plan runtime.ListPlan, limit *int, returned int, more bool) *runtime.PaginationOut {
	value := runtime.ListPagination(plan, limit, returned, more)
	return &value
}

// TrendingTags 是 trending_tags_illust tool 的输出 envelope。
type TrendingTags struct {
	Tags []pixiv.TrendingTagDTO `json:"tags"`
	Text string                 `json:"text"`
}

// TrendingTagsResult 构造 trending tags 的 MCP 摘要。
func TrendingTagsResult(out TrendingTags, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// TrendingTagsError 构造 trending tags 的 MCP error 摘要。
func TrendingTagsError(err error) (*mcp.CallToolResult, TrendingTags, error) {
	out := TrendingTags{Tags: []pixiv.TrendingTagDTO{}, Text: "Error: " + err.Error()}
	return TrendingTagsResult(out, true), out, nil
}

// CommentToolMetadata 只缓存上游明确提供的评论总数与访问控制信息。
type CommentToolMetadata struct {
	Total  *int64
	Access *pixiv.CommentAccessControl
}

// Set 从评论页记录首个可用的元数据。
func (m *CommentToolMetadata) Set(page pixiv.CommentPage) {
	if m.Total == nil && page.Total != nil {
		value := *page.Total
		m.Total = &value
	}
	if m.Access == nil && page.AccessControl != nil {
		value := *page.AccessControl
		m.Access = &value
	}
}

// Comments 是评论 tool 的输出 envelope。
type Comments struct {
	Comments      []pixiv.CommentDTO             `json:"comments"`
	Pagination    runtime.PaginationOut          `json:"pagination"`
	Total         *int64                         `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControlDTO `json:"access_control,omitempty"`
}

// CommentsResult 构造评论的 MCP 摘要。
func CommentsResult(out Comments, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d comments.", len(out.Comments))
	}
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

// CommentsError 构造评论的 MCP error 摘要。
func CommentsError(err error) (*mcp.CallToolResult, Comments, error) {
	out := Comments{Comments: []pixiv.CommentDTO{}, Pagination: runtime.PaginationOut{Page: 1}}
	return CommentsResult(out, true, records.ErrorMessage(err)), out, nil
}

// NovelDetail 是 novel_detail tool 的输出 envelope。
type NovelDetail struct {
	Records []pipeline.Record `json:"records"`
}

// NovelDetailResult 构造 novel detail 的 MCP 摘要。
func NovelDetailResult(out NovelDetail) *mcp.CallToolResult {
	return records.Result(out.Records, false, "")
}

// NovelDetailError 构造 novel detail 的 MCP error 摘要。
func NovelDetailError(err error) (*mcp.CallToolResult, NovelDetail, error) {
	out := NovelDetail{Records: []pipeline.Record{}}
	return records.Result(out.Records, true, records.ErrorMessage(err)), out, nil
}

// NovelContent 是 novel_content tool 的输出 envelope。
type NovelContent struct {
	Content pixiv.NovelContentDTO `json:"content"`
}

// NovelContentResult 构造 novel content 的 MCP 摘要。
func NovelContentResult(novelID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved novel content for %d.", novelID)}}}
}

// NovelContentError 构造 novel content 的 MCP error 摘要。
func NovelContentError(err error) (*mcp.CallToolResult, NovelContent, error) {
	out := NovelContent{Content: pixiv.NovelContentDTO{Blocks: []pixiv.NovelBlockDTO{}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: records.ErrorMessage(err)}}}, out, nil
}

// NovelSeries 是 novel_series tool 的输出 envelope。
type NovelSeries struct {
	Series     pixiv.NovelSeriesDTO  `json:"series"`
	Records    []pipeline.Record     `json:"records"`
	Pagination runtime.PaginationOut `json:"pagination"`
}

// NovelSeriesResult 构造 novel series 的 MCP 摘要。
func NovelSeriesResult(out NovelSeries) *mcp.CallToolResult {
	return records.Result(out.Records, false, "")
}

// NovelSeriesError 构造 novel series 的 MCP error 摘要。
func NovelSeriesError(err error) (*mcp.CallToolResult, NovelSeries, error) {
	out := NovelSeries{Records: []pipeline.Record{}}
	return records.Result(out.Records, true, records.ErrorMessage(err)), out, nil
}

// BookmarkTags 是 bookmark_tags tool 的输出 envelope。
type BookmarkTags struct {
	Tags       []pixiv.BookmarkTagDTO `json:"bookmark_tags"`
	Pagination runtime.PaginationOut  `json:"pagination"`
}

// BookmarkTagsResult 构造 bookmark tags 的 MCP 摘要。
func BookmarkTagsResult(out BookmarkTags, count int) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d bookmark tags.", count)}}}
}

// BookmarkTagsError 构造 bookmark tags 的 MCP error 摘要。
func BookmarkTagsError(err error) (*mcp.CallToolResult, BookmarkTags, error) {
	out := BookmarkTags{Tags: []pixiv.BookmarkTagDTO{}, Pagination: runtime.PaginationOut{Page: 1}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: records.ErrorMessage(err)}}}, out, nil
}

// BookmarkDetail 是 bookmark_detail tool 的输出 envelope。
type BookmarkDetail struct {
	Bookmarked bool     `json:"bookmarked"`
	Restrict   string   `json:"restrict,omitempty"`
	Tags       []string `json:"tags"`
}

// BookmarkDetailResult 构造 bookmark detail 的 MCP 摘要。
func BookmarkDetailResult(out BookmarkDetail, illustID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artwork %d bookmarked: %t.", illustID, out.Bookmarked)}}}
}

// BookmarkDetailError 构造 bookmark detail 的 MCP error 摘要。
func BookmarkDetailError(err error) (*mcp.CallToolResult, BookmarkDetail, error) {
	out := BookmarkDetail{Tags: []string{}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: records.ErrorMessage(err)}}}, out, nil
}

// Mutation 是 mutation tool 的输出 envelope。
type Mutation struct {
	Success  bool   `json:"success"`
	Action   string `json:"action"`
	IllustID int64  `json:"illust_id,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	Text     string `json:"text"`
}

// MutationResult 构造 mutation 的 MCP 摘要。
func MutationResult(out Mutation) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: !out.Success, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// RunMutation 执行一次 mutation 并统一包装成功/失败摘要。
func RunMutation(out Mutation, run func() error) (*mcp.CallToolResult, Mutation, error) {
	err := run()
	if err != nil {
		out.Text = "Error: " + err.Error()
		return MutationResult(out), out, nil
	}
	out.Success = true
	return MutationResult(out), out, nil
}

// ListComments 在账号池重放边界内收集 artwork 或 novel 的评论分页并输出统一
// envelope；novel 为 true 时读取小说评论。元数据只缓存上游明确提供的值。
func ListComments(ctx context.Context, app *runtime.App, id int64, novel bool, input runtime.PageLimitIn, limit *int) (*mcp.CallToolResult, Comments, error) {
	if id <= 0 {
		result, out, _ := CommentsError(errors.New("id must be a positive integer"))
		return result, out, nil
	}
	plan, err := runtime.ParseListPlan(input)
	if err != nil {
		result, out, _ := CommentsError(err)
		return result, out, nil
	}
	metadata := CommentToolMetadata{}
	items, more, err := runtime.CollectPooledPages(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Comment, sdk.Cursor, error) {
		var page pixiv.CommentPage
		var err error
		if novel {
			page, err = client.NovelComments(ctx, pixiv.NovelCommentsRequest{NovelID: id, Cursor: cursor})
		} else {
			page, err = client.ArtworkComments(ctx, pixiv.ArtworkCommentsRequest{ArtworkID: id, Cursor: cursor})
		}
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		if cursor.IsZero() {
			metadata = CommentToolMetadata{}
		}
		metadata.Set(page)
		return page.Page.Items, page.Page.Next, nil
	})
	if err != nil {
		result, out, _ := CommentsError(err)
		return result, out, nil
	}
	comments := make([]pixiv.CommentDTO, 0, len(items))
	for _, item := range items {
		comments = append(comments, pixiv.ToCommentDTO(item))
	}
	var accessControl *pixiv.CommentAccessControlDTO
	if metadata.Access != nil {
		dto := pixiv.ToCommentAccessControlDTO(*metadata.Access)
		accessControl = &dto
	}
	out := Comments{Comments: comments, Pagination: runtime.ListPagination(plan, limit, len(comments), more), Total: metadata.Total, AccessControl: accessControl}
	return CommentsResult(out, false, ""), out, nil
}

// ListUserRelations 在账号池重放边界内收集 followers/related/blocked 用户列表并
// 输出统一 records envelope。
func ListUserRelations(ctx context.Context, app *runtime.App, kind string, userID int64, restrict string, input runtime.PageLimitIn, limit *int) (*mcp.CallToolResult, Records, error) {
	userID, err := runtime.ResolveUserID(app, ctx, userID)
	if err != nil {
		return Error(err)
	}
	plan, err := runtime.ParseListPlan(input)
	if err != nil {
		return Error(err)
	}
	items, more, err := runtime.CollectPooledPages(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		var result sdk.Page[pixiv.UserPreview]
		var err error
		switch kind {
		case "followers":
			result, err = client.UserFollowers(ctx, pixiv.UserFollowersRequest{UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor})
		case "related":
			result, err = client.RelatedUsers(ctx, pixiv.RelatedUsersRequest{UserID: userID, Cursor: cursor})
		case "blocked":
			result, err = client.UserBlockedUsers(ctx, pixiv.UserBlockedUsersRequest{UserID: userID, Cursor: cursor})
		default:
			return nil, sdk.Cursor{}, errors.New("unknown user relation")
		}
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return Error(err)
	}
	recordItems, err := records.FromUserPreviews(items)
	if err != nil {
		return Error(err)
	}
	out := Records{Records: recordItems, Pagination: runtime.ListPagination(plan, limit, len(items), more)}
	return Result(out, false), out, nil
}
