// comment 领域：作品与小说评论读取、评论页映射和可用的 stamp 列表。
// 评论写操作在 mutation.go；请求类型与 continuation Spec 都在本文件。

package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type commentWire struct {
	ID            int64        `json:"id"`
	User          userWire     `json:"user"`
	Comment       string       `json:"comment"`
	Caption       string       `json:"caption"`
	Date          string       `json:"date"`
	CreateDate    string       `json:"created_at"`
	ParentComment *commentWire `json:"parent_comment"`
}

type commentAccessControlWire struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

// commentsWire 是 illust 与 novel 评论列表的共同响应结构。
type commentsWire struct {
	Comments      pixivapi.RequiredList[commentWire] `json:"comments"`
	NextURL       *string                            `json:"next_url"`
	TotalComments *int64                             `json:"total_comments"`
	AccessControl *commentAccessControlWire          `json:"access_control"`
	// CommentAccessControl 是当前 App API 标量；与 legacy object 分开解码，
	// 避免猜测其业务含义。
	CommentAccessControl *int64 `json:"comment_access_control"`
}

// mapComment 映射一条 artwork comment，并解析其父评论链。
func (c *Client) mapComment(value commentWire) (Comment, error) {
	created, err := parseUTCTime(commentDate(value))
	if err != nil {
		return Comment{}, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time")
	}
	result := Comment{ID: value.ID, User: c.mapUser(value.User), Comment: value.Comment, CreatedAt: created}
	if result.Comment == "" {
		result.Comment = value.Caption
	}
	if value.ParentComment != nil {
		parent, err := c.mapComment(*value.ParentComment)
		if err != nil {
			return Comment{}, err
		}
		result.ParentComment = &parent
	}
	return result, nil
}

func commentDate(value commentWire) string {
	if value.Date != "" {
		return value.Date
	}
	return value.CreateDate
}

func validCommentChain(value commentWire) bool {
	for {
		if value.ID <= 0 {
			return false
		}
		if value.ParentComment == nil {
			return true
		}
		value = *value.ParentComment
	}
}

// commentPage 映射 comment 列表，并把 offset continuation 编码为游标。
func (c *Client) commentPage(op string, query url.Values, values []commentWire, nextOffset int, hasNext bool, total *int64, access *CommentAccessControl) (CommentPage, error) {
	items := make([]Comment, len(values))
	for index, value := range values {
		mapped, err := c.mapComment(value)
		if err != nil {
			return CommentPage{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return CommentPage{}, err
	}
	page := CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}}
	if total != nil {
		page.Total = cloneValue(total)
	}
	page.AccessControl = cloneValue(access)
	return page, nil
}

// ArtworkComments lists comments on one artwork.
func (c *Client) ArtworkComments(ctx context.Context, request ArtworkCommentsRequest) (CommentPage, error) {
	if request.ArtworkID <= 0 {
		return CommentPage{}, newError("ArtworkComments", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	offset, err := c.continuationPositiveOffset("ArtworkComments", base, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw commentsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustComments, query, &raw); err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError("ArtworkComments", sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return CommentPage{}, newError("ArtworkComments", sdk.MalformedUpstreamResponse, "")
		}
	}
	var access *CommentAccessControl
	if raw.CommentAccessControl != nil {
		access = &CommentAccessControl{NumericValue: cloneValue(raw.CommentAccessControl)}
	} else if raw.AccessControl != nil {
		access = &CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	next, hasNext, err := commentsContinuation.offset(raw.NextURL)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	return c.commentPage("ArtworkComments", base, raw.Comments.Items, next, hasNext, raw.TotalComments, access)
}

// NovelComments lists comments on one novel.
func (c *Client) NovelComments(ctx context.Context, request NovelCommentsRequest) (CommentPage, error) {
	if request.NovelID <= 0 {
		return CommentPage{}, newError("NovelComments", sdk.InvalidArgument, "novel ID must be positive")
	}
	base := url.Values{pixivapi.ParamNovelID: {itoa(request.NovelID)}}
	offset, err := c.continuationPositiveOffset("NovelComments", base, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw commentsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelComments, query, &raw); err != nil {
		return CommentPage{}, classifyAppError(err, "NovelComments")
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError("NovelComments", sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return CommentPage{}, newError("NovelComments", sdk.MalformedUpstreamResponse, "")
		}
	}
	var access *CommentAccessControl
	if raw.CommentAccessControl != nil {
		access = &CommentAccessControl{NumericValue: cloneValue(raw.CommentAccessControl)}
	} else if raw.AccessControl != nil {
		access = &CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	next, hasNext, err := novelCommentsContinuation.offset(raw.NextURL)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "NovelComments")
	}
	return c.commentPage("NovelComments", base, raw.Comments.Items, next, hasNext, raw.TotalComments, access)
}

// stampsWire 是 /v1/stamps 的响应；上游当前不提供续页。
type stampsWire struct {
	Stamps  pixivapi.RequiredList[stampWire] `json:"stamps"`
	NextURL *json.RawMessage                 `json:"next_url"`
}

type stampWire struct {
	ID  int64  `json:"stamp_id"`
	URL string `json:"stamp_url"`
}

// Stamps lists the comment stamps available to the authenticated user. Each
// image is represented as a stable SDK resource; the upstream locator remains
// runtime-only and is never embedded in the opaque reference.
func (c *Client) Stamps(ctx context.Context, request StampsRequest) ([]Stamp, error) {
	items, err := c.fetchStamps(ctx, "Stamps")
	if err != nil {
		return nil, err
	}
	stamps := make([]Stamp, 0, len(items))
	for _, value := range items {
		image, err := c.newResourceWithVariant("stamp", value.ID, -1, "", value.URL)
		if err != nil {
			return nil, classifyAppError(err, "Stamps")
		}
		stamps = append(stamps, Stamp{ID: value.ID, Image: ImageResource{Resource: image}})
	}
	return stamps, nil
}

// fetchStamps 读取并校验 stamp 列表；resource 解析也复用同一份校验。
func (c *Client) fetchStamps(ctx context.Context, operation string) ([]stampWire, error) {
	var raw stampsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathStamps, nil, &raw); err != nil {
		return nil, classifyAppError(err, operation)
	}
	if !raw.Stamps.Present || !raw.Stamps.Valid || raw.NextURL != nil {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, value := range raw.Stamps.Items {
		if value.ID <= 0 || !c.validStampURL(value.URL) {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	return raw.Stamps.Items, nil
}

// validStampURL 要求 https、无 userinfo、落在 Pixiv media host allowlist，
// 且有实际路径。
func (c *Client) validStampURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range defaultResourceHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// ArtworkCommentsRequest lists comments on one artwork.
type ArtworkCommentsRequest struct {
	ArtworkID int64
	Cursor    sdk.Cursor
}

// NovelCommentsRequest lists comments on one novel.
type NovelCommentsRequest struct {
	NovelID int64
	Cursor  sdk.Cursor
}

// StampsRequest lists the comment stamps available to the authenticated user.
// The current upstream contract has no query or continuation fields.
type StampsRequest struct{}

var commentsContinuation = continuationSpec{
	path:  pixivapi.PathIllustComments,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamIllustID},
}

var novelCommentsContinuation = continuationSpec{
	path:  pixivapi.PathNovelComments,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamNovelID},
}
