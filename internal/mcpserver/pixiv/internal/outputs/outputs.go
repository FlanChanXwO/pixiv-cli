// Package outputs 定义 Pixiv MCP 工具共用的结构化输出与结果摘要。
package outputs

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Records 是实体记录列表的输出 envelope。
type Records struct {
	Records    []record.Record        `json:"records"`
	Pagination toolhost.PaginationOut `json:"pagination"`
	Filter     *BookmarkFilter        `json:"filter,omitempty"`
}

// BookmarkFilter 描述一次 bookmark 区间筛选的执行结果元数据。
type BookmarkFilter struct {
	Min          *int   `json:"min,omitempty"`
	Max          *int   `json:"max,omitempty"`
	Membership   string `json:"membership"`
	Strategy     string `json:"strategy"`
	Completeness string `json:"completeness"`
}

// Result 构造实体列表的 MCP 文本摘要。
func Result(out Records) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// Error 构造实体列表的 MCP error 摘要。
func Error(err error) (*mcp.CallToolResult, Records, error) {
	out := Records{Records: []record.Record{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// Detail 是 artwork、user 与 novel detail 共用的记录 envelope。
type Detail struct {
	Records []record.Record `json:"records"`
}

// DetailResult 构造单实体详情的 MCP 摘要。
func DetailResult(out Detail) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// DetailError 构造单实体详情的 MCP error 摘要。
func DetailError(err error) (*mcp.CallToolResult, Detail, error) {
	out := Detail{Records: []record.Record{}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// RecommendedPagination 分别表达每条推荐流的逻辑页与默认批次续读；
// next_cursor 是绑定单个 kind/filter 的 MCP envelope。
type RecommendedPagination struct {
	Illust *toolhost.PaginationOut `json:"illust,omitempty"`
	Manga  *toolhost.PaginationOut `json:"manga,omitempty"`
	Novel  *toolhost.PaginationOut `json:"novel,omitempty"`
	User   *toolhost.PaginationOut `json:"user,omitempty"`
}

// Recommended 是 recommended tool 的输出 envelope。
type Recommended struct {
	Records    []record.Record       `json:"records"`
	Pagination RecommendedPagination `json:"pagination"`
}

// NewRecommended 构造空的 recommended 输出。
func NewRecommended() Recommended {
	return Recommended{Records: []record.Record{}}
}

// RecommendedError 构造 recommended 的 MCP error 摘要。
func RecommendedError(err error) (*mcp.CallToolResult, Recommended, error) {
	out := NewRecommended()
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// RecommendedPage 把单条推荐流的分页转为指针输出。
func RecommendedPage(plan toolrun.ListPlan, limit *int, returned int, more bool) *toolhost.PaginationOut {
	value := toolhost.ListPagination(plan, limit, returned, more)
	return &value
}

// TrendingTags 是 trending_tags_illust tool 的输出 envelope。
type TrendingTags struct {
	Tags []pixiv.TrendingTag `json:"tags"`
	Text string              `json:"text"`
}

// TrendingTagsResult 构造 trending tags 的 MCP 摘要。
func TrendingTagsResult(out TrendingTags, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// TrendingTagsError 构造 trending tags 的 MCP error 摘要。
func TrendingTagsError(err error) (*mcp.CallToolResult, TrendingTags, error) {
	out := TrendingTags{Tags: []pixiv.TrendingTag{}, Text: "Error: " + err.Error()}
	return TrendingTagsResult(out, true), out, nil
}

// Comments 是评论 tool 的输出 envelope。
type Comments struct {
	Comments      []pixiv.Comment             `json:"comments"`
	Pagination    toolhost.PaginationOut      `json:"pagination"`
	Total         *int64                      `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControl `json:"access_control,omitempty"`
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
	out := Comments{Comments: []pixiv.Comment{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return CommentsResult(out, true, errorMessage(err)), out, nil
}

// NovelContent 是 novel_content tool 的输出 envelope。
type NovelContent struct {
	Content pixiv.NovelContent `json:"content"`
}

// NovelContentResult 构造 novel content 的 MCP 摘要。
func NovelContentResult(novelID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved novel content for %d.", novelID)}}}
}

// NovelContentError 构造 novel content 的 MCP error 摘要。
func NovelContentError(err error) (*mcp.CallToolResult, NovelContent, error) {
	out := NovelContent{Content: pixiv.NovelContent{Blocks: []pixiv.NovelBlock{}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// NovelSeries 是 novel_series tool 的输出 envelope。
type NovelSeries struct {
	Series     pixiv.NovelSeries      `json:"series"`
	Records    []record.Record        `json:"records"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// NovelSeriesResult 构造 novel series 的 MCP 摘要。
func NovelSeriesResult(out NovelSeries) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// NovelSeriesError 构造 novel series 的 MCP error 摘要。
func NovelSeriesError(err error) (*mcp.CallToolResult, NovelSeries, error) {
	out := NovelSeries{Records: []record.Record{}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// BookmarkTags 是 bookmark_tags tool 的输出 envelope。
type BookmarkTags struct {
	Tags       []pixiv.BookmarkTag    `json:"bookmark_tags"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// BookmarkTagsResult 构造 bookmark tags 的 MCP 摘要。
func BookmarkTagsResult(out BookmarkTags) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d bookmark tags.", len(out.Tags))}}}
}

// BookmarkTagsError 构造 bookmark tags 的 MCP error 摘要。
func BookmarkTagsError(err error) (*mcp.CallToolResult, BookmarkTags, error) {
	out := BookmarkTags{Tags: []pixiv.BookmarkTag{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// TypedBookmarkTag 是 bookmark_tags_all 的标签记录；content_type 保持
// artwork/novel 两条上游流的来源，不把同名标签错误合并。
type TypedBookmarkTag struct {
	Name        string `json:"name"`
	Count       int    `json:"count"`
	ContentType string `json:"content_type"`
}

// BookmarkTagsAll 是 bookmark_tags_all tool 的 additive output envelope。
type BookmarkTagsAll struct {
	Tags       []TypedBookmarkTag     `json:"bookmark_tags"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// BookmarkTagsAllResult 构造 bookmark_tags_all 的 MCP 摘要。
func BookmarkTagsAllResult(out BookmarkTagsAll) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d bookmark tags.", len(out.Tags))}}}
}

// BookmarkTagsAllError 构造 bookmark_tags_all 的 MCP error 摘要。
func BookmarkTagsAllError(err error) (*mcp.CallToolResult, BookmarkTagsAll, error) {
	out := BookmarkTagsAll{Tags: []TypedBookmarkTag{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// BookmarkDetail 是 bookmark_detail tool 的输出 envelope。
type BookmarkDetail struct {
	Bookmarked bool     `json:"bookmarked"`
	Restrict   string   `json:"restrict,omitempty"`
	Tags       []string `json:"tags"`
}

// BookmarkDetailResult 构造 artwork bookmark detail 的 MCP 摘要。
func BookmarkDetailResult(out BookmarkDetail, illustID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artwork %d bookmarked: %t.", illustID, out.Bookmarked)}}}
}

// NovelBookmarkDetailResult 构造 novel bookmark detail 的 MCP 摘要。
func NovelBookmarkDetailResult(out BookmarkDetail, novelID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Novel %d bookmarked: %t.", novelID, out.Bookmarked)}}}
}

// BookmarkDetailError 构造 bookmark detail 的 MCP error 摘要。
func BookmarkDetailError(err error) (*mcp.CallToolResult, BookmarkDetail, error) {
	out := BookmarkDetail{Tags: []string{}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// Mutation 是 mutation tool 的输出 envelope。
type Mutation struct {
	Success   bool   `json:"success"`
	Action    string `json:"action"`
	IllustID  int64  `json:"illust_id,omitempty"`
	NovelID   int64  `json:"novel_id,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	CommentID int64  `json:"comment_id,omitempty"`
	Text      string `json:"text"`
}

// MutationResult 构造 mutation 的 MCP 摘要。
func MutationResult(out Mutation) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: !out.Success, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// RunMutation 执行 mutation，保留上游写回字段并统一包装成功/失败摘要。
func RunMutation(out *Mutation, run func() error) (*mcp.CallToolResult, Mutation, error) {
	err := run()
	if err != nil {
		out.Text = "Error: " + err.Error()
		return MutationResult(*out), *out, nil
	}
	out.Success = true
	return MutationResult(*out), *out, nil
}

// RecordResult 统一实体查询的 MCP 文本摘要。完整实体只存在于 structured records，
// Content 绝不复制 JSON，以便客户端始终只消费一个机器可读载荷。
func RecordResult(records []record.Record, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d records.", len(records))
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// errorMessage 返回统一错误摘要。
func errorMessage(err error) string {
	return "Error: " + err.Error()
}
