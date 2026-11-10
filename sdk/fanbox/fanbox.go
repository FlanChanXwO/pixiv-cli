// Package fanbox provides a read-only FANBOX client bound to one session.
//
// [New] accepts the FANBOXSESSID value and [Options] without reading local
// configuration, account files, or browser cookies. [Client.CurrentUser] and
// [Client.ValidateSession] explicitly validate that session. Requests use Chrome
// TLS and optional challenge-only FlareSolverr recovery; logs use the current
// slog default logger. [Client.Close] ends pending solver work.
//
// [Client.Creator] creates a [CreatorRef] without I/O; Get loads a [Creator]
// with the same operations. [Client.Creators] returns bound summaries and
// query/account-bound pages. Decoded objects require a fresh client reference.
package fanbox

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// Options configures native HTTP, headers, and optional solver connection settings.
// HTTPClient remains caller-owned; an empty FlareSolverrURL disables recovery.
type Options = api.Options

// User exposes the authenticated session's non-secret identity fields.
type User = api.User

// Client binds every operation to the session supplied to New. It reads no
// account files and never substitutes another account after authentication fails.
type Client struct{ app *api.Client }

// New constructs a client without network I/O. sessionID is only the opaque
// FANBOXSESSID value, rather than a full Cookie header. ValidateSession explicitly
// checks its current validity. Connection settings are validated immediately.
func New(sessionID string, options Options) (*Client, error) {
	app, err := api.New(sessionID, options)
	if err != nil {
		return nil, err
	}
	return &Client{app: app}, nil
}

// Format keeps the session, proxies, and cached cookies out of diagnostics.
func (*Client) Format(state fmt.State, _ rune) { fmt.Fprint(state, "fanbox.Client{}") }

// CurrentUser fetches the authenticated identity from homepage metadata.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	return c.app.CurrentUser(ctx, "CurrentUser")
}

// ValidateSession checks the current session; an expired session returns CredentialsExpired.
func (c *Client) ValidateSession(ctx context.Context) error {
	_, err := c.app.CurrentUser(ctx, "ValidateSession")
	return err
}

// CloseIdleConnections releases SDK-owned idle connections. Injected HTTP
// transports remain caller-owned; finish ongoing operations before closing.
func (c *Client) CloseIdleConnections() { c.app.CloseIdleConnections() }

// Close cancels and joins pending solver requests and releases SDK-owned
// connections. Finish native requests before closing; caller transports stay open.
func (c *Client) Close(ctx context.Context) error { return c.app.Close(ctx) }

func newError(operation string, reason sdk.Reason, message string) *sdk.Error {
	return &sdk.Error{Operation: operation, Reason: reason, Message: message}
}
