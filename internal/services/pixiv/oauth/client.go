// Package oauth 管理 Pixiv 身份状态与 OAuth token 交换，不承载内容 API。
package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultBase         = protocol.OAuthBase
	DefaultClientID     = protocol.OAuthClientID
	DefaultClientSecret = protocol.OAuthClientSecret
	DefaultRedirectURI  = protocol.OAuthRedirectURI
	defaultUserAgent    = protocol.AppUserAgent
)

// ErrMalformedResponse 标识 OAuth 成功状态码却未提供可用 token payload。
// 它不携带原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

type Client struct {
	restyClient  *resty.Client
	baseURL      string
	refreshToken string
	// refreshTokenErr 只保存本地输入校验错误；Refresh 会直接返回它，确保 Cookie
	// 不会因被清空而伪装成普通的“缺少 token”错误。
	refreshTokenErr error
	accessToken     string
	userID          int64
	userName        string
	expiresAt       time.Time
	mu              sync.RWMutex
}

// APIError 是 OAuth 上游状态错误。它故意不保留响应体，避免调用方把
// OAuth 诊断内容（其中可能含 token 或 code）写入日志。
type APIError = protocol.Failure

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
	refreshToken, refreshTokenErr := credentials.ValidateRefreshTokenInput(refreshToken)
	c := &Client{restyClient: resty.New(), baseURL: DefaultBase, refreshToken: refreshToken, refreshTokenErr: refreshTokenErr}
	for _, opt := range opts {
		opt(c)
	}
	if c.restyClient == nil {
		c.restyClient = resty.New()
	}
	return c
}

func (c *Client) SetRefreshToken(token string) {
	token, inputErr := credentials.ValidateRefreshTokenInput(token)
	c.mu.Lock()
	c.refreshToken = strings.TrimSpace(token)
	c.refreshTokenErr = inputErr
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

// Expiry 返回 access token 的过期时间；上游未提供或已过期时为零值。
func (c *Client) Expiry() time.Time { c.mu.RLock(); defer c.mu.RUnlock(); return c.expiresAt }

func (c *Client) Refresh(ctx context.Context) error {
	c.mu.RLock()
	refreshToken := c.refreshToken
	refreshTokenErr := c.refreshTokenErr
	c.mu.RUnlock()
	if refreshTokenErr != nil {
		return refreshTokenErr
	}
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
		return protocol.MalformedResponse()
	}
	c.store(token)
	return nil
}

type AuthCodeToken struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
	Username     string
	ExpiresAt    time.Time
}

// GeneratePKCEPair 使用 OAuth S256 所需的随机 verifier/challenge 对。
func GeneratePKCEPair() (verifier, challenge string, err error) {
	verifier, err = RandomURLToken(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// RandomURLToken 返回 URL-safe、不可预测的随机 token。
func RandomURLToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
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
		return AuthCodeToken{}, protocol.MalformedResponse()
	}
	c.store(token)
	return token, nil
}

func (c *Client) exchange(ctx context.Context, form map[string]string) (authResponse, error) {
	resp, err := c.restyClient.R().SetContext(ctx).SetHeaders(oauthHeaders()).SetFormData(form).Post(c.baseURL + protocol.OAuthToken)
	if err != nil {
		return authResponse{}, protocol.Transport(err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return authResponse{}, protocol.HTTPStatus(resp.StatusCode())
	}
	if len(bytes.TrimSpace(resp.Body())) == 0 {
		return authResponse{}, protocol.MalformedResponse()
	}
	var result authResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return authResponse{}, protocol.MalformedResponse()
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
	c.expiresAt = token.ExpiresAt
	c.mu.Unlock()
}

func oauthHeaders() map[string]string {
	return protocol.OAuthHeaders()
}

type authResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	User         authUser `json:"user"`
	Response     struct {
		AccessToken  string   `json:"access_token"`
		RefreshToken string   `json:"refresh_token"`
		ExpiresIn    int64    `json:"expires_in"`
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
	expiry := func(expiresIn int64) time.Time {
		if expiresIn <= 0 {
			return time.Time{}
		}
		return time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	if r.Response.AccessToken != "" || r.Response.RefreshToken != "" || r.Response.User.ID != 0 {
		return AuthCodeToken{r.Response.AccessToken, r.Response.RefreshToken, int64(r.Response.User.ID), r.Response.User.Name, expiry(r.Response.ExpiresIn)}
	}
	return AuthCodeToken{r.AccessToken, r.RefreshToken, int64(r.User.ID), r.User.Name, expiry(r.ExpiresIn)}
}
