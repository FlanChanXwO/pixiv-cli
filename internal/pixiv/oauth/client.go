// Package oauth 管理 Pixiv 身份状态与 OAuth token 交换，不承载内容 API。
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultBase         = "https://oauth.secure.pixiv.net"
	DefaultClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	DefaultClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	DefaultRedirectURI  = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback"
	defaultUserAgent    = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
)

type Client struct {
	restyClient  *resty.Client
	baseURL      string
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

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

func WithAccessToken(token string) Option {
	return func(c *Client) { c.accessToken = strings.TrimSpace(token) }
}

func New(refreshToken string, opts ...Option) *Client {
	refreshToken, _ = utils.ParsePixivWebRefreshTokenInput(refreshToken)
	c := &Client{restyClient: resty.New(), baseURL: DefaultBase, refreshToken: refreshToken}
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
	c.refreshToken = strings.TrimSpace(token)
	c.mu.Unlock()
}

func (c *Client) RefreshTokenValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.refreshToken
}
func (c *Client) AccessToken() string   { c.mu.RLock(); defer c.mu.RUnlock(); return c.accessToken }
func (c *Client) UserID() int64         { c.mu.RLock(); defer c.mu.RUnlock(); return c.userID }
func (c *Client) UserName() string      { c.mu.RLock(); defer c.mu.RUnlock(); return c.userName }
func (c *Client) IsAuthenticated() bool { return c.AccessToken() != "" }

func (c *Client) Refresh(ctx context.Context) error {
	c.mu.RLock()
	refreshToken := c.refreshToken
	c.mu.RUnlock()
	if refreshToken == "" {
		return errors.New("missing PIXIV_REFRESH_TOKEN")
	}
	result, err := c.exchange(ctx, map[string]string{
		"client_id": DefaultClientID, "client_secret": DefaultClientSecret,
		"grant_type": "refresh_token", "include_policy": "true", "refresh_token": refreshToken,
	})
	if err != nil {
		return err
	}
	token := tokenFromResponse(result)
	if token.AccessToken == "" {
		return errors.New("token refresh response did not include access_token")
	}
	c.store(token)
	return nil
}

type AuthCodeToken struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
	Username     string
}

func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code, verifier string) (AuthCodeToken, error) {
	code, verifier = strings.TrimSpace(code), strings.TrimSpace(verifier)
	if code == "" {
		return AuthCodeToken{}, errors.New("authorization code cannot be empty")
	}
	if verifier == "" {
		return AuthCodeToken{}, errors.New("code verifier cannot be empty")
	}
	result, err := c.exchange(ctx, map[string]string{
		"client_id": DefaultClientID, "client_secret": DefaultClientSecret,
		"grant_type": "authorization_code", "include_policy": "true",
		"code": code, "code_verifier": verifier, "redirect_uri": DefaultRedirectURI,
	})
	if err != nil {
		return AuthCodeToken{}, err
	}
	token := tokenFromResponse(result)
	if token.RefreshToken == "" {
		return AuthCodeToken{}, errors.New("authorization_code response did not include refresh_token")
	}
	c.store(token)
	return token, nil
}

func (c *Client) exchange(ctx context.Context, form map[string]string) (authResponse, error) {
	resp, err := c.restyClient.R().SetContext(ctx).SetHeaders(oauthHeaders()).SetFormData(form).Post(c.baseURL + "/auth/token")
	if err != nil {
		return authResponse{}, err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return authResponse{}, fmt.Errorf("pixiv oauth error: status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	if len(bytes.TrimSpace(resp.Body())) == 0 {
		return authResponse{}, errors.New("empty response")
	}
	var result authResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return authResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func (c *Client) store(token AuthCodeToken) {
	c.mu.Lock()
	c.accessToken = token.AccessToken
	if token.RefreshToken != "" {
		c.refreshToken = token.RefreshToken
	}
	c.userID, c.userName = token.UserID, token.Username
	c.mu.Unlock()
}

func oauthHeaders() map[string]string {
	return map[string]string{
		"User-Agent": defaultUserAgent, "App-OS": "android", "App-OS-Version": "11",
		"App-Version": "5.0.234", "Content-Type": "application/x-www-form-urlencoded",
	}
}

type authResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         authUser `json:"user"`
	Response     struct {
		AccessToken  string   `json:"access_token"`
		RefreshToken string   `json:"refresh_token"`
		User         authUser `json:"user"`
	} `json:"response"`
}
type authUser struct {
	ID   jsonInt64 `json:"id"`
	Name string    `json:"name"`
}
type jsonInt64 int64

func (i *jsonInt64) UnmarshalJSON(body []byte) error {
	var n int64
	if json.Unmarshal(body, &n) == nil {
		*i = jsonInt64(n)
		return nil
	}
	var value string
	if err := json.Unmarshal(body, &value); err != nil {
		return err
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	*i = jsonInt64(n)
	return nil
}
func tokenFromResponse(r authResponse) AuthCodeToken {
	if r.Response.AccessToken != "" || r.Response.RefreshToken != "" || r.Response.User.ID != 0 {
		return AuthCodeToken{r.Response.AccessToken, r.Response.RefreshToken, int64(r.Response.User.ID), r.Response.User.Name}
	}
	return AuthCodeToken{r.AccessToken, r.RefreshToken, int64(r.User.ID), r.User.Name}
}
