package fanbox

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
)

const creatorPagesPosition = "pages"

func (c *Client) creatorPosts(ctx context.Context, operation string, base url.Values, token sdk.Cursor) (sdk.Page[*Post], error) {
	_, binding, err := c.pageQuery(ctx, operation, api.PathPostListCreator, base, sdk.Cursor{})
	if err != nil {
		return sdk.Page[*Post]{}, err
	}
	var pages []string
	if token.IsZero() {
		body, err := c.getBody(ctx, operation, api.PathPostPaginateCreator, base)
		if err != nil {
			return sdk.Page[*Post]{}, err
		}
		var source struct {
			PageURLs []string `json:"pageUrls"`
		}
		if json.Unmarshal(body, &source) != nil || source.PageURLs == nil {
			return sdk.Page[*Post]{}, newError(operation, sdk.MalformedUpstreamResponse, "missing FANBOX creator pages")
		}
		for _, raw := range source.PageURLs {
			position, err := continuationURLPosition(raw, operation, api.PathPostListCreator, binding.Query)
			if err != nil {
				return sdk.Page[*Post]{}, err
			}
			pages = append(pages, position.Encode())
		}
	} else {
		position, err := cursor.Decode(token, binding)
		if err != nil {
			return sdk.Page[*Post]{}, err
		}
		pages = position[creatorPagesPosition]
		if len(position) != 1 || len(pages) == 0 {
			return sdk.Page[*Post]{}, newError(operation, sdk.InvalidArgument, "invalid FANBOX creator pages cursor")
		}
	}
	queries := make([]url.Values, len(pages))
	seen := make(map[string]bool)
	for i, page := range pages {
		values, err := url.ParseQuery(page)
		position, valid := continuationPosition(values, base)
		if err != nil || !valid || seen[position.Encode()] {
			reason := sdk.MalformedUpstreamResponse
			if !token.IsZero() {
				reason = sdk.InvalidArgument
			}
			return sdk.Page[*Post]{}, newError(operation, reason, "invalid or repeated FANBOX creator page")
		}
		seen[position.Encode()] = true
		queries[i] = position
		for key, entries := range base {
			queries[i][key] = entries
		}
	}
	if len(pages) == 0 {
		return sdk.NewPage([]*Post{}, sdk.Cursor{}, nil), nil
	}
	items, continuation, err := c.readPosts(ctx, operation, api.PathPostListCreator, queries[0])
	if err != nil {
		return sdk.Page[*Post]{}, err
	}
	if _, err := nextCursor(continuation, operation, api.PathPostListCreator, queries[0], binding); err != nil {
		return sdk.Page[*Post]{}, err
	}
	var next sdk.Cursor
	if len(pages) > 1 {
		next, err = cursor.Encode(binding, url.Values{creatorPagesPosition: pages[1:]})
		if err != nil {
			return sdk.Page[*Post]{}, err
		}
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Post], error) {
		return c.creatorPosts(ctx, operation, base, next)
	}), nil
}
