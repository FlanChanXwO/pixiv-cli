package fanbox

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
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
	session *fanbox.Session
	opts    Options
}

// Open constructs a Client for the given session credentials. It performs no
// network I/O; ValidateSession verifies the session.
func Open(credentials SessionCredentials) (*Client, error) {
	return OpenWith(credentials, Options{})
}

// OpenWith is Open with explicit connection-level Options.
func OpenWith(credentials SessionCredentials, options Options) (*Client, error) {
	cookieHeader := "FANBOXSESSID=" + credentials.FANBOXSESSID
	var solver *fanbox.FlareSolverrOptions
	if options.FlareSolverr != nil {
		solver = &fanbox.FlareSolverrOptions{
			URL:      options.FlareSolverr.URL,
			ProxyURL: options.FlareSolverr.ProxyURL,
		}
	}
	session, err := fanbox.NewSessionWithOptions(cookieHeader, fanbox.SessionOptions{
		HTTPClient:   options.HTTPClient,
		ProxyURL:     options.ProxyURL,
		UserAgent:    options.UserAgent,
		FlareSolverr: solver,
	})
	if err != nil {
		reason := sdk.CredentialsExpired
		if errors.Is(err, fanbox.ErrInvalidOption) {
			reason = sdk.InvalidArgument
		}
		return nil, newError("Open", reason, err)
	}
	return &Client{session: session, opts: options}, nil
}

// CloseIdleConnections releases idle connections of the SDK-created session.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.session == nil {
		return
	}
	c.session.CloseIdleConnections()
}
