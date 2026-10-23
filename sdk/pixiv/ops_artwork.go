// artwork 读取操作：详情、分页列表、排行榜、评论、书签详情与动图元数据。
// 请求参数校验在 validation.go，游标编解码在 cursor.go，响应结构与映射在 wire.go。

package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Artwork returns one artwork by its stable ID, including every image page.
func (c *Client) Artwork(ctx context.Context, request ArtworkRequest) (Artwork, error) {
	if request.ArtworkID <= 0 {
		return Artwork{}, newError("Artwork", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.fetchArtworkDetail(ctx, "Artwork", request.ArtworkID)
	if err != nil {
		return Artwork{}, err
	}
	result, err := c.mapArtwork(*detail.Illust)
	if err != nil {
		return Artwork{}, err
	}
	result.Pages, err = c.mapArtworkPages(*detail.Illust)
	if err != nil {
		return Artwork{}, err
	}
	return result, nil
}

// ArtworkPages returns every image page of one artwork as usable resources.
func (c *Client) ArtworkPages(ctx context.Context, request ArtworkPagesRequest) ([]ArtworkPage, error) {
	if request.ArtworkID <= 0 {
		return nil, newError("ArtworkPages", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.fetchArtworkDetail(ctx, "ArtworkPages", request.ArtworkID)
	if err != nil {
		return nil, err
	}
	return c.mapArtworkPages(*detail.Illust)
}

func (c *Client) fetchArtworkDetail(ctx context.Context, operation string, artworkID int64) (artworkDetailWire, error) {
	var raw artworkDetailWire
	query := url.Values{pixivapi.ParamIllustID: {itoa(artworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustDetail, query, &raw); err != nil {
		return artworkDetailWire{}, classifyAppError(err, operation)
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return artworkDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return raw, nil
}

// RelatedArtworks lists artworks related to one artwork.
func (c *Client) RelatedArtworks(ctx context.Context, request RelatedArtworksRequest) (sdk.Page[Artwork], error) {
	if request.ArtworkID <= 0 {
		return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	params, err := c.continuationParams("RelatedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := base
	if params != nil {
		if params.Get(pixivapi.ParamIllustID) != itoa(request.ArtworkID) {
			return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidCursor, "cursor continuation artwork mismatch")
		}
		query = params
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRelated, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	items, err := artworkItems("RelatedArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextParams, hasNext, err := relatedParams(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	return c.artworkParamsPage("RelatedArtworks", base, items, nextParams, hasNext)
}

// ArtworkSeries lists artworks within one illustration series.
func (c *Client) ArtworkSeries(ctx context.Context, request ArtworkSeriesRequest) (sdk.Page[Artwork], error) {
	if request.SeriesID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustSeriesID: {itoa(request.SeriesID)}}
	query := cloneValues(base)
	key, value, err := c.continuationKeyValue("ArtworkSeries", base, request.Cursor, seriesContinuation)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if value > 0 {
		query.Set(key, itoa(value))
	}

	var raw artworkSeriesWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustSeries, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	if raw.SeriesDetail == nil || raw.SeriesDetail.User.ID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.MalformedUpstreamResponse, "")
	}
	items, err := artworkItems("ArtworkSeries", raw.Illusts, true)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextKey, nextValue, hasNext, err := seriesContinuation.keyValue(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	return c.artworkPage("ArtworkSeries", base, nextKey, items, nextValue, hasNext)
}

// ArtworkRanking lists the current artwork ranking.
func (c *Client) ArtworkRanking(ctx context.Context, request ArtworkRankingRequest) (sdk.Page[Artwork], error) {
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("ArtworkRanking", request.Mode); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateDate("ArtworkRanking", "date", request.Date); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamMode: {string(request.Mode)}}
	if request.Date != "" {
		base.Set(pixivapi.ParamDate, request.Date)
	}
	offset, err := c.continuationPositiveOffset("ArtworkRanking", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRanking, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	items, err := artworkItems("ArtworkRanking", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := rankingContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	return c.artworkPage("ArtworkRanking", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// RecommendedArtworks lists recommended artworks. 上游以多参数 next_url
// 表达续页（offset=0 特例 + bookmark 游标 + viewed 下标数组），cursor 以
// 结构化参数集整体回放，不保存 raw next_url。
func (c *Client) RecommendedArtworks(ctx context.Context, request RecommendedArtworksRequest) (sdk.Page[Artwork], error) {
	base := url.Values{}
	params, err := c.continuationParams("RecommendedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRecommended, params, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	items, err := artworkItems("RecommendedArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextParams, hasNext, err := recommendedContinuation.params(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	return c.artworkParamsPage("RecommendedArtworks", base, items, nextParams, hasNext)
}

// FollowingArtworks lists artworks by followed users.
func (c *Client) FollowingArtworks(ctx context.Context, request FollowingArtworksRequest) (sdk.Page[Artwork], error) {
	restrict, err := normalizeFollowingRestrict("FollowingArtworks", request.Restrict)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamRestrict: {string(restrict)}}
	offset, err := c.continuationPositiveOffset("FollowingArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustFollow, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	items, err := artworkItems("FollowingArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := followingContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	return c.artworkPage("FollowingArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// LatestArtworks lists the newest artworks.
func (c *Client) LatestArtworks(ctx context.Context, request LatestArtworksRequest) (sdk.Page[Artwork], error) {
	contentType, err := normalizeLatestArtworkContentType("LatestArtworks", request.ContentType)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	// Bind the resolved content type into the cursor digest so a continuation
	// produced for one feed (e.g. illust) cannot be replayed against another
	// (e.g. manga), which would resume at an offset for the wrong result set.
	base := url.Values{
		pixivapi.ParamContentType: {contentType},
		pixivapi.ParamFilter:      {"for_android"},
	}
	key, value, err := c.continuationKeyValue("LatestArtworks", base, request.Cursor, latestContinuation)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if value > 0 {
		query.Set(key, itoa(value))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustNew, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	items, err := artworkItems("LatestArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextKey, nextValue, hasNext, err := latestContinuation.keyValue(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	return c.artworkPage("LatestArtworks", base, nextKey, items, nextValue, hasNext)
}

// UserArtworks lists one user's artworks.
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworks", sdk.InvalidArgument, "user ID must be positive")
	}
	kind, err := normalizeUserArtworkKind("UserArtworks", request.Kind)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamUserID: {itoa(request.UserID)}, pixivapi.ParamType: {kind}}
	offset, err := c.continuationPositiveOffset("UserArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserIllusts, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	items, err := artworkItems("UserArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := userArtworksContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	return c.artworkPage("UserArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// UserArtworkBookmarks lists one user's bookmarked artworks.
func (c *Client) UserArtworkBookmarks(ctx context.Context, request UserArtworkBookmarksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworkBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserArtworkBookmarks", request.Restrict); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{
		pixivapi.ParamUserID:   {itoa(request.UserID)},
		pixivapi.ParamRestrict: {string(request.Restrict)},
	}
	if request.Tag != "" {
		base.Set(pixivapi.ParamTag, request.Tag)
	}
	maxID, err := c.continuationPositiveValue("UserArtworkBookmarks", base, request.Cursor, pixivapi.ParamMaxBookmarkID)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if maxID > 0 {
		query.Set(pixivapi.ParamMaxBookmarkID, itoa(maxID))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserBookmarksIllust, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	items, err := artworkItems("UserArtworkBookmarks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := bookmarkArtworksContinuation.value(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	return c.artworkPage("UserArtworkBookmarks", base, pixivapi.ParamMaxBookmarkID, items, next, hasNext)
}

// UserArtworkBookmarkTags lists the bookmark tags of one user's bookmarked
// artworks.
func (c *Client) UserArtworkBookmarkTags(ctx context.Context, request UserArtworkBookmarkTagsRequest) (sdk.Page[BookmarkTag], error) {
	if request.UserID <= 0 {
		return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserArtworkBookmarkTags", request.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	base := url.Values{
		pixivapi.ParamUserID:   {itoa(request.UserID)},
		pixivapi.ParamRestrict: {string(request.Restrict)},
	}
	offset, err := c.continuationPositiveOffset("UserArtworkBookmarkTags", base, request.Cursor)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw bookmarkTagsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserBookmarkTags, query, &raw); err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "")
	}
	items := make([]BookmarkTag, len(raw.Tags.Items))
	for index, value := range raw.Tags.Items {
		if value.Name == "" {
			return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "")
		}
		items[index] = BookmarkTag{Name: value.Name, Count: value.Count}
	}
	next, hasNext, err := bookmarkTagsContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	cursor, err := c.buildCursor("UserArtworkBookmarkTags", base, pixivapi.ParamOffset, int64(next), hasNext)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	return sdk.Page[BookmarkTag]{Items: items, Next: cursor}, nil
}

// MyPixivArtworks lists artworks from the current user's MyPixiv feed.
func (c *Client) MyPixivArtworks(ctx context.Context, request MyPixivArtworksRequest) (sdk.Page[Artwork], error) {
	base := url.Values{}
	offset, err := c.continuationPositiveOffset("MyPixivArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustMyPixiv, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	items, err := artworkItems("MyPixivArtworks", raw.Illusts, true)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := myPixivContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	return c.artworkPage("MyPixivArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// TrendingArtworkTags lists currently trending artwork tags.
func (c *Client) TrendingArtworkTags(ctx context.Context, request TrendingArtworkTagsRequest) ([]TrendingTag, error) {
	var raw trendingArtworksWire
	if err := c.app.GetJSON(ctx, pixivapi.PathTrendingTagsIllust, nil, &raw); err != nil {
		return nil, classifyAppError(err, "TrendingArtworkTags")
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return nil, newError("TrendingArtworkTags", sdk.MalformedUpstreamResponse, "")
	}
	items := make([]TrendingTag, len(raw.Tags.Items))
	for index, value := range raw.Tags.Items {
		if value.Tag == "" || !value.Illust.Present || !value.Illust.Valid || value.Illust.Value.ID <= 0 {
			return nil, newError("TrendingArtworkTags", sdk.MalformedUpstreamResponse, "")
		}
		mapped, err := c.mapArtwork(value.Illust.Value)
		if err != nil {
			return nil, err
		}
		items[index] = TrendingTag{Tag: value.Tag, TranslatedName: value.TranslatedName, Artwork: mapped}
	}
	return items, nil
}

// UgoiraMetadata returns the playable metadata of a ugoira artwork. The artwork
// must be a ugoira; other kinds return InvalidArgument.
func (c *Client) UgoiraMetadata(ctx context.Context, request UgoiraMetadataRequest) (UgoiraMetadata, error) {
	if request.ArtworkID <= 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.InvalidArgument, "artwork ID must be positive")
	}
	var raw ugoiraResponseWire
	query := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathUgoiraMetadata, query, &raw); err != nil {
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	if !raw.Metadata.Present || !raw.Metadata.Valid {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "")
	}
	metadata, err := c.mapUgoiraMetadata(request.ArtworkID, raw.Metadata.Value)
	if err != nil {
		// 映射阶段返回的已是公开 SDK 错误（例如 ResourceForbidden），不能再按
		// 上游失败重新分类，否则会丢掉原始 reason。
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return UgoiraMetadata{}, err
		}
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	return metadata, nil
}

// ArtworkComments lists comments on one artwork.
func (c *Client) ArtworkComments(ctx context.Context, request ArtworkCommentsRequest) (CommentPage, error) {
	if request.ArtworkID <= 0 {
		return CommentPage{}, newError("ArtworkComments", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	offset, err := c.continuationPositiveOffset("ArtworkComments", base, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkCommentsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustComments, query, &raw); err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return CommentPage{}, newError("ArtworkComments", sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return CommentPage{}, newError("ArtworkComments", sdk.MalformedUpstreamResponse, "")
		}
	}
	var access *CommentAccessControl
	if raw.CommentAccessControl != nil {
		access = &CommentAccessControl{NumericValue: cloneValue(raw.CommentAccessControl)}
	} else if raw.AccessControl != nil {
		access = &CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	next, hasNext, err := commentsContinuation.offset(raw.NextURL)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	return c.commentPage("ArtworkComments", base, raw.Comments.Items, next, hasNext, raw.TotalComments, access)
}

// ArtworkBookmark reads the current user's bookmark detail for one artwork.
func (c *Client) ArtworkBookmark(ctx context.Context, request ArtworkBookmarkRequest) (ArtworkBookmarkDetail, error) {
	if request.ArtworkID <= 0 {
		return ArtworkBookmarkDetail{}, newError("ArtworkBookmark", sdk.InvalidArgument, "artwork ID must be positive")
	}
	var raw bookmarkDetailWire
	query := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustBookmarkDetail, query, &raw); err != nil {
		// v2 对“未收藏”作品可能返回 404；SDK 的公开契约把它统一表示为
		// 空状态，因此只在该端点上转换 404，其他错误仍原样暴露。
		var failure pixivapi.Failure
		if !errors.As(err, &failure) || failure.Kind != pixivapi.FailureHTTPStatus || failure.StatusCode != http.StatusNotFound {
			return ArtworkBookmarkDetail{}, classifyAppError(err, "ArtworkBookmark")
		}
		return ArtworkBookmarkDetail{Tags: []string{}}, nil
	}
	if raw.Detail == nil || raw.Detail.IsBookmarked != nil && !*raw.Detail.IsBookmarked {
		// 未收藏响应中的 tags 可能是作品自身标签，不是收藏标签；统一归一为空状态。
		return ArtworkBookmarkDetail{Tags: []string{}}, nil
	}
	tags := []string{}
	for _, tag := range raw.Detail.Tags {
		if tag.IsRegistered {
			tags = append(tags, tag.Name)
		}
	}
	return ArtworkBookmarkDetail{Restrict: Restrict(raw.Detail.Restrict), Tags: tags}, nil
}

// artworkItems 校验 artwork 列表的必需字段并原样返回 wire 项。
func artworkItems(operation string, list pixivapi.RequiredList[artworkWire], requireUser bool) ([]artworkWire, error) {
	if !list.Present || !list.Valid {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range list.Items {
		if item.ID <= 0 || requireUser && item.User.ID <= 0 {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	return list.Items, nil
}

// continuationKeyValue 解码多 key 续页游标；零游标表示首页。
func (c *Client) continuationKeyValue(operation string, base url.Values, cur sdk.Cursor, spec continuationSpec) (string, int64, error) {
	if cur.IsZero() {
		return spec.keys[0], 0, nil
	}
	key, value, err := c.continuationFromCursor(operation, base, cur)
	if err != nil {
		return "", 0, err
	}
	for _, allowed := range spec.keys {
		if key == allowed {
			if value <= 0 {
				return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation value must be positive")
			}
			if key == pixivapi.ParamOffset && int64(int(value)) != value {
				return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation offset is out of range")
			}
			return key, value, nil
		}
	}
	return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation kind mismatch")
}

func cloneValues(values url.Values) url.Values {
	if values == nil {
		return url.Values{}
	}
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}
