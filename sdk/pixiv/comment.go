package pixiv

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
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

// commentsWire is the response shape artwork and novel comment lists share.
type commentsWire struct {
	Comments      api.RequiredList[commentWire] `json:"comments"`
	NextURL       *string                       `json:"next_url"`
	TotalComments *int64                        `json:"total_comments"`
	AccessControl *commentAccessControlWire     `json:"access_control"`
	// CommentAccessControl carries the current App API scalar. It decodes
	// separately from the object form so no business meaning is inferred for it.
	CommentAccessControl *int64 `json:"comment_access_control"`
}

// mapComment maps one artwork comment and validates its parent chain.
func (c *Client) mapComment(value commentWire) (Comment, error) {
	created, err := parseUTCTime(commentDate(value))
	if err != nil {
		return Comment{}, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time")
	}
	user, err := c.mapUser(value.User)
	if err != nil {
		return Comment{}, err
	}
	result := Comment{ID: value.ID, User: user, Comment: value.Comment, CreatedAt: created}
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

// commentPage maps a comment list and encodes its offset continuation as a
// cursor.
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
	return CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}, Total: total, AccessControl: access}, nil
}

// readComments validates the comment response, parent chain, access control, and
// continuation in one place; endpoints supply only the path and query.
func (c *Client) readComments(ctx context.Context, operation, path string, base url.Values, cursor sdk.Cursor, spec continuationSpec) (CommentPage, error) {
	offset, err := c.continuationPositiveOffset(operation, base, cursor)
	if err != nil {
		return CommentPage{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
	}
	var raw commentsWire
	if err := c.app.GetJSON(ctx, path, query, &raw); err != nil {
		return CommentPage{}, classifyAppError(err, operation)
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return CommentPage{}, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	var access *CommentAccessControl
	if raw.CommentAccessControl != nil {
		access = &CommentAccessControl{NumericValue: raw.CommentAccessControl}
	} else if raw.AccessControl != nil {
		access = &CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	next, hasNext, err := spec.offset(raw.NextURL)
	if err != nil {
		return CommentPage{}, classifyAppError(err, operation)
	}
	return c.commentPage(operation, base, raw.Comments.Items, next, hasNext, raw.TotalComments, access)
}

// ArtworkComments lists comments on one artwork.
func (c *Client) ArtworkComments(ctx context.Context, request ArtworkCommentsRequest) (CommentPage, error) {
	ctx = logging.WithOperation(ctx, "ArtworkComments")
	if request.ArtworkID <= 0 {
		return CommentPage{}, newError("ArtworkComments", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{api.ParamIllustID: {itoa(request.ArtworkID)}}
	return c.readComments(ctx, "ArtworkComments", api.PathIllustComments, base, request.Cursor, commentsContinuation)
}

// NovelComments lists comments on one novel.
func (c *Client) NovelComments(ctx context.Context, request NovelCommentsRequest) (CommentPage, error) {
	ctx = logging.WithOperation(ctx, "NovelComments")
	if request.NovelID <= 0 {
		return CommentPage{}, newError("NovelComments", sdk.InvalidArgument, "novel ID must be positive")
	}
	base := url.Values{api.ParamNovelID: {itoa(request.NovelID)}}
	return c.readComments(ctx, "NovelComments", api.PathNovelComments, base, request.Cursor, novelCommentsContinuation)
}

// stampsWire is the /v1/stamps response. Upstream provides no continuation for it.
type stampsWire struct {
	Stamps  api.RequiredList[stampWire] `json:"stamps"`
	NextURL *json.RawMessage            `json:"next_url"`
}

type stampWire struct {
	ID  int64  `json:"stamp_id"`
	URL string `json:"stamp_url"`
}

// Stamps lists the comment stamps available to the authenticated user. Each
// image is represented as a stable SDK resource; the upstream locator remains
// runtime-only and is never embedded in the opaque reference.
func (c *Client) Stamps(ctx context.Context, request StampsRequest) ([]Stamp, error) {
	ctx = logging.WithOperation(ctx, "Stamps")
	items, err := c.fetchStamps(ctx, "Stamps")
	if err != nil {
		return nil, err
	}
	stamps := make([]Stamp, 0, len(items))
	for _, value := range items {
		image, err := c.newResource("stamp", value.ID, -1, "", value.URL)
		if err != nil {
			return nil, classifyAppError(err, "Stamps")
		}
		stamps = append(stamps, Stamp{ID: value.ID, Image: ImageResource{Resource: image}})
	}
	return stamps, nil
}

// fetchStamps reads and validates the stamp list. Resource resolution reuses the
// same validation.
func (c *Client) fetchStamps(ctx context.Context, operation string) ([]stampWire, error) {
	var raw stampsWire
	if err := c.app.GetJSON(ctx, api.PathStamps, nil, &raw); err != nil {
		return nil, classifyAppError(err, operation)
	}
	if !raw.Stamps.Present || !raw.Stamps.Valid || raw.NextURL != nil {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, value := range raw.Stamps.Items {
		if value.ID <= 0 || !api.ValidStampURL(value.URL) {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	return raw.Stamps.Items, nil
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
	path:  api.PathIllustComments,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamIllustID},
}

var novelCommentsContinuation = continuationSpec{
	path:  api.PathNovelComments,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamNovelID},
}
