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

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultAPIBase      = protocol.AppAPIBase
	DefaultUserAgent    = protocol.AppUserAgent
	DefaultAppOS        = protocol.AppOS
	DefaultAppOSVersion = protocol.AppOSVersion
	DefaultAppVersion   = protocol.AppVersion
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

type Client struct {
	restyClient *resty.Client
	apiBase     string
	session     Session
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

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*model.IllustList, error) {
	q := url.Values{"word": {word}, "search_target": {target}, "sort": {sort}}
	setOptional(q, "duration", duration)
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppSearchIllust, q, "offset")
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	var raw illustDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppIllustDetail, url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
		return nil, err
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return nil, ErrMalformedResponse
	}
	out := mapIllustDetail(raw)
	return &out, nil
}

func (c *Client) IllustRelated(ctx context.Context, id int64, offset int) (*model.IllustList, error) {
	q := url.Values{"illust_id": {fmt.Sprint(id)}}
	setOffset(q, offset)
	return c.getIllustList(ctx, protocol.AppIllustRelated, q, "offset")
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
		return nil, ErrMalformedResponse
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
		return nil, ErrMalformedResponse
	}
	for _, item := range raw.TrendTags.Items {
		if item.Tag == "" || !item.Illust.Present || !item.Illust.Valid || item.Illust.Value.ID <= 0 {
			return nil, ErrMalformedResponse
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
		return nil, ErrMalformedResponse
	}
	for _, illust := range raw.Illusts.Items {
		if illust.ID <= 0 {
			return nil, ErrMalformedResponse
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
		return nil, ErrMalformedResponse
	}
	for _, preview := range raw.UserPreviews.Items {
		if preview.User.ID <= 0 {
			return nil, ErrMalformedResponse
		}
	}
	out := mapUserPreviewList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, ErrMalformedResponse
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
	var raw novelListDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if !raw.Novels.Present || !raw.Novels.Valid {
		return nil, ErrMalformedResponse
	}
	for _, novel := range raw.Novels.Items {
		if novel.ID <= 0 || novel.User.ID <= 0 {
			return nil, ErrMalformedResponse
		}
	}
	out := mapNovelList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, ErrMalformedResponse
		}
		value, err := continuationValueWithPolicy(*raw.NextURL, "offset", recommendationOffsetContinuation)
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
		return nil, ErrMalformedResponse
	}
	for _, preview := range raw.UserPreviews.Items {
		if preview.User.ID <= 0 {
			return nil, ErrMalformedResponse
		}
		for _, illust := range preview.Illusts {
			if illust.ID <= 0 {
				return nil, ErrMalformedResponse
			}
		}
		for _, novel := range preview.Novels {
			if novel.ID <= 0 || novel.User.ID <= 0 {
				return nil, ErrMalformedResponse
			}
		}
	}
	out := mapRecommendedUserList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, ErrMalformedResponse
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
		return ErrMalformedResponse
	}
	value, err := continuationValueWithPolicy(*rawURL, key, policy)
	if err != nil {
		return err
	}
	out.ContinuationExists = true
	if key == "max_bookmark_id" {
		out.NextMaxBookmarkID = value
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
		return 0, ErrMalformedResponse
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values[key]) != 1 {
		return 0, ErrMalformedResponse
	}
	value, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || value < 0 || value == 0 && !(policy == recommendationOffsetContinuation && key == "offset") {
		return 0, ErrMalformedResponse
	}
	if key == "offset" && int64(int(value)) != value {
		return 0, ErrMalformedResponse
	}
	return value, nil
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	var raw ugoiraMetadataResultDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUgoiraMetadata, url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
		return nil, err
	}
	metadata := raw.UgoiraMetadata.Value
	if !raw.UgoiraMetadata.Present || !raw.UgoiraMetadata.Valid || !metadata.ZipURLs.Present || !metadata.ZipURLs.Valid || metadata.ZipURLs.Value.Medium == "" ||
		!metadata.Frames.Present || !metadata.Frames.Valid || len(metadata.Frames.Items) == 0 {
		return nil, ErrMalformedResponse
	}
	for _, frame := range metadata.Frames.Items {
		if frame.File == "" {
			return nil, ErrMalformedResponse
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
	return protocol.AppHeaders(token)
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
		return protocol.HTTPStatus(resp.StatusCode())
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return protocol.MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return protocol.MalformedResponse()
	}
	return nil
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
