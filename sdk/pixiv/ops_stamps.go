package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
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
	// App API 的 sticker-only wire 形态要求 comment 为空；stamp_id 单独标识贴纸。
	if request.StampID <= 0 {
		return CommentMutationResult{}, newError("StampArtworkComment", sdk.InvalidArgument, "stamp ID must be positive")
	}
	return c.postArtworkComment(ctx, "StampArtworkComment", url.Values{
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
