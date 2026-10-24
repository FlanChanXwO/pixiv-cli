package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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
