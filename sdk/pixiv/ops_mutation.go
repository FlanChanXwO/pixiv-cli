package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/follow"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// AddArtworkBookmark bookmarks one artwork. Tags, when non-empty, are applied
// as bookmark tags.
func (c *Client) AddArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest) error {
	return c.addArtworkBookmark(ctx, request, "AddArtworkBookmark")
}

// addArtworkBookmark contains the shared implementation for the explicit
// artwork operation and its legacy wrapper so their validation and wire
// semantics cannot drift apart.
func (c *Client) addArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest, operation string) error {
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

// AddBookmark bookmarks one artwork. It is retained as a source-compatible
// wrapper around AddArtworkBookmark's implementation.
func (c *Client) AddBookmark(ctx context.Context, request AddBookmarkRequest) error {
	return c.addArtworkBookmark(ctx, AddArtworkBookmarkRequest{
		ArtworkID: request.ArtworkID,
		Restrict:  request.Restrict,
		Tags:      request.Tags,
	}, "AddBookmark")
}

// RemoveArtworkBookmark removes the current user's bookmark from one artwork.
func (c *Client) RemoveArtworkBookmark(ctx context.Context, request RemoveArtworkBookmarkRequest) error {
	return c.removeArtworkBookmark(ctx, request.ArtworkID, "RemoveArtworkBookmark")
}

// removeArtworkBookmark contains the shared implementation for the explicit
// artwork operation and its legacy wrapper.
func (c *Client) removeArtworkBookmark(ctx context.Context, artworkID int64, operation string) error {
	if artworkID <= 0 {
		return newError(operation, sdk.InvalidArgument, "artwork ID must be positive")
	}
	if err := c.artworkBookmark.Remove(ctx, artworkID); err != nil {
		return classifyAppError(err, operation)
	}
	return nil
}

// RemoveBookmark removes the current user's bookmark from one artwork. It is
// retained as a source-compatible wrapper around RemoveArtworkBookmark's
// implementation.
func (c *Client) RemoveBookmark(ctx context.Context, request RemoveBookmarkRequest) error {
	return c.removeArtworkBookmark(ctx, request.ArtworkID, "RemoveBookmark")
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

// SetAIArtworkVisibility sets whether AI-generated artworks are shown in the
// current user's feeds.
func (c *Client) SetAIArtworkVisibility(ctx context.Context, request SetAIArtworkVisibilityRequest) error {
	if err := c.userVisibility.Set(ctx, request.Visible); err != nil {
		return classifyAppError(err, "SetAIArtworkVisibility")
	}
	return nil
}
