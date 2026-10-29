package fanbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/api"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SessionCredentials holds exactly the FANBOXSESSID cookie value, not a full
// Cookie header. Every formatting path is redacted.
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

// CurrentUserRequest reads the current authenticated user's identity.
type CurrentUserRequest struct{}

// Options configures how a Client connects. Leaving HTTPClient nil makes the
// SDK create a browser-aligned session it closes through
// Client.CloseIdleConnections.
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

// FlareSolverrOptions configures the challenge solver and its separate upstream
// browser proxy. The service URL is reduced to its root, and the solver never
// receives the FANBOX session or a business request URL.
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Client sends requests under one FANBOXSESSID session. It never reads browsers,
// local account stores, or Pixiv credentials, and a failure never falls back to
// another credential or product.
type Client struct {
	// app owns the session, the transport, and the in-session caches for
	// resource locators and the verified account identity.
	app *api.Client
}

// Open creates a Client without network I/O; call ValidateSession to verify the
// session against upstream.
func Open(credentials SessionCredentials) (*Client, error) {
	return OpenWith(credentials, Options{})
}

// OpenWith applies the connection settings in options instead of the defaults
// Open uses; nil fields fall back to those defaults.
func OpenWith(credentials SessionCredentials, options Options) (*Client, error) {
	app, err := api.New(credentials.FANBOXSESSID, api.Options{
		HTTPClient:   options.HTTPClient,
		ProxyURL:     options.ProxyURL,
		UserAgent:    options.UserAgent,
		FlareSolverr: (*api.FlareSolverrOptions)(options.FlareSolverr),
		Logger:       options.Logger,
	})
	if err != nil {
		reason := sdk.CredentialsExpired
		if errors.Is(err, api.ErrInvalidOption) {
			reason = sdk.InvalidArgument
		}
		return nil, newError("Open", reason, err)
	}
	return &Client{app: app}, nil
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
	ctx = logging.WithOperation(ctx, "ValidateSession")
	if _, err := c.app.CurrentUser(ctx); err != nil {
		return classifyError("ValidateSession", err)
	}
	return nil
}

// CurrentUser returns the current authenticated user's identity.
func (c *Client) CurrentUser(ctx context.Context, request CurrentUserRequest) (User, error) {
	ctx = logging.WithOperation(ctx, "CurrentUser")
	identity, err := c.app.CurrentUser(ctx)
	if err != nil {
		return User{}, classifyError("CurrentUser", err)
	}
	return User(identity), nil
}
