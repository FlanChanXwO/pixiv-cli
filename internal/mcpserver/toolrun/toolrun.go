// Package toolrun owns shared MCP invocation diagnostics and list contracts.
package toolrun

import (
	"cmp"
	"context"
	"log/slog"
	"strconv"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scope assigns call IDs within one product runtime.
type Scope struct {
	logger  *slog.Logger
	counter atomic.Uint64
}

type invocationKey struct{}

// NewScope retains the execution logger; nil keeps tools silent.
func NewScope(logger *slog.Logger) *Scope {
	return &Scope{logger: cmp.Or(logger, slog.New(slog.DiscardHandler))}
}

// AddTool supplies call context and credential-safe diagnostics.
// SDK argument rejections keep their result layout with fixed text; handler
// results and errors pass unchanged to the MCP SDK.
func AddTool[In, Out any](scope *Scope, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	ProtectInput(server, tool.Name, func() *mcp.CallToolResult {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid_request"}}}
	})
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		*ctx.Value(invocationKey{}).(*bool) = true
		ctx = logging.WithCall(ctx, scope.logger, tool.Name, strconv.FormatUint(scope.counter.Add(1), 10))
		result, output, err := handler(ctx, request, input)
		if err != nil || (result != nil && result.IsError) {
			scope.logger.ErrorContext(ctx, "Tool failed", slog.String(logging.AttrReason, "tool failed"))
		}
		return result, output, err
	})
}

// ProtectInput replaces SDK validation diagnostics before a typed handler runs.
func ProtectInput(server *mcp.Server, name string, failure func() *mcp.CallToolResult) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params.Name != name {
				return next(ctx, method, request)
			}
			// Product-specific guards share the marker, regardless of middleware order.
			invoked, ok := ctx.Value(invocationKey{}).(*bool)
			if !ok {
				invoked = new(bool)
				ctx = context.WithValue(ctx, invocationKey{}, invoked)
			}
			result, err := next(ctx, method, request)
			failed, ok := result.(*mcp.CallToolResult)
			if *invoked || !ok || !failed.IsError || failed.StructuredContent != nil {
				return result, err
			}
			return failure(), nil
		}
	})
}
