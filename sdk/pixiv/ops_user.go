package pixiv

import (
	"context"
	"net/url"

	userentity "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user"
	userblocked "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/blocked"
	userfollowers "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/followers"
	userfollowing "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/following"
	usermypixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/mypixiv"
	userrecommended "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/recommended"
	userrelated "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/related"
	usersearch "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/search"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchUsers searches users.
func (c *Client) SearchUsers(ctx context.Context, request SearchUsersRequest) (sdk.Page[UserPreview], error) {
	if err := validateSearchWord("SearchUsers", request.Word); err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query := url.Values{"word": {request.Word}}
	offset, err := c.continuationOffset("SearchUsers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userSearch.Search(ctx, usersearch.Request{Word: request.Word, Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "SearchUsers")
	}
	return c.userPreviewPage("SearchUsers", query, list.Items, list.NextOffset, list.HasNext)
}

// User returns one user's detail by their stable ID.
func (c *Client) User(ctx context.Context, request UserRequest) (UserDetail, error) {
	if request.UserID <= 0 {
		return UserDetail{}, newError("User", sdk.InvalidArgument, "user ID must be positive")
	}
	detail, err := c.userDetail.Detail(ctx, request.UserID)
	if err != nil {
		return UserDetail{}, classifyAppError(err, "User")
	}
	return c.mapUserDetail(detail)
}

// RecommendedUsers lists recommended users with sample works.
func (c *Client) RecommendedUsers(ctx context.Context, request RecommendedUsersRequest) (sdk.Page[UserPreview], error) {
	query := url.Values{}
	offset, contExists, err := c.continuationOffsetExists("RecommendedUsers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userRecommended.List(ctx, userrecommended.Request{Offset: offset, ContinuationExists: contExists})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "RecommendedUsers")
	}
	items := make([]UserPreview, 0, len(list.Items))
	for _, p := range list.Items {
		items = append(items, c.mapUserPreview(p))
	}
	next, err := c.buildCursor("RecommendedUsers", query, "offset", int64(list.NextOffset), list.HasNext)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	return sdk.Page[UserPreview]{Items: items, Next: next}, nil
}

// RelatedUsers lists users related to one user.
func (c *Client) RelatedUsers(ctx context.Context, request RelatedUsersRequest) (sdk.Page[UserPreview], error) {
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("RelatedUsers", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"seed_user_id": {itoa(request.UserID)}}
	offset, err := c.continuationOffset("RelatedUsers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userRelated.List(ctx, userrelated.Request{SeedUserID: request.UserID, Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "RelatedUsers")
	}
	return c.userPreviewPage("RelatedUsers", query, list.Items, list.NextOffset, list.HasNext)
}

// UserFollowing lists the users one user follows.
func (c *Client) UserFollowing(ctx context.Context, request UserFollowingRequest) (sdk.Page[UserPreview], error) {
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("UserFollowing", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("UserFollowing", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userFollowing.List(ctx, userfollowing.Request{UserID: request.UserID, Restrict: string(request.Restrict), Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "UserFollowing")
	}
	return c.userPreviewPage("UserFollowing", query, list.Items, list.NextOffset, list.HasNext)
}

// UserFollowers lists the users following one user.
func (c *Client) UserFollowers(ctx context.Context, request UserFollowersRequest) (sdk.Page[UserPreview], error) {
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("UserFollowers", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("UserFollowers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userFollowers.List(ctx, userfollowers.Request{UserID: request.UserID, Restrict: string(request.Restrict), Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "UserFollowers")
	}
	return c.userPreviewPage("UserFollowers", query, list.Items, list.NextOffset, list.HasNext)
}

// UserBlockedUsers lists the users one user has blocked.
func (c *Client) UserBlockedUsers(ctx context.Context, request UserBlockedUsersRequest) (sdk.Page[UserPreview], error) {
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("UserBlockedUsers", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}}
	offset, err := c.continuationOffset("UserBlockedUsers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userBlocked.List(ctx, userblocked.Request{UserID: request.UserID, Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "UserBlockedUsers")
	}
	return c.userPreviewPage("UserBlockedUsers", query, list.Items, list.NextOffset, list.HasNext)
}

// MyPixivUsers lists the current user's MyPixiv feed users. It requires a
// verified identity, so it fails with Unauthorized on a New client whose
// user ID is unknown.
func (c *Client) MyPixivUsers(ctx context.Context, request MyPixivUsersRequest) (sdk.Page[UserPreview], error) {
	if c.userID <= 0 {
		return sdk.Page[UserPreview]{}, newError("MyPixivUsers", sdk.Unauthorized, "current user identity is unknown")
	}
	query := url.Values{}
	offset, err := c.continuationOffset("MyPixivUsers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userMyPixiv.List(ctx, usermypixiv.Request{UserID: c.userID, Offset: offset})
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, "MyPixivUsers")
	}
	return c.userPreviewPage("MyPixivUsers", query, list.Items, list.NextOffset, list.HasNext)
}

// CurrentUser returns the current authenticated user's detail. It requires a
// verified identity, so it fails with Unauthorized on a New client whose
// user ID is unknown.
func (c *Client) CurrentUser(ctx context.Context, request CurrentUserRequest) (UserDetail, error) {
	if c.userID <= 0 {
		return UserDetail{}, newError("CurrentUser", sdk.Unauthorized, "current user identity is unknown")
	}
	detail, err := c.userDetail.Current(ctx, c.userID)
	if err != nil {
		return UserDetail{}, classifyAppError(err, "CurrentUser")
	}
	return c.mapUserDetail(detail)
}

func (c *Client) userPreviewPage(op string, query url.Values, list []userentity.Preview, nextOffset int, hasNext bool) (sdk.Page[UserPreview], error) {
	items := make([]UserPreview, 0, len(list))
	for _, p := range list {
		items = append(items, UserPreview{User: c.mapUser(p.User)})
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	return sdk.Page[UserPreview]{Items: items, Next: next}, nil
}
