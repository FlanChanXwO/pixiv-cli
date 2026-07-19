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
	Word     string              `json:"word"`
	Target   SearchTarget        `json:"target"`
	Sort     SortMode            `json:"sort"`
	Duration string              `json:"duration"`
	Filters  SearchIllustFilters `json:"filters"`
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

func (c *Client) SearchIllustOptions(ctx context.Context, request SearchIllustOptionsRequest) (result *SearchIllustOptionsResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationSearchIllustOptions, started, err, 0, 0) }()
	if scoped, snapshotErr := c.operationClient(ctx, OperationSearchIllustOptions); snapshotErr != nil {
		return nil, snapshotErr
	} else if scoped != c {
		return scoped.SearchIllustOptions(ctx, request)
	}
	word := strings.TrimSpace(request.Word)
	if word == "" {
		return nil, invalidArgument(OperationSearchIllustOptions, 0, errors.New("word is required"))
	}
	if !c.authenticated {
		return nil, localRouteError(CodeUnsupported, OperationSearchIllustOptions, 0, 0, errors.New("search options require authenticated App API access"))
	}
	if err := c.requireRoute(OperationSearchIllustOptions, routeApp, 0, 0); err != nil {
		return nil, err
	}
	options, err := c.app.SearchIllustOptions(ctx, word)
	if err != nil {
		return nil, mapAppOperationError(err, OperationSearchIllustOptions, 0)
	}
	tools := append([]string(nil), options.Tools...)
	if tools == nil {
		tools = []string{}
	}
	return &SearchIllustOptionsResult{Tools: tools}, nil
}

// SearchIllust 返回一个作品搜索上游批次。
func (c *Client) SearchIllust(ctx context.Context, request SearchIllustRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationSearchIllust, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationSearchIllust); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.SearchIllust(ctx, request)
	}
	query := searchIllustQuery{strings.TrimSpace(request.Word), request.Target, request.Sort, strings.TrimSpace(request.Duration), normalizeSearchIllustFilters(request.Filters)}
	if query.Word == "" {
		return nil, invalidArgument(OperationSearchIllust, 0, errors.New("word is required"))
	}
	if query.Target == "" {
		query.Target = SearchTargetPartialMatchForTags
	}
	if query.Sort == "" {
		query.Sort = SortModeDateDesc
	}
	if !validSearchTarget(query.Target) || !validSortMode(query.Sort) || !validDuration(query.Duration) || !validSearchIllustFilters(query.Filters) {
		return nil, invalidArgument(OperationSearchIllust, 0, errors.New("search enum is invalid"))
	}
	digest := queryDigest(OperationSearchIllust, query)
	offset, err := c.cursorOffset(request.Cursor, OperationSearchIllust, digest, 0)
	if err != nil {
		return nil, err
	}
	route, err := c.selectRoute(OperationSearchIllust, 0, 0)
	if err != nil {
		return nil, err
	}
	if route == routeWeb {
		if query.Filters.Rating == SearchRatingR18 || query.Filters.Rating == SearchRatingR18G || query.Filters.Rating == SearchRatingMature {
			return nil, localRouteError(CodeUnauthorized, OperationSearchIllust, 0, 0, errors.New("authenticated App API search is required for the requested rating"))
		}
		list, err := c.web.SearchIllust(ctx, query.Word, string(query.Target), string(query.Sort), query.Duration, offset, internalSearchIllustFilters(query.Filters))
		if err != nil {
			return nil, mapWebListError(err, OperationSearchIllust, 0)
		}
		filterSearchIllustBatch(list, query.Filters)
		return publicIllustList(list, OperationSearchIllust, digest, "offset", c.cursorSource), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationSearchIllust, 0, 0)
	}
	list, err := c.app.SearchIllust(ctx, query.Word, string(query.Target), string(query.Sort), query.Duration, offset, internalSearchIllustFilters(query.Filters))
	if err != nil {
		return nil, mapAppOperationError(err, OperationSearchIllust, 0)
	}
	filterSearchIllustBatch(list, query.Filters)
	return publicIllustList(list, OperationSearchIllust, digest, "offset", c.cursorSource), nil
}

func filterSearchIllustBatch(list *model.IllustList, filters SearchIllustFilters) {
	filtered := make([]model.Illust, 0, len(list.Illusts))
	for _, illust := range list.Illusts {
		if searchRatingAccepts(filters.Rating, illust.XRestrict) && searchAIModeAccepts(filters.AIMode, illust.AIType) {
			filtered = append(filtered, illust)
		}
	}
	list.Illusts = filtered
}

func searchRatingAccepts(rating SearchRating, xRestrict int) bool {
	switch rating {
	case SearchRatingSFW:
		return xRestrict == 0
	case SearchRatingR18:
		return xRestrict == 1
	case SearchRatingR18G:
		return xRestrict == 2
	case SearchRatingMature:
		return xRestrict == 0 || xRestrict == 1
	default:
		return true
	}
}

func searchAIModeAccepts(mode SearchAIMode, aiType int) bool {
	switch mode {
	case SearchAIModeExclude:
		return aiType != 2
	case SearchAIModeOnly:
		return aiType == 2
	default:
		return true
	}
}

func normalizeSearchIllustFilters(filters SearchIllustFilters) SearchIllustFilters {
	if filters.Rating == "" {
		filters.Rating = SearchRatingAll
	}
	if filters.ContentType == "" {
		filters.ContentType = SearchContentTypeAll
	}
	if filters.AIMode == "" {
		filters.AIMode = SearchAIModeAll
	}
	if filters.AspectRatio == "" {
		filters.AspectRatio = SearchAspectRatioAll
	}
	if filters.Resolution == "" {
		filters.Resolution = SearchResolutionAll
	}
	filters.Tool = strings.TrimSpace(filters.Tool)
	return filters
}

func internalSearchIllustFilters(filters SearchIllustFilters) model.SearchIllustFilters {
	return model.SearchIllustFilters{
		Rating: string(filters.Rating), ContentType: string(filters.ContentType), AIMode: string(filters.AIMode),
		AspectRatio: string(filters.AspectRatio), Resolution: string(filters.Resolution), Tool: filters.Tool,
	}
}

func validSearchIllustFilters(filters SearchIllustFilters) bool {
	return oneOf(string(filters.Rating), "all", "sfw", "r18", "r18g", "mature") &&
		oneOf(string(filters.ContentType), "all", "illust-and-ugoira", "illust", "manga", "ugoira") &&
		oneOf(string(filters.AIMode), "all", "exclude", "only") &&
		oneOf(string(filters.AspectRatio), "all", "landscape", "portrait", "square") &&
		oneOf(string(filters.Resolution), "all", "high", "medium", "low")
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

// IllustRanking 返回一个排行榜上游批次。
func (c *Client) IllustRanking(ctx context.Context, request IllustRankingRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationIllustRanking, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationIllustRanking); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.IllustRanking(ctx, request)
	}
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
	offset, err := c.cursorOffset(request.Cursor, OperationIllustRanking, digest, 0)
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
		return publicIllustList(list, OperationIllustRanking, digest, "offset", c.cursorSource), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationIllustRanking, 0, 0)
	}
	list, err := c.app.IllustRanking(ctx, string(query.Mode), query.Date, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationIllustRanking, 0)
	}
	return publicIllustList(list, OperationIllustRanking, digest, "offset", c.cursorSource), nil
}

// IllustRecommended 返回一个认证推荐作品批次。
func (c *Client) IllustRecommended(ctx context.Context, request IllustRecommendedRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationIllustRecommended, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationIllustRecommended); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.IllustRecommended(ctx, request)
	}
	digest := queryDigest(OperationIllustRecommended, struct{}{})
	offset, err := c.cursorOffset(request.Cursor, OperationIllustRecommended, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationIllustRecommended, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.IllustRecommended(ctx, offset, request.Cursor != "")
	if err != nil {
		return nil, mapAppOperationError(err, OperationIllustRecommended, 0)
	}
	return publicIllustList(list, OperationIllustRecommended, digest, "offset", c.cursorSource), nil
}

// MangaRecommended 返回一个认证漫画推荐批次；它与插画推荐使用相同 catalog，但游标互不兼容。
func (c *Client) MangaRecommended(ctx context.Context, request IllustRecommendedRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationMangaRecommended, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationMangaRecommended); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.MangaRecommended(ctx, request)
	}
	digest := queryDigest(OperationMangaRecommended, struct{}{})
	offset, err := c.cursorOffset(request.Cursor, OperationMangaRecommended, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationMangaRecommended, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.MangaRecommended(ctx, offset, request.Cursor != "")
	if err != nil {
		return nil, mapAppOperationError(err, OperationMangaRecommended, 0)
	}
	return publicIllustList(list, OperationMangaRecommended, digest, "offset", c.cursorSource), nil
}

// NovelRecommended 返回一个认证小说推荐批次；其 opaque cursor 不能用于其他推荐种类。
func (c *Client) NovelRecommended(ctx context.Context, request NovelRecommendedRequest) (result *NovelListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationNovelRecommended, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationNovelRecommended); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.NovelRecommended(ctx, request)
	}
	digest := queryDigest(OperationNovelRecommended, struct{}{})
	offset, err := c.cursorOffset(request.Cursor, OperationNovelRecommended, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationNovelRecommended, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.NovelRecommended(ctx, offset, request.Cursor != "")
	if err != nil {
		return nil, mapAppOperationError(err, OperationNovelRecommended, 0)
	}
	return publicNovelList(list, OperationNovelRecommended, digest, c.cursorSource), nil
}

// UserRecommended 返回一个认证作者推荐批次及对应作品预览。
func (c *Client) UserRecommended(ctx context.Context, request UserRecommendedRequest) (result *UserRecommendedResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationUserRecommended, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationUserRecommended); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.UserRecommended(ctx, request)
	}
	digest := queryDigest(OperationUserRecommended, struct{}{})
	offset, err := c.cursorOffset(request.Cursor, OperationUserRecommended, digest, 0)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationUserRecommended, routeApp, 0, 0); err != nil {
		return nil, err
	}
	list, err := c.app.UserRecommended(ctx, offset, request.Cursor != "")
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserRecommended, 0)
	}
	return publicRecommendedUsers(list, OperationUserRecommended, digest, c.cursorSource), nil
}

// FollowingIllusts 返回当前认证账号所关注用户的一个作品批次。
func (c *Client) FollowingIllusts(ctx context.Context, request FollowingIllustsRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationFollowingIllusts, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationFollowingIllusts); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.FollowingIllusts(ctx, request)
	}
	query := restrictQuery{request.Restrict}
	if query.Restrict == "" {
		query.Restrict = RestrictPublic
	}
	if !validRestrict(query.Restrict) {
		return nil, invalidArgument(OperationFollowingIllusts, 0, errors.New("restrict is invalid"))
	}
	digest := queryDigest(OperationFollowingIllusts, query)
	offset, err := c.cursorOffset(request.Cursor, OperationFollowingIllusts, digest, 0)
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
	return publicIllustList(list, OperationFollowingIllusts, digest, "offset", c.cursorSource), nil
}

// SearchUser 返回一个用户搜索上游批次。
func (c *Client) SearchUser(ctx context.Context, request SearchUserRequest) (result *UserListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationSearchUser, started, err, 0, 0) }()
	if scoped, err := c.operationClient(ctx, OperationSearchUser); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.SearchUser(ctx, request)
	}
	query := searchUserQuery{strings.TrimSpace(request.Word)}
	if query.Word == "" {
		return nil, invalidArgument(OperationSearchUser, 0, errors.New("word is required"))
	}
	digest := queryDigest(OperationSearchUser, query)
	offset, err := c.cursorOffset(request.Cursor, OperationSearchUser, digest, 0)
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
		return publicUserList(list, OperationSearchUser, digest, c.cursorSource), nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationSearchUser, 0, 0)
	}
	list, err := c.app.SearchUser(ctx, query.Word, offset)
	if err != nil {
		return nil, mapAppOperationError(err, OperationSearchUser, 0)
	}
	return publicUserList(list, OperationSearchUser, digest, c.cursorSource), nil
}

// UserDetail 返回指定用户的稳定完整详情。
func (c *Client) UserDetail(ctx context.Context, request UserDetailRequest) (result *UserDetailResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationUserDetail, started, err, 0, request.UserID) }()
	if scoped, err := c.operationClient(ctx, OperationUserDetail); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.UserDetail(ctx, request)
	}
	if request.UserID <= 0 {
		return nil, invalidArgument(OperationUserDetail, request.UserID, errors.New("user id must be positive"))
	}
	if err := c.requireRoute(OperationUserDetail, routeApp, 0, request.UserID); err != nil {
		return nil, err
	}
	detail, err := c.app.UserDetail(ctx, request.UserID)
	if err != nil {
		return nil, mapAppOperationError(err, OperationUserDetail, request.UserID)
	}
	return mapUserDetail(*detail), nil
}

// UserArtworks 返回指定用户的一个作品批次。
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationUserArtworks, started, err, 0, request.UserID) }()
	if scoped, err := c.operationClient(ctx, OperationUserArtworks); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.UserArtworks(ctx, request)
	}
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
	offset, err := c.cursorOffset(request.Cursor, OperationUserArtworks, digest, request.UserID)
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
	return publicIllustList(list, OperationUserArtworks, digest, "offset", c.cursorSource), nil
}

// UserBookmarks 返回指定用户的一个收藏作品批次。
func (c *Client) UserBookmarks(ctx context.Context, request UserBookmarksRequest) (result *IllustListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationUserBookmarks, started, err, 0, request.UserID) }()
	if scoped, err := c.operationClient(ctx, OperationUserBookmarks); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.UserBookmarks(ctx, request)
	}
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
	maxID, err := c.cursorValue(request.Cursor, OperationUserBookmarks, digest, "max_bookmark_id", request.UserID)
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
	return publicIllustList(list, OperationUserBookmarks, digest, "max_bookmark_id", c.cursorSource), nil
}

// UserBookmarksCursor 将旧调用方持有的 max_bookmark_id 适配为绑定用户、可见性和
// tag 查询的公开 opaque cursor。它不请求上游，也不暴露 cursor 的编码细节；调用方
// 仍只能把返回值交回 UserBookmarks。
func (c *Client) UserBookmarksCursor(ctx context.Context, request UserBookmarksRequest, maxBookmarkID int64) (cursor Cursor, err error) {
	if scoped, err := c.operationClient(ctx, OperationUserBookmarks); err != nil {
		return "", err
	} else if scoped != c {
		return scoped.UserBookmarksCursor(ctx, request, maxBookmarkID)
	}
	if request.UserID <= 0 || maxBookmarkID <= 0 {
		return "", invalidArgument(OperationUserBookmarks, request.UserID, errors.New("user id and max bookmark id must be positive"))
	}
	query := userBookmarksQuery{request.UserID, request.Restrict, request.Tag}
	if query.Restrict == "" {
		query.Restrict = RestrictPublic
	}
	if !validRestrict(query.Restrict) {
		return "", invalidArgument(OperationUserBookmarks, request.UserID, errors.New("restrict is invalid"))
	}
	if request.Cursor != "" {
		return "", invalidArgument(OperationUserBookmarks, request.UserID, errors.New("legacy cursor cannot be combined with cursor"))
	}
	return encodeCursorForSource(OperationUserBookmarks, queryDigest(OperationUserBookmarks, query), "max_bookmark_id", maxBookmarkID, c.cursorSource), nil
}

// UserFollowing 返回指定用户关注的一个用户批次。
func (c *Client) UserFollowing(ctx context.Context, request UserFollowingRequest) (result *UserListResult, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationUserFollowing, started, err, 0, request.UserID) }()
	if scoped, err := c.operationClient(ctx, OperationUserFollowing); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.UserFollowing(ctx, request)
	}
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
	offset, err := c.cursorOffset(request.Cursor, OperationUserFollowing, digest, request.UserID)
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
	return publicUserList(list, OperationUserFollowing, digest, c.cursorSource), nil
}

func (c *Client) cursorOffset(cursor Cursor, operation Operation, digest string, userID int64) (int, error) {
	value, err := c.cursorValue(cursor, operation, digest, "offset", userID)
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, invalidArgument(operation, userID, errors.New("cursor offset is invalid"))
	}
	return int(value), nil
}

func (c *Client) cursorValue(cursor Cursor, operation Operation, digest, kind string, userID int64) (int64, error) {
	value, err := decodeCursorForSource(cursor, operation, digest, kind, c.cursorSource, c.cursorSource != "")
	if err != nil {
		return 0, invalidArgument(operation, userID, err)
	}
	return value, nil
}

func publicIllustList(list *model.IllustList, operation Operation, digest, kind, source string) *IllustListResult {
	result := &IllustListResult{Illusts: make([]Illust, len(list.Illusts))}
	for index, item := range list.Illusts {
		result.Illusts[index] = mapIllust(item)
	}
	if list.ContinuationExists {
		value := int64(list.NextOffset)
		if kind == "max_bookmark_id" {
			value = list.NextMaxBookmarkID
		}
		result.NextCursor = encodeCursorForSource(operation, digest, kind, value, source)
	}
	return result
}

func publicUserList(list *model.UserPreviewList, operation Operation, digest, source string) *UserListResult {
	result := &UserListResult{UserPreviews: make([]UserPreview, len(list.UserPreviews))}
	for index, item := range list.UserPreviews {
		result.UserPreviews[index] = UserPreview{User: mapUser(item.User)}
	}
	if list.ContinuationExists {
		result.NextCursor = encodeCursorForSource(operation, digest, "offset", int64(list.NextOffset), source)
	}
	return result
}

func publicNovelList(list *model.NovelList, operation Operation, digest, source string) *NovelListResult {
	result := &NovelListResult{Novels: make([]Novel, len(list.Novels))}
	for index, item := range list.Novels {
		result.Novels[index] = mapNovel(item)
	}
	if list.ContinuationExists {
		result.NextCursor = encodeCursorForSource(operation, digest, "offset", int64(list.NextOffset), source)
	}
	return result
}

func publicRecommendedUsers(list *model.RecommendedUserList, operation Operation, digest, source string) *UserRecommendedResult {
	result := &UserRecommendedResult{UserPreviews: make([]RecommendedUserPreview, len(list.UserPreviews))}
	for index, item := range list.UserPreviews {
		preview := RecommendedUserPreview{User: mapUser(item.User), Illusts: make([]Illust, len(item.Illusts)), Novels: make([]Novel, len(item.Novels))}
		for illustIndex, illust := range item.Illusts {
			preview.Illusts[illustIndex] = mapIllust(illust)
		}
		for novelIndex, novel := range item.Novels {
			preview.Novels[novelIndex] = mapNovel(novel)
		}
		result.UserPreviews[index] = preview
	}
	if list.ContinuationExists {
		result.NextCursor = encodeCursorForSource(operation, digest, "offset", int64(list.NextOffset), source)
	}
	return result
}

func mapUser(user model.User) User {
	return User{
		ID: user.ID, Name: user.Name, Account: user.Account, Comment: user.Comment, IsFollowed: user.IsFollowed,
		ProfileImageURLs: ProfileImageURLs{Medium: user.ProfileImageURLs.Medium},
	}
}

func mapUserDetail(detail model.UserDetail) *UserDetailResult {
	return &UserDetailResult{
		User: mapUser(detail.User),
		Profile: Profile{
			Webpage: detail.Profile.Webpage, Gender: detail.Profile.Gender, Birth: detail.Profile.Birth, BirthDay: detail.Profile.BirthDay,
			BirthYear: detail.Profile.BirthYear, Region: detail.Profile.Region, AddressID: detail.Profile.AddressID,
			CountryCode: detail.Profile.CountryCode, Job: detail.Profile.Job, JobID: detail.Profile.JobID,
			TotalFollowUsers: detail.Profile.TotalFollowUsers, TotalMyPixivUsers: detail.Profile.TotalMyPixivUsers,
			TotalIllusts: detail.Profile.TotalIllusts, TotalManga: detail.Profile.TotalManga, TotalNovels: detail.Profile.TotalNovels,
			TotalIllustBookmarksPublic: detail.Profile.TotalIllustBookmarksPublic, TotalIllustSeries: detail.Profile.TotalIllustSeries,
			TotalNovelSeries: detail.Profile.TotalNovelSeries, BackgroundImageURL: detail.Profile.BackgroundImageURL,
			TwitterAccount: detail.Profile.TwitterAccount, TwitterURL: detail.Profile.TwitterURL, PawooURL: detail.Profile.PawooURL,
			IsPremium: detail.Profile.IsPremium, IsUsingCustomProfileImage: detail.Profile.IsUsingCustomProfileImage,
		},
		ProfilePublicity: ProfilePublicity{
			Gender: detail.ProfilePublicity.Gender, Region: detail.ProfilePublicity.Region, BirthDay: detail.ProfilePublicity.BirthDay,
			BirthYear: detail.ProfilePublicity.BirthYear, Job: detail.ProfilePublicity.Job, Pawoo: detail.ProfilePublicity.Pawoo,
		},
		Workspace: Workspace{
			PC: detail.Workspace.PC, Monitor: detail.Workspace.Monitor, Tool: detail.Workspace.Tool, Scanner: detail.Workspace.Scanner,
			Tablet: detail.Workspace.Tablet, Mouse: detail.Workspace.Mouse, Printer: detail.Workspace.Printer, Desktop: detail.Workspace.Desktop,
			Music: detail.Workspace.Music, Desk: detail.Workspace.Desk, Chair: detail.Workspace.Chair, Comment: detail.Workspace.Comment,
			WorkspaceImageURL: detail.Workspace.WorkspaceImageURL,
		},
	}
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
