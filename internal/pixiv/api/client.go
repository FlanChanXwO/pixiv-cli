package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultAPIBase           = "https://app-api.pixiv.net"
	DefaultOAuthBase         = "https://oauth.secure.pixiv.net"
	DefaultOAuthClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	DefaultOAuthClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	DefaultUserAgent         = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	DefaultAppOS             = "android"
	DefaultAppOSVersion      = "11"
	DefaultAppVersion        = "5.0.234"
)

type Client struct {
	restyClient  *resty.Client
	apiBase      string
	oauthBase    string
	refreshToken string
	accessToken  string
	userID       int64
	userName     string
	mu           sync.RWMutex
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.restyClient = resty.NewWithClient(httpClient)
		}
	}
}

func WithBaseURLs(apiBase, oauthBase string) Option {
	return func(c *Client) {
		if apiBase != "" {
			c.apiBase = strings.TrimRight(apiBase, "/")
		}
		if oauthBase != "" {
			c.oauthBase = strings.TrimRight(oauthBase, "/")
		}
	}
}

// WithAccessToken 注入已取得的 access token，供无需刷新流程的 SDK 调用复用 App API。
func WithAccessToken(token string) Option {
	return func(c *Client) {
		c.accessToken = strings.TrimSpace(token)
	}
}

func New(refreshToken string, opts ...Option) *Client {
	refreshToken, _ = utils.ParsePixivWebRefreshTokenInput(refreshToken)
	c := &Client{
		restyClient:  resty.New(),
		apiBase:      DefaultAPIBase,
		oauthBase:    DefaultOAuthBase,
		refreshToken: refreshToken,
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

func (c *Client) SetRefreshToken(token string) {
	token, _ = utils.ParsePixivWebRefreshTokenInput(token)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshToken = strings.TrimSpace(token)
}

func (c *Client) RefreshTokenValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.refreshToken
}

func (c *Client) UserID() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userID
}

func (c *Client) UserName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userName
}

func (c *Client) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessToken != ""
}

func (c *Client) Refresh(ctx context.Context) error {
	c.mu.RLock()
	refreshToken := c.refreshToken
	c.mu.RUnlock()
	if refreshToken == "" {
		return errors.New("missing PIXIV_REFRESH_TOKEN")
	}

	form := map[string]string{
		"client_id":      DefaultOAuthClientID,
		"client_secret":  DefaultOAuthClientSecret,
		"grant_type":     "refresh_token",
		"include_policy": "true",
		"refresh_token":  refreshToken,
	}

	var result authResponse
	if err := c.doJSON(ctx, http.MethodPost, c.oauthBase+"/auth/token", requestOptions{
		Headers: oauthHeaders(),
		Form:    form,
	}, &result); err != nil {
		return err
	}
	if result.Response.AccessToken == "" && result.AccessToken == "" {
		return errors.New("token refresh response did not include access_token")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if result.Response.AccessToken != "" {
		c.accessToken = result.Response.AccessToken
		if result.Response.RefreshToken != "" {
			c.refreshToken = result.Response.RefreshToken
		}
		c.userID = int64(result.Response.User.ID)
		c.userName = result.Response.User.Name
		return nil
	}
	c.accessToken = result.AccessToken
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}
	c.userID = int64(result.User.ID)
	c.userName = result.User.Name
	return nil
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*IllustList, error) {
	q := url.Values{"word": {word}, "search_target": {target}, "sort": {sort}}
	setOptional(q, "duration", duration)
	setOffset(q, offset)
	return getJSON[IllustList](ctx, c, "/v1/search/illust", q)
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*IllustDetail, error) {
	return getJSON[IllustDetail](ctx, c, "/v1/illust/detail", url.Values{"illust_id": {fmt.Sprint(id)}})
}

func (c *Client) IllustRelated(ctx context.Context, id int64, offset int) (*IllustList, error) {
	q := url.Values{"illust_id": {fmt.Sprint(id)}}
	setOffset(q, offset)
	return getJSON[IllustList](ctx, c, "/v2/illust/related", q)
}

func (c *Client) IllustRanking(ctx context.Context, mode, date string, offset int) (*IllustList, error) {
	q := url.Values{"mode": {mode}}
	setOptional(q, "date", date)
	setOffset(q, offset)
	return getJSON[IllustList](ctx, c, "/v1/illust/ranking", q)
}

func (c *Client) SearchUser(ctx context.Context, word string, offset int) (*UserPreviewList, error) {
	q := url.Values{"word": {word}}
	setOffset(q, offset)
	return getJSON[UserPreviewList](ctx, c, "/v1/search/user", q)
}

func (c *Client) UserDetail(ctx context.Context, userID int64) (*User, error) {
	result, err := getJSON[userDetailResult](ctx, c, "/v1/user/detail", url.Values{"user_id": {fmt.Sprint(userID)}})
	if err != nil {
		return nil, err
	}
	return &result.User, nil
}

func (c *Client) IllustRecommended(ctx context.Context, offset int) (*IllustList, error) {
	q := url.Values{}
	setOffset(q, offset)
	return getJSON[IllustList](ctx, c, "/v1/illust/recommended", q)
}

func (c *Client) TrendingTagsIllust(ctx context.Context) (*TrendTags, error) {
	return getJSON[TrendTags](ctx, c, "/v1/trending-tags/illust", nil)
}

func (c *Client) IllustFollow(ctx context.Context, restrict string, offset int) (*IllustList, error) {
	q := url.Values{"restrict": {restrict}}
	setOffset(q, offset)
	return getJSON[IllustList](ctx, c, "/v2/illust/follow", q)
}

func (c *Client) UserBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*IllustList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOptional(q, "tag", tag)
	if maxBookmarkID > 0 {
		q.Set("max_bookmark_id", fmt.Sprint(maxBookmarkID))
	}
	return getJSON[IllustList](ctx, c, "/v1/user/bookmarks/illust", q)
}

func (c *Client) UserFollowing(ctx context.Context, userID int64, restrict string, offset int) (*UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	return getJSON[UserPreviewList](ctx, c, "/v1/user/following", q)
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*UgoiraMetadataResult, error) {
	return getJSON[UgoiraMetadataResult](ctx, c, "/v1/ugoira/metadata", url.Values{"illust_id": {fmt.Sprint(id)}})
}

func (c *Client) Download(ctx context.Context, rawURL string, dst io.Writer) error {
	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetHeader("Referer", "https://app-api.pixiv.net/").
		SetHeader("User-Agent", DefaultUserAgent).
		Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.RawBody().Close()
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status())
	}
	_, err = io.Copy(dst, resp.RawBody())
	return err
}

type requestOptions struct {
	Headers map[string]string
	Query   url.Values
	Form    map[string]string
}

func getJSON[T any](ctx context.Context, c *Client, path string, query url.Values) (*T, error) {
	var out T
	if err := c.getJSONWithRetry(ctx, path, query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) getJSONWithRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSON(ctx, path, query, out)
	if !isAuthError(err) {
		return err
	}
	if refreshErr := c.Refresh(ctx); refreshErr != nil {
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
	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

func oauthHeaders() map[string]string {
	headers := baseHeaders()
	headers["Content-Type"] = "application/x-www-form-urlencoded"
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
	if len(opts.Form) > 0 {
		req.SetFormData(opts.Form)
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
		return errors.New("empty response")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
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

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         authUser
	Response     struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         authUser
	} `json:"response"`
}

type authUser struct {
	ID   jsonInt64 `json:"id"`
	Name string    `json:"name"`
}

type userDetailResult struct {
	User User `json:"user"`
}

type jsonInt64 int64

func (i *jsonInt64) UnmarshalJSON(body []byte) error {
	var n int64
	if err := json.Unmarshal(body, &n); err == nil {
		*i = jsonInt64(n)
		return nil
	}
	var text string
	if err := json.Unmarshal(body, &text); err != nil {
		return err
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*i = jsonInt64(n)
	return nil
}
