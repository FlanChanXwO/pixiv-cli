package pixiv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
)

const (
	DefaultAPIBase           = "https://app-api.pixiv.net"
	DefaultOAuthBase         = "https://oauth.secure.pixiv.net"
	DefaultOAuthClientID     = "MOBrBDSOnz6cTIM6GAl6Ytjj"
	DefaultOAuthClientSecret = "lsyM0L2M6vWypx4Y"
	DefaultUserAgent         = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	DefaultAppOS             = "android"
	DefaultAppOSVersion      = "11"
	DefaultAppVersion        = "5.0.234"
)

type Client struct {
	httpClient   *http.Client
	apiBase      string
	oauthBase    string
	refreshToken string
	accessToken  string
	userID       int64
	mu           sync.RWMutex
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
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

func New(refreshToken string, opts ...Option) *Client {
	refreshToken, _ = pixivutil.ParseRefreshTokenInput(refreshToken)
	c := &Client{
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		apiBase:      DefaultAPIBase,
		oauthBase:    DefaultOAuthBase,
		refreshToken: refreshToken,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) SetRefreshToken(token string) {
	token, _ = pixivutil.ParseRefreshTokenInput(token)
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

	form := url.Values{}
	form.Set("client_id", DefaultOAuthClientID)
	form.Set("client_secret", DefaultOAuthClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("include_policy", "true")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthBase+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("App-OS", DefaultAppOS)
	req.Header.Set("App-OS-Version", DefaultAppOSVersion)
	req.Header.Set("App-Version", DefaultAppVersion)

	var result authResponse
	if err := c.doJSON(req, &result); err != nil {
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
		c.userID = result.Response.User.ID
		return nil
	}
	c.accessToken = result.AccessToken
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}
	c.userID = result.User.ID
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", "https://app-api.pixiv.net/")
	req.Header.Set("User-Agent", DefaultUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
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
	u := c.apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	c.addAPIHeaders(req)
	return c.doJSON(req, out)
}

func (c *Client) addAPIHeaders(req *http.Request) {
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("App-OS", DefaultAppOS)
	req.Header.Set("App-OS-Version", DefaultAppOSVersion)
	req.Header.Set("App-Version", DefaultAppVersion)
	req.Header.Set("Referer", "https://app-api.pixiv.net/")
	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{StatusCode: resp.StatusCode, Body: string(body)}
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
	User         User   `json:"user"`
	Response     struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         User   `json:"user"`
	} `json:"response"`
}
