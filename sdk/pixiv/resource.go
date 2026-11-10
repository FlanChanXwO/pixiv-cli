package pixiv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func (c *Client) resourceBody(ctx context.Context, operation, rawURL string) (io.ReadCloser, error) {
	response, err := c.openResource(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, classifyError(operation+".Open", api.HTTPStatus(response.StatusCode), false)
	}
	return response.Body, nil
}

func (c *Client) saveResource(ctx context.Context, operation, rawURL, path string) error {
	// 1. Open the validated resource before creating a temporary destination.
	body, err := c.resourceBody(ctx, operation, rawURL)
	if err != nil {
		return err
	}
	defer body.Close()
	// 2. Finish the copy and close before replacing an existing destination.
	name := strings.ToLower(operation)
	file, err := os.CreateTemp(filepath.Dir(path), ".pixiv-"+name+"-*")
	if err != nil {
		return newError(operation+".Save", sdk.LocalStateError, "create "+name+" file failed", err)
	}
	defer os.Remove(file.Name())
	_, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr, ctx.Err()); err != nil {
		return newError(operation+".Save", sdk.LocalStateError, "write "+name+" failed", err)
	}
	// 3. Publish the complete file; earlier failures leave the old file intact.
	if err := os.Rename(file.Name(), path); err != nil {
		return newError(operation+".Save", sdk.LocalStateError, "replace "+name+" failed", err)
	}
	return nil
}

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
