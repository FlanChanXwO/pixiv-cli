// Package pixiv 聚合 Pixiv MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package，共享
// runtime/records/filters/outputs 在 internal 子包，HTTP transport 与服务生命周期由父包提供。
package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_list"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_login_start"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_status"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_use"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/artwork_media"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/blocked_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_add"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_list_all"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_remove"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags_all"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/comment_create"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/comment_delete"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/comment_reply"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/comment_stamp"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/follow_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/gallery_animation"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_ranking"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_related"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_series"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_bookmark_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_content"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_series"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/reverse_search"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_illust"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/show_gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_following"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_latest"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/trending_tags_illust"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/unfollow_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_artworks"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_bookmarks"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_novels"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_relations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 是 MCP 请求的本地值；只携带传输覆写与账号选择，不持有
// client 或凭据。
type Account = runtime.Account

// SDKPorts 是 MCP 对 Pixiv SDK 的窄端口：打开独立认证快照、在账号池重放边界内
// 执行操作。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts = runtime.SDKPorts

// LocalDownloads configures stdio filesystem delivery through the shared downloader.
type LocalDownloads = runtime.LocalDownloads

// ReverseSearchPorts 是 reverse_search 的启动时配置与能力注入。
type ReverseSearchPorts = runtime.ReverseSearchPorts

// Register 将 Pixiv tools 注册到共享 server，账号和 SDK 生命周期仍由 Pixiv runtime 持有。
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	if ports.Media != nil {
		gallery.Register(server, ports.Media)
	}
	app := runtime.NewApp(ports, account)
	if ports.Downloads != nil {
		download.Register(app, server)
	}
	if ports.Accounts.Load != nil {
		account_use.Register(app, server)
		account_list.Register(app, server)
	}
	if ports.Accounts.Load != nil || ports.Login != nil {
		account_status.Register(app, server)
	}
	if ports.Login != nil {
		account_login_start.Register(app, server)
	}
	artwork_media.Register(app, server)
	if ports.Media != nil {
		gallery_animation.Register(app, server)
		show_gallery.Register(app, server)
	}
	bookmark_add.Register(app, server)
	blocked_users.Register(app, server)
	comment_create.Register(app, server)
	comment_delete.Register(app, server)
	bookmark_detail.Register(app, server)
	bookmark_list_all.Register(app, server)
	bookmark_tags.Register(app, server)
	bookmark_tags_all.Register(app, server)
	follow_user.Register(app, server)
	comments.Register(app, server)
	illust_detail.Register(app, server)
	illust_ranking.Register(app, server)
	illust_recommended.Register(app, server)
	illust_related.Register(app, server)
	illust_series.Register(app, server)
	mypixiv.Register(app, server)
	novel_bookmark_detail.Register(app, server)
	novel_content.Register(app, server)
	novel_detail.Register(app, server)
	novel_series.Register(app, server)
	recommended.Register(app, server)
	comment_reply.Register(app, server)
	bookmark_remove.Register(app, server)
	reverse_search.Register(app, server)
	search_illust.Register(app, server)
	search_novel.Register(app, server)
	search_user.Register(app, server)
	comment_stamp.Register(app, server)
	timeline_following.Register(app, server)
	timeline_latest.Register(app, server)
	trending_tags_illust.Register(app, server)
	unfollow_user.Register(app, server)
	user_artworks.Register(app, server)
	user_bookmarks.Register(app, server)
	user_detail.Register(app, server)
	user_relations.Register(app, server)
	user_novels.Register(app, server)
}
