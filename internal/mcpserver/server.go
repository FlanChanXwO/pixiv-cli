package mcpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownloadManager interface {
	SetDownloadPath(string) error
	Download(context.Context, []int64) ([]download.DownloadedArtwork, error)
}

type App struct {
	downloads DownloadManager
	// newDownloads 在每个 SDK operation 的稳定认证 snapshot 上创建下载器。
	// 固定 downloads 仅保留给未注入 SDK 的嵌入测试兼容路径。
	newDownloads func(application.SDKClient) DownloadManager
	logger       *slog.Logger
	sdk          application.SDKService
	sdkRequest   application.SDKClientRequest
	// sdkGate 串行化同一 MCP 实例中会刷新 OAuth 的 SDK operation，避免两个
	// OpenDefault 快照同时消费同一个 rotation token；channel select 可响应 ctx。
	sdkGate chan struct{}
}

// toolErrorCapture 将 handler 已转换为 MCP error result 的原始安全错误交给注册层。
// go-sdk 对 result 与 error 同时存在时会重包 result，因此不能直接返回 handler error。
type toolErrorCapture struct{ err error }

type toolErrorCaptureKey struct{}

func recordToolError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	capture, _ := ctx.Value(toolErrorCaptureKey{}).(*toolErrorCapture)
	if capture != nil && capture.err == nil {
		capture.err = err
	}
}

// New 保留构造参数位置以便嵌入方平滑升级；第一个参数不再被读取，所有 Pixiv
// 能力必须由 public SDK service 提供。
func New(_ any, downloads DownloadManager, logger *slog.Logger) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, logger: logger})
}

// NewWithSDK 通过公共 SDK 为每个 MCP tool 建立独立 operation snapshot。
// 首个参数仅是已废弃的兼容占位，绝不构成内容、认证或资源调用链。
func NewWithSDK(_ any, downloads DownloadManager, logger *slog.Logger, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, logger: logger, sdk: service, sdkRequest: request})
}

// NewWithSDKDownloadFactory 为生产 MCP 注入 snapshot-scoped 下载器构造器。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(application.SDKClient) DownloadManager, logger *slog.Logger, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, newDownloads: newDownloads, logger: logger, sdk: service, sdkRequest: request})
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newServer(app *App) *mcp.Server {
	if app.sdkGate == nil {
		app.sdkGate = make(chan struct{}, 1)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "2.0.0"}, &mcp.ServerOptions{
		Instructions: "Pixiv MCP server for searching, browsing, and downloading Pixiv content.",
	})
	app.register(server)
	return server
}

// addTool 在注册层统一观测所有 MCP tool（包括旧兼容 tool）。wrapper 不读取
// request arguments，避免 token、cookie、URL 或用户查询进入日志；日志仍只经注入的
// stderr logger 输出，绝不触碰 JSON-RPC transport。
func addTool[In, Out any](a *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		started := time.Now()
		capture := &toolErrorCapture{}
		result, output, err = handler(context.WithValue(ctx, toolErrorCaptureKey{}, capture), request, input)
		logErr := err
		resultError := result != nil && result.IsError
		if err == nil && capture.err != nil {
			// 仅供日志标记失败。把它作为 handler 返回 error 会让 go-sdk 重新包装
			// CallToolResult，丢失调用方已有的 Content/StructuredContent。
			logErr = capture.err
		}
		a.operationLog(tool.Name, started, resultError || err != nil || capture.err != nil, logErr, 0, 0)
		return result, output, err
	})
}

type textOut struct {
	Text string `json:"text"`
}

func toolText(text string) (*mcp.CallToolResult, textOut, error) {
	return nil, textOut{Text: text}, nil
}

func toolTextError(ctx context.Context, err error, text string) (*mcp.CallToolResult, textOut, error) {
	recordToolError(ctx, err)
	return toolText(text)
}

type emptyIn struct{}

var errLegacyValidation = errors.New("legacy tool validation failed")
