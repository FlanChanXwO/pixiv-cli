// Package pixiv provides an account-bound client for the Pixiv App API.
//
// [New] accepts a caller-owned [CredentialStore] and connection [Options]. The
// client reads current credentials before refreshing, performs OAuth outside
// storage locks, and saves through a revision comparison before using new tokens.
// [NewMemoryCredentials] supplies storage for applications that need no file.
// All content requests require credentials; configuration and account selection
// remain with the caller. Logs use the current slog default logger; use
// [slog.SetDefault] to route events into the application's handler.
//
// [BeginLogin] creates a standalone [LoginSession]. The caller opens its login
// URL and persists the credentials returned by Complete before content requests.
//
// # In-memory credentials
//
// To start from a refresh token without file persistence:
//
//	store, err := pixiv.NewMemoryCredentials(pixiv.NewCredentials(refreshToken))
//	if err != nil {
//		return err
//	}
//	client, err := pixiv.New(store, pixiv.Options{})
//	if err != nil {
//		return err
//	}
//	defer client.CloseIdleConnections()
//
// For persistent storage, implement [CredentialStore]. Load returns the latest
// saved revision; CompareAndSwap completes persistence before returning true.
// Release storage locks when each method returns: OAuth runs between calls.
// The client adopts a newer saved revision after a conflict, and a failed save
// fails the operation before the rotated credentials can be used.
package pixiv

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Options configures the connection shared by App, OAuth, and resource requests.
// HTTPClient stays caller-owned; nil creates a client whose idle connections are
// released by CloseIdleConnections. Positive RequestInterval enables pacing.
type Options struct {
	HTTPClient      *http.Client
	AcceptLanguage  string
	RequestInterval time.Duration
	ResourceHosts   []string // Additional media hosts, beyond the official Pixiv CDN allowlist.
}

// Client binds operations to one account and automatically refreshes access
// tokens. It reads no configuration or account files. Concurrent operations share
// a credential gate; only credential preparation, not content I/O, is serialized.
type Client struct {
	store         CredentialStore
	http          *http.Client
	ownsHTTP      bool
	language      string
	resourceHosts []string
	gate          chan struct{}
	cached        Credentials  // Protected by gate; only persisted revisions are cached.
	userID        atomic.Int64 // Identity reads must not race its first binding.
}

// New constructs a client without network or credential-store I/O. The store
// must remain bound to one account for the client's lifetime.
func New(store CredentialStore, options Options) (*Client, error) {
	if store == nil {
		return nil, newError("New", sdk.InvalidArgument, "credential store is required", nil)
	}
	httpClient, owned := api.NewHTTPClient(options.HTTPClient, options.RequestInterval, protocolLogger())
	return &Client{store: store, http: httpClient, ownsHTTP: owned, language: strings.TrimSpace(options.AcceptLanguage), resourceHosts: append([]string(nil), options.ResourceHosts...), gate: make(chan struct{}, 1)}, nil
}

// Format keeps cached credentials and connection settings out of diagnostics.
func (*Client) Format(state fmt.State, _ rune) { fmt.Fprint(state, "pixiv.Client{}") }

// UserID reports the identity established by loaded or refreshed credentials,
// or zero before an operation has established it.
func (c *Client) UserID() int64 { return c.userID.Load() }

// CloseIdleConnections releases SDK-owned idle connections. Injected clients
// remain caller-owned; finish concurrent operations before closing connections.
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.http.CloseIdleConnections()
	}
}

func (c *Client) bindIdentity(id int64) error {
	if id == 0 {
		return nil
	}
	c.userID.CompareAndSwap(0, id)
	if c.userID.Load() != id {
		return newError("credentials", sdk.LocalStateError, "credential identity does not match this client", nil)
	}
	return nil
}

func (c *Client) app(value Credentials) *api.Client {
	return api.New(api.Config{HTTPClient: c.http, AccessToken: value.AccessToken.Value(), UserID: value.UserID, AcceptLanguage: c.language, ResourceHosts: c.resourceHosts, Logger: protocolLogger()})
}

// getJSON is the safe read boundary: only a definite 401 permits one auth replay.
// Mutations must prepare credentials without replaying an uncertain write.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	value, err := c.credentials(ctx, Credentials{})
	if err != nil {
		return err
	}
	err = c.app(value).GetJSON(ctx, path, query, out)
	if isUnauthorized(err) {
		value, err = c.credentials(ctx, value)
		if err != nil {
			return err
		}
		err = c.app(value).GetJSON(ctx, path, query, out)
	}
	if err != nil {
		return classifyError(path, err, false)
	}
	return nil
}

// defaultHandler resolves slog's current handler for each protocol log event.
// shortcut: protocol clients require logger parameters, remove this bridge when they call slog Context functions directly.
type defaultHandler func() slog.Handler

// Enabled consults the current default handler, including after construction.
func (h defaultHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h().Enabled(ctx, level)
}

// Handle forwards the protocol record with its original context and timestamp.
func (h defaultHandler) Handle(ctx context.Context, record slog.Record) error {
	return h().Handle(ctx, record)
}

// WithAttrs preserves late resolution while attaching the protocol's attributes.
func (h defaultHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return defaultHandler(func() slog.Handler { return h().WithAttrs(attrs) })
}

// WithGroup preserves late resolution while grouping the protocol's attributes.
func (h defaultHandler) WithGroup(name string) slog.Handler {
	return defaultHandler(func() slog.Handler { return h().WithGroup(name) })
}
func protocolLogger() *slog.Logger {
	return slog.New(defaultHandler(func() slog.Handler { return slog.Default().Handler() }))
}
