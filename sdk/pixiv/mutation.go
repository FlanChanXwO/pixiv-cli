// mutation 领域：收藏、关注和评论的写操作。
// 读操作见 bookmark.go、comment.go、user.go；请求类型在本文件。

package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/follow"
	usernovelbookmarks "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
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

// postComment 发送 create/reply/stamp 共用的 form，并要求响应给出 comment ID。
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
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("PostArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("PostArtworkComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	return c.postComment(ctx, "PostArtworkComment", pixivapi.PathIllustCommentAdd, url.Values{
		pixivapi.ParamIllustID: {itoa(request.ArtworkID)},
		pixivapi.ParamComment:  {request.Comment},
	})
}

// ReplyArtworkComment replies to one artwork comment.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) ReplyArtworkComment(ctx context.Context, request ReplyArtworkCommentRequest) (CommentMutationResult, error) {
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.ParentCommentID <= 0 {
		return CommentMutationResult{}, newError("ReplyArtworkComment", sdk.InvalidArgument, "parent comment ID must be positive")
	}
	return c.postComment(ctx, "ReplyArtworkComment", pixivapi.PathIllustCommentAdd, url.Values{
		pixivapi.ParamIllustID:        {itoa(request.ArtworkID)},
		pixivapi.ParamComment:         {request.Comment},
		pixivapi.ParamParentCommentID: {itoa(request.ParentCommentID)},
	})
}

// DeleteArtworkComment deletes one artwork comment by its comment ID.
func (c *Client) DeleteArtworkComment(ctx context.Context, request DeleteArtworkCommentRequest) error {
	if request.CommentID <= 0 {
		return newError("DeleteArtworkComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.app.PostForm(ctx, pixivapi.PathIllustCommentDelete, url.Values{
		pixivapi.ParamCommentID: {itoa(request.CommentID)},
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
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("PostNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("PostNovelComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	return c.postComment(ctx, "PostNovelComment", pixivapi.PathNovelCommentAdd, url.Values{
		pixivapi.ParamNovelID: {itoa(request.NovelID)},
		pixivapi.ParamComment: {request.Comment},
	})
}

// ReplyNovelComment replies to one novel comment.
//
// The returned ID is copied directly from the upstream response. It is not a
// read-back confirmation and must not be inferred from a subsequent comment
// listing.
func (c *Client) ReplyNovelComment(ctx context.Context, request ReplyNovelCommentRequest) (CommentMutationResult, error) {
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.ParentCommentID <= 0 {
		return CommentMutationResult{}, newError("ReplyNovelComment", sdk.InvalidArgument, "parent comment ID must be positive")
	}
	return c.postComment(ctx, "ReplyNovelComment", pixivapi.PathNovelCommentAdd, url.Values{
		pixivapi.ParamNovelID:         {itoa(request.NovelID)},
		pixivapi.ParamComment:         {request.Comment},
		pixivapi.ParamParentCommentID: {itoa(request.ParentCommentID)},
	})
}

// DeleteNovelComment deletes one novel comment by its comment ID.
func (c *Client) DeleteNovelComment(ctx context.Context, request DeleteNovelCommentRequest) error {
	if request.CommentID <= 0 {
		return newError("DeleteNovelComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.app.PostForm(ctx, pixivapi.PathNovelCommentDelete, url.Values{
		pixivapi.ParamCommentID: {itoa(request.CommentID)},
	}); err != nil {
		return classifyAppError(err, "DeleteNovelComment")
	}
	return nil
}

// StampArtworkComment posts a stamp comment on one artwork. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampArtworkComment(ctx context.Context, request StampArtworkCommentRequest) (CommentMutationResult, error) {
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	// App API 的 sticker-only wire 形态要求 comment 为空；stamp_id 单独标识贴纸。
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	return c.postComment(ctx, "StampArtworkComment", pixivapi.PathIllustCommentAdd, url.Values{
		pixivapi.ParamIllustID: {itoa(request.ArtworkID)},
		pixivapi.ParamComment:  {request.Comment},
		pixivapi.ParamStampID:  {itoa(request.StampID)},
	})
}

// StampNovelComment posts a stamp comment on one novel. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampNovelComment(ctx context.Context, request StampNovelCommentRequest) (CommentMutationResult, error) {
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	// App API 的 sticker-only wire 形态要求 comment 为空；stamp_id 单独标识贴纸。
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	return c.postComment(ctx, "StampNovelComment", pixivapi.PathNovelCommentAdd, url.Values{
		pixivapi.ParamNovelID: {itoa(request.NovelID)},
		pixivapi.ParamComment: {request.Comment},
		pixivapi.ParamStampID: {itoa(request.StampID)},
	})
}

// AddArtworkBookmark bookmarks one artwork. Tags, when non-empty, are applied
// as bookmark tags.
func (c *Client) AddArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest) error {
	return c.addArtworkBookmark(ctx, request)
}

func (c *Client) addArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest) error {
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
		pixivapi.ParamIllustID: {itoa(request.ArtworkID)},
		pixivapi.ParamRestrict: {string(request.Restrict)},
	}
	for _, tag := range request.Tags {
		form.Add(pixivapi.ParamTags, tag)
	}
	if err := c.app.PostForm(ctx, pixivapi.PathBookmarkAdd, form); err != nil {
		return classifyAppError(err, operation)
	}
	return nil
}

// RemoveArtworkBookmark removes the current user's bookmark from one artwork.
func (c *Client) RemoveArtworkBookmark(ctx context.Context, request RemoveArtworkBookmarkRequest) error {
	return c.removeArtworkBookmark(ctx, request.ArtworkID)
}

func (c *Client) removeArtworkBookmark(ctx context.Context, artworkID int64) error {
	const operation = "RemoveArtworkBookmark"
	if artworkID <= 0 {
		return newError(operation, sdk.InvalidArgument, "artwork ID must be positive")
	}
	if err := c.app.PostForm(ctx, pixivapi.PathBookmarkDelete, url.Values{
		pixivapi.ParamIllustID: {itoa(artworkID)},
	}); err != nil {
		return classifyAppError(err, operation)
	}
	return nil
}

// AddNovelBookmark bookmarks one novel. Tags, when non-empty, are applied as
// bookmark tags.
func (c *Client) AddNovelBookmark(ctx context.Context, request AddNovelBookmarkRequest) error {
	if request.NovelID <= 0 {
		return newError("AddNovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict("AddNovelBookmark", request.Restrict); err != nil {
		return err
	}
	if err := c.userNovelBookmarks.Add(ctx, usernovelbookmarks.AddRequest{NovelID: request.NovelID, Restrict: string(request.Restrict), Tags: request.Tags}); err != nil {
		return classifyAppError(err, "AddNovelBookmark")
	}
	return nil
}

// RemoveNovelBookmark removes the current user's bookmark from one novel.
func (c *Client) RemoveNovelBookmark(ctx context.Context, request RemoveNovelBookmarkRequest) error {
	if request.NovelID <= 0 {
		return newError("RemoveNovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	if err := c.userNovelBookmarks.Remove(ctx, request.NovelID); err != nil {
		return classifyAppError(err, "RemoveNovelBookmark")
	}
	return nil
}

// FollowUser follows one user.
func (c *Client) FollowUser(ctx context.Context, request FollowUserRequest) error {
	if request.UserID <= 0 {
		return newError("FollowUser", sdk.InvalidArgument, "user ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict("FollowUser", request.Restrict); err != nil {
		return err
	}
	if err := c.userFollow.Add(ctx, follow.Request{UserID: request.UserID, Restrict: string(request.Restrict)}); err != nil {
		return classifyAppError(err, "FollowUser")
	}
	return nil
}

// UnfollowUser unfollows one user.
func (c *Client) UnfollowUser(ctx context.Context, request UnfollowUserRequest) error {
	if request.UserID <= 0 {
		return newError("UnfollowUser", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := c.userFollow.Remove(ctx, request.UserID); err != nil {
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
