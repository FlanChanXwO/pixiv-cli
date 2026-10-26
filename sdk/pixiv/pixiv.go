// Public Pixiv construction and verified identity; transport state belongs to pixivapi.

package pixiv

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi/oauth"
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
	// Logger receives the transport's request outcomes and internal decisions
	// such as token refresh and retry waits. Nil keeps the SDK silent. Logs
	// never include queries, headers, bodies, tokens, cookies, or signed URLs.
	Logger *slog.Logger
}

// Client is a Pixiv App API client holding a single access token. It never
// refreshes on its own: construct with Open after rotating credentials, or with
// New for an explicitly provided token. Content operations fail with
// Unauthorized when no token is present and never fall back to an anonymous
// or Web path.
type Client struct {
	// app owns the App API transport and the access token; every operation goes
	// through it directly.
	app *pixivapi.Client

	userID   int64
	userName string
	// cursorInstance is a non-secret binding for identity-scoped cursors
	// produced before OAuth has verified the account identity.
	cursorInstance string
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
	httpClient, selfHTTP := pixivapi.NewHTTPClient(options.HTTPClient, options.Pacing.MinInterval, options.Logger)
	oauthClient := oauth.New(refreshToken, oauth.Config{HTTPClient: httpClient, Logger: options.Logger})
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
	client, err := newClient(httpClient, selfHTTP, token, options, credentials.UserID)
	if err != nil {
		return nil, Credentials{}, newError("Open", sdk.LocalStateError, "cannot initialize cursor instance")
	}
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
	httpClient, selfHTTP := pixivapi.NewHTTPClient(options.HTTPClient, options.Pacing.MinInterval, options.Logger)
	client, err := newClient(httpClient, selfHTTP, accessToken, options, 0)
	if err != nil {
		return nil, newError("New", sdk.LocalStateError, "cannot initialize cursor instance")
	}
	return client, nil
}

func newClient(httpClient *http.Client, selfHTTP bool, accessToken string, options Options, userID int64) (*Client, error) {
	var cursorInstance string
	if userID <= 0 {
		var err error
		cursorInstance, err = newCursorInstanceID()
		if err != nil {
			return nil, err
		}
	}
	app := pixivapi.New(pixivapi.Config{
		HTTPClient:     httpClient,
		OwnHTTPClient:  selfHTTP,
		ResourceHosts:  options.ResourcePolicy.AllowedHosts,
		AccessToken:    accessToken,
		AcceptLanguage: options.AcceptLanguage,
		UserID:         userID,
		Logger:         options.Logger,
	})
	return &Client{
		app:            app,
		userID:         userID,
		cursorInstance: cursorInstance,
	}, nil
}

// CloseIdleConnections releases idle connections of the SDK-created HTTP
// client. It is a no-op when a caller-provided client was injected.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.app == nil {
		return
	}
	c.app.CloseIdleConnections()
}

// UserID returns the verified identity bound at Open, or zero when the client
// was created with New and no identity is known.
func (c *Client) UserID() int64 { return c.userID }

// Username returns the verified username bound at Open, or the empty string
// when unknown.
func (c *Client) Username() string { return c.userName }
