// FANBOX API 路径与 query 参数名。请求发送在 protocol.go，媒体定位规则在 resource.go。

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
