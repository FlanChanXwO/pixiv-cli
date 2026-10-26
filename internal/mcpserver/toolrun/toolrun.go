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

// NewScope retains the execution logger; nil keeps tools silent.
func NewScope(logger *slog.Logger) *Scope {
	return &Scope{logger: cmp.Or(logger, slog.New(slog.DiscardHandler))}
}

// AddTool supplies call context and fixed failure diagnostics.
// Handler results and errors pass unchanged to the MCP SDK.
func AddTool[In, Out any](scope *Scope, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
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
			result, err := next(ctx, method, request)
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params.Name != name {
				return result, err
			}
			failed, ok := result.(*mcp.CallToolResult)
			if !ok || !failed.IsError || failed.StructuredContent != nil {
				return result, err
			}
			return failure(), nil
		}
	})
}
