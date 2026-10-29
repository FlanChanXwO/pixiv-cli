package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type commentMutationWire struct {
	CommentID *int64 `json:"comment_id"`
	Comment   *struct {
		ID *int64 `json:"id"`
	} `json:"comment"`
}

func (w commentMutationWire) commentID() *int64 {
	if w.CommentID != nil {
		return w.CommentID
	}
	if w.Comment != nil {
		return w.Comment.ID
	}
	return nil
}

// postComment posts the form that create, reply, and stamp share and requires the
// response to report a comment ID.
func (c *Client) postComment(ctx context.Context, operation, path string, form url.Values) (CommentMutationResult, error) {
	var raw commentMutationWire
	if err := c.app.PostFormJSON(ctx, path, form, &raw); err != nil {
		return CommentMutationResult{}, classifyAppError(err, operation)
	}
	commentID := raw.commentID()
	if commentID == nil || *commentID <= 0 {
		return CommentMutationResult{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return CommentMutationResult{CommentID: *commentID}, nil
}

// PostArtworkComment posts a top-level comment on one artwork.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) PostArtworkComment(ctx context.Context, request PostArtworkCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "PostArtworkComment")
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("PostArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("PostArtworkComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	return c.postComment(ctx, "PostArtworkComment", api.PathIllustCommentAdd, url.Values{
		api.ParamIllustID: {itoa(request.ArtworkID)},
		api.ParamComment:  {request.Comment},
	})
}

// ReplyArtworkComment replies to one artwork comment.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) ReplyArtworkComment(ctx context.Context, request ReplyArtworkCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "ReplyArtworkComment")
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.ParentCommentID <= 0 {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "parent comment ID must be positive")
	}
	return c.postComment(ctx, "ReplyArtworkComment", api.PathIllustCommentAdd, url.Values{
		api.ParamIllustID:        {itoa(request.ArtworkID)},
		api.ParamComment:         {request.Comment},
		api.ParamParentCommentID: {itoa(request.ParentCommentID)},
	})
}

// DeleteArtworkComment deletes one artwork comment by its comment ID.
func (c *Client) DeleteArtworkComment(ctx context.Context, request DeleteArtworkCommentRequest) error {
	ctx = logging.WithOperation(ctx, "DeleteArtworkComment")
	if request.CommentID <= 0 {
		return newError("DeleteArtworkComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.app.PostForm(ctx, api.PathIllustCommentDelete, url.Values{
		api.ParamCommentID: {itoa(request.CommentID)},
	}); err != nil {
		return classifyAppError(err, "DeleteArtworkComment")
	}
	return nil
}

// PostNovelComment posts a top-level comment on one novel.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) PostNovelComment(ctx context.Context, request PostNovelCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "PostNovelComment")
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("PostNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("PostNovelComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	return c.postComment(ctx, "PostNovelComment", api.PathNovelCommentAdd, url.Values{
		api.ParamNovelID: {itoa(request.NovelID)},
		api.ParamComment: {request.Comment},
	})
}

// ReplyNovelComment replies to one novel comment.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) ReplyNovelComment(ctx context.Context, request ReplyNovelCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "ReplyNovelComment")
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.ParentCommentID <= 0 {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "parent comment ID must be positive")
	}
	return c.postComment(ctx, "ReplyNovelComment", api.PathNovelCommentAdd, url.Values{
		api.ParamNovelID:         {itoa(request.NovelID)},
		api.ParamComment:         {request.Comment},
		api.ParamParentCommentID: {itoa(request.ParentCommentID)},
	})
}

// DeleteNovelComment deletes one novel comment by its comment ID.
func (c *Client) DeleteNovelComment(ctx context.Context, request DeleteNovelCommentRequest) error {
	ctx = logging.WithOperation(ctx, "DeleteNovelComment")
	if request.CommentID <= 0 {
		return newError("DeleteNovelComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.app.PostForm(ctx, api.PathNovelCommentDelete, url.Values{
		api.ParamCommentID: {itoa(request.CommentID)},
	}); err != nil {
		return classifyAppError(err, "DeleteNovelComment")
	}
	return nil
}

// StampArtworkComment posts a stamp comment on one artwork. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampArtworkComment(ctx context.Context, request StampArtworkCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "StampArtworkComment")
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	// The App API sticker-only wire form requires an empty comment; stamp_id
	// identifies the sticker on its own.
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	return c.postComment(ctx, "StampArtworkComment", api.PathIllustCommentAdd, url.Values{
		api.ParamIllustID: {itoa(request.ArtworkID)},
		api.ParamComment:  {request.Comment},
		api.ParamStampID:  {itoa(request.StampID)},
	})
}

// StampNovelComment posts a stamp comment on one novel. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampNovelComment(ctx context.Context, request StampNovelCommentRequest) (CommentMutationResult, error) {
	ctx = logging.WithOperation(ctx, "StampNovelComment")
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	// The App API sticker-only wire form requires an empty comment; stamp_id
	// identifies the sticker on its own.
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	return c.postComment(ctx, "StampNovelComment", api.PathNovelCommentAdd, url.Values{
		api.ParamNovelID: {itoa(request.NovelID)},
		api.ParamComment: {request.Comment},
		api.ParamStampID: {itoa(request.StampID)},
	})
}

// AddArtworkBookmark bookmarks one artwork. Tags, when non-empty, are applied
// as bookmark tags.
func (c *Client) AddArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest) error {
	ctx = logging.WithOperation(ctx, "AddArtworkBookmark")
	const operation = "AddArtworkBookmark"
	if request.ArtworkID <= 0 {
		return newError(operation, sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict(operation, request.Restrict); err != nil {
		return err
	}
	form := url.Values{
		api.ParamIllustID: {itoa(request.ArtworkID)},
		api.ParamRestrict: {string(request.Restrict)},
	}
	for _, tag := range request.Tags {
		form.Add(api.ParamTags, tag)
	}
	if err := c.app.PostForm(ctx, api.PathBookmarkAdd, form); err != nil {
		return classifyAppError(err, operation)
	}
	return nil
}

// RemoveArtworkBookmark removes the current user's bookmark from one artwork.
func (c *Client) RemoveArtworkBookmark(ctx context.Context, request RemoveArtworkBookmarkRequest) error {
	ctx = logging.WithOperation(ctx, "RemoveArtworkBookmark")
	const operation = "RemoveArtworkBookmark"
	if request.ArtworkID <= 0 {
		return newError(operation, sdk.InvalidArgument, "artwork ID must be positive")
	}
	if err := c.app.PostForm(ctx, api.PathBookmarkDelete, url.Values{
		api.ParamIllustID: {itoa(request.ArtworkID)},
	}); err != nil {
		return classifyAppError(err, operation)
	}
	return nil
}

// AddNovelBookmark bookmarks one novel. Tags, when non-empty, are applied as
// bookmark tags.
func (c *Client) AddNovelBookmark(ctx context.Context, request AddNovelBookmarkRequest) error {
	ctx = logging.WithOperation(ctx, "AddNovelBookmark")
	if request.NovelID <= 0 {
		return newError("AddNovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict("AddNovelBookmark", request.Restrict); err != nil {
		return err
	}
	form := url.Values{
		api.ParamNovelID:  {itoa(request.NovelID)},
		api.ParamRestrict: {string(request.Restrict)},
	}
	for _, tag := range request.Tags {
		form.Add(api.ParamTags, tag)
	}
	if err := c.app.PostForm(ctx, api.PathNovelBookmarkAdd, form); err != nil {
		return classifyAppError(err, "AddNovelBookmark")
	}
	return nil
}

// RemoveNovelBookmark removes the current user's bookmark from one novel.
func (c *Client) RemoveNovelBookmark(ctx context.Context, request RemoveNovelBookmarkRequest) error {
	ctx = logging.WithOperation(ctx, "RemoveNovelBookmark")
	if request.NovelID <= 0 {
		return newError("RemoveNovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	if err := c.app.PostForm(ctx, api.PathNovelBookmarkDelete, url.Values{
		api.ParamNovelID: {itoa(request.NovelID)},
	}); err != nil {
		return classifyAppError(err, "RemoveNovelBookmark")
	}
	return nil
}

// FollowUser adds the authenticated account to one user's followers. An empty
// Restrict defaults to public.
func (c *Client) FollowUser(ctx context.Context, request FollowUserRequest) error {
	ctx = logging.WithOperation(ctx, "FollowUser")
	if request.UserID <= 0 {
		return newError("FollowUser", sdk.InvalidArgument, "user ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict("FollowUser", request.Restrict); err != nil {
		return err
	}
	if err := c.app.PostForm(ctx, api.PathFollowAdd, url.Values{
		api.ParamUserID:   {itoa(request.UserID)},
		api.ParamRestrict: {string(request.Restrict)},
	}); err != nil {
		return classifyAppError(err, "FollowUser")
	}
	return nil
}

// UnfollowUser unfollows one user.
func (c *Client) UnfollowUser(ctx context.Context, request UnfollowUserRequest) error {
	ctx = logging.WithOperation(ctx, "UnfollowUser")
	if request.UserID <= 0 {
		return newError("UnfollowUser", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := c.app.PostForm(ctx, api.PathFollowDelete, url.Values{
		api.ParamUserID: {itoa(request.UserID)},
	}); err != nil {
		return classifyAppError(err, "UnfollowUser")
	}
	return nil
}

// PostArtworkCommentRequest posts a top-level comment on one artwork.
type PostArtworkCommentRequest struct {
	ArtworkID int64
	Comment   string
}

// ReplyArtworkCommentRequest replies to one artwork comment.
type ReplyArtworkCommentRequest struct {
	ArtworkID       int64
	Comment         string
	ParentCommentID int64
}

// DeleteArtworkCommentRequest deletes one artwork comment by its comment ID.
type DeleteArtworkCommentRequest struct {
	CommentID int64
}

// StampArtworkCommentRequest posts a stamp comment on one artwork. Comment is
// optional and may be empty for the sticker-only wire form; StampID is an
// independent upstream field.
type StampArtworkCommentRequest struct {
	ArtworkID int64
	Comment   string
	StampID   int64
}

// PostNovelCommentRequest posts a top-level comment on one novel.
type PostNovelCommentRequest struct {
	NovelID int64
	Comment string
}

// ReplyNovelCommentRequest replies to one novel comment.
type ReplyNovelCommentRequest struct {
	NovelID         int64
	Comment         string
	ParentCommentID int64
}

// DeleteNovelCommentRequest deletes one novel comment by its comment ID.
type DeleteNovelCommentRequest struct {
	CommentID int64
}

// StampNovelCommentRequest posts a stamp comment on one novel. Comment is
// optional and may be empty for the sticker-only wire form; StampID is an
// independent upstream field.
type StampNovelCommentRequest struct {
	NovelID int64
	Comment string
	StampID int64
}

// AddArtworkBookmarkRequest bookmarks one artwork. Tags are applied as
// bookmark tags when non-empty.
type AddArtworkBookmarkRequest struct {
	ArtworkID int64
	Restrict  Restrict
	Tags      []string
}

// RemoveArtworkBookmarkRequest removes the current user's bookmark from one
// artwork.
type RemoveArtworkBookmarkRequest struct {
	ArtworkID int64
}

// AddNovelBookmarkRequest bookmarks one novel. Tags are applied as bookmark
// tags when non-empty.
type AddNovelBookmarkRequest struct {
	NovelID  int64
	Restrict Restrict
	Tags     []string
}

// RemoveNovelBookmarkRequest removes the current user's bookmark from one
// novel.
type RemoveNovelBookmarkRequest struct {
	NovelID int64
}

// FollowUserRequest follows one user.
type FollowUserRequest struct {
	UserID   int64
	Restrict Restrict
}

// UnfollowUserRequest unfollows one user.
type UnfollowUserRequest struct {
	UserID int64
}
