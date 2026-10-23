// FANBOX tool registration and the product tool set.
package tools

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fanboxTools struct{ app *toolhost.App }

// Register adds every FANBOX tool.
func Register(app *toolhost.App, server *mcp.Server) {
	t := fanboxTools{app: app}
	t.registerCurrentuser(server)
	t.registerCreator(server)
	t.registerCreators(server)
	t.registerCreatortags(server)
	t.registerCreatorposts(server)
	t.registerPost(server)
	t.registerFeed(server)
	t.registerResolveurl(server)
	t.registerOpenresource(server)
}
