package pixiv

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/webapi"
)

type searchIllustQuery struct {
	Word     string       `json:"word"`
	Target   SearchTarget `json:"target"`
	Sort     SortMode     `json:"sort"`
	Duration string       `json:"duration"`
}
type rankingQuery struct {
	Mode RankingMode `json:"mode"`
	Date string      `json:"date"`
}
type restrictQuery struct {
	Restrict Restrict `json:"restrict"`
}
type searchUserQuery struct {
	Word string `json:"word"`
}
type userArtworksQuery struct {
	UserID int64      `json:"user_id"`
	Type   IllustType `json:"type"`
}
type userBookmarksQuery struct {
	UserID   int64    `json:"user_id"`
	Restrict Restrict `json:"restrict"`
	Tag      string   `json:"tag"`
}
type userFollowingQuery struct {
	UserID   int64    `json:"user_id"`
	Restrict Restrict `json:"restrict"`
}

// SearchIllust 返回一个作品搜索上游批次。
func (c *Client) SearchIllust(ctx context.Context, request SearchIllustRequest) (*IllustListResult, error) {
	query := searchIllustQuery{strings.TrimSpace(request.Word), request.Target, request.Sort, strings.TrimSpace(request.Duration)}
	if query.Word == "" {
		return nil, invalidArgument(OperationSearchIllust, 0, errors.New("word is required"))
	}
	if query.Target == "" {
		query.Target = SearchTargetPartialMatchForTags
	}
	if query.Sort == "" {
		query.Sort = SortModeDateDesc
	}
	if !validSearchTarget(query.Target) || !validSortMode(query.Sort) || !validDuration(query.Duration) {
		return nil, invalidArgument(OperationSearchIllust, 0, errors.New("search enum is invalid"))
	}
	digest := queryDigest(OperationSearchIllust, query)
	offset, err := cursorOffset(request.Cursor, OperationSearchIllust, digest, 0)
	if err != nil {
		return nil, err
	}
	route, err := c.selectRoute(OperationSearchIllust, 0, 0)
	if err != nil {
		return nil, err
	}
	if route == routeWeb {
		list, err := c.web.SearchIllust(ctx, query.Word, string(query.Target), string(query.Sort), query.Duration, offset)
		if err != nil {
			return nil, mapWebListError(err, OperationSearchIllust, 0)
		}
		return publicIllustList(list, OperationSearchIllust, digest, "offset"), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationSearchIllust, 0, 0)
	}
	list, err := c.app.SearchIllust(ctx, query.Word, string(query.Target), string(query.Sort), query.Duration, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationSearchIllust, 0)
	}
	return publicIllustList(list, OperationSearchIllust, digest, "offset"), nil
}

// IllustRanking 返回一个排行榜上游批次。
func (c *Client) IllustRanking(ctx context.Context, request IllustRankingRequest) (*IllustListResult, error) {
	query := rankingQuery{request.Mode, request.Date}
	if query.Mode == "" {
		query.Mode = RankingModeDay
	}
	if !validRankingMode(query.Mode) {
		return nil, invalidArgument(OperationIllustRanking, 0, errors.New("ranking mode is invalid"))
	}
	if !validRankingDate(query.Date) {
		return nil, invalidArgument(OperationIllustRanking, 0, errors.New("ranking date must use YYYY-MM-DD"))
	}
	digest := queryDigest(OperationIllustRanking, query)
	offset, err := cursorOffset(request.Cursor, OperationIllustRanking, digest, 0)
	if err != nil {
		return nil, err
	}
	route, err := c.selectRoute(OperationIllustRanking, 0, 0)
	if err != nil {
		return nil, err
	}
	if route == routeWeb {
		list, err := c.web.IllustRanking(ctx, string(query.Mode), query.Date, offset)
		if err != nil {
			return nil, mapWebListError(err, OperationIllustRanking, 0)
		}
		return publicIllustList(list, OperationIllustRanking, digest, "offset"), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationIllustRanking, 0, 0)
	}
	list, err := c.app.IllustRanking(ctx, string(query.Mode), query.Date, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationIllustRanking, 0)
	}
	return publicIllustList(list, OperationIllustRanking, digest, "offset"), nil
}

// IllustRecommended 返回一个认证推荐作品批次。
func (c *Client) IllustRecommended(ctx context.Context, request IllustRecommendedRequest) (*IllustListResult, error) {
	digest := queryDigest(OperationIllustRecommended, struct{}{})
	offset, err := cursorOffset(request.Cursor, OperationIllustRecommended, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationIllustRecommended, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.IllustRecommended(ctx, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationIllustRecommended, 0)
	}
	return publicIllustList(list, OperationIllustRecommended, digest, "offset"), nil
}

// FollowingIllusts 返回当前认证账号所关注用户的一个作品批次。
func (c *Client) FollowingIllusts(ctx context.Context, request FollowingIllustsRequest) (*IllustListResult, error) {
	query := restrictQuery{request.Restrict}
	if query.Restrict == "" {
		query.Restrict = RestrictPublic
	}
	if !validRestrict(query.Restrict) {
		return nil, invalidArgument(OperationFollowingIllusts, 0, errors.New("restrict is invalid"))
	}
	digest := queryDigest(OperationFollowingIllusts, query)
	offset, err := cursorOffset(request.Cursor, OperationFollowingIllusts, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationFollowingIllusts, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.IllustFollow(ctx, string(query.Restrict), offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationFollowingIllusts, 0)
	}
	return publicIllustList(list, OperationFollowingIllusts, digest, "offset"), nil
}

// SearchUser 返回一个用户搜索上游批次。
func (c *Client) SearchUser(ctx context.Context, request SearchUserRequest) (*UserListResult, error) {
	query := searchUserQuery{strings.TrimSpace(request.Word)}
	if query.Word == "" {
		return nil, invalidArgument(OperationSearchUser, 0, errors.New("word is required"))
	}
	digest := queryDigest(OperationSearchUser, query)
	offset, err := cursorOffset(request.Cursor, OperationSearchUser, digest, 0)
	if err != nil {
		return nil, err
	}
	route, err := c.selectRoute(OperationSearchUser, 0, 0)
	if err != nil {
		return nil, err
	}
	if route == routeWeb {
		list, err := c.web.SearchUser(ctx, query.Word, offset)
		if err != nil {
			return nil, mapWebListError(err, OperationSearchUser, 0)
		}
		return publicUserList(list, OperationSearchUser, digest), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationSearchUser, 0, 0)
	}
	list, err := c.app.SearchUser(ctx, query.Word, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationSearchUser, 0)
	}
	return publicUserList(list, OperationSearchUser, digest), nil
}

// UserDetail 返回指定用户的稳定摘要。
func (c *Client) UserDetail(ctx context.Context, request UserDetailRequest) (*UserDetailResult, error) {
	if request.UserID <= 0 {
		return nil, invalidArgument(OperationUserDetail, request.UserID, errors.New("user id must be positive"))
	}
	if err := c.requireRoute(OperationUserDetail, routeApp, 0, request.UserID); err != nil {
		return nil, err
	}
	user, err := c.app.UserDetail(ctx, request.UserID)
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserDetail, request.UserID)
	}
	return &UserDetailResult{User: mapUser(*user)}, nil
}

// UserArtworks 返回指定用户的一个作品批次。
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (*IllustListResult, error) {
	if request.UserID <= 0 {
		return nil, invalidArgument(OperationUserArtworks, request.UserID, errors.New("user id must be positive"))
	}
	query := userArtworksQuery{request.UserID, request.Type}
	if query.Type == "" {
		query.Type = IllustTypeIllust
	}
	if !validIllustType(query.Type) {
		return nil, invalidArgument(OperationUserArtworks, request.UserID, errors.New("illust type is invalid"))
	}
	digest := queryDigest(OperationUserArtworks, query)
	offset, err := cursorOffset(request.Cursor, OperationUserArtworks, digest, request.UserID)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationUserArtworks, routeApp, 0, request.UserID); err != nil {
		return nil, err
	}
	list, err := c.app.UserArtworks(ctx, request.UserID, string(query.Type), offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserArtworks, request.UserID)
	}
	return publicIllustList(list, OperationUserArtworks, digest, "offset"), nil
}

// UserBookmarks 返回指定用户的一个收藏作品批次。
func (c *Client) UserBookmarks(ctx context.Context, request UserBookmarksRequest) (*IllustListResult, error) {
	if request.UserID <= 0 {
		return nil, invalidArgument(OperationUserBookmarks, request.UserID, errors.New("user id must be positive"))
	}
	query := userBookmarksQuery{request.UserID, request.Restrict, request.Tag}
	if query.Restrict == "" {
		query.Restrict = RestrictPublic
	}
	if !validRestrict(query.Restrict) {
		return nil, invalidArgument(OperationUserBookmarks, request.UserID, errors.New("restrict is invalid"))
	}
	digest := queryDigest(OperationUserBookmarks, query)
	maxID, err := cursorValue(request.Cursor, OperationUserBookmarks, digest, "max_bookmark_id", request.UserID)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationUserBookmarks, routeApp, 0, request.UserID); err != nil {
		return nil, err
	}
	list, err := c.app.UserBookmarks(ctx, request.UserID, string(query.Restrict), request.Tag, maxID)
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserBookmarks, request.UserID)
	}
	return publicIllustList(list, OperationUserBookmarks, digest, "max_bookmark_id"), nil
}

// UserFollowing 返回指定用户关注的一个用户批次。
func (c *Client) UserFollowing(ctx context.Context, request UserFollowingRequest) (*UserListResult, error) {
	if request.UserID <= 0 {
		return nil, invalidArgument(OperationUserFollowing, request.UserID, errors.New("user id must be positive"))
	}
	query := userFollowingQuery{request.UserID, request.Restrict}
	if query.Restrict == "" {
		query.Restrict = RestrictPublic
	}
	if !validRestrict(query.Restrict) {
		return nil, invalidArgument(OperationUserFollowing, request.UserID, errors.New("restrict is invalid"))
	}
	digest := queryDigest(OperationUserFollowing, query)
	offset, err := cursorOffset(request.Cursor, OperationUserFollowing, digest, request.UserID)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationUserFollowing, routeApp, 0, request.UserID); err != nil {
		return nil, err
	}
	list, err := c.app.UserFollowing(ctx, request.UserID, string(query.Restrict), offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserFollowing, request.UserID)
	}
	return publicUserList(list, OperationUserFollowing, digest), nil
}

func cursorOffset(cursor Cursor, operation Operation, digest string, userID int64) (int, error) {
	value, err := cursorValue(cursor, operation, digest, "offset", userID)
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, invalidArgument(operation, userID, errors.New("cursor offset is invalid"))
	}
	return int(value), nil
}

func cursorValue(cursor Cursor, operation Operation, digest, kind string, userID int64) (int64, error) {
	value, err := decodeCursor(cursor, operation, digest, kind)
	if err != nil {
		return 0, invalidArgument(operation, userID, err)
	}
	return value, nil
}

func publicIllustList(list *model.IllustList, operation Operation, digest, kind string) *IllustListResult {
	result := &IllustListResult{Illusts: make([]Illust, len(list.Illusts))}
	for index, item := range list.Illusts {
		result.Illusts[index] = mapIllust(item)
	}
	if list.ContinuationExists {
		value := int64(list.NextOffset)
		if kind == "max_bookmark_id" {
			value = list.NextMaxBookmarkID
		}
		result.NextCursor = encodeCursor(operation, digest, kind, value)
	}
	return result
}

func publicUserList(list *model.UserPreviewList, operation Operation, digest string) *UserListResult {
	result := &UserListResult{UserPreviews: make([]UserPreview, len(list.UserPreviews))}
	for index, item := range list.UserPreviews {
		result.UserPreviews[index] = UserPreview{User: mapUser(item.User)}
	}
	if list.ContinuationExists {
		result.NextCursor = encodeCursor(operation, digest, "offset", int64(list.NextOffset))
	}
	return result
}

func mapUser(user model.User) User {
	return User{ID: user.ID, Name: user.Name, Account: user.Account, Comment: user.Comment, IsFollowed: user.IsFollowed}
}

func invalidArgument(operation Operation, userID int64, cause error) error {
	return newUserError(CodeInvalidArgument, operation, "", false, 0, userID, cause)
}
func mapAppOperationError(err error, operation Operation, userID int64) error {
	mapped := mapAppError(err, operation, 0)
	if typed, ok := mapped.(*Error); ok {
		typed.UserID = userID
	}
	return mapped
}

func mapWebListError(err error, operation Operation, userID int64) error {
	if errors.Is(err, webapi.ErrUnrepresentablePagination) {
		return invalidArgument(operation, userID, errors.New("cursor offset cannot be represented by web pagination"))
	}
	mapped := mapWebError(err, operation, 0)
	if typed, ok := mapped.(*Error); ok {
		typed.UserID = userID
	}
	return mapped
}

func validSearchTarget(value SearchTarget) bool {
	return value == SearchTargetPartialMatchForTags || value == SearchTargetExactMatchForTags || value == SearchTargetTitleAndCaption
}

func validSortMode(value SortMode) bool { return value == SortModeDateDesc || value == SortModeDateAsc }

func validRankingMode(value RankingMode) bool {
	return value == RankingModeDay || value == RankingModeDayMale || value == RankingModeDayFemale ||
		value == RankingModeWeek || value == RankingModeWeekOriginal || value == RankingModeWeekRookie ||
		value == RankingModeMonth
}

func validRankingDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validRestrict(value Restrict) bool { return value == RestrictPublic || value == RestrictPrivate }

func validIllustType(value IllustType) bool {
	return value == IllustTypeIllust || value == IllustTypeManga || value == IllustTypeUgoira
}

func validDuration(value string) bool {
	return value == "" || value == "within_last_day" || value == "within_last_week" || value == "within_last_month"
}
