package mcpserver

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownloadManager interface {
	SetDownloadPath(string) error
	Download(context.Context, application.DownloadRequest) ([]download.DownloadedArtwork, error)
}

type App struct {
	downloads DownloadManager
	// newDownloads 在每个 SDK operation 的稳定认证 snapshot 上创建下载器。
	// 固定 downloads 仅保留给未注入 SDK 的嵌入测试兼容路径。
	newDownloads func(application.SDKClient) DownloadManager
	sdk          application.SDKService
	sdkRequest   application.SDKClientRequest
	// sdkGate 串行化同一 MCP 实例中会刷新 OAuth 的 SDK operation，避免两个
	// OpenDefault 快照同时消费同一个 rotation token；channel select 可响应 ctx。
	sdkGate chan struct{}
}

// New 保留构造参数位置以便嵌入方平滑升级；第一个参数不再被读取，所有 Pixiv
// 能力必须由 public SDK service 提供。
func New(_ any, downloads DownloadManager) *mcp.Server {
	return newServer(&App{downloads: downloads})
}

// NewWithSDK 通过公共 SDK 为每个 MCP tool 建立独立 operation snapshot。
// 首个参数仅是已废弃的兼容占位，绝不构成内容、认证或资源调用链。
func NewWithSDK(_ any, downloads DownloadManager, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	return newServer(&App{downloads: downloads, sdk: service, sdkRequest: request})
}

// NewWithSDKDownloadFactory 为生产 MCP 注入 snapshot-scoped 下载器构造器。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(application.SDKClient) DownloadManager, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	return newServer(&App{downloads: downloads, newDownloads: newDownloads, sdk: service, sdkRequest: request})
}

func newServer(app *App) *mcp.Server {
	if app.sdkGate == nil {
		app.sdkGate = make(chan struct{}, 1)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "3.0.0"}, &mcp.ServerOptions{
		Instructions: "Pixiv MCP server for searching, browsing, and downloading Pixiv content.",
	})
	app.register(server)
	return server
}

// addTool 统一保留注册入口；失败结果直接由各 handler 的 CallToolResult 表达。
func addTool[In, Out any](_ *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, handler)
}

type emptyIn struct{}

var errLegacyValidation = errors.New("legacy tool validation failed")
