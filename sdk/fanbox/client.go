// FANBOX SDK 入口：客户端构造、连接选项，以及会话校验。
// 业务操作分布在 creator.go、post.go，响应结构与映射在 wire.go。

package fanbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	protocol "github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SessionCredentials carries the FANBOX session cookie value. It never accepts
// a full Cookie header; only the FANBOXSESSID value is used. Every formatting
// path is redacted.
type SessionCredentials struct {
	FANBOXSESSID string `json:"-"`
}

// String returns a redacted summary that never contains the session value.
func (SessionCredentials) String() string { return "fanbox.SessionCredentials{}" }

// GoString returns the same redacted summary as String.
func (SessionCredentials) GoString() string { return "fanbox.SessionCredentials{}" }

// Format renders the redacted summary for any formatting verb.
func (SessionCredentials) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "fanbox.SessionCredentials{}")
}

// Options are connection-level settings for constructing a Client.
type Options struct {
	// HTTPClient is used for FANBOX API and media requests. It belongs to the
	// caller and is never closed by the SDK. When nil, the SDK creates a
	// production session with a browser-aligned TLS fingerprint and manages its
	// idle connections, released by Client.CloseIdleConnections.
	HTTPClient *http.Client
	// ProxyURL optionally routes production requests through an HTTP(S) proxy.
	// It must not carry userinfo.
	ProxyURL string
	// UserAgent overrides only the native FANBOX HTTP User-Agent header. It does
	// not change the TLS profile or provide a Cloudflare bypass guarantee.
	UserAgent string
	// FlareSolverr enables challenge-only recovery when explicitly configured.
	// A nil value keeps the client entirely independent of FlareSolverr.
	FlareSolverr *FlareSolverrOptions
	// Logger receives the transport's request outcomes and challenge-recovery
	// decisions. Nil keeps the SDK silent. Logs never include queries, headers,
	// bodies, cookies, or signed URLs.
	Logger *slog.Logger
}

// FlareSolverrOptions configures the direct solver service and its independent
// browser upstream proxy. The service URL is normalized to its root and the
// solver is never given the FANBOX session or a business request URL.
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Client is a FANBOX client bound to one FANBOXSESSID session. It does not
// read browsers, local account stores, or Pixiv credentials, and never falls
// back between credentials or products.
type Client struct {
	// app owns the session, the transport, and the in-session caches for
	// resource locators and the verified account identity.
	app *protocol.Client
}

// Open constructs a Client for the given session credentials. It performs no
// network I/O; ValidateSession verifies the session.
func Open(credentials SessionCredentials) (*Client, error) {
	return OpenWith(credentials, Options{})
}

// OpenWith is Open with explicit connection-level Options.
func OpenWith(credentials SessionCredentials, options Options) (*Client, error) {
	cookieHeader := "FANBOXSESSID=" + credentials.FANBOXSESSID
	var solver *protocol.FlareSolverrOptions
	if options.FlareSolverr != nil {
		solver = &protocol.FlareSolverrOptions{
			URL:      options.FlareSolverr.URL,
			ProxyURL: options.FlareSolverr.ProxyURL,
		}
	}
	session, err := protocol.NewSessionWithOptions(cookieHeader, protocol.SessionOptions{
		HTTPClient:   options.HTTPClient,
		ProxyURL:     options.ProxyURL,
		UserAgent:    options.UserAgent,
		FlareSolverr: solver,
		Logger:       options.Logger,
	})
	if err != nil {
		reason := sdk.CredentialsExpired
		if errors.Is(err, protocol.ErrInvalidOption) {
			reason = sdk.InvalidArgument
		}
		return nil, newError("Open", reason, err)
	}
	return &Client{app: protocol.NewClient(session)}, nil
}

// CloseIdleConnections releases idle connections of the SDK-created session.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.app == nil {
		return
	}
	c.app.CloseIdleConnections()
}

// ValidateSession verifies the current session identity. An expired session
// returns CredentialsExpired.
func (c *Client) ValidateSession(ctx context.Context) error {
	ctx = protocol.WithOperation(ctx, "ValidateSession")
	if _, err := c.currentUser(ctx); err != nil {
		return classifyError("ValidateSession", err)
	}
	return nil
}

// CurrentUser returns the current authenticated user's identity.
func (c *Client) CurrentUser(ctx context.Context, request CurrentUserRequest) (User, error) {
	ctx = protocol.WithOperation(ctx, "CurrentUser")
	identity, err := c.currentUser(ctx)
	if err != nil {
		return User{}, classifyError("CurrentUser", err)
	}
	return mapUser(identity), nil
}

func (c *Client) currentUser(ctx context.Context) (protocol.Identity, error) {
	return c.app.CurrentUser(ctx)
}
