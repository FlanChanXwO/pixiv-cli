// Package pixiv 是 Pixiv 产品的 MCP 装配入口：把工具注册到共享 server，并转出组合根
// 注入的账号值、SDK 端口与反向搜图配置。
//
// 包的组成：[Register] 完成工具注册，[Account]、[SDKPorts]、[LocalDownloads] 与
// [ReverseSearchPorts] 是 toolhost 的类型别名。工具实现与执行宿主在 tools 和
// internal/toolhost；SDK client 由调用方的 runtime 持有。
package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 携带组合根注入的账号选择与传输覆写。
type Account = toolhost.Account

// SDKPorts 让 MCP 打开独立认证快照，并在账号池重放边界内执行操作。实现由组合根注入；
// MCP 通过这组函数取用依赖。
type SDKPorts = toolhost.SDKPorts

// LocalDownloads 配置 stdio 下通过共享 downloader 交付文件的路径与模板。
type LocalDownloads = toolhost.LocalDownloads

// ReverseSearchPorts 携带启动时为 reverse_search 注入的能力与配置。
type ReverseSearchPorts = toolhost.ReverseSearchPorts

// Register 把 Pixiv 工具注册到共享 server；账号和 SDK 的生命周期仍由调用方的 Pixiv
// runtime 持有。
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	tools.Register(toolhost.NewApp(ports, account), server)
}
