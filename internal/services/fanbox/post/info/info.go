// Package info owns the FANBOX post.info endpoint.
package info

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/wire"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Transport is the narrow JSON capability required by post.info.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// Request selects one post by ID.
type Request struct {
	PostID string
}

// Client owns the post.info route and response envelope.
type Client struct {
	transport Transport
}

// New constructs a post.info endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// Get fetches and normalizes one post.
func (c *Client) Get(ctx context.Context, request Request) (post.Post, error) {
	if c == nil || c.transport == nil {
		return post.Post{}, errors.New("FANBOX post.info transport is not configured")
	}
	id := strings.TrimSpace(request.PostID)
	if id == "" {
		return post.Post{}, errors.New("FANBOX post id is required")
	}
	endpoint := protocol.APIBaseURL + "post.info?" + url.Values{"postId": {id}}.Encode()
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := c.transport.GetJSON(ctx, endpoint, &response); err != nil {
		return post.Post{}, err
	}
	return wire.DecodePostInfo(response.Body)
}
