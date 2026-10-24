// novel 读取操作：详情、排行、推荐、系列、搜索、时间线与评论。
// 请求参数校验在 validation.go，游标编解码在 cursor.go，响应结构与映射在 wire.go。

package pixiv

import (
	"bytes"
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	novelentity "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	usernovelbookmarks "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
	usernovels "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novels"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchNovels searches novels. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchNovels(ctx context.Context, request SearchNovelsRequest) (sdk.Page[Novel], error) {
	if err := validateSearchWord("SearchNovels", request.Word); err != nil {
		return sdk.Page[Novel]{}, err
	}
	if request.Target == "" {
		request.Target = SearchTargetPartialMatchForTags
	}
	if request.Sort == "" {
		request.Sort = SortModeDateDesc
	}
	if err := validateSearchTarget("SearchNovels", request.Target); err != nil {
		return sdk.Page[Novel]{}, err
	}
	if err := validateSortMode("SearchNovels", request.Sort); err != nil {
		return sdk.Page[Novel]{}, err
	}
	if err := validateDuration("SearchNovels", request.Duration); err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{
		pixivapi.ParamWord:         {request.Word},
		pixivapi.ParamSearchTarget: {string(request.Target)},
		pixivapi.ParamSort:         {string(request.Sort)},
	}
	if request.Duration != "" {
		query.Set(pixivapi.ParamDuration, string(request.Duration))
	}
	offset, err := c.continuationPositiveOffset("SearchNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "SearchNovels", pixivapi.PathSearchNovel, query, offset, novelSearchContinuation, true)
}

// Novel returns one novel by its stable ID.
func (c *Client) Novel(ctx context.Context, request NovelRequest) (Novel, error) {
	if request.NovelID <= 0 {
		return Novel{}, newError("Novel", sdk.InvalidArgument, "novel ID must be positive")
	}
	detail, err := c.fetchNovelDetail(ctx, "Novel", request.NovelID)
	if err != nil {
		return Novel{}, err
	}
	return c.mapNovel(*detail.Novel)
}

func (c *Client) fetchNovelDetail(ctx context.Context, operation string, novelID int64) (novelDetailWire, error) {
	var raw novelDetailWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelDetail, url.Values{
		pixivapi.ParamNovelID: {itoa(novelID)},
	}, &raw); err != nil {
		return novelDetailWire{}, classifyAppError(err, operation)
	}
	if raw.Novel == nil || raw.Novel.ID <= 0 || raw.Novel.User.ID <= 0 {
		return novelDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, series := range []*novelSeriesRefWire{raw.SeriesNext, raw.SeriesPrev} {
		if series != nil && series.ID <= 0 {
			return novelDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	return raw, nil
}

// NovelSeries returns a novel series with its paged novels.
func (c *Client) NovelSeries(ctx context.Context, request NovelSeriesRequest) (NovelSeriesResult, error) {
	if request.SeriesID <= 0 {
		return NovelSeriesResult{}, newError("NovelSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	base := url.Values{pixivapi.ParamSeriesID: {itoa(request.SeriesID)}}
	_, lastOrder, err := c.continuationValue("NovelSeries", base, request.Cursor, novelSeriesContinuation)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	query := cloneValues(base)
	if lastOrder > 0 {
		query.Set(pixivapi.ParamLastOrder, itoa(lastOrder))
	}

	var raw novelSeriesWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelSeries, query, &raw); err != nil {
		return NovelSeriesResult{}, classifyAppError(err, "NovelSeries")
	}
	if raw.Detail == nil || raw.Detail.ID <= 0 || raw.Detail.User.ID <= 0 {
		return NovelSeriesResult{}, newError("NovelSeries", sdk.MalformedUpstreamResponse, "")
	}
	items, err := c.novelItems("NovelSeries", raw.Novels, false)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	nextLastOrder, hasNext, err := novelSeriesContinuation.value(raw.NextURL)
	if err != nil {
		return NovelSeriesResult{}, classifyAppError(err, "NovelSeries")
	}
	next, err := c.buildCursor("NovelSeries", base, pixivapi.ParamLastOrder, nextLastOrder, hasNext)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	return NovelSeriesResult{
		Series: NovelSeries{
			ID:          raw.Detail.ID,
			Title:       raw.Detail.Title,
			Caption:     raw.Detail.Caption,
			User:        c.mapUser(raw.Detail.User),
			IsConcluded: raw.Detail.IsConcluded,
		},
		Novels: sdk.Page[Novel]{Items: items, Next: next},
	}, nil
}

// NovelRanking lists the current novel ranking.
func (c *Client) NovelRanking(ctx context.Context, request NovelRankingRequest) (sdk.Page[Novel], error) {
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("NovelRanking", request.Mode); err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{
		pixivapi.ParamFilter: {"for_android"},
		pixivapi.ParamMode:   {string(request.Mode)},
	}
	offset, err := c.continuationPositiveOffset("NovelRanking", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "NovelRanking", pixivapi.PathNovelRanking, query, offset, novelRankingContinuation, false)
}

// NovelContent reports that structured novel content is unavailable through
// the App API. WebView content is outside the v1 SDK contract.
func (c *Client) NovelContent(ctx context.Context, request NovelContentRequest) (NovelContent, error) {
	if request.NovelID <= 0 {
		return NovelContent{}, newError("NovelContent", sdk.InvalidArgument, "novel ID must be positive")
	}
	return NovelContent{}, newError("NovelContent", sdk.ContentUnavailable, "novel content is unsupported by the v1 App API")
}

// NovelComments lists comments on one novel.
func (c *Client) NovelComments(ctx context.Context, request NovelCommentsRequest) (CommentPage, error) {
	if request.NovelID <= 0 {
		return CommentPage{}, newError("NovelComments", sdk.InvalidArgument, "novel ID must be positive")
	}
	base := url.Values{pixivapi.ParamNovelID: {itoa(request.NovelID)}}
	offset, err := c.continuationPositiveOffset("NovelComments", base, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw commentsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelComments, query, &raw); err != nil {
		return CommentPage{}, classifyAppError(err, "NovelComments")
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError("NovelComments", sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return CommentPage{}, newError("NovelComments", sdk.MalformedUpstreamResponse, "")
		}
	}
	var access *CommentAccessControl
	if raw.CommentAccessControl != nil {
		access = &CommentAccessControl{NumericValue: cloneValue(raw.CommentAccessControl)}
	} else if raw.AccessControl != nil {
		access = &CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	next, hasNext, err := novelCommentsContinuation.offset(raw.NextURL)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "NovelComments")
	}
	return c.commentPage("NovelComments", base, raw.Comments.Items, next, hasNext, raw.TotalComments, access)
}

// RecommendedNovels lists recommended novels. RecommendedNovels
// lists recommended novels. 续页以上游 next_url 的多参数
// 集整体回放（offset、already_recommended、bookmark 游标）。
func (c *Client) RecommendedNovels(ctx context.Context, request RecommendedNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{}
	params, err := c.continuationParams("RecommendedNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}

	var raw novelListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelRecommended, params, &raw); err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "RecommendedNovels")
	}
	items, err := c.novelItems("RecommendedNovels", raw.Novels, false)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	if raw.NextURL != nil && *raw.NextURL == "" {
		return sdk.Page[Novel]{}, newError("RecommendedNovels", sdk.MalformedUpstreamResponse, "")
	}
	nextParams, hasNext, err := novelRecommendedContinuation.params(raw.NextURL)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "RecommendedNovels")
	}
	var next sdk.Cursor
	if hasNext {
		next, err = c.buildContinuationCursor("RecommendedNovels", query, continuationEnvelope{Params: nextParams})
		if err != nil {
			return sdk.Page[Novel]{}, err
		}
	}
	return sdk.Page[Novel]{Items: items, Next: next}, nil
}

// FollowingNovels lists novels by followed users.
func (c *Client) FollowingNovels(ctx context.Context, request FollowingNovelsRequest) (sdk.Page[Novel], error) {
	restrict, err := normalizeFollowingRestrict("FollowingNovels", request.Restrict)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{pixivapi.ParamRestrict: {string(restrict)}}
	offset, err := c.continuationPositiveOffset("FollowingNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "FollowingNovels", pixivapi.PathNovelFollow, query, offset, novelFollowingContinuation, false)
}

// LatestNovels lists the newest novels.
func (c *Client) LatestNovels(ctx context.Context, request LatestNovelsRequest) (sdk.Page[Novel], error) {
	base := url.Values{pixivapi.ParamFilter: {"for_android"}}
	_, maxNovelID, err := c.continuationValue("LatestNovels", base, request.Cursor, novelLatestContinuation)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := cloneValues(base)
	if maxNovelID > 0 {
		query.Set(pixivapi.ParamMaxNovelID, itoa(maxNovelID))
	}

	var raw novelListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathNovelNew, query, &raw); err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "LatestNovels")
	}
	items, err := c.novelItems("LatestNovels", raw.Novels, false)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	next, hasNext, err := novelLatestContinuation.value(raw.NextURL)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "LatestNovels")
	}
	cursor, err := c.buildCursor("LatestNovels", base, pixivapi.ParamMaxNovelID, next, hasNext)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: cursor}, nil
}

// UserNovels lists one user's novels.
func (c *Client) UserNovels(ctx context.Context, request UserNovelsRequest) (sdk.Page[Novel], error) {
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovels", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}}
	offset, err := c.continuationPositiveOffset("UserNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.userNovels.List(ctx, usernovels.Request{UserID: request.UserID, Offset: offset})
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "UserNovels")
	}
	return c.novelEntityPage("UserNovels", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// UserNovelBookmarks lists one user's bookmarked novels.
func (c *Client) UserNovelBookmarks(ctx context.Context, request UserNovelBookmarksRequest) (sdk.Page[Novel], error) {
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovelBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserNovelBookmarks", request.Restrict); err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	maxID, err := c.continuationPositiveValue("UserNovelBookmarks", query, request.Cursor, "max_bookmark_id")
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.userNovelBookmarks.List(ctx, usernovelbookmarks.Request{
		UserID: request.UserID, Restrict: string(request.Restrict), Tag: request.Tag, MaxBookmarkID: maxID,
	})
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "UserNovelBookmarks")
	}
	return c.novelEntityPage("UserNovelBookmarks", query, "max_bookmark_id", list.Items, list.NextMaxBookmarkID, list.HasNext)
}

// UserNovelBookmarkTags lists the bookmark tags of one user's bookmarked
// novels. The candidate upstream endpoint currently has no continuation
// contract, so a non-zero cursor is rejected instead of being guessed.
func (c *Client) UserNovelBookmarkTags(ctx context.Context, request UserNovelBookmarkTagsRequest) (sdk.Page[BookmarkTag], error) {
	if request.UserID <= 0 {
		return sdk.Page[BookmarkTag]{}, newError("UserNovelBookmarkTags", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserNovelBookmarkTags", request.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if !request.Cursor.IsZero() {
		return sdk.Page[BookmarkTag]{}, newError("UserNovelBookmarkTags", sdk.InvalidCursor, "novel bookmark tags continuation is not supported")
	}
	result, err := c.userNovelBookmarks.Tags(ctx, usernovelbookmarks.TagsRequest{
		UserID: request.UserID, Restrict: string(request.Restrict),
	})
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserNovelBookmarkTags")
	}
	items := make([]BookmarkTag, 0, len(result.Items))
	for _, tag := range result.Items {
		items = append(items, BookmarkTag{Name: tag.Name, Count: tag.Count})
	}
	return sdk.Page[BookmarkTag]{Items: items}, nil
}

// NovelBookmark reads the current user's bookmark detail for one novel.
// The upstream route is a candidate and is kept endpoint-oriented; it does
// not expose novel metadata or mutation outcome through this method.
func (c *Client) NovelBookmark(ctx context.Context, request NovelBookmarkRequest) (NovelBookmarkDetail, error) {
	if request.NovelID <= 0 {
		return NovelBookmarkDetail{}, newError("NovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	detail, err := c.userNovelBookmarks.Detail(ctx, request.NovelID)
	if err != nil {
		return NovelBookmarkDetail{}, classifyAppError(err, "NovelBookmark")
	}
	return NovelBookmarkDetail{Restrict: Restrict(detail.Restrict), Tags: append([]string{}, detail.Tags...)}, nil
}

// MyPixivNovels lists novels from the current user's MyPixiv feed.
func (c *Client) MyPixivNovels(ctx context.Context, request MyPixivNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{}
	offset, err := c.continuationPositiveOffset("MyPixivNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "MyPixivNovels", pixivapi.PathNovelMyPixiv, query, offset, novelMyPixivContinuation, false)
}

// novelOffsetPage 是所有 offset 续页 novel 列表的共同实现，端点之间只有
// 路径、已校验的 base query 与 continuation allowlist 不同。
func (c *Client) novelOffsetPage(ctx context.Context, operation, path string, base url.Values, offset int, spec continuationSpec, requireSearchFields bool) (sdk.Page[Novel], error) {
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}
	var raw novelListWire
	if err := c.app.GetJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, operation)
	}
	items, err := c.novelItems(operation, raw.Novels, requireSearchFields)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	next, hasNext, err := spec.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, operation)
	}
	cursor, err := c.buildCursor(operation, base, pixivapi.ParamOffset, int64(next), hasNext)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: cursor}, nil
}

// continuationValue 读取单一 key 的续页游标；零游标表示首页。
func (c *Client) continuationValue(operation string, base url.Values, cur sdk.Cursor, spec continuationSpec) (string, int64, error) {
	if cur.IsZero() {
		return spec.keys[0], 0, nil
	}
	return c.continuationKeyValue(operation, base, cur, spec)
}

// novelEntityPage 映射尚未并入 SDK 的 user 小说端点实体。
func (c *Client) novelEntityPage(operation string, query url.Values, key string, values []novelentity.Novel, nextValue int64, hasNext bool) (sdk.Page[Novel], error) {
	items := make([]Novel, len(values))
	for index, value := range values {
		mapped, err := c.mapNovelEntity(value)
		if err != nil {
			return sdk.Page[Novel]{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(operation, query, key, nextValue, hasNext)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: next}, nil
}

func (c *Client) fetchNovelContent(ctx context.Context, operation string, novelID int64) ([]byte, error) {
	body, err := c.app.GetRaw(ctx, pixivapi.PathNovelContent, url.Values{pixivapi.ParamNovelID: {itoa(novelID)}})
	if err != nil {
		return nil, classifyAppError(err, operation)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return append([]byte(nil), body...), nil
}
