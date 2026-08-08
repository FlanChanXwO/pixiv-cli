// Package mcpserver 是 MCP 产品 server 的最薄兼容入口。
//
// Pixiv 与 FANBOX 的 tool 注册和协议适配分别位于子包；这里仅保留旧调用方
// 所需的 Pixiv 构造函数转发，避免 bootstrap 重新复制 MCP wiring。
package mcpserver

import (
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	pixivserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownloadManager = pixivserver.DownloadManager

// New 保留旧的嵌入调用签名；兼容参数不会进入 Pixiv 内容或认证调用链。
func New(legacy any, downloads DownloadManager) *mcp.Server {
	return pixivserver.New(legacy, downloads)
}

// NewWithSDK 通过 public SDK service 构造 Pixiv MCP server。
func NewWithSDK(legacy any, downloads DownloadManager, service pixivapp.SDKService, request pixivapp.SDKClientRequest) *mcp.Server {
	return pixivserver.NewWithSDK(legacy, downloads, service, request)
}

// NewWithSDKDownloadFactory 构造带 operation snapshot 下载器的 Pixiv MCP server。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(pixivapp.ClientSet) DownloadManager, service pixivapp.SDKService, request pixivapp.SDKClientRequest) *mcp.Server {
	return pixivserver.NewWithSDKDownloadFactory(downloads, newDownloads, service, request)
}
