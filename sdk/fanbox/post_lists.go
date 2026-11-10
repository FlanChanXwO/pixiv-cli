package fanbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// PostsRequest selects all posts or a tag on the referenced creator.
// Repeat Tag and the same creator/account when restoring Cursor.
type PostsRequest struct {
	Tag    string
	Cursor sdk.Cursor
}

// FeedRequest resumes an account-bound home or supporting feed.
type FeedRequest struct{ Cursor sdk.Cursor }

// Posts lists this creator's posts, optionally filtered by tag.
func (r *CreatorRef) Posts(ctx context.Context, request PostsRequest) (sdk.Page[*Post], error) {
	const operation = "Creator.Posts"
	if err := r.bound(operation); err != nil {
		return sdk.Page[*Post]{}, err
	}
	base := url.Values{api.ParamCreatorID: {strings.TrimSpace(r.ID)}}
	path := api.PathPostListCreator
	if request.Tag != "" {
		tag := strings.TrimSpace(request.Tag)
		if tag == "" {
			return sdk.Page[*Post]{}, newError(operation, sdk.InvalidArgument, "tag must contain a non-space character")
		}
		base.Set(api.ParamTag, tag)
		path = api.PathPostListTagged
	}
	return r.client.postList(ctx, operation, path, base, request.Cursor)
}

// Home lists the current account's home feed.
func (c *Client) Home(ctx context.Context, request FeedRequest) (sdk.Page[*Post], error) {
	return c.postList(ctx, "Home", api.PathPostListHome, url.Values{}, request.Cursor)
}

// Supporting lists posts from creators supported by the current account.
func (c *Client) Supporting(ctx context.Context, request FeedRequest) (sdk.Page[*Post], error) {
	return c.postList(ctx, "Supporting", api.PathPostListSupporting, url.Values{}, request.Cursor)
}

type postPageWire struct {
	Posts []postWire `json:"posts"`
	Items []postWire `json:"items"`
	paginationWire
}

func (c *Client) postList(ctx context.Context, operation, path string, base url.Values, token sdk.Cursor) (sdk.Page[*Post], error) {
	query, binding, err := c.pageQuery(ctx, operation, path, base, token)
	if err != nil {
		return sdk.Page[*Post]{}, err
	}
	if token.IsZero() && path != api.PathPostListTagged {
		query.Set(api.ParamLimit, strconv.Itoa(api.PostListPageLimit))
	}
	body, err := c.getBody(ctx, operation, path, query)
	if err != nil {
		return sdk.Page[*Post]{}, err
	}
	var page postPageWire
	if json.Unmarshal(body, &page) != nil {
		return sdk.Page[*Post]{}, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX post list")
	}
	source := page.Posts
	if source == nil && (path == api.PathPostListHome || path == api.PathPostListSupporting) {
		source = page.Items
	}
	if source == nil {
		return sdk.Page[*Post]{}, newError(operation, sdk.MalformedUpstreamResponse, "missing FANBOX post list")
	}
	items := make([]*Post, len(source))
	for i, entry := range source {
		items[i], err = c.mapPost(operation, entry)
		if err != nil {
			return sdk.Page[*Post]{}, err
		}
	}
	next, err := nextCursor(page.continuation(), operation, path, query, binding)
	if err != nil {
		return sdk.Page[*Post]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Post], error) {
		return c.postList(ctx, operation, path, base, next)
	}), nil
}
