// Package fanbox 聚合 FANBOX MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package，stdio
// runtime 归 internal/mcpserver/stdio。
package fanbox

import (
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/creator"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/creatorposts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/creators"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/creatortags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/currentuser"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/home"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/openresource"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/resolveurl"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/supporting"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/taggedposts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New 构造 FANBOX MCP server。
func New(service *fanboxapp.Service) *mcp.Server {
	return NewWithProxy(service, nil)
}

// NewWithProxy constructs the server with a native FANBOX proxy override. It
// does not alter FlareSolverr service or upstream proxy configuration.
func NewWithProxy(service *fanboxapp.Service, proxyOverride *string) *mcp.Server {
	app := runtime.NewApp(service, proxyOverride)
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli-fanbox", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "FANBOX MCP server for browsing creators, posts, tags, and media resources.",
	})
	register(app, server)
	return server
}

func register(app *runtime.App, server *mcp.Server) {
	currentuser.Register(app, server)
	creator.Register(app, server)
	creators.Register(app, server)
	creatortags.Register(app, server)
	creatorposts.Register(app, server)
	taggedposts.Register(app, server)
	post.Register(app, server)
	home.Register(app, server)
	supporting.Register(app, server)
	resolveurl.Register(app, server)
	openresource.Register(app, server)
}
