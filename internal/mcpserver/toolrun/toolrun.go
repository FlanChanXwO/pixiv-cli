// Package toolrun 提供两个产品共用的 MCP 调用支持：一次调用内的诊断 scope、工具注册
// 包装，以及输入校验失败的统一结果。
//
// 包的组成：[Scope] 与 [NewScope] 分配调用 ID 并持有日志入口，[AddTool] 注册工具并
// 记录失败，[ProtectInput] 把 SDK 参数的原始诊断换成工具声明的安全结果。逻辑分页的
// 输入解析与输出字段在 paging.go。
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

// Scope 在一个产品运行时内分配递增的调用 ID，并保存执行日志入口。
type Scope struct {
	logger  *slog.Logger
	counter atomic.Uint64
}

type invocationKey struct{}

// NewScope 保留执行日志；logger 为 nil 时工具保持静默。
func NewScope(logger *slog.Logger) *Scope {
	return &Scope{logger: cmp.Or(logger, slog.New(slog.DiscardHandler))}
}

// AddTool 注册工具，并为每次调用附上调用上下文与不含凭据的诊断。SDK 参数拒绝时保留
// 固定文案的结果布局；handler 的结果与错误原样交给 MCP SDK。
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

// ProtectInput 在 typed handler 执行前拦截 SDK 的参数校验诊断，换成 failure 给出的安全
// 结果；handler 已经执行时不动其结果。
func ProtectInput(server *mcp.Server, name string, failure func() *mcp.CallToolResult) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params.Name != name {
				return next(ctx, method, request)
			}
			// 各产品自己的 guard 共用这个标记；中间件安装顺序不影响结论。
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
