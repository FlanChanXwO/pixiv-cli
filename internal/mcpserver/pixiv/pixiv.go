// Package pixiv 聚合 Pixiv MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package，共享
// runtime/records/filters/outputs 在 internal 子包，stdio runtime 由父包提供。
package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/add_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/blocked_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/download"
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
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_content"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_detail"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_series"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/related_users"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/remove_bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_illust"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_user"
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
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DownloadManager 是下载器在 MCP 侧的窄能力接口。
type DownloadManager = runtime.DownloadManager

// Account 是 MCP 请求的本地值；只携带传输覆写与账号选择，不持有
// client 或凭据。
type Account = runtime.Account

// SDKPorts 是 MCP 对 Pixiv SDK 的窄端口：打开独立认证快照、在账号池重放边界内
// 执行操作。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts = runtime.SDKPorts

// New 保留构造参数位置以便嵌入方平滑升级；第一个参数不再被读取，所有 Pixiv
// 能力必须由 public SDK ports 提供。
func New(_ any, downloads DownloadManager) *mcp.Server {
	return newServer(runtime.NewApp(downloads, nil, SDKPorts{}, Account{}))
}

// NewWithSDK 通过 services Facade 的窄端口为每个 MCP tool 建立独立 client snapshot。
// 首个参数仅是已废弃的兼容占位，绝不构成内容、认证或资源调用链。
func NewWithSDK(_ any, downloads DownloadManager, ports SDKPorts, account Account) *mcp.Server {
	return newServer(runtime.NewApp(downloads, nil, ports, account))
}

// NewWithSDKDownloadFactory 为生产 MCP 注入 snapshot-scoped 下载器构造器。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(*pixiv.Client) DownloadManager, ports SDKPorts, account Account) *mcp.Server {
	return newServer(runtime.NewApp(downloads, newDownloads, ports, account))
}

func newServer(app *runtime.App) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "3.0.0"}, &mcp.ServerOptions{
		Instructions: "Pixiv MCP server for searching, browsing, and downloading Pixiv content.",
	})
	register(app, server)
	return server
}

func register(app *runtime.App, server *mcp.Server) {
	add_bookmark.Register(app, server)
	blocked_users.Register(app, server)
	bookmark_detail.Register(app, server)
	bookmark_tags.Register(app, server)
	download.Register(app, server)
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
	novel_comments.Register(app, server)
	novel_content.Register(app, server)
	novel_detail.Register(app, server)
	novel_series.Register(app, server)
	recommended.Register(app, server)
	related_users.Register(app, server)
	remove_bookmark.Register(app, server)
	search_illust.Register(app, server)
	search_novel.Register(app, server)
	search_user.Register(app, server)
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
