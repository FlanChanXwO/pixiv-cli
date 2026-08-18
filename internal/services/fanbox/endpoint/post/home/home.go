// Package home owns the FANBOX authenticated home feed endpoint.
package home

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

// Transport is the narrow JSON capability required by home.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// Request selects a home continuation.
type Request struct{ NextURL string }

// Client owns the home route and body.items response conversion.
type Client struct{ transport Transport }

// New constructs a home endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// List fetches one home page.
func (c *Client) List(ctx context.Context, request Request) (post.Page, error) {
	if c == nil || c.transport == nil {
		return post.Page{}, errors.New("FANBOX home transport is not configured")
	}
	target := protocol.APIBaseURL + "post.listHome?" + url.Values{"limit": {strconv.Itoa(protocol.PostListPageLimit)}}.Encode()
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
