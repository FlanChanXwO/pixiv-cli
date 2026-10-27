// Package outputs 定义 Pixiv MCP 工具的结构化输出与文本摘要，让正文只放简短提示，
// 完整数据只出现在 structured content。
//
// 包的组成：[Records] 与 [Detail] 是列表和详情共用的记录 envelope，[Result]、
// [DetailResult] 与 [RecordResult] 生成对应摘要；评论、小说、收藏标签、mutation 等
// 专用 envelope 也在本文件。账号、登录与媒体结果分别在 account.go、login.go、
// media.go，它们还提供对应工具的输入校验中间件。
package outputs

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Records 承载实体列表工具的输出。Pagination 始终存在，Filter 只在收藏筛选实际执行时
// 出现。
type Records struct {
	Records    []record.Record        `json:"records"`
	Pagination toolhost.PaginationOut `json:"pagination"`
	Filter     *BookmarkFilter        `json:"filter,omitempty"`
}

// BookmarkFilter 说明收藏筛选用了什么策略、是否完整，以及实际生效的区间。
type BookmarkFilter struct {
	Min          *int   `json:"min,omitempty"`
	Max          *int   `json:"max,omitempty"`
	Membership   string `json:"membership"`
	Strategy     string `json:"strategy"`
	Completeness string `json:"completeness"`
}

// Result 生成实体列表的文本摘要，结构化数据仍由调用方填入 Records。
func Result(out Records) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// Error 返回 isError 的实体列表结果，并保留空的 records 与第一页分页。
func Error(err error) (*mcp.CallToolResult, Records, error) {
	out := Records{Records: []record.Record{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// Detail 承载单实体详情工具的输出，记录数组通常只有一个元素。
type Detail struct {
	Records []record.Record `json:"records"`
}

// DetailResult 生成单实体详情的文本摘要。
func DetailResult(out Detail) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// DetailError 返回 isError 的详情结果，records 为空数组。
func DetailError(err error) (*mcp.CallToolResult, Detail, error) {
	out := Detail{Records: []record.Record{}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// RecommendedPagination 分别保存四条推荐流的分页；某条流没有结果时对应字段为 nil。
// next_cursor 只对单一的 kind 与筛选组合有效。
type RecommendedPagination struct {
	Illust *toolhost.PaginationOut `json:"illust,omitempty"`
	Manga  *toolhost.PaginationOut `json:"manga,omitempty"`
	Novel  *toolhost.PaginationOut `json:"novel,omitempty"`
	User   *toolhost.PaginationOut `json:"user,omitempty"`
}

// Recommended 承载 recommended 工具的输出。
type Recommended struct {
	Records    []record.Record       `json:"records"`
	Pagination RecommendedPagination `json:"pagination"`
}

// NewRecommended 返回 records 为非 nil 空数组的输出，便于直接序列化。
func NewRecommended() Recommended {
	return Recommended{Records: []record.Record{}}
}

// RecommendedError 返回 isError 的 recommended 结果，分页为空。
func RecommendedError(err error) (*mcp.CallToolResult, Recommended, error) {
	out := NewRecommended()
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// RecommendedPage 把一条推荐流的分页包成指针，供 RecommendedPagination 使用。
func RecommendedPage(plan toolrun.ListPlan, limit *int, returned int, more bool) *toolhost.PaginationOut {
	value := toolhost.ListPagination(plan, limit, returned, more)
	return &value
}

// TrendingTags 承载 trending_tags_illust 工具的输出。
type TrendingTags struct {
	Tags []pixiv.TrendingTag `json:"tags"`
	Text string              `json:"text"`
}

// TrendingTagsResult 用 out.Text 生成摘要；没有标签时仍返回成功结果。
func TrendingTagsResult(out TrendingTags, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// TrendingTagsError 返回 isError 的趋势标签结果，tags 为空数组。
func TrendingTagsError(err error) (*mcp.CallToolResult, TrendingTags, error) {
	out := TrendingTags{Tags: []pixiv.TrendingTag{}, Text: "Error: " + err.Error()}
	return TrendingTagsResult(out, true), out, nil
}

// Comments 承载评论工具的输出；Total 与 AccessControl 可能缺失。
type Comments struct {
	Comments      []pixiv.Comment             `json:"comments"`
	Pagination    toolhost.PaginationOut      `json:"pagination"`
	Total         *int64                      `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControl `json:"access_control,omitempty"`
}

// CommentsResult 生成评论摘要，message 为空时报告返回条数。
func CommentsResult(out Comments, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d comments.", len(out.Comments))
	}
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

// CommentsError 返回 isError 的评论结果，列表为空、分页为第一页。
func CommentsError(err error) (*mcp.CallToolResult, Comments, error) {
	out := Comments{Comments: []pixiv.Comment{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return CommentsResult(out, true, errorMessage(err)), out, nil
}

// NovelContent 承载 novel_content 工具的输出。
type NovelContent struct {
	Content pixiv.NovelContent `json:"content"`
}

// NovelContentResult 生成小说正文的文本摘要。
func NovelContentResult(novelID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved novel content for %d.", novelID)}}}
}

// NovelContentError 返回 isError 的正文结果，blocks 为空数组。
func NovelContentError(err error) (*mcp.CallToolResult, NovelContent, error) {
	out := NovelContent{Content: pixiv.NovelContent{Blocks: []pixiv.NovelBlock{}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// NovelSeries 承载 novel_series 工具的输出。
type NovelSeries struct {
	Series     pixiv.NovelSeries      `json:"series"`
	Records    []record.Record        `json:"records"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// NovelSeriesResult 生成小说系列的文本摘要。
func NovelSeriesResult(out NovelSeries) *mcp.CallToolResult {
	return RecordResult(out.Records, false, "")
}

// NovelSeriesError 返回 isError 的系列结果，records 为空数组。
func NovelSeriesError(err error) (*mcp.CallToolResult, NovelSeries, error) {
	out := NovelSeries{Records: []record.Record{}}
	return RecordResult(out.Records, true, errorMessage(err)), out, nil
}

// BookmarkTags 承载 bookmark_tags 工具的输出。
type BookmarkTags struct {
	Tags       []pixiv.BookmarkTag    `json:"bookmark_tags"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// BookmarkTagsResult 生成收藏标签的文本摘要。
func BookmarkTagsResult(out BookmarkTags) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d bookmark tags.", len(out.Tags))}}}
}

// BookmarkTagsError 返回 isError 的收藏标签结果，列表为空、分页为第一页。
func BookmarkTagsError(err error) (*mcp.CallToolResult, BookmarkTags, error) {
	out := BookmarkTags{Tags: []pixiv.BookmarkTag{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// TypedBookmarkTag 表示 bookmark_tags_all 中的一条标签。content_type 保留标签来自
// artwork 还是 novel，同名标签分别成条。
type TypedBookmarkTag struct {
	Name        string `json:"name"`
	Count       int    `json:"count"`
	ContentType string `json:"content_type"`
}

// BookmarkTagsAll 承载 bookmark_tags_all 工具的输出，两种内容类型的标签合并在同一个数组
// 里。
type BookmarkTagsAll struct {
	Tags       []TypedBookmarkTag     `json:"bookmark_tags"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

// BookmarkTagsAllResult 生成 bookmark_tags_all 的文本摘要。
func BookmarkTagsAllResult(out BookmarkTagsAll) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d bookmark tags.", len(out.Tags))}}}
}

// BookmarkTagsAllError 返回 isError 的结果，标签为空数组、分页为第一页。
func BookmarkTagsAllError(err error) (*mcp.CallToolResult, BookmarkTagsAll, error) {
	out := BookmarkTagsAll{Tags: []TypedBookmarkTag{}, Pagination: toolhost.PaginationOut{PaginationOut: toolrun.PaginationOut{Page: 1}}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// BookmarkDetail 承载 bookmark_detail 工具的输出。
type BookmarkDetail struct {
	Bookmarked bool     `json:"bookmarked"`
	Restrict   string   `json:"restrict,omitempty"`
	Tags       []string `json:"tags"`
}

// BookmarkDetailResult 生成作品收藏详情的文本摘要。
func BookmarkDetailResult(out BookmarkDetail, illustID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artwork %d bookmarked: %t.", illustID, out.Bookmarked)}}}
}

// NovelBookmarkDetailResult 生成小说收藏详情的文本摘要。
func NovelBookmarkDetailResult(out BookmarkDetail, novelID int64) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Novel %d bookmarked: %t.", novelID, out.Bookmarked)}}}
}

// BookmarkDetailError 返回 isError 的收藏详情结果，tags 为空数组。
func BookmarkDetailError(err error) (*mcp.CallToolResult, BookmarkDetail, error) {
	out := BookmarkDetail{Tags: []string{}}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: errorMessage(err)}}}, out, nil
}

// Mutation 承载写操作工具的输出；各 ID 字段按操作类型取舍，零值会被省略。
type Mutation struct {
	Success   bool   `json:"success"`
	Action    string `json:"action"`
	IllustID  int64  `json:"illust_id,omitempty"`
	NovelID   int64  `json:"novel_id,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	CommentID int64  `json:"comment_id,omitempty"`
	Text      string `json:"text"`
}

// MutationResult 按 Success 生成 isError 标记，文本取自 out.Text。
func MutationResult(out Mutation) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: !out.Success, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

// RunMutation 执行写操作，失败时把错误写进 out.Text 并保留已填的字段，成功时设置
// Success。无论成败都返回结构化结果，错误随 envelope 交给 MCP SDK。
func RunMutation(out *Mutation, run func() error) (*mcp.CallToolResult, Mutation, error) {
	err := run()
	if err != nil {
		out.Text = "Error: " + err.Error()
		return MutationResult(*out), *out, nil
	}
	out.Success = true
	return MutationResult(*out), *out, nil
}

// RecordResult 生成实体查询的文本摘要。完整实体只放进 structured records，Content
// 不复制 JSON，客户端始终只解析一个机器可读载荷。
func RecordResult(records []record.Record, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d records.", len(records))
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// errorMessage 给错误文本加上统一的 Error 前缀。
func errorMessage(err error) string {
	return "Error: " + err.Error()
}
