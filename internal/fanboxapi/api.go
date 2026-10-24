// Package fanboxapi owns the FANBOX protocol: the session transport and its
// challenge handling, API routes and query names, and the media URL policy.
// The public SDK business client lives in sdk/fanbox.
package fanboxapi

// FANBOX API 路径，相对 APIBaseURL。
const (
	PathCreatorGet           = "creator.get"
	PathCreatorListFollowing = "creator.listFollowing"
	PathPlanListSupporting   = "plan.listSupporting"
	PathTagGetFeatured       = "tag.getFeatured"
	PathPostInfo             = "post.info"
	PathPostListHome         = "post.listHome"
	PathPostListCreator      = "post.listCreator"
	PathPostListTagged       = "post.listTagged"
	PathPostListSupporting   = "post.listSupporting"
)

// query 参数名。
const (
	ParamCreatorID = "creatorId"
	ParamPostID    = "postId"
	ParamTag       = "tag"
	ParamLimit     = "limit"
)
