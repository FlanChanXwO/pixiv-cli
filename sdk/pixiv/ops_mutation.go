package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/follow"
	usernovelbookmarks "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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
	if err := c.artworkBookmark.Add(ctx, bookmark.AddRequest{ArtworkID: request.ArtworkID, Restrict: string(request.Restrict), Tags: request.Tags}); err != nil {
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
	if err := c.artworkBookmark.Remove(ctx, artworkID); err != nil {
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
