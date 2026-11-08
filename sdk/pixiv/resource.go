package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// openResource is the read boundary for client-bound resource objects. The
// caller owns the returned body and interprets its HTTP status and headers.
// A nil client represents a deserialized, unbound object. Resource requests
// require saved account credentials but never forward them to the media host.
func (c *Client) openResource(ctx context.Context, rawURL string) (*api.Response, error) {
	if c == nil {
		return nil, newError("resource.Open", sdk.InvalidArgument, "resource is unbound", nil)
	}
	// 1. Reject a forbidden locator before credential I/O or token rotation.
	transport := c.app(Credentials{})
	if err := transport.ValidateResourceURL(rawURL); err != nil {
		return nil, newError("resource.Open", sdk.ResourceForbidden, "resource URL is not allowed", nil)
	}
	// 2. Complete any credential rotation and persistence before media I/O.
	if _, err := c.credentials(ctx, Credentials{}); err != nil {
		return nil, err
	}
	// 3. The protocol layer strips the cookie jar and revalidates redirects.
	response, err := transport.OpenResource(ctx, api.OpenRequest{URL: rawURL})
	if err != nil {
		return nil, classifyError("resource.Open", err, false)
	}
	return response, nil
}
