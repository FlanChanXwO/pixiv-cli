package pixiv

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Pacing configures the minimum wall-clock interval between App API requests.
// The zero value disables pacing. An interval is honored across all operations
// on a Client, including OAuth refreshes and resource reads.
type Pacing struct {
	MinInterval time.Duration
}

// ResourcePolicy controls which upstream hosts the resource client is allowed
// to open. The zero value allows only official Pixiv media hosts. AllowedHosts
// lists additional hosts (for example a self-hosted reverse proxy) that are
// accepted in addition to the official set.
type ResourcePolicy struct {
	AllowedHosts []string
}

// Options are connection-level settings for constructing a Client. They never
// include account paths, configuration, browser access, Web fallback, or a
// public base URL override.
type Options struct {
	// HTTPClient is the HTTP client used for App API, OAuth, and resource
	// requests. It belongs to the caller and is never closed by the SDK; when
	// nil the SDK creates its own client whose idle connections can be released
	// with Client.CloseIdleConnections.
	HTTPClient *http.Client
	// AcceptLanguage is sent as the Accept-Language header for language
	// negotiation. It does not change the model schema.
	AcceptLanguage string
	Pacing         Pacing
	ResourcePolicy ResourcePolicy
}

// Client is a Pixiv App API client holding a single access token. It never
// refreshes on its own: construct with Open after rotating credentials, or with
// New for an explicitly provided token. Content operations fail with
// Unauthorized when no token is present and never fall back to an anonymous
// or Web path.
type Client struct {
	app       *appapi.Client
	resClient *resource.Client
	opts      Options

	httpClient *http.Client
	selfHTTP   bool

	userID    int64
	userName  string
	expiresAt time.Time
}

// Open rotates refreshToken once through OAuth and returns a Client holding
// only the resulting access token, together with the rotated Credentials that
// the caller must persist before issuing content requests. When the access
// token later expires, operations return CredentialsExpired and the caller
// must Open again.
func Open(ctx context.Context, refreshToken string) (*Client, Credentials, error) {
	return OpenWith(ctx, refreshToken, Options{})
}

// OpenWith is Open with explicit connection-level Options.
func OpenWith(ctx context.Context, refreshToken string, options Options) (*Client, Credentials, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, Credentials{}, newError("Open", sdk.CredentialsExpired, "refresh token is required")
	}
	httpClient, selfHTTP := buildHTTPClient(options)
	oauthClient := oauth.New(refreshToken, oauth.WithHTTPClient(httpClient))
	if err := oauthClient.Refresh(ctx); err != nil {
		return nil, Credentials{}, classifyOAuthError(err, "Open")
	}
	token := oauthClient.AccessToken()
	if token == "" {
		return nil, Credentials{}, newError("Open", sdk.UpstreamError, "oauth response did not include an access token")
	}
	if oauthClient.UserID() <= 0 {
		return nil, Credentials{}, newError("Open", sdk.MalformedUpstreamResponse, "oauth response did not include account identity")
	}
	credentials := Credentials{
		UserID:       oauthClient.UserID(),
		Username:     oauthClient.UserName(),
		ExpiresAt:    oauthClient.Expiry(),
		accessToken:  token,
		refreshToken: oauthClient.RefreshTokenValue(),
	}
	client := newClient(httpClient, selfHTTP, token, options, credentials.UserID)
	client.userName = credentials.Username
	return client, credentials, nil
}

// New returns a Client for an explicitly provided access token. It performs no
// network I/O, reads no files, and never infers a user ID from the token; the
// token is not refreshed when it expires.
func New(accessToken string) (*Client, error) {
	return NewWith(accessToken, Options{})
}

// NewWith is New with explicit connection-level Options.
func NewWith(accessToken string, options Options) (*Client, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, newError("New", sdk.InvalidArgument, "access token is required")
	}
	httpClient, selfHTTP := buildHTTPClient(options)
	return newClient(httpClient, selfHTTP, accessToken, options, 0), nil
}

func newClient(httpClient *http.Client, selfHTTP bool, accessToken string, options Options, userID int64) *Client {
	appOptions := []appapi.Option{
		appapi.WithHTTPClient(httpClient),
		appapi.WithAccessToken(accessToken),
		appapi.WithAcceptLanguage(options.AcceptLanguage),
	}
	if userID > 0 {
		appOptions = append(appOptions, appapi.WithUserID(userID))
	}
	app := appapi.New(appOptions...)
	return &Client{
		app:        app,
		resClient:  resource.NewApp(httpClient),
		opts:       options,
		httpClient: httpClient,
		selfHTTP:   selfHTTP,
		userID:     userID,
	}
}

// CloseIdleConnections releases idle connections of the SDK-created HTTP
// client. It is a no-op when a caller-provided client was injected.
func (c *Client) CloseIdleConnections() {
	if c == nil || !c.selfHTTP || c.httpClient == nil {
		return
	}
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
		return
	}
	if closer, ok := c.httpClient.Transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// UserID returns the verified identity bound at Open, or zero when the client
// was created with New and no identity is known.
func (c *Client) UserID() int64 { return c.userID }

// Username returns the verified username bound at Open, or the empty string
// when unknown.
func (c *Client) Username() string { return c.userName }

func buildHTTPClient(options Options) (*http.Client, bool) {
	if options.HTTPClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		c := &http.Client{Transport: transport}
		if options.Pacing.MinInterval > 0 {
			c.Transport = &pacingRoundTripper{inner: c.Transport, interval: options.Pacing.MinInterval}
		}
		return withDiagnosticTransport(c), true
	}
	base := options.HTTPClient
	if options.Pacing.MinInterval <= 0 {
		return base, false
	}
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	derived := &http.Client{
		Transport:     &pacingRoundTripper{inner: transport, interval: options.Pacing.MinInterval},
		Jar:           base.Jar,
		Timeout:       base.Timeout,
		CheckRedirect: base.CheckRedirect,
	}
	return withDiagnosticTransport(derived), false
}

func withDiagnosticTransport(client *http.Client) *http.Client {
	if client == nil {
		return nil
	}
	derived := *client
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	derived.Transport = &diagnosticRoundTripper{inner: transport}
	return &derived
}

// diagnosticRoundTripper observes only a CLI/MCP diagnostic scope carried by
// request.Context. Public SDK callers have no scope and therefore remain
// completely silent; query strings, headers other than the safe User-Agent,
// and transport errors are never placed in an event.
type diagnosticRoundTripper struct {
	inner http.RoundTripper
}

func (r *diagnosticRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if r == nil || r.inner == nil {
		return nil, errors.New("Pixiv HTTP transport is not configured")
	}
	operation := "retrieving"
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		operation = "sending"
	}
	route := "App API"
	if request.URL != nil && (request.URL.Hostname() == "oauth.secure.pixiv.net" || strings.Contains(request.URL.Path, "/auth/")) {
		route = "OAuth transport"
	} else if request.URL != nil && strings.HasSuffix(request.URL.Hostname(), "pximg.net") {
		route = "media transport"
	}
	resourcePath := "/"
	if request.URL != nil && request.URL.EscapedPath() != "" {
		resourcePath = request.URL.EscapedPath()
	}
	var agent string
	if request.Header != nil {
		agent = request.Header.Get("User-Agent")
	}
	response, err := r.inner.RoundTrip(request)
	if err != nil {
		diagnostics.Emit(request.Context(), diagnostics.Event{
			Module:    diagnostics.ModulePixivNetwork,
			Kind:      diagnostics.EventFailed,
			Operation: "network request",
			Route:     route,
		})
		return nil, err
	}
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	diagnostics.Emit(request.Context(), diagnostics.Event{
		Module:    diagnostics.ModulePixivNetwork,
		Kind:      diagnostics.EventNetworkRequest,
		Operation: operation,
		Resource:  resourcePath,
		Route:     route,
		UserAgent: agent,
		Status:    status,
	})
	return response, nil
}

func (r *diagnosticRoundTripper) CloseIdleConnections() {
	if r == nil {
		return
	}
	if closer, ok := r.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type pacingRoundTripper struct {
	inner    http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func (p *pacingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := p.interval - time.Since(p.last)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	p.last = time.Now()
	p.mu.Unlock()
	return p.inner.RoundTrip(req)
}

func (p *pacingRoundTripper) CloseIdleConnections() {
	if closer, ok := p.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
