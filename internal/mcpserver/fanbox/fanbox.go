// Package fanbox registers FANBOX tools into the unified MCP server.
package fanbox

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account exposes the tool-owned transport override to composition roots.
type Account = tools.Account

// SDKPorts exposes the tool-owned SDK and execution-log ports.
type SDKPorts = tools.SDKPorts

// Register keeps FANBOX account and transport policy independent from Pixiv.
func Register(server *mcp.Server, ports SDKPorts, proxyOverride *string) {
	tools.Register(server, ports, Account{HTTPSProxyOverride: proxyOverride})
}
