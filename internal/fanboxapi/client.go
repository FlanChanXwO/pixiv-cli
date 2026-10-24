// FANBOX 传输客户端：把会话、媒体定位缓存和账号身份缓存封装成业务 Client 持有的
// 单个依赖。路由常量在 api.go，媒体 URL 规则在 resource.go。

package fanboxapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
)

// ErrMissingIdentity 表示会话有效但首页元数据没有可用的 FANBOX 用户 ID。
var ErrMissingIdentity = errors.New("FANBOX identity has no valid user id")

// Locator 是一次已验证的媒体定位：当前可用的 URL 和下载时的附件名。
type Locator struct {
	URL      string
	Filename string
}

// Client 是 SDK 唯一的传输依赖，持有会话及其运行时缓存。
type Client struct {
	session  *Session
	mu       sync.RWMutex
	locators map[string]Locator

	identityMu sync.Mutex
	userID     int64
}

// NewClient wraps one FANBOX session.
func NewClient(session *Session) *Client {
	return &Client{session: session, locators: make(map[string]Locator)}
}

// GetJSON performs one authenticated request against a route under APIBaseURL.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := APIBaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return c.session.GetJSON(ctx, endpoint, target)
}

// GetJSONURL performs one authenticated request against a server-provided
// continuation. The continuation is validated against the FANBOX host policy
// before it reaches the transport.
func (c *Client) GetJSONURL(ctx context.Context, rawURL string, target any) error {
	if err := ValidateAPIURL(rawURL); err != nil {
		return err
	}
	return c.session.GetJSON(ctx, rawURL, target)
}

// OpenMedia validates and opens one media resource.
func (c *Client) OpenMedia(ctx context.Context, rawURL string, request MediaRequest) (*http.Response, error) {
	if err := ValidateURL(rawURL); err != nil {
		return nil, err
	}
	return c.session.OpenMediaWithRequest(ctx, rawURL, request)
}

// CacheLocator remembers the current locator for an opaque resource reference.
func (c *Client) CacheLocator(ref string, locator Locator) {
	c.mu.Lock()
	c.locators[ref] = locator
	c.mu.Unlock()
}

// CachedLocator returns the in-session locator for an opaque resource reference.
func (c *Client) CachedLocator(ref string) (Locator, bool) {
	c.mu.RLock()
	locator, ok := c.locators[ref]
	c.mu.RUnlock()
	return locator, ok
}

// CurrentUser reads the authenticated account identity from the session.
func (c *Client) CurrentUser(ctx context.Context) (Identity, error) {
	return c.session.CurrentUser(ctx)
}

// VerifiedUserID returns the authenticated FANBOX user id, verifying it once
// per client. The id is non-secret and may be bound into identity-scoped
// cursors; the session cookie never leaves the session.
func (c *Client) VerifiedUserID(ctx context.Context) (int64, error) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.userID > 0 {
		return c.userID, nil
	}
	identity, err := c.CurrentUser(ctx)
	if err != nil {
		return 0, err
	}
	if identity.UserID <= 0 {
		return 0, ErrMissingIdentity
	}
	c.userID = identity.UserID
	return c.userID, nil
}

// CloseIdleConnections releases idle connections held by the session transport.
func (c *Client) CloseIdleConnections() { c.session.CloseIdleConnections() }
