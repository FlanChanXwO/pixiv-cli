// Pixiv tool registration and the product tool set.
package tools

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pixivTools struct{ app *toolhost.App }

// Register adds every Pixiv tool. A tool whose media, account, login, download,
// or reverse-search port is absent stays unregistered, as before.
func Register(app *toolhost.App, server *mcp.Server, ports toolhost.SDKPorts) {
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
