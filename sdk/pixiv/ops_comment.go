package pixiv

import (
	"context"

	artworkcomments "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/comments"
	novelcomments "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/comments"
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
	result, err := c.artworkComments.Create(ctx, artworkcomments.CreateRequest{
		ArtworkID: request.ArtworkID,
		Comment:   request.Comment,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "PostArtworkComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
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
	result, err := c.artworkComments.Reply(ctx, artworkcomments.ReplyRequest{
		ArtworkID:       request.ArtworkID,
		Comment:         request.Comment,
		ParentCommentID: request.ParentCommentID,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "ReplyArtworkComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
}

// DeleteArtworkComment deletes one artwork comment by its comment ID.
func (c *Client) DeleteArtworkComment(ctx context.Context, request DeleteArtworkCommentRequest) error {
	if request.CommentID <= 0 {
		return newError("DeleteArtworkComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.artworkComments.Delete(ctx, request.CommentID); err != nil {
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
	result, err := c.novelComments.Create(ctx, novelcomments.CreateRequest{
		NovelID: request.NovelID,
		Comment: request.Comment,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "PostNovelComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
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
	result, err := c.novelComments.Reply(ctx, novelcomments.ReplyRequest{
		NovelID:         request.NovelID,
		Comment:         request.Comment,
		ParentCommentID: request.ParentCommentID,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "ReplyNovelComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
}

// DeleteNovelComment deletes one novel comment by its comment ID.
func (c *Client) DeleteNovelComment(ctx context.Context, request DeleteNovelCommentRequest) error {
	if request.CommentID <= 0 {
		return newError("DeleteNovelComment", sdk.InvalidArgument, "comment ID must be positive")
	}
	if err := c.novelComments.Delete(ctx, request.CommentID); err != nil {
		return classifyAppError(err, "DeleteNovelComment")
	}
	return nil
}
