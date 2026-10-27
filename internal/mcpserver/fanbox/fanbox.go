// Package fanbox 把 FANBOX 工具注册进共享 MCP server。
//
// 包的组成：[Register] 完成工具注册，[Account] 与 [SDKPorts] 承载组合根注入的
// 传输与 SDK 端口，两者直接复用 tools 包的定义。具体工具实现在 tools 子包，本包只做
// 产品级装配。
package fanbox

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 携带组合根传入的 FANBOX 传输覆写。
type Account = tools.Account

// SDKPorts 提供打开 FANBOX client lease 的函数与执行日志。
type SDKPorts = tools.SDKPorts

// Register 只添加 FANBOX 工具；账号、传输覆写与 Pixiv 的另一套端口互不影响。
func Register(server *mcp.Server, ports SDKPorts, proxyOverride *string) {
	tools.Register(server, ports, Account{HTTPSProxyOverride: proxyOverride})
}
