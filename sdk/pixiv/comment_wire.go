package pixiv

import (
	"cmp"
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func commentRoutes(novel bool) (list, add, remove, parameter string) {
	if novel {
		return api.PathNovelComments, api.PathNovelCommentAdd, api.PathNovelCommentDelete, api.ParamNovelID
	}
	return api.PathIllustComments, api.PathIllustCommentAdd, api.PathIllustCommentDelete, api.ParamIllustID
}

type commentWire struct {
	ID        int64        `json:"id"`
	User      userWire     `json:"user"`
	Comment   string       `json:"comment"`
	Caption   string       `json:"caption"`
	Date      string       `json:"date"`
	CreatedAt string       `json:"created_at"`
	Parent    *commentWire `json:"parent_comment"`
}

func (c *Client) mapComment(w commentWire, target commentTarget) (*Comment, error) {
	if w.ID <= 0 {
		return nil, newError("Comment", sdk.MalformedUpstreamResponse, "comment ID must be positive", nil)
	}
	created, err := time.Parse(time.RFC3339, cmp.Or(w.Date, w.CreatedAt))
	if err != nil {
		return nil, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time", err)
	}
	author, err := c.mapUser(w.User)
	if err != nil {
		return nil, err
	}
	comment := &Comment{CommentRef: &CommentRef{ID: w.ID, client: c, target: target}, commentData: commentData{Text: cmp.Or(w.Comment, w.Caption), Author: author, CreatedAt: created.UTC()}}
	if w.Parent != nil {
		comment.Parent, err = c.mapComment(*w.Parent, target)
		if err != nil {
			return nil, err
		}
	}
	return comment, nil
}

func (c *Client) comments(ctx context.Context, target commentTarget, token sdk.Cursor) (CommentPage, error) {
	path, _, _, parameter := commentRoutes(target.novel)
	base := url.Values{parameter: {strconv.FormatInt(target.id, 10)}}
	query, binding, err := c.pageQuery(ctx, path, base, token)
	if err != nil {
		return CommentPage{}, err
	}
	var raw struct {
		Comments api.RequiredList[commentWire] `json:"comments"`
		NextURL  *string                       `json:"next_url"`
		Total    *int64                        `json:"total_comments"`
		Access   *struct {
			CanComment bool `json:"can_comment"`
			IsLocked   bool `json:"is_locked"`
		} `json:"access_control"`
		NumericAccess *int64 `json:"comment_access_control"`
	}
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return CommentPage{}, err
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError(path, sdk.MalformedUpstreamResponse, "missing comments", nil)
	}
	items := make([]*Comment, len(raw.Comments.Items))
	for i, w := range raw.Comments.Items {
		items[i], err = c.mapComment(w, target)
		if err != nil {
			return CommentPage{}, err
		}
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return CommentPage{}, err
	}
	var access *CommentAccessControl
	if raw.NumericAccess != nil {
		access = &CommentAccessControl{NumericValue: raw.NumericAccess}
	} else if raw.Access != nil {
		access = &CommentAccessControl{CanComment: raw.Access.CanComment, IsLocked: raw.Access.IsLocked}
	}
	// The typed loader retains response metadata that a generic Page cannot carry.
	load := func(ctx context.Context) (CommentPage, error) { return c.comments(ctx, target, next) }
	page := sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Comment], error) { p, err := load(ctx); return p.Page, err })
	return CommentPage{Page: page, Total: raw.Total, AccessControl: access, load: load}, nil
}
