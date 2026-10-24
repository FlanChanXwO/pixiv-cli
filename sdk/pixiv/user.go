// user 领域：用户详情与各类用户列表。
// 请求类型与映射在本文件；共用 user 映射在 response.go。

package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user"
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

// mapUserEntity 映射 user 端点实体；user 端点并入 SDK 后由 wire.go 的 mapUser
// 取代（G08）。
func (c *Client) mapUserEntity(m user.User) User {
	out := User{
		ID:         m.ID,
		Name:       m.Name,
		Account:    m.Account,
		Comment:    m.Comment,
		IsFollowed: m.IsFollowed,
	}
	if m.ProfileImageURLs.Medium != nil && *m.ProfileImageURLs.Medium != "" {
		if res, err := c.newResourceWithVariant("user_profile", m.ID, -1, "medium", *m.ProfileImageURLs.Medium); err == nil {
			out.ProfileImage = ImageResource{Resource: res, Variant: "medium"}
		}
	}
	return out
}

func (c *Client) mapUserDetail(d user.Detail) (UserDetail, error) {
	out := UserDetail{User: c.mapUserEntity(d.User)}
	out.Profile = UserProfile{
		Webpage:                   strValue(d.Profile.Webpage),
		Gender:                    d.Profile.Gender,
		BirthDay:                  d.Profile.BirthDay,
		BirthYear:                 d.Profile.BirthYear,
		Region:                    d.Profile.Region,
		CountryCode:               d.Profile.CountryCode,
		Job:                       d.Profile.Job,
		TotalFollowUsers:          d.Profile.TotalFollowUsers,
		TotalMyPixivUsers:         d.Profile.TotalMyPixivUsers,
		TotalIllusts:              d.Profile.TotalIllusts,
		TotalManga:                d.Profile.TotalManga,
		TotalNovels:               d.Profile.TotalNovels,
		TotalIllustBookmarks:      d.Profile.TotalIllustBookmarksPublic,
		TotalIllustSeries:         d.Profile.TotalIllustSeries,
		TotalNovelSeries:          d.Profile.TotalNovelSeries,
		BackgroundImageURL:        strValue(d.Profile.BackgroundImageURL),
		TwitterAccount:            d.Profile.TwitterAccount,
		TwitterURL:                strValue(d.Profile.TwitterURL),
		PawooURL:                  strValue(d.Profile.PawooURL),
		IsPremium:                 d.Profile.IsPremium,
		IsUsingCustomProfileImage: d.Profile.IsUsingCustomProfileImage,
	}
	out.ProfilePublicity = UserProfilePublicity{
		Gender:    d.ProfilePublicity.Gender,
		Region:    d.ProfilePublicity.Region,
		BirthDay:  d.ProfilePublicity.BirthDay,
		BirthYear: d.ProfilePublicity.BirthYear,
		Job:       d.ProfilePublicity.Job,
		Pawoo:     d.ProfilePublicity.Pawoo,
	}
	out.Workspace = UserWorkspace{
		PC:                d.Workspace.PC,
		Monitor:           d.Workspace.Monitor,
		Tool:              d.Workspace.Tool,
		Scanner:           d.Workspace.Scanner,
		Tablet:            d.Workspace.Tablet,
		Mouse:             d.Workspace.Mouse,
		Printer:           d.Workspace.Printer,
		Desktop:           d.Workspace.Desktop,
		Music:             d.Workspace.Music,
		Desk:              d.Workspace.Desk,
		Chair:             d.Workspace.Chair,
		Comment:           d.Workspace.Comment,
		WorkspaceImageURL: strValue(d.Workspace.WorkspaceImageURL),
	}
	return out, nil
}

func (c *Client) mapUserPreview(p user.Preview) UserPreview {
	out := UserPreview{User: c.mapUserEntity(p.User), Illusts: make([]Artwork, 0, len(p.Illusts)), Novels: make([]Novel, 0, len(p.Novels))}
	for _, m := range p.Illusts {
		if a, err := c.mapArtworkEntity(m); err == nil {
			out.Illusts = append(out.Illusts, a)
		}
	}
	for _, m := range p.Novels {
		if n, err := c.mapNovelEntity(m); err == nil {
			out.Novels = append(out.Novels, n)
		}
	}
	return out
}

// SearchUsers searches users.
func (c *Client) SearchUsers(ctx context.Context, request SearchUsersRequest) (sdk.Page[UserPreview], error) {
	if err := validateSearchWord("SearchUsers", request.Word); err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query := url.Values{"word": {request.Word}}
	offset, err := c.continuationPositiveOffset("SearchUsers", query, request.Cursor)
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
	offset, err := c.continuationPositiveOffset("RelatedUsers", query, request.Cursor)
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
	restrict, err := normalizeFollowingRestrict("UserFollowing", request.Restrict)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(restrict)}}
	offset, err := c.continuationPositiveOffset("UserFollowing", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userFollowing.List(ctx, userfollowing.Request{UserID: request.UserID, Restrict: string(restrict), Offset: offset})
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
	restrict, err := normalizeFollowingRestrict("UserFollowers", request.Restrict)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(restrict)}}
	offset, err := c.continuationPositiveOffset("UserFollowers", query, request.Cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	list, err := c.userFollowers.List(ctx, userfollowers.Request{UserID: request.UserID, Restrict: string(restrict), Offset: offset})
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
	offset, err := c.continuationPositiveOffset("UserBlockedUsers", query, request.Cursor)
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
	offset, err := c.continuationPositiveOffset("MyPixivUsers", query, request.Cursor)
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
		items = append(items, UserPreview{User: c.mapUserEntity(p.User)})
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	return sdk.Page[UserPreview]{Items: items, Next: next}, nil
}

// SearchUsersRequest searches users.
type SearchUsersRequest struct {
	Word   string
	Cursor sdk.Cursor
}

// UserRequest selects one user by their stable ID.
type UserRequest struct {
	UserID int64
}

// RecommendedUsersRequest lists recommended users.
type RecommendedUsersRequest struct {
	Cursor sdk.Cursor
}

// RelatedUsersRequest lists users related to one user.
type RelatedUsersRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// UserFollowingRequest lists the users one user follows.
type UserFollowingRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// UserFollowersRequest lists the users following one user.
type UserFollowersRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// UserBlockedUsersRequest lists the users one user has blocked.
type UserBlockedUsersRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// MyPixivUsersRequest lists the current user's MyPixiv feed users.
type MyPixivUsersRequest struct {
	Cursor sdk.Cursor
}

// CurrentUserRequest reads the current authenticated user's detail.
type CurrentUserRequest struct{}
