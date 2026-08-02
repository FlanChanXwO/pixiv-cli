package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultAPIBase      = protocol.AppAPIBase
	DefaultUserAgent    = protocol.AppUserAgent
	DefaultAppOS        = protocol.AppOS
	DefaultAppOSVersion = protocol.AppOSVersion
	DefaultAppVersion   = protocol.AppVersion
	// androidListFilter 与当前 Android App profile 对齐；只用于已证实要求该
	// filter 的列表端点，避免把未确认参数扩散到聚合或关注流。
	androidListFilter = "for_android"
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

type Client struct {
	restyClient    *resty.Client
	apiBase        string
	session        Session
	acceptLanguage string
	userID         int64
	// disableRetryAfterRetry 只由上层明确需要观察首个 429 的调度器启用。
	// 默认仍遵守既有的有效 Retry-After 单次重试契约。
	disableRetryAfterRetry bool
}

// Session 是 App 内容 API 仅需知道的最小认证边界。
type Session interface {
	AccessToken() string
	Refresh(context.Context) error
}

type staticSession struct{ token string }

func (s staticSession) AccessToken() string { return s.token }
func (staticSession) Refresh(context.Context) error {
	return errors.New("access token cannot be refreshed")
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.restyClient = resty.NewWithClient(httpClient)
		}
	}
}

func WithBaseURL(apiBase string) Option {
	return func(c *Client) {
		if apiBase != "" {
			c.apiBase = strings.TrimRight(apiBase, "/")
		}
	}
}

func WithSession(session Session) Option {
	return func(c *Client) { c.session = session }
}

// WithAccessToken 注入已取得的 access token，供无需刷新流程的 SDK 调用复用 App API。
func WithAccessToken(token string) Option {
	return func(c *Client) {
		c.session = staticSession{token: strings.TrimSpace(token)}
	}
}

// WithAcceptLanguage 注入语言协商头；空值不设置。
func WithAcceptLanguage(language string) Option {
	return func(c *Client) {
		c.acceptLanguage = strings.TrimSpace(language)
	}
}

// WithUserID 注入经过验证的当前账号 ID，供需要 X-User-Id 的 App endpoint（如小说内容）使用。
func WithUserID(userID int64) Option {
	return func(c *Client) { c.userID = userID }
}

// WithDisableRetryAfterRetry 使读取请求把首个有效 Retry-After 限流直接交给调用方。
// 它不改变认证刷新重试，也不会让 mutation 被重放。
func WithDisableRetryAfterRetry() Option {
	return func(c *Client) { c.disableRetryAfterRetry = true }
}

func New(opts ...Option) *Client {
	c := &Client{
		restyClient: resty.New(),
		apiBase:     DefaultAPIBase,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.restyClient == nil {
		c.restyClient = resty.New()
	}
	return c
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration, startDate, endDate string, offset int, filterOptions ...model.SearchIllustFilters) (*model.IllustList, error) {
	q := url.Values{"word": {word}, "search_target": {target}, "sort": {sort}}
	setOptional(q, "duration", duration)
	setOptional(q, "start_date", startDate)
	setOptional(q, "end_date", endDate)
	if len(filterOptions) > 0 {
		setSearchIllustFilters(q, filterOptions[0])
	}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppSearchIllust, q, "offset")
}

// SearchNovel 返回一个 App API 小说搜索批次。小说搜索不复用推荐接口的 offset=0 continuation 语义。
func (c *Client) SearchNovel(ctx context.Context, word, target, sort, duration string, offset int) (*model.NovelList, error) {
	q := url.Values{"word": {word}, "search_target": {target}, "sort": {sort}}
	setOptional(q, "duration", duration)
	setOffset(q, offset)
	return c.getNovelSearchList(ctx, protocol.AppSearchNovel, q)
}

func setSearchIllustFilters(query url.Values, filters model.SearchIllustFilters) {
	// 后端可验证参数：tool / ratio_pattern / content_type / resolution bounds / search_ai_type。
	// rating 与 only AI 不在此编码；exclude AI 发 search_ai_type=1，本地后筛见 public SDK。
	query.Set("search_ai_type", "0")
	if filters.AIMode == "exclude" {
		query.Set("search_ai_type", "1")
	}
	switch filters.AspectRatio {
	case "landscape", "portrait", "square":
		query.Set("ratio_pattern", filters.AspectRatio)
	}
	switch filters.ContentType {
	case "illust-and-ugoira":
		query.Set("content_type", "illust_and_ugoira")
	case "illust", "manga", "ugoira":
		query.Set("content_type", filters.ContentType)
	}
	setOptional(query, "tool", filters.Tool)
	if filters.BookmarkMin != nil {
		query.Set("bookmark_num_min", strconv.Itoa(*filters.BookmarkMin))
	}
	if filters.BookmarkMax != nil {
		query.Set("bookmark_num_max", strconv.Itoa(*filters.BookmarkMax))
	}
	switch filters.Resolution {
	case "high":
		query.Set("width_min", "3000")
		query.Set("height_min", "3000")
	case "medium":
		query.Set("width_min", "1000")
		query.Set("width_max", "2999")
		query.Set("height_min", "1000")
		query.Set("height_max", "2999")
	case "low":
		query.Set("width_max", "999")
		query.Set("height_max", "999")
	}
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	var raw illustDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppIllustDetail, url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
		return nil, err
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return nil, protocol.MalformedResponse()
	}
	out := mapIllustDetail(raw)
	return &out, nil
}

func (c *Client) IllustRelated(ctx context.Context, id int64, offset int) (*model.IllustList, error) {
	q := url.Values{"illust_id": {fmt.Sprint(id)}}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustRelated, q, "offset")
}

// IllustSeries 读取插画系列的一个批次。next_url 只抽取 last_order 数值，绝不将
// 上游 URL 作为下一跳请求目标。
func (c *Client) IllustSeries(ctx context.Context, seriesID int64, lastOrder int64) (*model.IllustList, error) {
	q := url.Values{"illust_series_id": {fmt.Sprint(seriesID)}}
	if lastOrder > 0 {
		q.Set("last_order", fmt.Sprint(lastOrder))
	}
	var raw illustSeriesDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppIllustSeries, q, &raw); err != nil {
		return nil, err
	}
	if raw.IllustSeriesDetail == nil || raw.IllustSeriesDetail.User.ID <= 0 || !raw.Illusts.Present || !raw.Illusts.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, illust := range raw.Illusts.Items {
		if illust.ID <= 0 || illust.User.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
	}
	listDTO := illustListDTO{Illusts: raw.Illusts, NextURL: raw.NextURL}
	out := mapIllustList(listDTO)
	out.SeriesUserID = raw.IllustSeriesDetail.User.ID
	if err := applyListContinuation(raw.NextURL, "last_order", positiveContinuation, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) IllustRanking(ctx context.Context, mode, date string, offset int) (*model.IllustList, error) {
	q := url.Values{"mode": {mode}}
	setOptional(q, "date", date)
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustRanking, q, "offset")
}

func (c *Client) SearchUser(ctx context.Context, word string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"word": {word}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppSearchUser, q)
}

func (c *Client) UserDetail(ctx context.Context, userID int64) (*model.UserDetail, error) {
	var raw userDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUserDetail, url.Values{"user_id": {fmt.Sprint(userID)}}, &raw); err != nil {
		return nil, err
	}
	if !raw.User.Present || !raw.User.Valid || raw.User.Value.ID <= 0 ||
		!raw.Profile.Present || !raw.Profile.Valid ||
		!raw.ProfilePublicity.Present || !raw.ProfilePublicity.Valid || !raw.ProfilePublicity.Value.valid() ||
		!raw.Workspace.Present || !raw.Workspace.Valid {
		return nil, protocol.MalformedResponse()
	}
	out := mapUserDetail(raw)
	return &out, nil
}

func (c *Client) IllustRecommended(ctx context.Context, offset int, continuationExists bool) (*model.IllustList, error) {
	q := url.Values{}
	setRecommendationOffset(q, offset, continuationExists)
	return c.getIllustListAllowingZeroOffset(ctx, protocol.AppIllustRecommended, q)
}

// MangaRecommended 使用插画推荐 catalog 的漫画筛选；Pixiv 没有独立 manga 推荐 endpoint。
func (c *Client) MangaRecommended(ctx context.Context, offset int, continuationExists bool) (*model.IllustList, error) {
	q := url.Values{"content_type": {"manga"}}
	setRecommendationOffset(q, offset, continuationExists)
	return c.getIllustListAllowingZeroOffset(ctx, protocol.AppIllustRecommended, q)
}

// NovelRecommended 返回小说推荐的单个 App API 批次。
func (c *Client) NovelRecommended(ctx context.Context, offset int, continuationExists bool) (*model.NovelList, error) {
	q := url.Values{}
	setRecommendationOffset(q, offset, continuationExists)
	return c.getNovelList(ctx, protocol.AppNovelRecommended, q)
}

// UserRecommended 返回作者推荐及其可用作品预览的单个 App API 批次。
func (c *Client) UserRecommended(ctx context.Context, offset int, continuationExists bool) (*model.RecommendedUserList, error) {
	q := url.Values{}
	setRecommendationOffset(q, offset, continuationExists)
	return c.getRecommendedUserList(ctx, protocol.AppUserRecommended, q)
}

func (c *Client) TrendingTagsIllust(ctx context.Context) (*model.TrendTags, error) {
	var raw trendTagsDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppTrendingTagsIllust, nil, &raw); err != nil {
		return nil, err
	}
	if !raw.TrendTags.Present || !raw.TrendTags.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, item := range raw.TrendTags.Items {
		if item.Tag == "" || !item.Illust.Present || !item.Illust.Valid || item.Illust.Value.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapTrendTags(raw)
	return &out, nil
}

func (c *Client) IllustFollow(ctx context.Context, restrict string, offset int) (*model.IllustList, error) {
	q := url.Values{"restrict": {restrict}}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustFollow, q, "offset")
}

// IllustNew 返回全站最新插画或漫画的单个 App API 批次。
func (c *Client) IllustNew(ctx context.Context, contentType string, offset int) (*model.IllustList, error) {
	q := url.Values{"content_type": {contentType}, "filter": {androidListFilter}}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustNew, q, "offset")
}

// NovelNew 返回全站最新小说的单个 App API 批次。
func (c *Client) NovelNew(ctx context.Context, offset int) (*model.NovelList, error) {
	q := url.Values{"filter": {androidListFilter}}
	setOffset(q, offset)
	return c.getNovelListWithContinuationPolicy(ctx, protocol.AppNovelNew, q, positiveContinuation, false)
}

// NovelFollow 返回当前认证账号所关注用户的小说新作批次。
func (c *Client) NovelFollow(ctx context.Context, restrict string, offset int) (*model.NovelList, error) {
	q := url.Values{"restrict": {restrict}}
	setOffset(q, offset)
	return c.getNovelListWithContinuationPolicy(ctx, protocol.AppNovelFollow, q, positiveContinuation, false)
}

// UserMyPixiv 返回指定用户的 MyPixiv 用户列表。
func (c *Client) UserMyPixiv(ctx context.Context, userID int64, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "filter": {androidListFilter}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppUserMyPixiv, q)
}

// IllustMyPixiv 返回当前认证账号 MyPixiv 的插画聚合批次。
func (c *Client) IllustMyPixiv(ctx context.Context, offset int) (*model.IllustList, error) {
	q := url.Values{}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustMyPixiv, q, "offset")
}

// NovelMyPixiv 返回当前认证账号 MyPixiv 的小说聚合批次。
func (c *Client) NovelMyPixiv(ctx context.Context, offset int) (*model.NovelList, error) {
	q := url.Values{}
	setOffset(q, offset)
	return c.getNovelListWithContinuationPolicy(ctx, protocol.AppNovelMyPixiv, q, positiveContinuation, false)
}

// UserNovels 返回指定用户的小说批次。
func (c *Client) UserNovels(ctx context.Context, userID int64, offset int) (*model.NovelList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "filter": {androidListFilter}}
	setOffset(q, offset)
	return c.getNovelListWithContinuationPolicy(ctx, protocol.AppUserNovels, q, positiveContinuation, false)
}

// UserArtworks 返回用户作品的单个 App API 批次。
func (c *Client) UserArtworks(ctx context.Context, userID int64, illustType string, offset int) (*model.IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "type": {illustType}}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppUserIllusts, q, "offset")
}

func (c *Client) UserBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*model.IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOptional(q, "tag", tag)
	if maxBookmarkID > 0 {
		q.Set("max_bookmark_id", fmt.Sprint(maxBookmarkID))
	}
	return c.getIllustList(ctx, protocol.AppUserBookmarks, q, "max_bookmark_id")
}

func (c *Client) UserFollowing(ctx context.Context, userID int64, restrict string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppUserFollowing, q)
}

func (c *Client) getIllustList(ctx context.Context, path string, query url.Values, continuationKey string) (*model.IllustList, error) {
	return c.getIllustListWithContinuationPolicy(ctx, path, query, continuationKey, positiveContinuation)
}

func (c *Client) getIllustListAllowingZeroOffset(ctx context.Context, path string, query url.Values) (*model.IllustList, error) {
	return c.getIllustListWithContinuationPolicy(ctx, path, query, "offset", recommendationOffsetContinuation)
}

func (c *Client) getIllustListWithContinuationPolicy(ctx context.Context, path string, query url.Values, continuationKey string, policy continuationPolicy) (*model.IllustList, error) {
	var raw illustListDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, illust := range raw.Illusts.Items {
		if illust.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapIllustList(raw)
	if err := applyListContinuation(raw.NextURL, continuationKey, policy, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) getUserPreviewList(ctx context.Context, path string, query url.Values) (*model.UserPreviewList, error) {
	var raw userPreviewListDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if !raw.UserPreviews.Present || !raw.UserPreviews.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, preview := range raw.UserPreviews.Items {
		if preview.User.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapUserPreviewList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValue(*raw.NextURL, "offset")
		if err != nil {
			return nil, err
		}
		out.NextOffset, out.ContinuationExists = int(value), true
	}
	return &out, nil
}

func (c *Client) getNovelList(ctx context.Context, path string, query url.Values) (*model.NovelList, error) {
	return c.getNovelListWithContinuationPolicy(ctx, path, query, recommendationOffsetContinuation, false)
}

func (c *Client) getNovelSearchList(ctx context.Context, path string, query url.Values) (*model.NovelList, error) {
	return c.getNovelListWithContinuationPolicy(ctx, path, query, positiveContinuation, true)
}

func (c *Client) getNovelListWithContinuationPolicy(ctx context.Context, path string, query url.Values, policy continuationPolicy, requireSearchFields bool) (*model.NovelList, error) {
	var raw novelListDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if !raw.Novels.Present || !raw.Novels.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, novel := range raw.Novels.Items {
		if novel.ID <= 0 || novel.User.ID <= 0 || requireSearchFields && (novel.XRestrict == nil || novel.TextLength == nil || novel.IsOriginal == nil) {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapNovelList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValueWithPolicy(*raw.NextURL, "offset", policy)
		if err != nil {
			return nil, err
		}
		out.NextOffset, out.ContinuationExists = int(value), true
	}
	return &out, nil
}

func (c *Client) getRecommendedUserList(ctx context.Context, path string, query url.Values) (*model.RecommendedUserList, error) {
	var raw recommendedUserListDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if !raw.UserPreviews.Present || !raw.UserPreviews.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, preview := range raw.UserPreviews.Items {
		if preview.User.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
		for _, illust := range preview.Illusts {
			if illust.ID <= 0 {
				return nil, protocol.MalformedResponse()
			}
		}
		for _, novel := range preview.Novels {
			if novel.ID <= 0 || novel.User.ID <= 0 {
				return nil, protocol.MalformedResponse()
			}
		}
	}
	out := mapRecommendedUserList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValueWithPolicy(*raw.NextURL, "offset", recommendationOffsetContinuation)
		if err != nil {
			return nil, err
		}
		out.NextOffset, out.ContinuationExists = int(value), true
	}
	return &out, nil
}

func applyListContinuation(rawURL *string, key string, policy continuationPolicy, out *model.IllustList) error {
	if rawURL == nil {
		return nil
	}
	if *rawURL == "" {
		return protocol.MalformedResponse()
	}
	value, err := continuationValueWithPolicy(*rawURL, key, policy)
	if err != nil {
		return err
	}
	out.ContinuationExists = true
	if key == "max_bookmark_id" {
		out.NextMaxBookmarkID = value
	} else if key == "last_order" {
		out.NextValue = value
	} else {
		out.NextOffset = int(value)
	}
	return nil
}

// continuationValue 只提取已知数值参数；next_url 永不成为后续请求目标。
func continuationValue(rawURL, key string) (int64, error) {
	return continuationValueWithPolicy(rawURL, key, positiveContinuation)
}

type continuationPolicy uint8

const (
	positiveContinuation continuationPolicy = iota
	recommendationOffsetContinuation
)

func continuationValueWithPolicy(rawURL, key string, policy continuationPolicy) (int64, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values[key]) != 1 {
		return 0, protocol.MalformedResponse()
	}
	value, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || value < 0 || value == 0 && !(policy == recommendationOffsetContinuation && key == "offset") {
		return 0, protocol.MalformedResponse()
	}
	if key == "offset" && int64(int(value)) != value {
		return 0, protocol.MalformedResponse()
	}
	return value, nil
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	var raw ugoiraMetadataResultDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUgoiraMetadata, url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
		return nil, err
	}
	metadata := raw.UgoiraMetadata.Value
	if !raw.UgoiraMetadata.Present || !raw.UgoiraMetadata.Valid || !metadata.ZipURLs.Present || !metadata.ZipURLs.Valid ||
		(metadata.ZipURLs.Value.Medium == "" && metadata.ZipURLs.Value.Original == "") ||
		!metadata.Frames.Present || !metadata.Frames.Valid || len(metadata.Frames.Items) == 0 {
		return nil, protocol.MalformedResponse()
	}
	for _, frame := range metadata.Frames.Items {
		if frame.File == "" {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapUgoiraMetadata(raw)
	return &out, nil
}

type requestOptions struct {
	Headers map[string]string
	Query   url.Values
}

func (c *Client) getJSONWithRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSONWithAuthRetry(ctx, path, query, out)
	if c.disableRetryAfterRetry {
		return err
	}
	retryAfter, shouldRetry := retryAfterForRateLimit(err)
	if !shouldRetry {
		return err
	}
	// 产品契约只允许依据服务端有效 Retry-After 重试一次，避免对读取端点进行猜测性重放。
	if err := waitForRetryAfter(ctx, retryAfter); err != nil {
		return protocol.Transport(err)
	}
	return c.getJSONWithAuthRetry(ctx, path, query, out)
}

func (c *Client) getJSONWithAuthRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSON(ctx, path, query, out)
	if !isAuthAPIResponse(err) {
		return err
	}
	if c.session == nil {
		return err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		// 原始刷新失败可能携带 OAuth 传输细节；保留已脱敏的认证状态失败。
		return err
	}
	return c.getJSON(ctx, path, query, out)
}

func retryAfterForRateLimit(err error) (time.Duration, bool) {
	var failure protocol.Failure
	if !errors.As(err, &failure) || failure.Kind != protocol.FailureHTTPStatus || failure.StatusCode != http.StatusTooManyRequests || !failure.HasRetryAfter {
		return 0, false
	}
	return failure.RetryAfter, true
}

func waitForRetryAfter(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, c.apiBase+path, requestOptions{
		Headers: c.apiHeaders(),
		Query:   query,
	}, out)
}

// AddBookmark 收藏作品。成功响应可为空，因此不走 JSON 解码路径。
func (c *Client) AddBookmark(ctx context.Context, illustID int64, restrict string, tags []string) error {
	form := url.Values{"illust_id": {fmt.Sprint(illustID)}, "restrict": {restrict}}
	for _, tag := range tags {
		form.Add("tags[]", tag)
	}
	return c.postFormWithRetry(ctx, protocol.AppBookmarkAdd, form)
}

// RemoveBookmark 取消收藏作品。
func (c *Client) RemoveBookmark(ctx context.Context, illustID int64) error {
	return c.postFormWithRetry(ctx, protocol.AppBookmarkDelete, url.Values{"illust_id": {fmt.Sprint(illustID)}})
}

// FollowUser 关注用户。
func (c *Client) FollowUser(ctx context.Context, userID int64, restrict string) error {
	return c.postFormWithRetry(ctx, protocol.AppFollowAdd, url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}})
}

// UnfollowUser 取消关注用户。
func (c *Client) UnfollowUser(ctx context.Context, userID int64) error {
	return c.postFormWithRetry(ctx, protocol.AppFollowDelete, url.Values{"user_id": {fmt.Sprint(userID)}})
}

func (c *Client) postFormWithRetry(ctx context.Context, path string, form url.Values) error {
	err := c.postForm(ctx, path, form)
	if !isAuthAPIResponse(err) {
		return err
	}
	if c.session == nil {
		return err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		return err
	}
	return c.postForm(ctx, path, form)
}

// isAuthAPIResponse 仅识别明确的认证 HTTP 状态，避免响应正文中的词汇触发 mutation 重放。
func isAuthAPIResponse(err error) bool {
	var apiErr protocol.Failure
	return errors.As(err, &apiErr) && apiErr.Kind == protocol.FailureHTTPStatus && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) error {
	return c.doForm(ctx, http.MethodPost, c.apiBase+path, requestOptions{Headers: c.apiHeaders()}, form)
}

func (c *Client) apiHeaders() map[string]string {
	token := ""
	if c.session != nil {
		token = c.session.AccessToken()
	}
	headers := protocol.AppHeaders(token)
	if c.acceptLanguage != "" {
		headers["Accept-Language"] = c.acceptLanguage
	}
	if c.userID > 0 {
		headers["X-User-Id"] = strconv.FormatInt(c.userID, 10)
	}
	return headers
}

func (c *Client) doJSON(ctx context.Context, method, rawURL string, opts requestOptions, out any) error {
	req := c.restyClient.R().SetContext(ctx)
	if len(opts.Headers) > 0 {
		req.SetHeaders(opts.Headers)
	}
	if len(opts.Query) > 0 {
		req.SetQueryParamsFromValues(opts.Query)
	}
	resp, err := req.Execute(method, rawURL)
	if err != nil {
		return protocol.Transport(err)
	}
	body := resp.Body()
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header().Get("Retry-After"), time.Now())
		return protocol.HTTPStatusWithRetryAfter(resp.StatusCode(), retryAfter, present)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return protocol.MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return protocol.MalformedResponse()
	}
	return nil
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		// time.Duration 以 int64 纳秒表示；不能表达的服务端秒数若乘法溢出会
		// 伪装成负等待，必须按无效 Retry-After 保留原始限流错误。
		const maxDurationSeconds = int64(1<<63-1) / int64(time.Second)
		if seconds < 0 || seconds > maxDurationSeconds {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	duration := when.Sub(now)
	if duration < 0 {
		duration = 0
	}
	return duration, true
}

// doForm 保留所有 2xx 成功状态；mutation endpoint 不保证返回 JSON body。
func (c *Client) doForm(ctx context.Context, method, rawURL string, opts requestOptions, form url.Values) error {
	req := c.restyClient.R().SetContext(ctx).SetFormDataFromValues(form)
	if len(opts.Headers) > 0 {
		req.SetHeaders(opts.Headers)
	}
	resp, err := req.Execute(method, rawURL)
	if err != nil {
		return protocol.Transport(err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return protocol.HTTPStatus(resp.StatusCode())
	}
	return nil
}

// APIError 保留内部兼容名称；实际失败统一由 protocol.Failure 脱敏表示。
type APIError = protocol.Failure

func setOptional(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func setOffset(q url.Values, offset int) {
	if offset > 0 {
		q.Set("offset", fmt.Sprint(offset))
	}
}

func setRecommendationOffset(q url.Values, offset int, continuationExists bool) {
	if continuationExists {
		q.Set("offset", fmt.Sprint(offset))
		return
	}
	setOffset(q, offset)
}
