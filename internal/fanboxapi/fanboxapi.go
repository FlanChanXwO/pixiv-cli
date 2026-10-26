// Package fanboxapi owns authenticated requests, challenge recovery and resource caches.
package fanboxapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
)

const (
	// WebBaseURL and APIBaseURL are fixed FANBOX protocol endpoints. sdk/fanbox
	// uses them with the route constants in api.go.
	WebBaseURL = "https://www.fanbox.cc/"
	APIBaseURL = "https://api.fanbox.cc/"
	// PostListPageLimit is the explicit upstream parameter used by FANBOX post
	// list routes. It is not a local result cap; continuation remains owned by
	// server-provided URLs.
	PostListPageLimit = 10
	// This Firefox User-Agent is the existing HTTP header baseline; the TLS
	// client profile is configured independently in transport.go.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0"
)

// 分类 sentinel 错误；公开 SDK 层依据它们做进一步分类。
var (
	// ErrForbidden 表示 FANBOX 拒绝访问（非 Cloudflare challenge）。
	ErrForbidden = errors.New("fanbox: forbidden")
	// ErrChallenge 表示 403 响应体或 header 表明触发了 Cloudflare challenge。
	ErrChallenge = errors.New("fanbox: challenge required")
	// ErrNotAuthenticated 表示会话缺失或过期（401）。
	ErrNotAuthenticated = errors.New("fanbox: not authenticated")
	// ErrInvalidOption identifies a malformed explicit FANBOX connection option.
	// The public SDK maps it to InvalidArgument without echoing the value.
	ErrInvalidOption = errors.New("fanbox: invalid option")
	// ErrMalformedResponse identifies invalid API JSON or identity metadata.
	ErrMalformedResponse = errors.New("fanbox: malformed response")
)

// Locator is a validated media URL and its attachment filename.
type Locator struct {
	URL      string
	Filename string
}

// Client owns the FANBOX transport, credentials, solver state and in-session caches.
type Client struct {
	httpClient   *http.Client
	ownsHTTP     bool
	userAgent    string
	cookie       string
	flareSolverr *FlareSolverrOptions
	log          *slog.Logger

	solverMu     sync.Mutex
	solverState  *solverState
	solverActive *solverCall
	locatorMu    sync.RWMutex
	locators     map[string]Locator
	identityMu   sync.Mutex
	userID       int64
}

// FlareSolverrOptions carries the two independent addresses used by the
// optional challenge recovery path. Solver state is kept on one Client only.
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Options contains explicit FANBOX native and solver configuration.
// The solver is inactive when FlareSolverr is nil.
type Options struct {
	HTTPClient   *http.Client
	ProxyURL     string
	UserAgent    string
	FlareSolverr *FlareSolverrOptions
	// Logger 记录请求结果和挑战恢复决策；nil 时不输出。
	Logger *slog.Logger
}

// New constructs one transport runtime without performing network I/O.
func New(cookieHeader string, options Options) (*Client, error) {
	agent, err := validateUserAgent(options.UserAgent)
	if err != nil {
		return nil, err
	}
	proxyURL, err := validateProxyURL(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	solver, err := normalizeFlareSolverrOptions(options.FlareSolverr)
	if err != nil {
		return nil, err
	}
	client := options.HTTPClient
	ownsHTTP := client == nil
	if ownsHTTP {
		transport, err := newBrowserTransport(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("create FANBOX TLS transport: %w", err)
		}
		client = &http.Client{Transport: transport}
	} else if client.Transport == nil {
		return nil, errors.New("FANBOX injected HTTP client requires an explicit transport")
	}
	cookie, err := normalizeCookieHeader(cookieHeader)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient:   client,
		ownsHTTP:     ownsHTTP,
		userAgent:    agent,
		cookie:       cookie,
		flareSolverr: solver,
		log:          cmp.Or(options.Logger, slog.New(slog.DiscardHandler)),
		locators:     make(map[string]Locator),
	}, nil
}

// String and GoString keep credentials out of diagnostic formatting.
func (*Client) String() string   { return "fanboxapi.Client{}" }
func (*Client) GoString() string { return "fanboxapi.Client{}" }

// CloseIdleConnections releases only SDK-created HTTP connections.
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.httpClient.CloseIdleConnections()
	}
}

// GetJSON performs one authenticated request against a route under APIBaseURL.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := APIBaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return c.GetJSONURL(ctx, endpoint, target)
}

// CacheLocator remembers the current locator for an opaque resource reference.
func (c *Client) CacheLocator(ref string, locator Locator) {
	c.locatorMu.Lock()
	c.locators[ref] = locator
	c.locatorMu.Unlock()
}

// CachedLocator returns the in-session locator for an opaque resource reference.
func (c *Client) CachedLocator(ref string) (Locator, bool) {
	c.locatorMu.RLock()
	locator, ok := c.locators[ref]
	c.locatorMu.RUnlock()
	return locator, ok
}

// VerifiedUserID returns the authenticated FANBOX user id, verifying it once
// per client. The id is non-secret and may be bound into identity-scoped
// cursors; the session cookie never leaves the transport.
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
	c.userID = identity.UserID
	return c.userID, nil
}
