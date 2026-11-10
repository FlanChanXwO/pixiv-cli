package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	// APIHost names the origin used to validate and rebuild API continuations.
	APIHost = "api.fanbox.cc"
	// APIBaseURL prefixes the product-owned endpoint paths.
	APIBaseURL = "https://" + APIHost + "/"
	acceptJSON = "application/json, text/plain, */*"
)

// FANBOX read endpoint paths are relative to APIBaseURL.
const (
	// PathCreatorGet fetches a creator profile.
	PathCreatorGet = "creator.get"
	// PathCreatorListFollowing lists the current account's followed creators.
	PathCreatorListFollowing = "creator.listFollowing"
	// PathPlanListSupporting lists the current account's supported creators.
	PathPlanListSupporting = "plan.listSupporting"
	// PathTagGetFeatured fetches featured tags for a creator.
	PathTagGetFeatured = "tag.getFeatured"
	// PathPostInfo fetches one post's details.
	PathPostInfo = "post.info"
	// PathPostListCreator lists a creator's posts.
	PathPostListCreator = "post.listCreator"
	// PathPostListTagged lists a creator's posts under a tag.
	PathPostListTagged = "post.listTagged"
	// PathPostListHome lists the current account's home feed.
	PathPostListHome = "post.listHome"
	// PathPostListSupporting lists posts from supported creators.
	PathPostListSupporting = "post.listSupporting"
)

const (
	// ParamCreatorID selects a creator.
	ParamCreatorID = "creatorId"
	// ParamPostID selects a post.
	ParamPostID = "postId"
	// ParamTag selects a creator's tag.
	ParamTag = "tag"
	// ParamLimit sets the initial post list size.
	ParamLimit = "limit"
)

// PostListPageLimit preserves the upstream initial post-page size.
const PostListPageLimit = 10

// GetJSON reads a product-owned API endpoint, preserving transport and close
// failures while excluding raw upstream documents from decoding errors.
func (c *Client) GetJSON(ctx context.Context, operation, path string, query url.Values, target any) error {
	endpoint := APIBaseURL + path
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	response, err := c.do(ctx, operation, endpoint, requestProfile{accept: acceptJSON, method: http.MethodGet})
	if err != nil {
		return err
	}
	document, err := readResponse(ctx, operation, response)
	if err != nil {
		return err
	}
	if json.Unmarshal(document, target) != nil {
		return failure(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX API JSON", nil)
	}
	return nil
}
