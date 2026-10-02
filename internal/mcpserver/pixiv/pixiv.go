// Package pixiv 聚合 Pixiv MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package，共享
// runtime/records/filters/outputs 在 internal 子包，HTTP transport 与服务生命周期由父包提供。
package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_list"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_status"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/account_use"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/add_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/add_novel_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/blocked_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_list_all"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags_all"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/create_artwork_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/create_novel_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/delete_artwork_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/delete_novel_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/follow_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_ranking"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_related"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_series"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_illusts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_novels"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_bookmark_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_bookmark_tags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_content"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_series"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/related_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/remove_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/remove_novel_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/reply_artwork_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/reply_novel_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/reverse_search"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_illust"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/stamp_artwork_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/stamp_novel_comment"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_illust_following"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_illust_latest"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_novel_following"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_novel_latest"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/trending_tags_illust"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/unfollow_user"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_artworks"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_bookmarks"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_followers"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_following"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_novel_bookmarks"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_novels"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 是 MCP 请求的本地值；只携带传输覆写与账号选择，不持有
// client 或凭据。
type Account = runtime.Account

// SDKPorts 是 MCP 对 Pixiv SDK 的窄端口：打开独立认证快照、在账号池重放边界内
// 执行操作。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts = runtime.SDKPorts

// ReverseSearchPorts 是 reverse_search 的启动时配置与能力注入。
type ReverseSearchPorts = runtime.ReverseSearchPorts

// Register 将 Pixiv tools 注册到共享 server，账号和 SDK 生命周期仍由 Pixiv runtime 持有。
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	app := runtime.NewApp(ports, account)
	account_use.Register(app, server)
	account_status.Register(app, server)
	account_list.Register(app, server)
	add_bookmark.Register(app, server)
	add_novel_bookmark.Register(app, server)
	blocked_users.Register(app, server)
	create_artwork_comment.Register(app, server)
	create_novel_comment.Register(app, server)
	delete_artwork_comment.Register(app, server)
	delete_novel_comment.Register(app, server)
	bookmark_detail.Register(app, server)
	bookmark_list_all.Register(app, server)
	bookmark_tags.Register(app, server)
	bookmark_tags_all.Register(app, server)
	follow_user.Register(app, server)
	illust_comments.Register(app, server)
	illust_detail.Register(app, server)
	illust_ranking.Register(app, server)
	illust_recommended.Register(app, server)
	illust_related.Register(app, server)
	illust_series.Register(app, server)
	mypixiv_illusts.Register(app, server)
	mypixiv_novels.Register(app, server)
	mypixiv_users.Register(app, server)
	novel_bookmark_detail.Register(app, server)
	novel_bookmark_tags.Register(app, server)
	novel_comments.Register(app, server)
	novel_content.Register(app, server)
	novel_detail.Register(app, server)
	novel_series.Register(app, server)
	recommended.Register(app, server)
	related_users.Register(app, server)
	reply_artwork_comment.Register(app, server)
	reply_novel_comment.Register(app, server)
	remove_bookmark.Register(app, server)
	remove_novel_bookmark.Register(app, server)
	reverse_search.Register(app, server)
	search_illust.Register(app, server)
	search_novel.Register(app, server)
	search_user.Register(app, server)
	stamp_artwork_comment.Register(app, server)
	stamp_novel_comment.Register(app, server)
	timeline_illust_following.Register(app, server)
	timeline_illust_latest.Register(app, server)
	timeline_novel_following.Register(app, server)
	timeline_novel_latest.Register(app, server)
	trending_tags_illust.Register(app, server)
	unfollow_user.Register(app, server)
	user_artworks.Register(app, server)
	user_bookmarks.Register(app, server)
	user_detail.Register(app, server)
	user_followers.Register(app, server)
	user_following.Register(app, server)
	user_novel_bookmarks.Register(app, server)
	user_novels.Register(app, server)
}
