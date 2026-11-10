package pixiv

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type commentTarget struct {
	id    int64
	novel bool
}

// CommentRef addresses an artwork or novel comment on its creating client.
// References constructed from an owner also carry the target needed for replies.
type CommentRef struct {
	ID     int64 `json:"id"`
	client *Client
	target commentTarget
}

// ArtworkComment constructs a reference for deletion without fetching the comment.
// For replies, use ArtworkRef.Comment to supply the owning artwork.
func (c *Client) ArtworkComment(id int64) *CommentRef { return &CommentRef{ID: id, client: c} }

// NovelComment constructs a novel comment reference for deletion without fetching it.
// For replies, use NovelRef.Comment to supply the owning novel.
func (c *Client) NovelComment(id int64) *CommentRef {
	return &CommentRef{ID: id, client: c, target: commentTarget{novel: true}}
}

// Comment constructs a comment reference carrying this artwork as its reply target.
func (r *ArtworkRef) Comment(id int64) *CommentRef {
	if r == nil {
		return &CommentRef{ID: id}
	}
	return &CommentRef{ID: id, client: r.client, target: commentTarget{id: r.ID}}
}

// Comment constructs a comment reference carrying this novel as its reply target.
func (r *NovelRef) Comment(id int64) *CommentRef {
	if r == nil {
		return &CommentRef{ID: id}
	}
	return &CommentRef{ID: id, client: r.client, target: commentTarget{id: r.ID, novel: true}}
}

func (r *CommentRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "comment is unbound; create a reference through its client or owner", nil)
	}
	if r.ID <= 0 {
		return newError(operation, sdk.InvalidArgument, "comment ID must be positive", nil)
	}
	return nil
}

// UnmarshalJSON restores the comment ID without retaining client or reply-target bindings.
func (r *CommentRef) UnmarshalJSON(data []byte) error {
	var decoded struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return newError("CommentRef.UnmarshalJSON", sdk.InvalidArgument, "malformed comment reference", err)
	}
	*r = CommentRef{ID: decoded.ID}
	return nil
}

// Comment carries a comment's text, optional stamp, author, timestamp, and parent chain with bound operations.
type Comment struct {
	*CommentRef
	commentData
}

type commentData struct {
	Text      string    `json:"text"`
	Author    *User     `json:"author"` // ID is zero when upstream supplies no author identity.
	CreatedAt time.Time `json:"created_at"`
	Parent    *Comment  `json:"parent_comment"`
	Stamp     *Stamp    `json:"stamp,omitempty"`
}

// UnmarshalJSON restores comment data without retaining any earlier account binding.
func (c *Comment) UnmarshalJSON(data []byte) error {
	var decoded struct {
		commentData
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return newError("Comment.UnmarshalJSON", sdk.InvalidArgument, "malformed comment", err)
	}
	*c = Comment{CommentRef: &CommentRef{ID: decoded.ID}, commentData: decoded.commentData}
	return nil
}

// CommentsOptions resumes the same owner's comment list.
type CommentsOptions struct{ Cursor sdk.Cursor }

// CommentAccessControl preserves the upstream object or numeric access-control state.
// CanComment and IsLocked apply only when NumericValue is nil.
type CommentAccessControl struct {
	CanComment   bool   `json:"can_comment"`
	IsLocked     bool   `json:"is_locked"`
	NumericValue *int64 `json:"-"`
}

// MarshalJSON emits the numeric form without inferring boolean permissions.
func (c CommentAccessControl) MarshalJSON() ([]byte, error) {
	if c.NumericValue != nil {
		return json.Marshal(struct {
			Value int64 `json:"comment_access_control"`
		}{Value: *c.NumericValue})
	}
	return json.Marshal(struct {
		CanComment bool `json:"can_comment"`
		IsLocked   bool `json:"is_locked"`
	}{CanComment: c.CanComment, IsLocked: c.IsLocked})
}

// UnmarshalJSON restores the numeric or object access-control form and replaces prior state.
func (c *CommentAccessControl) UnmarshalJSON(data []byte) error {
	var value struct {
		CanComment   bool   `json:"can_comment"`
		IsLocked     bool   `json:"is_locked"`
		NumericValue *int64 `json:"comment_access_control"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return newError("CommentAccessControl.UnmarshalJSON", sdk.InvalidArgument, "malformed comment access control", err)
	}
	*c = CommentAccessControl{CanComment: value.CanComment, IsLocked: value.IsLocked, NumericValue: value.NumericValue}
	return nil
}

// CommentPage adds optional totals and access-control metadata to a comment batch.
// Next retains this metadata for each new batch; Page.Next returns only the generic page.
type CommentPage struct {
	Page          sdk.Page[*Comment]    `json:"page"`
	Total         *int64                `json:"total"`
	AccessControl *CommentAccessControl `json:"access_control"`
	load          func(context.Context) (CommentPage, error)
}

// UnmarshalJSON restores an unbound page, clearing any previous continuation loader.
func (p *CommentPage) UnmarshalJSON(data []byte) error {
	type value CommentPage
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return newError("CommentPage.UnmarshalJSON", sdk.InvalidArgument, "malformed comment page", err)
	}
	*p = CommentPage(decoded)
	return nil
}

// Next reads the next comment batch with its own totals and access-control state.
func (p CommentPage) Next(ctx context.Context) (CommentPage, error) {
	if p.Page.Cursor.IsZero() {
		return CommentPage{Page: sdk.NewPage[*Comment](nil, sdk.Cursor{}, nil)}, nil
	}
	if p.load == nil {
		return CommentPage{}, newError("Comments.Next", sdk.InvalidArgument, "comment page is unbound; resume its cursor through the owner", nil)
	}
	return p.load(ctx)
}

// Comments lists comments on this artwork with upstream totals and access control.
func (r *ArtworkRef) Comments(ctx context.Context, options CommentsOptions) (CommentPage, error) {
	if err := r.bound("Artwork.Comments"); err != nil {
		return CommentPage{}, err
	}
	return r.client.comments(ctx, commentTarget{id: r.ID}, options.Cursor)
}

// Comments lists comments on this novel with upstream totals and access control.
func (r *NovelRef) Comments(ctx context.Context, options CommentsOptions) (CommentPage, error) {
	if err := r.bound("Novel.Comments"); err != nil {
		return CommentPage{}, err
	}
	return r.client.comments(ctx, commentTarget{id: r.ID, novel: true}, options.Cursor)
}
