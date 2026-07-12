package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	return getMapped(ctx, c, "/v1/search/illust", q, mapIllustList)
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	return getMapped(ctx, c, "/v1/illust/detail", url.Values{"illust_id": {fmt.Sprint(id)}}, mapIllustDetail)
}

func (c *Client) IllustRelated(ctx context.Context, id int64, offset int) (*model.IllustList, error) {
	q := url.Values{"illust_id": {fmt.Sprint(id)}}
	setOffset(q, offset)
	return getMapped(ctx, c, "/v2/illust/related", q, mapIllustList)
}

func (c *Client) IllustRanking(ctx context.Context, mode, date string, offset int) (*model.IllustList, error) {
	q := url.Values{"mode": {mode}}
	setOptional(q, "date", date)
	setOffset(q, offset)
	return getMapped(ctx, c, "/v1/illust/ranking", q, mapIllustList)
}

func (c *Client) SearchUser(ctx context.Context, word string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"word": {word}}
	setOffset(q, offset)
	return getMapped(ctx, c, "/v1/search/user", q, mapUserPreviewList)
}

func (c *Client) UserDetail(ctx context.Context, userID int64) (*model.User, error) {
	return getMapped(ctx, c, "/v1/user/detail", url.Values{"user_id": {fmt.Sprint(userID)}}, func(dto userDetailDTO) model.User { return mapUser(dto.User) })
}

func (c *Client) IllustRecommended(ctx context.Context, offset int) (*model.IllustList, error) {
	q := url.Values{}
	setOffset(q, offset)
	return getMapped(ctx, c, "/v1/illust/recommended", q, mapIllustList)
}

func (c *Client) TrendingTagsIllust(ctx context.Context) (*model.TrendTags, error) {
	return getMapped(ctx, c, "/v1/trending-tags/illust", nil, mapTrendTags)
}

func (c *Client) IllustFollow(ctx context.Context, restrict string, offset int) (*model.IllustList, error) {
	q := url.Values{"restrict": {restrict}}
	setOffset(q, offset)
	return getMapped(ctx, c, "/v2/illust/follow", q, mapIllustList)
}

func (c *Client) UserBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*model.IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOptional(q, "tag", tag)
	if maxBookmarkID > 0 {
		q.Set("max_bookmark_id", fmt.Sprint(maxBookmarkID))
	}
	return getMapped(ctx, c, "/v1/user/bookmarks/illust", q, mapIllustList)
}

func (c *Client) UserFollowing(ctx context.Context, userID int64, restrict string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	return getMapped(ctx, c, "/v1/user/following", q, mapUserPreviewList)
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	return getMapped(ctx, c, "/v1/ugoira/metadata", url.Values{"illust_id": {fmt.Sprint(id)}}, mapUgoiraMetadata)
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
