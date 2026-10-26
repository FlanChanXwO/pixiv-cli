// user 领域：用户详情和各类用户列表的读取。
// 请求类型、响应结构与映射、continuation Spec 都在本文件；共用 user 映射在 response.go。

package pixiv

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type userDetailWire struct {
	User             pixivapi.RequiredObject[userWire]             `json:"user"`
	Profile          pixivapi.RequiredObject[profileWire]          `json:"profile"`
	ProfilePublicity pixivapi.RequiredObject[profilePublicityWire] `json:"profile_publicity"`
	Workspace        pixivapi.RequiredObject[workspaceWire]        `json:"workspace"`
}

type profileWire struct {
	Webpage                    *string `json:"webpage"`
	Gender                     string  `json:"gender"`
	Birth                      string  `json:"birth"`
	BirthDay                   string  `json:"birth_day"`
	BirthYear                  int     `json:"birth_year"`
	Region                     string  `json:"region"`
	AddressID                  int64   `json:"address_id"`
	CountryCode                string  `json:"country_code"`
	Job                        string  `json:"job"`
	JobID                      int64   `json:"job_id"`
	TotalFollowUsers           int     `json:"total_follow_users"`
	TotalMyPixivUsers          int     `json:"total_mypixiv_users"`
	TotalIllusts               int     `json:"total_illusts"`
	TotalManga                 int     `json:"total_manga"`
	TotalNovels                int     `json:"total_novels"`
	TotalIllustBookmarksPublic int     `json:"total_illust_bookmarks_public"`
	TotalIllustSeries          int     `json:"total_illust_series"`
	TotalNovelSeries           int     `json:"total_novel_series"`
	BackgroundImageURL         *string `json:"background_image_url"`
	TwitterAccount             string  `json:"twitter_account"`
	TwitterURL                 *string `json:"twitter_url"`
	PawooURL                   *string `json:"pawoo_url"`
	IsPremium                  bool    `json:"is_premium"`
	IsUsingCustomProfileImage  bool    `json:"is_using_custom_profile_image"`
}

type profilePublicityWire struct {
	Gender    profileVisibility `json:"gender"`
	Region    profileVisibility `json:"region"`
	BirthDay  profileVisibility `json:"birth_day"`
	BirthYear profileVisibility `json:"birth_year"`
	Job       profileVisibility `json:"job"`
	Pawoo     profileVisibility `json:"pawoo"`
}

// profileVisibility 接受 boolean 或 "public"/"private"，并记录缺失与非法的值。
type profileVisibility struct {
	Value   bool
	Present bool
	Valid   bool
}

func (v *profileVisibility) UnmarshalJSON(data []byte) error {
	*v = profileVisibility{Present: true}
	switch string(bytes.TrimSpace(data)) {
	case "true":
		v.Value, v.Valid = true, true
		return nil
	case "false":
		v.Valid = true
		return nil
	}
	var visibility string
	if err := json.Unmarshal(data, &visibility); err != nil {
		return nil
	}
	switch visibility {
	case "public":
		v.Value, v.Valid = true, true
	case "private":
		v.Valid = true
	}
	return nil
}

func (d profilePublicityWire) valid() bool {
	for _, value := range []profileVisibility{d.Gender, d.Region, d.BirthDay, d.BirthYear, d.Job, d.Pawoo} {
		if value.Present && !value.Valid {
			return false
		}
	}
	return true
}

type workspaceWire struct {
	PC                string  `json:"pc"`
	Monitor           string  `json:"monitor"`
	Tool              string  `json:"tool"`
	Scanner           string  `json:"scanner"`
	Tablet            string  `json:"tablet"`
	Mouse             string  `json:"mouse"`
	Printer           string  `json:"printer"`
	Desktop           string  `json:"desktop"`
	Music             string  `json:"music"`
	Desk              string  `json:"desk"`
	Chair             string  `json:"chair"`
	Comment           string  `json:"comment"`
	WorkspaceImageURL *string `json:"workspace_image_url"`
}

// userPreviewsWire 是用户搜索与用户列表共用的响应。
type userPreviewsWire struct {
	UserPreviews pixivapi.RequiredList[userPreviewWire] `json:"user_previews"`
	NextURL      *string                                `json:"next_url"`
}

type userPreviewWire struct {
	User    userWire      `json:"user"`
	Illusts []artworkWire `json:"illusts"`
	Novels  []novelWire   `json:"novels"`
}

// blockedPreviewWire 额外接受 /v2/user/list 历史上直接返回的 user 对象。
type blockedPreviewWire struct {
	userPreviewWire
}

func (w *blockedPreviewWire) UnmarshalJSON(data []byte) error {
	var envelope struct {
		User    *userWire     `json:"user"`
		Illusts []artworkWire `json:"illusts"`
		Novels  []novelWire   `json:"novels"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if envelope.User != nil {
		w.userPreviewWire = userPreviewWire{User: *envelope.User, Illusts: envelope.Illusts, Novels: envelope.Novels}
		return nil
	}
	// 历史 envelope 直接给出 user 对象，没有 "user" 包装。
	return json.Unmarshal(data, &w.User)
}

// blockedUsersWire 是 /v2/user/list 的响应；两种历史 envelope 都按必需列表校验。
type blockedUsersWire struct {
	UserPreviews pixivapi.RequiredList[blockedPreviewWire] `json:"user_previews"`
	Users        pixivapi.RequiredList[blockedPreviewWire] `json:"users"`
	NextURL      *string                                   `json:"next_url"`
}

func (w blockedUsersWire) list() ([]userPreviewWire, error) {
	// /v2/user/list 先返回 users；user_previews 只是历史兼容形态。
	list := w.Users
	if !list.Present {
		list = w.UserPreviews
	}
	if !list.Present || !list.Valid {
		return nil, pixivapi.MalformedResponse()
	}
	items := make([]userPreviewWire, len(list.Items))
	for index, value := range list.Items {
		items[index] = value.userPreviewWire
	}
	return items, nil
}

// SearchUsers searches users.
func (c *Client) SearchUsers(ctx context.Context, request SearchUsersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "SearchUsers")
	if err := validateSearchWord("SearchUsers", request.Word); err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	base := url.Values{pixivapi.ParamWord: {request.Word}}
	return c.userPreviewsPage(ctx, "SearchUsers", pixivapi.PathSearchUser, base, base, request.Cursor, userSearchContinuation, false)
}

// User returns one user's detail by their stable ID.
func (c *Client) User(ctx context.Context, request UserRequest) (UserDetail, error) {
	ctx = pixivapi.WithOperation(ctx, "User")
	if request.UserID <= 0 {
		return UserDetail{}, newError("User", sdk.InvalidArgument, "user ID must be positive")
	}
	detail, err := c.fetchUserDetail(ctx, "User", request.UserID, false)
	if err != nil {
		return UserDetail{}, err
	}
	return c.mapUserDetail(detail), nil
}

// CurrentUser returns the current authenticated user's detail. It requires a
// verified identity, so it fails with Unauthorized on a New client whose
// user ID is unknown.
func (c *Client) CurrentUser(ctx context.Context, request CurrentUserRequest) (UserDetail, error) {
	ctx = pixivapi.WithOperation(ctx, "CurrentUser")
	if c.userID <= 0 {
		return UserDetail{}, newError("CurrentUser", sdk.Unauthorized, "current user identity is unknown")
	}
	detail, err := c.fetchUserDetail(ctx, "CurrentUser", c.userID, true)
	if err != nil {
		return UserDetail{}, err
	}
	return c.mapUserDetail(detail), nil
}

func (c *Client) fetchUserDetail(ctx context.Context, operation string, userID int64, filter bool) (userDetailWire, error) {
	query := url.Values{pixivapi.ParamUserID: {itoa(userID)}}
	if filter {
		query.Set(pixivapi.ParamFilter, "for_android")
	}
	var raw userDetailWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserDetail, query, &raw); err != nil {
		return userDetailWire{}, classifyAppError(err, operation)
	}
	if !raw.User.Present || !raw.User.Valid || raw.User.Value.ID <= 0 ||
		!raw.Profile.Present || !raw.Profile.Valid ||
		!raw.ProfilePublicity.Present || !raw.ProfilePublicity.Valid || !raw.ProfilePublicity.Value.valid() ||
		!raw.Workspace.Present || !raw.Workspace.Valid {
		return userDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return raw, nil
}

func (c *Client) mapUserDetail(value userDetailWire) UserDetail {
	profile := value.Profile.Value
	publicity := value.ProfilePublicity.Value
	workspace := value.Workspace.Value
	return UserDetail{
		User: c.mapUser(value.User.Value),
		Profile: UserProfile{
			Webpage:                   strValue(profile.Webpage),
			Gender:                    profile.Gender,
			BirthDay:                  profile.BirthDay,
			BirthYear:                 profile.BirthYear,
			Region:                    profile.Region,
			CountryCode:               profile.CountryCode,
			Job:                       profile.Job,
			TotalFollowUsers:          profile.TotalFollowUsers,
			TotalMyPixivUsers:         profile.TotalMyPixivUsers,
			TotalIllusts:              profile.TotalIllusts,
			TotalManga:                profile.TotalManga,
			TotalNovels:               profile.TotalNovels,
			TotalIllustBookmarks:      profile.TotalIllustBookmarksPublic,
			TotalIllustSeries:         profile.TotalIllustSeries,
			TotalNovelSeries:          profile.TotalNovelSeries,
			BackgroundImageURL:        strValue(profile.BackgroundImageURL),
			TwitterAccount:            profile.TwitterAccount,
			TwitterURL:                strValue(profile.TwitterURL),
			PawooURL:                  strValue(profile.PawooURL),
			IsPremium:                 profile.IsPremium,
			IsUsingCustomProfileImage: profile.IsUsingCustomProfileImage,
		},
		ProfilePublicity: UserProfilePublicity{
			Gender:    publicity.Gender.Value,
			Region:    publicity.Region.Value,
			BirthDay:  publicity.BirthDay.Value,
			BirthYear: publicity.BirthYear.Value,
			Job:       publicity.Job.Value,
			Pawoo:     publicity.Pawoo.Value,
		},
		Workspace: UserWorkspace{
			PC:                workspace.PC,
			Monitor:           workspace.Monitor,
			Tool:              workspace.Tool,
			Scanner:           workspace.Scanner,
			Tablet:            workspace.Tablet,
			Mouse:             workspace.Mouse,
			Printer:           workspace.Printer,
			Desktop:           workspace.Desktop,
			Music:             workspace.Music,
			Desk:              workspace.Desk,
			Chair:             workspace.Chair,
			Comment:           workspace.Comment,
			WorkspaceImageURL: strValue(workspace.WorkspaceImageURL),
		},
	}
}

// RecommendedUsers lists recommended users with sample works.
func (c *Client) RecommendedUsers(ctx context.Context, request RecommendedUsersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "RecommendedUsers")
	base := url.Values{}
	return c.userPreviewsPage(ctx, "RecommendedUsers", pixivapi.PathUserRecommended, base, base, request.Cursor, userRecommendedContinuation, true)
}

// RelatedUsers lists users related to one user.
func (c *Client) RelatedUsers(ctx context.Context, request RelatedUsersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "RelatedUsers")
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("RelatedUsers", sdk.InvalidArgument, "user ID must be positive")
	}
	base := url.Values{pixivapi.ParamSeedUserID: {itoa(request.UserID)}}
	return c.userPreviewsPage(ctx, "RelatedUsers", pixivapi.PathUserRelated, base, base, request.Cursor, userRelatedContinuation, false)
}

// UserFollowing lists the users one user follows.
func (c *Client) UserFollowing(ctx context.Context, request UserFollowingRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "UserFollowing")
	return c.userRelationshipPage(ctx, "UserFollowing", pixivapi.PathUserFollowing, request.UserID, request.Restrict, request.Cursor, userFollowingContinuation)
}

// UserFollowers lists the users following one user.
func (c *Client) UserFollowers(ctx context.Context, request UserFollowersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "UserFollowers")
	return c.userRelationshipPage(ctx, "UserFollowers", pixivapi.PathUserFollower, request.UserID, request.Restrict, request.Cursor, userFollowerContinuation)
}

func (c *Client) userRelationshipPage(ctx context.Context, operation, path string, userID int64, restrict Restrict, cursor sdk.Cursor, spec continuationSpec) (sdk.Page[UserPreview], error) {
	if userID <= 0 {
		return sdk.Page[UserPreview]{}, newError(operation, sdk.InvalidArgument, "user ID must be positive")
	}
	value, err := normalizeFollowingRestrict(operation, restrict)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	base := url.Values{
		pixivapi.ParamUserID:   {itoa(userID)},
		pixivapi.ParamRestrict: {string(value)},
	}
	return c.userPreviewsPage(ctx, operation, path, base, base, cursor, spec, false)
}

// UserBlockedUsers lists the users one user has blocked.
func (c *Client) UserBlockedUsers(ctx context.Context, request UserBlockedUsersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "UserBlockedUsers")
	if request.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError("UserBlockedUsers", sdk.InvalidArgument, "user ID must be positive")
	}
	base := url.Values{pixivapi.ParamUserID: {itoa(request.UserID)}}
	query := url.Values{pixivapi.ParamUserID: {itoa(request.UserID)}, pixivapi.ParamFilter: {"for_android"}}
	return c.blockedUsersPage(ctx, "UserBlockedUsers", pixivapi.PathUserBlocked, base, query, request.Cursor)
}

// MyPixivUsers lists the current user's MyPixiv feed users. It requires a
// verified identity, so it fails with Unauthorized on a New client whose
// user ID is unknown.
func (c *Client) MyPixivUsers(ctx context.Context, request MyPixivUsersRequest) (sdk.Page[UserPreview], error) {
	ctx = pixivapi.WithOperation(ctx, "MyPixivUsers")
	if c.userID <= 0 {
		return sdk.Page[UserPreview]{}, newError("MyPixivUsers", sdk.Unauthorized, "current user identity is unknown")
	}
	base := url.Values{}
	query := url.Values{pixivapi.ParamUserID: {itoa(c.userID)}, pixivapi.ParamFilter: {"for_android"}}
	return c.userPreviewsPage(ctx, "MyPixivUsers", pixivapi.PathUserMyPixiv, base, query, request.Cursor, userMyPixivContinuation, false)
}

// userPreviewsPage 是所有 user_previews 列表端点的共同实现。base 绑定游标摘要，
// query 是实际发出的 query；两者只在端点原有的 filter 处理上不同。
func (c *Client) userPreviewsPage(ctx context.Context, operation, path string, base, query url.Values, cursor sdk.Cursor, spec continuationSpec, withSamples bool) (sdk.Page[UserPreview], error) {
	query = cloneValues(query)
	if spec.allowZero {
		offset, continuationExists, err := c.continuationOffsetExists(operation, base, cursor)
		if err != nil {
			return sdk.Page[UserPreview]{}, err
		}
		if continuationExists {
			// 推荐流用显式的 offset=0 表示第二页；省略它会回到第一页。
			query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
		}
	} else {
		offset, err := c.continuationPositiveOffset(operation, base, cursor)
		if err != nil {
			return sdk.Page[UserPreview]{}, err
		}
		if offset > 0 {
			query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
		}
	}

	var raw userPreviewsWire
	if err := c.app.GetJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, operation)
	}
	if !raw.UserPreviews.Present || !raw.UserPreviews.Valid {
		return sdk.Page[UserPreview]{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return c.userPreviewsResult(operation, base, raw.UserPreviews.Items, raw.NextURL, spec, withSamples)
}

// blockedUsersPage 解码 /v2/user/list，其余与 user_previews 列表共用映射与游标。
func (c *Client) blockedUsersPage(ctx context.Context, operation, path string, base, query url.Values, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
	offset, err := c.continuationPositiveOffset(operation, base, cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query = cloneValues(query)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}
	var raw blockedUsersWire
	if err := c.app.GetJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, operation)
	}
	list, err := raw.list()
	if err != nil {
		return sdk.Page[UserPreview]{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return c.userPreviewsResult(operation, base, list, raw.NextURL, userBlockedContinuation, false)
}

func (c *Client) userPreviewsResult(operation string, base url.Values, values []userPreviewWire, nextURL *string, spec continuationSpec, withSamples bool) (sdk.Page[UserPreview], error) {
	items, err := c.mapUserPreviews(operation, values, withSamples)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	next, hasNext, err := spec.next(nextURL)
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, operation)
	}
	nextCursor, err := c.buildCursor(operation, base, pixivapi.ParamOffset, int64(next), hasNext)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	return sdk.Page[UserPreview]{Items: items, Next: nextCursor}, nil
}

func (c *Client) mapUserPreviews(operation string, values []userPreviewWire, withSamples bool) ([]UserPreview, error) {
	items := make([]UserPreview, len(values))
	for index, value := range values {
		if value.User.ID <= 0 {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
		preview, err := c.mapUserPreview(operation, value, withSamples)
		if err != nil {
			return nil, err
		}
		items[index] = preview
	}
	return items, nil
}

// mapUserPreview 映射一条 user_previews 项；只有推荐端点携带供参考的作品与小说。
func (c *Client) mapUserPreview(operation string, value userPreviewWire, withSamples bool) (UserPreview, error) {
	preview := UserPreview{User: c.mapUser(value.User), Illusts: []Artwork{}, Novels: []Novel{}}
	if !withSamples {
		return preview, nil
	}
	preview.Illusts = make([]Artwork, 0, len(value.Illusts))
	for _, item := range value.Illusts {
		if item.ID <= 0 || item.User.ID <= 0 {
			return UserPreview{}, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
		// 推荐样例缺字段时保持既有契约：不交付该样例，也不让整页失败。
		if mapped, err := c.mapArtwork(item); err == nil {
			preview.Illusts = append(preview.Illusts, mapped)
		}
	}
	preview.Novels = make([]Novel, 0, len(value.Novels))
	for _, item := range value.Novels {
		if item.ID <= 0 || item.User.ID <= 0 {
			return UserPreview{}, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
		if mapped, err := c.mapNovel(item); err == nil {
			preview.Novels = append(preview.Novels, mapped)
		}
	}
	return preview, nil
}

var (
	userSearchContinuation = continuationSpec{
		path:  pixivapi.PathSearchUser,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamWord},
	}
	userRecommendedContinuation = continuationSpec{
		path:      pixivapi.PathUserRecommended,
		keys:      []string{pixivapi.ParamOffset},
		allowZero: true,
	}
	userRelatedContinuation = continuationSpec{
		path:  pixivapi.PathUserRelated,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamSeedUserID},
	}
	userFollowingContinuation = continuationSpec{
		path:  pixivapi.PathUserFollowing,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict},
	}
	userFollowerContinuation = continuationSpec{
		path:  pixivapi.PathUserFollower,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict},
	}
	userBlockedContinuation = continuationSpec{
		path:  pixivapi.PathUserBlocked,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamFilter},
	}
	userMyPixivContinuation = continuationSpec{
		path:  pixivapi.PathUserMyPixiv,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamFilter},
	}
)

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

func strValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
