// Package pixiv exposes the Pixiv MCP product: its account value, SDK ports,
// and the single registration entry point for all Pixiv tools.
package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Account = toolhost.Account

// SDKPorts 是 MCP 对 Pixiv SDK 的窄端口：打开独立认证快照、在账号池重放边界内
// 执行操作。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts = toolhost.SDKPorts

// LocalDownloads configures stdio filesystem delivery through the shared downloader.
type LocalDownloads = toolhost.LocalDownloads

// ReverseSearchPorts 是 reverse_search 的启动时配置与能力注入。
type ReverseSearchPorts = toolhost.ReverseSearchPorts

// Register 将 Pixiv tools 注册到共享 server，账号和 SDK 生命周期仍由 Pixiv runtime 持有。
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	tools.Register(toolhost.NewApp(ports, account), server)
}
