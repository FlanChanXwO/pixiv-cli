// Package tools 实现 Pixiv 的 MCP 工具：输入 schema、SDK 调用、分页与结构化输出。
//
// 包的组成：[Register] 按注入端口注册全部 Pixiv 工具，其余文件按实体分组实现。
// 作品、收藏、评论、小说、用户、搜索、时间线与媒体各有独立文件；公共输出在
// internal/outputs，执行与分页在 internal/toolhost。端口缺失的工具不会注册。
package tools

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pixivTools struct{ app *toolhost.App }

// Register 按可用端口注册 Pixiv 工具。缺少媒体、账号、登录、下载或反向搜图端口时，
// 依赖它的工具不会注册；其余工具照常可用。
func Register(app *toolhost.App, server *mcp.Server) {
	ports := app.SDKPorts()
	if ports.Media != nil {
		gallery.Register(server, ports.Media)
	}
	t := pixivTools{app: app}
	if ports.Downloads != nil {
		t.registerDownload(server)
	}
	if ports.Accounts.Load != nil {
		t.registerAccountUse(server)
		t.registerAccountList(server)
	}
	if ports.Accounts.Load != nil || ports.Login != nil {
		t.registerAccountStatus(server)
	}
	if ports.Login != nil {
		t.registerAccountLoginStart(server)
	}
	t.registerArtworkMedia(server)
	if ports.Media != nil {
		t.registerGalleryAnimation(server)
		t.registerShowGallery(server)
	}
	t.registerBookmarkAdd(server)
	t.registerBlockedUsers(server)
	t.registerCommentCreate(server)
	t.registerCommentDelete(server)
	t.registerBookmarkDetail(server)
	t.registerBookmarkListAll(server)
	t.registerBookmarkTags(server)
	t.registerBookmarkTagsAll(server)
	t.registerFollowUser(server)
	t.registerComments(server)
	t.registerIllustDetail(server)
	t.registerIllustRanking(server)
	t.registerIllustRecommended(server)
	t.registerIllustRelated(server)
	t.registerIllustSeries(server)
	t.registerMypixiv(server)
	t.registerNovelBookmarkDetail(server)
	t.registerNovelContent(server)
	t.registerNovelDetail(server)
	t.registerNovelSeries(server)
	t.registerRecommended(server)
	t.registerCommentReply(server)
	t.registerBookmarkRemove(server)
	t.registerReverseSearch(server)
	t.registerSearchIllust(server)
	t.registerSearchNovel(server)
	t.registerSearchUser(server)
	t.registerCommentStamp(server)
	t.registerTimelineFollowing(server)
	t.registerTimelineLatest(server)
	t.registerTrendingTagsIllust(server)
	t.registerUnfollowUser(server)
	t.registerUserArtworks(server)
	t.registerUserBookmarks(server)
	t.registerUserDetail(server)
	t.registerUserRelations(server)
	t.registerUserNovels(server)
}
