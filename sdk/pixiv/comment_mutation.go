package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// CommentOptions provides text, a stamp, or both. StampID zero selects text-only posting.
// Text may be empty when sending a positive StampID.
type CommentOptions struct {
	Text    string
	StampID int64
}

// PostComment publishes a text or stamp comment, returning the upstream-assigned reference.
// It sends the mutation once and performs no read-back request.
func (r *ArtworkRef) PostComment(ctx context.Context, options CommentOptions) (*CommentRef, error) {
	if err := r.bound("Artwork.PostComment"); err != nil {
		return nil, err
	}
	return r.client.postComment(ctx, commentTarget{id: r.ID}, 0, options)
}

// PostComment publishes a text or stamp comment on this novel, sending the mutation once.
func (r *NovelRef) PostComment(ctx context.Context, options CommentOptions) (*CommentRef, error) {
	if err := r.bound("Novel.PostComment"); err != nil {
		return nil, err
	}
	return r.client.postComment(ctx, commentTarget{id: r.ID, novel: true}, 0, options)
}

// Reply publishes a reply to this comment, returning the upstream-assigned reference.
// Create the reference through its owner or a comment list to retain a reply target.
func (r *CommentRef) Reply(ctx context.Context, options CommentOptions) (*CommentRef, error) {
	if err := r.bound("Comment.Reply"); err != nil {
		return nil, err
	}
	if r.target.id <= 0 {
		return nil, newError("Comment.Reply", sdk.InvalidArgument, "reply requires an artwork or novel owner", nil)
	}
	return r.client.postComment(ctx, r.target, r.ID, options)
}

// Delete removes this comment for the client account, sending the mutation once.
func (r *CommentRef) Delete(ctx context.Context) error {
	if err := r.bound("Comment.Delete"); err != nil {
		return err
	}
	_, _, path, _ := commentRoutes(r.target.novel)
	return r.client.postForm(ctx, path, url.Values{api.ParamCommentID: {strconv.FormatInt(r.ID, 10)}})
}

func (c *Client) postComment(ctx context.Context, target commentTarget, parentID int64, options CommentOptions) (*CommentRef, error) {
	if options.StampID < 0 || options.Text == "" && options.StampID == 0 {
		return nil, newError("Comment.Post", sdk.InvalidArgument, "comment requires text or a positive stamp ID", nil)
	}
	_, path, _, parameter := commentRoutes(target.novel)
	form := url.Values{parameter: {strconv.FormatInt(target.id, 10)}, api.ParamComment: {options.Text}}
	if options.StampID > 0 {
		form.Set(api.ParamStampID, strconv.FormatInt(options.StampID, 10))
	}
	if parentID > 0 {
		form.Set(api.ParamParentCommentID, strconv.FormatInt(parentID, 10))
	}
	value, err := c.credentials(ctx, Credentials{})
	if err != nil {
		return nil, err
	}
	var raw struct {
		CommentID *int64 `json:"comment_id"`
		Comment   *struct {
			ID *int64 `json:"id"`
		} `json:"comment"`
	}
	// An uncertain write is never replayed, including after a 401 or Retry-After.
	if err := c.app(value).PostFormJSON(ctx, path, form, &raw); err != nil {
		return nil, classifyError(path, err, false)
	}
	id := raw.CommentID
	if id == nil && raw.Comment != nil {
		id = raw.Comment.ID
	}
	if id == nil || *id <= 0 {
		return nil, newError(path, sdk.MalformedUpstreamResponse, "comment response has no positive ID", nil)
	}
	return &CommentRef{ID: *id, client: c, target: target}, nil
}
