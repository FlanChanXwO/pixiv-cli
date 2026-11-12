package fanbox

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// User exposes the authenticated session's non-secret identity fields.
type User = api.User

// CurrentUser fetches the authenticated identity from homepage metadata.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	return c.app.CurrentUser(ctx, "CurrentUser")
}

// ValidateSession checks the current session; an expired session returns CredentialsExpired.
func (c *Client) ValidateSession(ctx context.Context) error {
	_, err := c.app.CurrentUser(ctx, "ValidateSession")
	return err
}
