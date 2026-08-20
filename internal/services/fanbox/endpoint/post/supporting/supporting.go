// Package supporting owns the FANBOX supporting-creators feed endpoint.
package supporting

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/wire"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Transport is the narrow JSON capability required by supporting.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// Request selects a supporting-feed continuation.
type Request struct{ NextURL string }

// Client owns the supporting route and body.items response conversion.
type Client struct{ transport Transport }

// New constructs a supporting endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// List fetches one supporting-feed page.
func (c *Client) List(ctx context.Context, request Request) (post.Page, error) {
	if c == nil || c.transport == nil {
		return post.Page{}, errors.New("FANBOX supporting transport is not configured")
	}
	target := protocol.APIBaseURL + "post.listSupporting?" + url.Values{"limit": {strconv.Itoa(protocol.PostListPageLimit)}}.Encode()
	if strings.TrimSpace(request.NextURL) != "" {
		target = request.NextURL
		if err := protocol.ValidateAPIURL(target); err != nil {
			return post.Page{}, err
		}
	}
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := c.transport.GetJSON(ctx, target, &response); err != nil {
		return post.Page{}, err
	}
	return wire.DecodePage(response.Body, true)
}
