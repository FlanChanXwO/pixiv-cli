// Package fanbox 实现独立的 FANBOX MCP stdio server。它只注册只读 tool；stdout
// 保留给 JSON-RPC；运行期失败保留 structured result 并设置 isError=true。每个
// tool 调用都通过 service.OpenClient 建立独立 client，不共享客户端状态。
package fanbox

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// App 是 FANBOX MCP 应用。
type App struct {
	service        *fanboxapp.Service
	proxyOverride  *string
	requestCounter atomic.Uint64
}

// New 构造 FANBOX MCP server。
func New(service *fanboxapp.Service) *mcp.Server {
	return NewWithProxy(service, nil)
}

// NewWithProxy constructs the server with a native FANBOX proxy override. It
// does not alter FlareSolverr service or upstream proxy configuration.
func NewWithProxy(service *fanboxapp.Service, proxyOverride *string) *mcp.Server {
	app := &App{service: service, proxyOverride: proxyOverride}
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli-fanbox", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "FANBOX MCP server for browsing creators, posts, tags, and media resources.",
	})
	app.register(server)
	return server
}

func (a *App) register(server *mcp.Server) {
	addTool(a, server, &mcp.Tool{Name: "fanbox_current_user", Description: "Show the current authenticated FANBOX user."}, a.currentUser)
	addTool(a, server, &mcp.Tool{Name: "fanbox_creator", Description: "Get one FANBOX creator profile."}, a.creator)
	addTool(a, server, &mcp.Tool{Name: "fanbox_creators", Description: "List supporting or following FANBOX creators."}, a.creators)
	addTool(a, server, &mcp.Tool{Name: "fanbox_creator_tags", Description: "List tags used by a FANBOX creator."}, a.creatorTags)
	addTool(a, server, &mcp.Tool{Name: "fanbox_creator_posts", Description: "List posts from a FANBOX creator."}, a.creatorPosts)
	addTool(a, server, &mcp.Tool{Name: "fanbox_tagged_posts", Description: "List posts from a creator for one tag."}, a.taggedPosts)
	addTool(a, server, &mcp.Tool{Name: "fanbox_post", Description: "Get one FANBOX post."}, a.post)
	addTool(a, server, &mcp.Tool{Name: "fanbox_home", Description: "Browse the FANBOX home feed."}, a.home)
	addTool(a, server, &mcp.Tool{Name: "fanbox_supporting", Description: "Browse posts from supporting creators."}, a.supporting)
	addTool(a, server, &mcp.Tool{Name: "fanbox_resolve_url", Description: "Resolve a FANBOX page URL into a typed reference."}, a.resolveURL)
	addTool(a, server, &mcp.Tool{Name: "fanbox_open_resource", Description: "Open a FANBOX media resource by ref and return its headers and status without the bytes."}, a.openResource)
}

func addTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		requestID := app.requestCounter.Add(1)
		scoped := diagnostics.WithChildScope(ctx, diagnostics.ModuleFanboxMCPServer, requestID)
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

func (a *App) openClient(ctx context.Context) (*fanbox.Client, error) {
	if a.service == nil {
		return nil, errors.New("fanbox service is not configured")
	}
	return a.service.OpenClientWithProxy(ctx, a.proxyOverride)
}

// fanboxResult 统一 MCP tool 的文本摘要；完整实体只存在于 structured output。
func fanboxResult[Out any](out Out, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = "OK"
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}
