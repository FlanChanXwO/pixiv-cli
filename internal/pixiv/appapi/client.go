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

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultAPIBase      = "https://app-api.pixiv.net"
	DefaultUserAgent    = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	DefaultAppOS        = "android"
	DefaultAppOSVersion = "11"
	DefaultAppVersion   = "5.0.234"
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = errors.New("app api returned a malformed response")

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
	c.restyClient.SetTimeout(60 * time.Second)
	for _, opt := range opts {
		opt(c)
	}
	if c.restyClient == nil {
		c.restyClient = resty.New()
		c.restyClient.SetTimeout(60 * time.Second)
	}
	return c
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*model.IllustList, error) {
	q := url.Values{"word": {word}, "search_target": {target}, "sort": {sort}}
	setOptional(q, "duration", duration)
	setOffset(q, offset)
	return c.getIllustList(ctx, "/v1/search/illust", q, "offset")
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	var raw illustDetailDTO
	if err := c.getJSONWithRetry(ctx, "/v1/illust/detail", url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
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
	return c.getIllustList(ctx, "/v2/illust/related", q, "offset")
}

func (c *Client) IllustRanking(ctx context.Context, mode, date string, offset int) (*model.IllustList, error) {
	q := url.Values{"mode": {mode}}
	setOptional(q, "date", date)
	setOffset(q, offset)
	return c.getIllustList(ctx, "/v1/illust/ranking", q, "offset")
}

func (c *Client) SearchUser(ctx context.Context, word string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"word": {word}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, "/v1/search/user", q)
}

func (c *Client) UserDetail(ctx context.Context, userID int64) (*model.User, error) {
	var raw userDetailDTO
	if err := c.getJSONWithRetry(ctx, "/v1/user/detail", url.Values{"user_id": {fmt.Sprint(userID)}}, &raw); err != nil {
		return nil, err
	}
	if raw.User == nil || raw.User.ID <= 0 {
		return nil, ErrMalformedResponse
	}
	out := mapUser(*raw.User)
	return &out, nil
}

func (c *Client) IllustRecommended(ctx context.Context, offset int) (*model.IllustList, error) {
	q := url.Values{}
	setOffset(q, offset)
	return c.getIllustList(ctx, "/v1/illust/recommended", q, "offset")
}

func (c *Client) TrendingTagsIllust(ctx context.Context) (*model.TrendTags, error) {
	var raw trendTagsDTO
	if err := c.getJSONWithRetry(ctx, "/v1/trending-tags/illust", nil, &raw); err != nil {
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
	return c.getIllustList(ctx, "/v2/illust/follow", q, "offset")
}

// UserArtworks 返回用户作品的单个 App API 批次。
func (c *Client) UserArtworks(ctx context.Context, userID int64, illustType string, offset int) (*model.IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "type": {illustType}}
	setOffset(q, offset)
	return c.getIllustList(ctx, "/v1/user/illusts", q, "offset")
}

func (c *Client) UserBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*model.IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOptional(q, "tag", tag)
	if maxBookmarkID > 0 {
		q.Set("max_bookmark_id", fmt.Sprint(maxBookmarkID))
	}
	return c.getIllustList(ctx, "/v1/user/bookmarks/illust", q, "max_bookmark_id")
}

func (c *Client) UserFollowing(ctx context.Context, userID int64, restrict string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, "/v1/user/following", q)
}

func (c *Client) getIllustList(ctx context.Context, path string, query url.Values, continuationKey string) (*model.IllustList, error) {
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
	if err := applyListContinuation(raw.NextURL, continuationKey, &out); err != nil {
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

func applyListContinuation(rawURL *string, key string, out *model.IllustList) error {
	if rawURL == nil {
		return nil
	}
	if *rawURL == "" {
		return ErrMalformedResponse
	}
	value, err := continuationValue(*rawURL, key)
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
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, ErrMalformedResponse
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values[key]) != 1 {
		return 0, ErrMalformedResponse
	}
	value, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrMalformedResponse
	}
	if key == "offset" && int64(int(value)) != value {
		return 0, ErrMalformedResponse
	}
	return value, nil
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	var raw ugoiraMetadataResultDTO
	if err := c.getJSONWithRetry(ctx, "/v1/ugoira/metadata", url.Values{"illust_id": {fmt.Sprint(id)}}, &raw); err != nil {
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

func getMapped[Raw, Out any](ctx context.Context, c *Client, path string, query url.Values, mapper func(Raw) Out) (*Out, error) {
	var raw Raw
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	out := mapper(raw)
	return &out, nil
}

func (c *Client) getJSONWithRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSON(ctx, path, query, out)
	if !isAuthError(err) {
		return err
	}
	if c.session == nil {
		return err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		return fmt.Errorf("%w; token refresh failed: %v", err, refreshErr)
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
	return c.postFormWithRetry(ctx, "/v2/illust/bookmark/add", form)
}

// RemoveBookmark 取消收藏作品。
func (c *Client) RemoveBookmark(ctx context.Context, illustID int64) error {
	return c.postFormWithRetry(ctx, "/v1/illust/bookmark/delete", url.Values{"illust_id": {fmt.Sprint(illustID)}})
}

// FollowUser 关注用户。
func (c *Client) FollowUser(ctx context.Context, userID int64, restrict string) error {
	return c.postFormWithRetry(ctx, "/v1/user/follow/add", url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}})
}

// UnfollowUser 取消关注用户。
func (c *Client) UnfollowUser(ctx context.Context, userID int64) error {
	return c.postFormWithRetry(ctx, "/v1/user/follow/delete", url.Values{"user_id": {fmt.Sprint(userID)}})
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
		return fmt.Errorf("%w; token refresh failed: %v", err, refreshErr)
	}
	return c.postForm(ctx, path, form)
}

// isAuthAPIResponse 仅识别明确的认证 HTTP 状态，避免响应正文中的词汇触发 mutation 重放。
func isAuthAPIResponse(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) error {
	return c.doForm(ctx, http.MethodPost, c.apiBase+path, requestOptions{Headers: c.apiHeaders()}, form)
}

func (c *Client) apiHeaders() map[string]string {
	headers := baseHeaders()
	headers["Referer"] = "https://app-api.pixiv.net/"
	token := ""
	if c.session != nil {
		token = c.session.AccessToken()
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

func baseHeaders() map[string]string {
	return map[string]string{
		"User-Agent":     DefaultUserAgent,
		"App-OS":         DefaultAppOS,
		"App-OS-Version": DefaultAppOSVersion,
		"App-Version":    DefaultAppVersion,
	}
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
		return err
	}
	body := resp.Body()
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return APIError{StatusCode: resp.StatusCode(), Body: string(body)}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return ErrMalformedResponse
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedResponse, err)
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
		return err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return APIError{StatusCode: resp.StatusCode(), Body: string(resp.Body())}
	}
	return nil
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("pixiv api error: status %d", e.StatusCode)
	}
	return fmt.Sprintf("pixiv api error: status %d: %s", e.StatusCode, e.Body)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "invalid_grant") || strings.Contains(text, "oauth") || strings.Contains(text, "unauthorized")
}

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
