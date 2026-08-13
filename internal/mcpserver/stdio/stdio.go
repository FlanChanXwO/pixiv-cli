// Package stdio 拥有 MCP stdio transport 与 runtime lifecycle。
//
// stdout 只用于 JSON-RPC；MCP 命令不经过通用 input resolver，也不写非协议
// 输出。stdio 包只负责把构造好的 MCP server 绑定到标准输入输出，reverse
// close 语义由调用方在命令退出路径统一处理。
package stdio

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run 在一个 stdio transport 上运行 server 直到 ctx 取消或连接结束。失败
// 保留调用方错误的 cause 与稳定 reason，不向 stdout 写非 JSON-RPC 内容。
func Run(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}
