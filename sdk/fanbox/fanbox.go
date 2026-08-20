package fanbox

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/creators"
	creatorTags "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/tags"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/home"
	postinfo "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/info"
	postposts "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/posts"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/supporting"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
	fanboxresource "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/resource"
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
	session      *protocol.Session
	creators     *creators.Client
	creatorTags  *creatorTags.Client
	creatorPosts *postposts.Client
	postInfo     *postinfo.Client
	home         *home.Client
	supporting   *supporting.Client
	resource     *fanboxresource.Client

	// resourceMu guards resourceURLs, the in-session locator cache keyed by the
	// opaque ResourceRef string. The cache holds the currently-usable URL only;
	// the ref envelope encodes stable identity, never the locator. OpenResource
	// re-resolves from trusted metadata when a ref arrives without a cached
	// locator (for example across process restarts).
	resourceMu   sync.RWMutex
	resourceURLs map[string]string

	// identityMu guards userID, the lazily-verified FANBOX account identity.
	// The user id is non-secret and is bound into identity-scoped cursors
	// (Home, Supporting, Creators) so a cursor minted under one account cannot
	// be replayed against another. The session cookie never enters this value.
	identityMu sync.Mutex
	userID     int64
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
	})
	if err != nil {
		reason := sdk.CredentialsExpired
		if errors.Is(err, protocol.ErrInvalidOption) {
			reason = sdk.InvalidArgument
		}
		return nil, newError("Open", reason, err)
	}
	return &Client{
		session:      session,
		creators:     creators.New(session),
		creatorTags:  creatorTags.New(session),
		creatorPosts: postposts.New(session),
		postInfo:     postinfo.New(session),
		home:         home.New(session),
		supporting:   supporting.New(session),
		resource:     fanboxresource.New(session),
		resourceURLs: make(map[string]string),
	}, nil
}

// CloseIdleConnections releases idle connections of the SDK-created session.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.session == nil {
		return
	}
	c.session.CloseIdleConnections()
}
