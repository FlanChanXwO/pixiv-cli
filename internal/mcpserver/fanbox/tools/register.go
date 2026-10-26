// FANBOX tool registration and the product tool set.
package tools

import (
	"context"
	"errors"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account carries the per-runtime FANBOX transport override.
type Account struct{ HTTPSProxyOverride *string }

// SDKPorts provides the client lease owner and execution logger.
// A successful OpenLease returns a valid client lease, owned by the caller.
type SDKPorts struct {
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*fanbox.Client], error)
	Logger    *slog.Logger
}

type fanboxTools struct {
	open    func(context.Context, Account) (*lifecycle.Lease[*fanbox.Client], error)
	account Account
	calls   *toolrun.Scope
}

func (t fanboxTools) openClient(ctx context.Context) (*lifecycle.Lease[*fanbox.Client], error) {
	if t.open == nil {
		return nil, errors.New("fanbox service is not configured")
	}
	return t.open(ctx, t.account)
}

// Register adds every FANBOX tool.
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	t := fanboxTools{open: ports.OpenLease, account: account, calls: toolrun.NewScope(ports.Logger)}
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
