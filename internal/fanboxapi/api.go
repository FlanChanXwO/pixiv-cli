// FANBOX API routes and query parameter names.

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
