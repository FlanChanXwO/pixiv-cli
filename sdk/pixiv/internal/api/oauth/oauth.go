// Package oauth exchanges Pixiv App tokens and generates PKCE login material.
//
// [New] constructs a [Client] from [Config] without I/O; its mutex protects stored
// tokens and identity. [AuthCodeToken] carries an exchange result, and
// [GeneratePKCEPair] creates the login verifier and challenge.
package oauth

import (
	"bytes"
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

	"github.com/FlanChanXwO/pixiv-cli/internal/netdiagnostic"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv/internal/api"
)

// Client protects stored tokens and identity with mu; HTTP ownership stays with
// the SDK client or login session that supplied it.
type Client struct {
	httpClient   *http.Client
	refreshToken string
	// refreshTokenErr preserves invalid-cookie input as a local validation failure.
	refreshTokenErr error
	accessToken     string
	userID          int64
	userName        string
	expiresAt       time.Time
	mu              sync.RWMutex
}

// Config supplies the exchange connection; zero uses a default HTTP client.
type Config struct {
	HTTPClient *http.Client
}

// New validates the refresh token locally without contacting OAuth.
func New(refreshToken string, config Config) *Client {
	refreshToken, refreshTokenErr := validateRefreshTokenInput(refreshToken)
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		httpClient:      httpClient,
		refreshToken:    refreshToken,
		refreshTokenErr: refreshTokenErr,
	}
}

// RefreshTokenValue reads the current refresh token under a read lock.
// The caller must keep the returned credential out of logs.
func (c *Client) RefreshTokenValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.refreshToken
}

// AccessToken reads the last exchanged access token, or empty before exchange.
func (c *Client) AccessToken() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.accessToken }

// UserID reads the last exchanged account ID, or zero before exchange.
func (c *Client) UserID() int64 { c.mu.RLock(); defer c.mu.RUnlock(); return c.userID }

// UserName reads the last exchanged account name, or empty before exchange.
func (c *Client) UserName() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.userName }

// Expiry reads token expiry, or zero when the server supplied no positive lifetime.
func (c *Client) Expiry() time.Time { c.mu.RLock(); defer c.mu.RUnlock(); return c.expiresAt }

// Refresh exchanges a valid refresh token and updates stored credentials.
// Missing or invalid input fails locally before network I/O.
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
	result, err := c.exchange(ctx, "Open", map[string]string{
		"client_id": api.OAuthClientID, "client_secret": api.OAuthClientSecret,
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
		return api.MalformedResponse()
	}
	c.store(token)
	slog.LogAttrs(ctx, slog.LevelDebug, "Refresh token rotated", slog.String(api.AttrSDKOperation, "Open"))
	return nil
}

// AuthCodeToken carries exchanged tokens and identity; zero ExpiresAt means the
// server supplied no positive lifetime.
type AuthCodeToken struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
	Username     string
	ExpiresAt    time.Time
}

// GeneratePKCEPair creates a cryptographically random verifier and its S256 challenge.
func GeneratePKCEPair() (verifier, challenge string, err error) {
	verifier, err = RandomURLToken(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// RandomURLToken returns an unpredictable URL-safe token.
func RandomURLToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

// ExchangeAuthorizationCode exchanges nonempty code and verifier values, updates
// stored credentials, and requires both access and refresh tokens in the response.
func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code, verifier string) (AuthCodeToken, error) {
	code, verifier = strings.TrimSpace(code), strings.TrimSpace(verifier)
	if code == "" {
		return AuthCodeToken{}, errors.New("authorization code cannot be empty")
	}
	if verifier == "" {
		return AuthCodeToken{}, errors.New("code verifier cannot be empty")
	}
	result, err := c.exchange(ctx, "Complete", map[string]string{
		"client_id": api.OAuthClientID, "client_secret": api.OAuthClientSecret,
		"grant_type": "authorization_code", "include_policy": "true",
		"code": code, "code_verifier": verifier, "redirect_uri": api.OAuthRedirectURI,
	})
	if err != nil {
		return AuthCodeToken{}, err
	}
	token, err := tokenFromResponse(result)
	if err != nil {
		return AuthCodeToken{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return AuthCodeToken{}, api.MalformedResponse()
	}
	c.store(token)
	return token, nil
}

func (c *Client) exchange(ctx context.Context, operation string, form map[string]string) (result authResponse, err error) {
	var details []slog.Attr
	defer func() {
		if err != nil {
			attrs := []slog.Attr{slog.String(api.AttrSDKOperation, operation), slog.String(api.AttrError, err.Error())}
			slog.LogAttrs(ctx, slog.LevelDebug, "Request failed", append(attrs, details...)...)
		}
	}()
	values := make(url.Values, len(form))
	for key, value := range form {
		values.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.OAuthBase+api.PathOAuthToken, strings.NewReader(values.Encode()))
	if err != nil {
		details = netdiagnostic.Attrs(err)
		return authResponse{}, api.Transport(err)
	}
	for name, value := range api.OAuthHeaders() {
		req.Header.Set(name, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		details = netdiagnostic.Attrs(err)
		return authResponse{}, api.Transport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		details = netdiagnostic.Attrs(err)
		return authResponse{}, api.Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter, present := api.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return authResponse{}, api.HTTPStatusWithRetryAfter(resp.StatusCode, retryAfter, present)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return authResponse{}, api.MalformedResponse()
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return authResponse{}, api.MalformedResponse()
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

// UnmarshalJSON accepts numeric and decimal-string account IDs.
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

// tokenFromResponse accepts both upstream token envelopes and rejects lifetimes
// that exceed Duration instead of manufacturing an expiry.
func tokenFromResponse(r authResponse) (AuthCodeToken, error) {
	wire := r.tokenWire
	if r.Response.AccessToken != "" || r.Response.RefreshToken != "" || r.Response.User.ID != 0 {
		wire = r.Response
	}
	const maxSeconds = int64(1<<63-1) / int64(time.Second)
	if wire.ExpiresIn > maxSeconds {
		return AuthCodeToken{}, api.MalformedResponse()
	}
	var expiry time.Time
	if wire.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(wire.ExpiresIn) * time.Second)
	}
	return AuthCodeToken{AccessToken: wire.AccessToken, RefreshToken: wire.RefreshToken, UserID: int64(wire.User.ID), Username: wire.User.Name, ExpiresAt: expiry}, nil
}
