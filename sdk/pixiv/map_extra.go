package pixiv

import (
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// commentPage maps an adapter comment list into a public CommentPage, encoding
// the offset continuation into an opaque cursor.
func (c *Client) commentPage(op string, query url.Values, values []novel.Comment, nextOffset int, hasNext bool, total *int64, access *novel.CommentAccessControl) (CommentPage, error) {
	items := make([]Comment, 0, len(values))
	for _, m := range values {
		comment, err := c.mapComment(m)
		if err != nil {
			return CommentPage{}, err
		}
		items = append(items, comment)
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return CommentPage{}, err
	}
	page := CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}}
	if total != nil {
		value := *total
		page.Total = &value
	}
	if access != nil {
		page.AccessControl = &CommentAccessControl{
			CanComment: access.CanComment,
			IsLocked:   access.IsLocked,
		}
	}
	return page, nil
}

func (c *Client) mapComment(m novel.Comment) (Comment, error) {
	created, err := parseUTCTime(m.CreateDate)
	if err != nil {
		return Comment{}, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time")
	}
	out := Comment{ID: m.ID, User: c.mapNovelUser(m.User), Comment: m.Comment, CreatedAt: created}
	if m.ParentComment != nil {
		parent, err := c.mapComment(*m.ParentComment)
		if err != nil {
			return Comment{}, err
		}
		out.ParentComment = &parent
	}
	return out, nil
}
