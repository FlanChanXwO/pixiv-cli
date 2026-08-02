package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// AddBookmark bookmarks one artwork. Tags, when non-empty, are applied as
// bookmark tags.
func (c *Client) AddBookmark(ctx context.Context, request AddBookmarkRequest) error {
	if request.ArtworkID <= 0 {
		return newError("AddBookmark", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := c.app.AddBookmark(ctx, request.ArtworkID, string(request.Restrict), request.Tags); err != nil {
		return classifyAppError(err, "AddBookmark")
	}
	return nil
}

// RemoveBookmark removes the current user's bookmark from one artwork.
func (c *Client) RemoveBookmark(ctx context.Context, request RemoveBookmarkRequest) error {
	if request.ArtworkID <= 0 {
		return newError("RemoveBookmark", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	if err := c.app.RemoveBookmark(ctx, request.ArtworkID); err != nil {
		return classifyAppError(err, "RemoveBookmark")
	}
	return nil
}

// FollowUser follows one user.
func (c *Client) FollowUser(ctx context.Context, request FollowUserRequest) error {
	if request.UserID <= 0 {
		return newError("FollowUser", sdk.CodeInvalidArgument, "user ID must be positive")
	}
	if request.Restrict == "" {
		request.Restrict = RestrictPublic
	}
	if err := c.app.FollowUser(ctx, request.UserID, string(request.Restrict)); err != nil {
		return classifyAppError(err, "FollowUser")
	}
	return nil
}

// UnfollowUser unfollows one user.
func (c *Client) UnfollowUser(ctx context.Context, request UnfollowUserRequest) error {
	if request.UserID <= 0 {
		return newError("UnfollowUser", sdk.CodeInvalidArgument, "user ID must be positive")
	}
	if err := c.app.UnfollowUser(ctx, request.UserID); err != nil {
		return classifyAppError(err, "UnfollowUser")
	}
	return nil
}

// SetAIArtworkVisibility sets whether AI-generated artworks are shown in the
// current user's feeds.
func (c *Client) SetAIArtworkVisibility(ctx context.Context, request SetAIArtworkVisibilityRequest) error {
	if err := c.app.SetAIArtworkVisibility(ctx, request.Visible); err != nil {
		return classifyAppError(err, "SetAIArtworkVisibility")
	}
	return nil
}
