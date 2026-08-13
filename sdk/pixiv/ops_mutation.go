package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/follow"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// AddBookmark bookmarks one artwork. Tags, when non-empty, are applied as
// bookmark tags.
func (c *Client) AddBookmark(ctx context.Context, request AddBookmarkRequest) error {
	if request.ArtworkID <= 0 {
		return newError("AddBookmark", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := validateRestrict("AddBookmark", request.Restrict); err != nil {
		return err
	}
	if err := c.artworkBookmark.Add(ctx, bookmark.AddRequest{ArtworkID: request.ArtworkID, Restrict: string(request.Restrict), Tags: request.Tags}); err != nil {
		return classifyAppError(err, "AddBookmark")
	}
	return nil
}

// RemoveBookmark removes the current user's bookmark from one artwork.
func (c *Client) RemoveBookmark(ctx context.Context, request RemoveBookmarkRequest) error {
	if request.ArtworkID <= 0 {
		return newError("RemoveBookmark", sdk.InvalidArgument, "artwork ID must be positive")
	}
	if err := c.artworkBookmark.Remove(ctx, request.ArtworkID); err != nil {
		return classifyAppError(err, "RemoveBookmark")
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

// SetAIArtworkVisibility sets whether AI-generated artworks are shown in the
// current user's feeds.
func (c *Client) SetAIArtworkVisibility(ctx context.Context, request SetAIArtworkVisibilityRequest) error {
	if err := c.userVisibility.Set(ctx, request.Visible); err != nil {
		return classifyAppError(err, "SetAIArtworkVisibility")
	}
	return nil
}
