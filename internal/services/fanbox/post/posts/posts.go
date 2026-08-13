// Package posts owns creator post-list endpoint families.
package posts

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/wire"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Transport is the narrow JSON capability required by creator post lists.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// Request selects one creator's post list. Tag is optional for the tagged
// route and must be empty for the creator route.
type Request struct {
	CreatorID string
	Tag       string
	NextURL   string
}

// Client owns creator post-list routes and response conversion.
type Client struct {
	transport Transport
}

// New constructs a creator post-list endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// Creator lists all posts for a creator page.
func (c *Client) Creator(ctx context.Context, request Request) (post.Page, error) {
	id := strings.TrimSpace(request.CreatorID)
	if err := c.validate(id); err != nil {
		return post.Page{}, err
	}
	endpoint := protocol.APIBaseURL + "post.listCreator?" + url.Values{
		"creatorId": {id},
		"limit":     {strconv.Itoa(protocol.PostListPageLimit)},
	}.Encode()
	return c.fetch(ctx, endpoint, request.NextURL)
}

// Tagged lists posts for one creator and tag.
func (c *Client) Tagged(ctx context.Context, request Request) (post.Page, error) {
	id := strings.TrimSpace(request.CreatorID)
	tag := strings.TrimSpace(request.Tag)
	if err := c.validate(id); err != nil {
		return post.Page{}, err
	}
	if tag == "" {
		return post.Page{}, errors.New("FANBOX tag is required")
	}
	endpoint := protocol.APIBaseURL + "post.listTagged?" + url.Values{"creatorId": {id}, "tag": {tag}}.Encode()
	return c.fetch(ctx, endpoint, request.NextURL)
}

func (c *Client) validate(creatorID string) error {
	if c == nil || c.transport == nil {
		return errors.New("FANBOX creator posts transport is not configured")
	}
	if creatorID == "" {
		return errors.New("FANBOX creator id is required")
	}
	return nil
}

func (c *Client) fetch(ctx context.Context, endpoint, nextURL string) (post.Page, error) {
	target := endpoint
	if strings.TrimSpace(nextURL) != "" {
		target = nextURL
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
	return wire.DecodePage(response.Body, false)
}
