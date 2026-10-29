package pixiv

import (
	"bytes"
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// novelWire is the shape every App API novel response shares. Search requires
// x_restrict, text_length, and is_original, while other endpoints treat a missing
// value as zero; the three stay pointers so each endpoint decides whether to
// require them.
type novelWire struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	XRestrict      *int          `json:"x_restrict"`
	TextLength     *int          `json:"text_length"`
	IsOriginal     *bool         `json:"is_original"`
	User           userWire      `json:"user"`
	Tags           []tagWire     `json:"tags"`
	ImageURLs      imageURLsWire `json:"image_urls"`
	CreateDate     string        `json:"create_date"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalView      int           `json:"total_view"`
}

type novelSeriesRefWire struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type novelDetailWire struct {
	Novel      *novelWire          `json:"novel"`
	SeriesNext *novelSeriesRefWire `json:"series_next"`
	SeriesPrev *novelSeriesRefWire `json:"series_prev"`
}

type novelListWire struct {
	Novels  api.RequiredList[novelWire] `json:"novels"`
	NextURL *string                     `json:"next_url"`
}

type novelSeriesDetailWire struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Caption     string   `json:"caption"`
	User        userWire `json:"user"`
	IsConcluded bool     `json:"is_concluded"`
}

type novelSeriesWire struct {
	Detail  *novelSeriesDetailWire      `json:"novel_series_detail"`
	Novels  api.RequiredList[novelWire] `json:"novels"`
	NextURL *string                     `json:"next_url"`
}

// mapNovel maps one upstream novel response to the public model. The cover and
// user profile are created through the policy-checked resource boundary.
func (c *Client) mapNovel(value novelWire) (Novel, error) {
	published, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Novel{}, newError("Novel", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	user, err := c.mapUser(value.User)
	if err != nil {
		return Novel{}, err
	}
	result := Novel{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		User:           user,
		Tags:           mapTags(value.Tags),
		PublishedAt:    published,
		XRestrict:      intValue(value.XRestrict),
		TextLength:     intValue(value.TextLength),
		IsOriginal:     value.IsOriginal != nil && *value.IsOriginal,
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalView,
	}
	for _, size := range []struct {
		variant string
		value   string
	}{
		{"original", value.ImageURLs.Original},
		{"large", value.ImageURLs.Large},
		{"medium", value.ImageURLs.Medium},
		{"square_medium", value.ImageURLs.SquareMedium},
	} {
		if size.value == "" {
			continue
		}
		resource, err := c.newResource("novel_cover", value.ID, -1, size.variant, size.value)
		if err != nil {
			return Novel{}, err
		}
		result.Cover = ImageResource{Resource: resource, Variant: size.variant}
		break
	}
	return result, nil
}

func (c *Client) novelItems(operation string, list api.RequiredList[novelWire], requireSearchFields bool) ([]Novel, error) {
	if !list.Present || !list.Valid {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	items := make([]Novel, len(list.Items))
	for index, value := range list.Items {
		if value.ID <= 0 || value.User.ID <= 0 ||
			requireSearchFields && (value.XRestrict == nil || value.TextLength == nil || value.IsOriginal == nil) {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
		mapped, err := c.mapNovel(value)
		if err != nil {
			return nil, err
		}
		items[index] = mapped
	}
	return items, nil
}

// SearchNovels searches novels. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchNovels(ctx context.Context, request SearchNovelsRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "SearchNovels")
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
		api.ParamWord:         {request.Word},
		api.ParamSearchTarget: {string(request.Target)},
		api.ParamSort:         {string(request.Sort)},
	}
	if request.Duration != "" {
		query.Set(api.ParamDuration, string(request.Duration))
	}
	offset, err := c.continuationPositiveOffset("SearchNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "SearchNovels", api.PathSearchNovel, query, query, offset, novelSearchContinuation, true)
}

// Novel returns one novel by its stable ID.
func (c *Client) Novel(ctx context.Context, request NovelRequest) (Novel, error) {
	ctx = logging.WithOperation(ctx, "Novel")
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
	if err := c.app.GetJSON(ctx, api.PathNovelDetail, url.Values{
		api.ParamNovelID: {itoa(novelID)},
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
	ctx = logging.WithOperation(ctx, "NovelSeries")
	if request.SeriesID <= 0 {
		return NovelSeriesResult{}, newError("NovelSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	base := url.Values{api.ParamSeriesID: {itoa(request.SeriesID)}}
	_, lastOrder, err := c.continuationKeyValue("NovelSeries", base, request.Cursor, novelSeriesContinuation)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	query := cloneValues(base)
	if lastOrder > 0 {
		query.Set(api.ParamLastOrder, itoa(lastOrder))
	}

	var raw novelSeriesWire
	if err := c.app.GetJSON(ctx, api.PathNovelSeries, query, &raw); err != nil {
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
	next, err := c.buildCursor("NovelSeries", base, api.ParamLastOrder, nextLastOrder, hasNext)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	user, err := c.mapUser(raw.Detail.User)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	return NovelSeriesResult{
		Series: NovelSeries{
			ID:          raw.Detail.ID,
			Title:       raw.Detail.Title,
			Caption:     raw.Detail.Caption,
			User:        user,
			IsConcluded: raw.Detail.IsConcluded,
		},
		Novels: sdk.Page[Novel]{Items: items, Next: next},
	}, nil
}

// NovelRanking lists the current novel ranking.
func (c *Client) NovelRanking(ctx context.Context, request NovelRankingRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "NovelRanking")
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("NovelRanking", request.Mode); err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{
		api.ParamFilter: {"for_android"},
		api.ParamMode:   {string(request.Mode)},
	}
	offset, err := c.continuationPositiveOffset("NovelRanking", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "NovelRanking", api.PathNovelRanking, query, query, offset, novelRankingContinuation, false)
}

// NovelContent reports that structured novel content is unavailable through
// the App API. WebView content is outside the v1 SDK contract.
func (c *Client) NovelContent(ctx context.Context, request NovelContentRequest) (NovelContent, error) {
	ctx = logging.WithOperation(ctx, "NovelContent")
	if request.NovelID <= 0 {
		return NovelContent{}, newError("NovelContent", sdk.InvalidArgument, "novel ID must be positive")
	}
	return NovelContent{}, newError("NovelContent", sdk.ContentUnavailable, "novel content is unsupported by the v1 App API")
}

// RecommendedNovels lists recommended novels. The next page replays the
// multi-parameter upstream next_url as a unit (offset, already_recommended, and
// bookmark cursor).
func (c *Client) RecommendedNovels(ctx context.Context, request RecommendedNovelsRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "RecommendedNovels")
	query := url.Values{}
	params, err := c.continuationParams("RecommendedNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}

	var raw novelListWire
	if err := c.app.GetJSON(ctx, api.PathNovelRecommended, params, &raw); err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "RecommendedNovels")
	}
	items, err := c.novelItems("RecommendedNovels", raw.Novels, false)
	if err != nil {
		return sdk.Page[Novel]{}, err
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
	ctx = logging.WithOperation(ctx, "FollowingNovels")
	restrict, err := normalizeFollowingRestrict("FollowingNovels", request.Restrict)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{api.ParamRestrict: {string(restrict)}}
	offset, err := c.continuationPositiveOffset("FollowingNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "FollowingNovels", api.PathNovelFollow, query, query, offset, novelFollowingContinuation, false)
}

// LatestNovels lists the newest novels.
func (c *Client) LatestNovels(ctx context.Context, request LatestNovelsRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "LatestNovels")
	base := url.Values{api.ParamFilter: {"for_android"}}
	_, maxNovelID, err := c.continuationKeyValue("LatestNovels", base, request.Cursor, novelLatestContinuation)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := cloneValues(base)
	if maxNovelID > 0 {
		query.Set(api.ParamMaxNovelID, itoa(maxNovelID))
	}

	var raw novelListWire
	if err := c.app.GetJSON(ctx, api.PathNovelNew, query, &raw); err != nil {
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
	cursor, err := c.buildCursor("LatestNovels", base, api.ParamMaxNovelID, next, hasNext)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: cursor}, nil
}

// UserNovels lists one user's novels.
func (c *Client) UserNovels(ctx context.Context, request UserNovelsRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "UserNovels")
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovels", sdk.InvalidArgument, "user ID must be positive")
	}
	base := url.Values{"user_id": {itoa(request.UserID)}}
	offset, err := c.continuationPositiveOffset("UserNovels", base, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "filter": {"for_android"}}
	return c.novelOffsetPage(ctx, "UserNovels", api.PathUserNovels, base, query, offset, novelUserContinuation, false)
}

// MyPixivNovels lists novels from the current user's MyPixiv feed.
func (c *Client) MyPixivNovels(ctx context.Context, request MyPixivNovelsRequest) (sdk.Page[Novel], error) {
	ctx = logging.WithOperation(ctx, "MyPixivNovels")
	query := url.Values{}
	offset, err := c.continuationPositiveOffset("MyPixivNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return c.novelOffsetPage(ctx, "MyPixivNovels", api.PathNovelMyPixiv, query, query, offset, novelMyPixivContinuation, false)
}

// novelOffsetPage backs every offset-paginated novel list. Endpoints differ only
// in path, query, cursor binding value, continuation allowlist, and whether
// search fields are required.
func (c *Client) novelOffsetPage(ctx context.Context, operation, path string, base, query url.Values, offset int, spec continuationSpec, requireSearchFields bool) (sdk.Page[Novel], error) {
	query = cloneValues(query)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
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
	cursor, err := c.buildCursor(operation, base, api.ParamOffset, int64(next), hasNext)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: cursor}, nil
}

func (c *Client) fetchNovelContent(ctx context.Context, operation string, novelID int64) ([]byte, error) {
	body, err := c.app.GetRaw(ctx, api.PathNovelContent, url.Values{api.ParamNovelID: {itoa(novelID)}})
	if err != nil {
		return nil, classifyAppError(err, operation)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return body, nil
}

// SearchNovelsRequest searches novels. Repeat the original fields when
// continuing with Cursor.
type SearchNovelsRequest struct {
	Word     string
	Target   SearchTarget
	Sort     SortMode
	Duration DurationFilter
	Cursor   sdk.Cursor
}

// NovelRequest selects one novel by its stable ID.
type NovelRequest struct {
	NovelID int64
}

// NovelSeriesRequest lists novels within one novel series.
type NovelSeriesRequest struct {
	SeriesID int64
	Cursor   sdk.Cursor
}

// NovelContentRequest reads the structured content of one novel.
type NovelContentRequest struct {
	NovelID int64
}

// RecommendedNovelsRequest lists recommended novels.
type RecommendedNovelsRequest struct {
	Cursor sdk.Cursor
}

// NovelRankingRequest lists the current novel ranking.
type NovelRankingRequest struct {
	Mode   RankingMode
	Cursor sdk.Cursor
}

// FollowingNovelsRequest lists novels by followed users.
type FollowingNovelsRequest struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// LatestNovelsRequest lists the newest novels.
type LatestNovelsRequest struct {
	Cursor sdk.Cursor
}

// UserNovelsRequest lists one user's novels.
type UserNovelsRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// MyPixivNovelsRequest lists novels from the current user's MyPixiv feed.
type MyPixivNovelsRequest struct {
	Cursor sdk.Cursor
}

var novelSearchContinuation = continuationSpec{
	path: api.PathSearchNovel,
	keys: []string{api.ParamOffset},
	query: []string{
		api.ParamWord, api.ParamSearchTarget, api.ParamSort, api.ParamDuration,
	},
}

var novelRankingContinuation = continuationSpec{
	path:  api.PathNovelRanking,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamFilter, api.ParamMode},
}

var novelRecommendedContinuation = continuationSpec{
	path: api.PathNovelRecommended,
	keys: []string{
		api.ParamOffset, api.ParamAlreadyRecommended,
		api.ParamMaxRecommend, api.ParamIncludeRankingNovels, api.ParamIncludePrivacy,
	},
	allowZero: true,
}

var novelSeriesContinuation = continuationSpec{
	path:  api.PathNovelSeries,
	keys:  []string{api.ParamLastOrder},
	query: []string{api.ParamSeriesID},
}

var novelFollowingContinuation = continuationSpec{
	path:  api.PathNovelFollow,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamRestrict},
}

var novelLatestContinuation = continuationSpec{
	path:  api.PathNovelNew,
	keys:  []string{api.ParamMaxNovelID},
	query: []string{api.ParamFilter},
}

var novelMyPixivContinuation = continuationSpec{
	path: api.PathNovelMyPixiv,
	keys: []string{api.ParamOffset},
}

var novelUserContinuation = continuationSpec{
	path:  api.PathUserNovels,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamUserID, api.ParamFilter},
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
