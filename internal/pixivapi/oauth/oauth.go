// Package oauth 管理 Pixiv 身份状态与 OAuth token 交换，不承载内容 API。
package oauth

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
)

type Client struct {
	httpClient   *http.Client
	ownsHTTP     bool
	refreshToken string
	// refreshTokenErr 只保存本地输入校验错误；Refresh 会直接返回它，确保 Cookie
	// 不会因被清空而伪装成普通的“缺少 token”错误。
	refreshTokenErr error
	accessToken     string
	userID          int64
	userName        string
	expiresAt       time.Time
	log             *slog.Logger
	mu              sync.RWMutex
}

// Config 是 OAuth 客户端的连接配置；零值使用默认 HTTP client。
type Config struct {
	HTTPClient *http.Client
	// OwnHTTPClient marks a client created for this OAuth runtime.
	OwnHTTPClient bool
	// Logger 记录 token 轮换结果；nil 时不输出。
	Logger *slog.Logger
}

// New builds an OAuth client; refreshToken is validated without network I/O.
func New(refreshToken string, cfg Config) *Client {
	refreshToken, refreshTokenErr := ValidateRefreshTokenInput(refreshToken)
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	logger := cmp.Or(cfg.Logger, slog.New(slog.DiscardHandler))
	return &Client{
		httpClient:      httpClient,
		ownsHTTP:        cfg.OwnHTTPClient,
		refreshToken:    refreshToken,
		refreshTokenErr: refreshTokenErr,
		log:             logger,
	}
}

func (c *Client) RefreshTokenValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.refreshToken
}
func (c *Client) AccessToken() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.accessToken }
func (c *Client) UserID() int64       { c.mu.RLock(); defer c.mu.RUnlock(); return c.userID }
func (c *Client) UserName() string    { c.mu.RLock(); defer c.mu.RUnlock(); return c.userName }

// Expiry 返回 access token 的过期时间；上游未提供正的有效时长时为零值。
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
		return errors.New("refresh token is required")
	}
	result, err := c.exchange(ctx, map[string]string{
		"client_id": pixivapi.OAuthClientID, "client_secret": pixivapi.OAuthClientSecret,
		"grant_type": "refresh_token", "include_policy": "true", "refresh_token": refreshToken,
	})
	if err != nil {
		return err
	}
	token, err := tokenFromResponse(result)
	if err != nil {
		return err
	}
	if token.AccessToken == "" {
		return pixivapi.MalformedResponse()
	}
	c.store(token)
	// OpenWith 的入口操作名是 "Open"；传输日志同样带上它。
	c.log.LogAttrs(ctx, slog.LevelDebug, "Refresh token rotated", slog.String(logging.AttrSDKOperation, "Open"))
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
		"client_id": pixivapi.OAuthClientID, "client_secret": pixivapi.OAuthClientSecret,
		"grant_type": "authorization_code", "include_policy": "true",
		"code": code, "code_verifier": verifier, "redirect_uri": pixivapi.OAuthRedirectURI,
	})
	if err != nil {
		return AuthCodeToken{}, err
	}
	token, err := tokenFromResponse(result)
	if err != nil {
		return AuthCodeToken{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return AuthCodeToken{}, pixivapi.MalformedResponse()
	}
	c.store(token)
	return token, nil
}

func (c *Client) exchange(ctx context.Context, form map[string]string) (authResponse, error) {
	values := make(url.Values, len(form))
	for key, value := range form {
		values.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pixivapi.OAuthBase+pixivapi.OAuthToken, strings.NewReader(values.Encode()))
	if err != nil {
		return authResponse{}, pixivapi.Transport(err)
	}
	for name, value := range pixivapi.OAuthHeaders() {
		req.Header.Set(name, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return authResponse{}, pixivapi.Transport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return authResponse{}, pixivapi.Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authResponse{}, pixivapi.HTTPStatus(resp.StatusCode)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return authResponse{}, pixivapi.MalformedResponse()
	}
	var result authResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return authResponse{}, pixivapi.MalformedResponse()
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

type authResponse struct {
	tokenWire
	Response tokenWire `json:"response"`
}

type tokenWire struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	User         authUser `json:"user"`
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

// tokenFromResponse accepts the two upstream envelopes without manufacturing
// an expiry when the response's seconds cannot fit Go's duration representation.
func tokenFromResponse(r authResponse) (AuthCodeToken, error) {
	wire := r.tokenWire
	if r.Response.AccessToken != "" || r.Response.RefreshToken != "" || r.Response.User.ID != 0 {
		wire = r.Response
	}
	const maxSeconds = int64(1<<63-1) / int64(time.Second)
	if wire.ExpiresIn > maxSeconds {
		return AuthCodeToken{}, pixivapi.MalformedResponse()
	}
	var expiry time.Time
	if wire.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(wire.ExpiresIn) * time.Second)
	}
	return AuthCodeToken{AccessToken: wire.AccessToken, RefreshToken: wire.RefreshToken, UserID: int64(wire.User.ID), Username: wire.User.Name, ExpiresAt: expiry}, nil
}

// CloseIdleConnections releases only this exchange's owned HTTP connections.
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.httpClient.CloseIdleConnections()
	}
}
