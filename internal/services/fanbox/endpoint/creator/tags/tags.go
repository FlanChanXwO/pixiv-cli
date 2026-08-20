// Package tags owns the FANBOX creator featured-tags endpoint.
package tags

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Transport is the narrow JSON capability required by the tags endpoint.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// Tag is a normalized creator tag.
type Tag struct {
	Name string
	URL  string
}

// Request selects a creator's featured tags.
type Request struct {
	CreatorID string
}

// Client owns the tag endpoint route and wire conversion.
type Client struct {
	transport Transport
}

// New constructs a tags endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// List fetches a creator's featured tags.
func (c *Client) List(ctx context.Context, request Request) ([]Tag, error) {
	if c == nil || c.transport == nil {
		return nil, errors.New("FANBOX tags transport is not configured")
	}
	id := strings.TrimSpace(request.CreatorID)
	if id == "" {
		return nil, errors.New("FANBOX creator id is required")
	}
	endpoint := protocol.APIBaseURL + "tag.getFeatured?" + url.Values{"creatorId": {id}}.Encode()
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := c.transport.GetJSON(ctx, endpoint, &response); err != nil {
		return nil, err
	}
	var direct []tagDTO
	if err := json.Unmarshal(response.Body, &direct); err == nil {
		return mapTags(direct)
	}
	var wrapped struct {
		Tags []tagDTO `json:"tags"`
	}
	if err := json.Unmarshal(response.Body, &wrapped); err != nil {
		return nil, errors.New("decode FANBOX creator tags")
	}
	return mapTags(wrapped.Tags)
}

type tagDTO struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

func mapTags(items []tagDTO) ([]Tag, error) {
	result := make([]Tag, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Tag)
		if name == "" {
			return nil, errors.New("FANBOX creator tag response includes an empty tag")
		}
		result = append(result, Tag{Name: name, URL: item.URL})
	}
	return result, nil
}
