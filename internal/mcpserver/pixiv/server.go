package pixiv

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownloadManager interface {
	SetDownloadPath(string) error
	Download(context.Context, downloadapp.DownloadRequest) ([]downloadapp.DownloadedArtwork, error)
}

type App struct {
	downloads DownloadManager
	// newDownloads 在每个 SDK operation 的稳定认证 snapshot 上创建下载器。
	// 固定 downloads 仅保留给未注入 SDK 的嵌入测试兼容路径。
	newDownloads func(pixivapp.ClientSet) DownloadManager
	sdk          pixivapp.SDKService
	sdkRequest   pixivapp.SDKClientRequest
	// sdkGate 串行化同一 MCP 实例中会刷新 OAuth 的 SDK operation，避免两个
	// OpenDefault 快照同时消费同一个 rotation token；channel select 可响应 ctx。
	sdkGate        chan struct{}
	requestCounter atomic.Uint64
}

// New 保留构造参数位置以便嵌入方平滑升级；第一个参数不再被读取，所有 Pixiv
// 能力必须由 public SDK service 提供。
func New(_ any, downloads DownloadManager) *mcp.Server {
	return newServer(&App{downloads: downloads})
}

// NewWithSDK 通过公共 SDK 为每个 MCP tool 建立独立 operation snapshot。
// 首个参数仅是已废弃的兼容占位，绝不构成内容、认证或资源调用链。
func NewWithSDK(_ any, downloads DownloadManager, service pixivapp.SDKService, request pixivapp.SDKClientRequest) *mcp.Server {
	return newServer(&App{downloads: downloads, sdk: service, sdkRequest: request})
}

// NewWithSDKDownloadFactory 为生产 MCP 注入 snapshot-scoped 下载器构造器。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(pixivapp.ClientSet) DownloadManager, service pixivapp.SDKService, request pixivapp.SDKClientRequest) *mcp.Server {
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
// wrapper 只增加 stderr diagnostics scope，不改变 handler 的输入、structured
// result、isError 或错误返回。
func addTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		requestID := app.requestCounter.Add(1)
		scoped := diagnostics.WithChildScope(ctx, diagnostics.ModulePixivMCPServer, requestID)
		startedAt := time.Now()
		diagnostics.Emit(scoped, diagnostics.Event{Kind: diagnostics.EventStarted, Operation: "tool " + tool.Name})
		result, output, err := handler(scoped, request, input)
		if err != nil || (result != nil && result.IsError) {
			diagnostics.Emit(scoped, diagnostics.Event{
				Kind:      diagnostics.EventFailed,
				Operation: "tool " + tool.Name,
				Reason:    diagnostics.ReasonToolFailed,
				Duration:  time.Since(startedAt),
			})
		} else {
			diagnostics.Emit(scoped, diagnostics.Event{
				Kind:      diagnostics.EventCompleted,
				Operation: "tool " + tool.Name,
				Duration:  time.Since(startedAt),
			})
		}
		return result, output, err
	})
}

type emptyIn struct{}

var errInputValidation = errors.New("tool input validation failed")
