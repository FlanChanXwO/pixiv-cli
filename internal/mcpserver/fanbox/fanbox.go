// Package fanbox 聚合 FANBOX MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package，stdio
// runtime 由父包提供。
package fanbox

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creator"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creatorposts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creators"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creatortags"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/currentuser"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/home"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/openresource"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/resolveurl"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/supporting"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/taggedposts"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account carries only per-request transport overrides into the MCP SDK port.
type Account struct {
	HTTPSProxyOverride *string
}

// SDKPorts is the narrow FANBOX capability injected by the composition root.
// The MCP server does not know how accounts are stored or rotated.
type SDKPorts struct {
	// Open is retained as a raw-client adapter for existing embeddings. New
	// composition roots should inject OpenLease.
	Open      func(context.Context, Account) (*fanboxsdk.Client, error)
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*fanboxsdk.Client], error)
}

// New 构造 FANBOX MCP server。
func New(ports SDKPorts) *mcp.Server {
	return NewWithProxy(ports, nil)
}

// NewWithProxy constructs the server with a native FANBOX proxy override. It
// does not alter FlareSolverr service or upstream proxy configuration.
func NewWithProxy(ports SDKPorts, proxyOverride *string) *mcp.Server {
	runtimePorts := runtime.SDKPorts{Open: func(ctx context.Context, account runtime.Account) (*fanboxsdk.Client, error) {
		if ports.Open == nil {
			return nil, errors.New("fanbox SDK ports are not configured")
		}
		return ports.Open(ctx, Account{HTTPSProxyOverride: account.HTTPSProxyOverride})
	}}
	if ports.OpenLease != nil {
		runtimePorts.OpenLease = func(ctx context.Context, account runtime.Account) (*lifecycle.Lease[*fanboxsdk.Client], error) {
			return ports.OpenLease(ctx, Account{HTTPSProxyOverride: account.HTTPSProxyOverride})
		}
	}
	if ports.Open == nil {
		runtimePorts.Open = nil
	}
	app := runtime.NewApp(runtimePorts, runtime.Account{HTTPSProxyOverride: proxyOverride})
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
