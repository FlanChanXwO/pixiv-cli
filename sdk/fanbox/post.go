// FANBOX 文章操作：单篇详情、home/supporting 信息流，以及创作者文章列表。
// 响应结构与映射在 wire.go，游标编解码在 cursor.go。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	protocol "github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Post returns one post by its stable ID.
func (c *Client) Post(ctx context.Context, request PostRequest) (Post, error) {
	ctx = protocol.WithOperation(ctx, "Post")
	if request.PostID == "" {
		return Post{}, newError("Post", sdk.InvalidArgument, errors.New("post id is required"))
	}
	source, err := c.postInfo(ctx, request.PostID)
	if err != nil {
		return Post{}, classifyError("Post", err)
	}
	post, err := c.mapPost(source)
	if err != nil {
		return Post{}, classifyResponseError("Post", err)
	}
	return post, nil
}

// postInfo fetches the raw post.info response; resource re-resolution reads the
// same body without building resources a second time.
func (c *Client) postInfo(ctx context.Context, postID string) (postWire, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return postWire{}, errors.New("FANBOX post id is required")
	}
	var response struct {
		Body postInfoWire `json:"body"`
	}
	query := url.Values{protocol.ParamPostID: {postID}}
	if err := c.app.GetJSON(ctx, protocol.PathPostInfo, query, &response); err != nil {
		return postWire{}, err
	}
	return response.Body.Post, nil
}

// CreatorPosts lists one creator's posts.
func (c *Client) CreatorPosts(ctx context.Context, request CreatorPostsRequest) (sdk.Page[Post], error) {
	ctx = protocol.WithOperation(ctx, "CreatorPosts")
	if request.CreatorID == "" {
		return sdk.Page[Post]{}, newError("CreatorPosts", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	creatorID := strings.TrimSpace(request.CreatorID)
	if creatorID == "" {
		return sdk.Page[Post]{}, classifyError("CreatorPosts", errors.New("FANBOX creator id is required"))
	}
	cursorQuery := url.Values{protocol.ParamCreatorID: {request.CreatorID}}
	query := url.Values{protocol.ParamCreatorID: {creatorID}}
	return c.postList(ctx, "CreatorPosts", protocol.PathPostListCreator, cursorQuery, query, true, request.Cursor)
}

// TaggedPosts lists one creator's posts for a single tag.
func (c *Client) TaggedPosts(ctx context.Context, request TaggedPostsRequest) (sdk.Page[Post], error) {
	ctx = protocol.WithOperation(ctx, "TaggedPosts")
	if request.CreatorID == "" || request.Tag == "" {
		return sdk.Page[Post]{}, newError("TaggedPosts", sdk.InvalidArgument, errors.New("creator id and tag are required"))
	}
	creatorID := strings.TrimSpace(request.CreatorID)
	if creatorID == "" {
		return sdk.Page[Post]{}, classifyError("TaggedPosts", errors.New("FANBOX creator id is required"))
	}
	tag := strings.TrimSpace(request.Tag)
	if tag == "" {
		return sdk.Page[Post]{}, classifyError("TaggedPosts", errors.New("FANBOX tag is required"))
	}
	cursorQuery := url.Values{protocol.ParamCreatorID: {request.CreatorID}, protocol.ParamTag: {request.Tag}}
	query := url.Values{protocol.ParamCreatorID: {creatorID}, protocol.ParamTag: {tag}}
	return c.postList(ctx, "TaggedPosts", protocol.PathPostListTagged, cursorQuery, query, false, request.Cursor)
}

// Home lists the current user's home feed.
func (c *Client) Home(ctx context.Context, request HomeRequest) (sdk.Page[Post], error) {
	ctx = protocol.WithOperation(ctx, "Home")
	return c.postList(ctx, "Home", protocol.PathPostListHome, nil, nil, true, request.Cursor)
}

// Supporting lists posts from the current user's supporting creators.
func (c *Client) Supporting(ctx context.Context, request SupportingRequest) (sdk.Page[Post], error) {
	ctx = protocol.WithOperation(ctx, "Supporting")
	return c.postList(ctx, "Supporting", protocol.PathPostListSupporting, nil, nil, true, request.Cursor)
}

// postList fetches one post page. cursorQuery binds the cursor and query is sent
// on the first page; pages carrying a server continuation use it verbatim.
// First pages add the explicit upstream page limit when requested. The two query
// values differ when a request carries surrounding whitespace: the digest keeps
// the caller's value so existing cursors stay valid, while the wire sees it trimmed.
func (c *Client) postList(ctx context.Context, operation, path string, cursorQuery, query url.Values, limit bool, cursor sdk.Cursor) (sdk.Page[Post], error) {
	nextURL, err := c.continuationURL(ctx, operation, cursorQuery, cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	if limit && strings.TrimSpace(nextURL) == "" {
		query = cloneValues(query)
		query.Set(protocol.ParamLimit, strconv.Itoa(protocol.PostListPageLimit))
	}
	var body json.RawMessage
	acceptItems := path == protocol.PathPostListHome || path == protocol.PathPostListSupporting
	if err := c.getListBody(ctx, path, query, nextURL, &body, operation); err != nil {
		return sdk.Page[Post]{}, err
	}
	var page postPageWire
	if err := json.Unmarshal(body, &page); err != nil {
		return sdk.Page[Post]{}, classifyError(operation, errors.New("decode FANBOX post list response"))
	}
	wirePosts := page.Posts
	if acceptItems && wirePosts == nil {
		wirePosts = page.Items
	}
	items := make([]Post, 0, len(wirePosts))
	for _, source := range wirePosts {
		post, err := c.mapPost(source)
		if err != nil {
			return sdk.Page[Post]{}, classifyResponseError(operation, err)
		}
		items = append(items, post)
	}
	next, err := c.buildCursor(ctx, operation, cursorQuery, firstContinuation(page.PageURLs, page.NextURL))
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	return sdk.Page[Post]{Items: items, Next: next}, nil
}

func cloneValues(source url.Values) url.Values {
	clone := make(url.Values, len(source))
	for key, values := range source {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
