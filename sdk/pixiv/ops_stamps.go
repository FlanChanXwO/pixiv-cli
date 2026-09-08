package pixiv

import (
	"context"

	artworkcomments "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/comments"
	novelcomments "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/comments"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Stamps lists the comment stamps available to the authenticated user. Each
// image is represented as a stable SDK resource; the upstream locator remains
// runtime-only and is never embedded in the opaque reference.
func (c *Client) Stamps(ctx context.Context, request StampsRequest) ([]Stamp, error) {
	result, err := c.stamps.List(ctx)
	if err != nil {
		return nil, classifyAppError(err, "Stamps")
	}
	items := make([]Stamp, 0, len(result.Items))
	for _, value := range result.Items {
		image, err := c.newResourceWithVariant("stamp", value.ID, -1, "", value.URL)
		if err != nil {
			return nil, classifyAppError(err, "Stamps")
		}
		items = append(items, Stamp{ID: value.ID, Image: ImageResource{Resource: image}})
	}
	return items, nil
}

// StampArtworkComment posts a stamp comment on one artwork. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampArtworkComment(ctx context.Context, request StampArtworkCommentRequest) (CommentMutationResult, error) {
	if request.ArtworkID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	result, err := c.artworkComments.Stamp(ctx, artworkcomments.StampRequest{
		ArtworkID: request.ArtworkID,
		Comment:   request.Comment,
		StampID:   request.StampID,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "StampArtworkComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
}

// StampNovelComment posts a stamp comment on one novel. StampID is sent
// independently from Comment and never becomes a reply parent ID.
func (c *Client) StampNovelComment(ctx context.Context, request StampNovelCommentRequest) (CommentMutationResult, error) {
	if request.NovelID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "novel ID must be positive")
	}
	if request.Comment == "" {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "comment body must not be empty")
	}
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampNovelComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	result, err := c.novelComments.Stamp(ctx, novelcomments.StampRequest{
		NovelID: request.NovelID,
		Comment: request.Comment,
		StampID: request.StampID,
	})
	if err != nil {
		return CommentMutationResult{}, classifyAppError(err, "StampNovelComment")
	}
	return CommentMutationResult{CommentID: result.CommentID}, nil
}
