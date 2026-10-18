package mcpserver

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New 构造统一协议 server；两个产品保留独立 SDK、账号和请求生命周期。
func New(pixivPorts pixiv.SDKPorts, account pixiv.Account, fanboxPorts fanbox.SDKPorts, fanboxProxy *string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "3.0.0"}, &mcp.ServerOptions{
		Instructions: "Browse Pixiv and FANBOX through one MCP server. Product accounts remain independent.",
	})
	pixiv.Register(server, pixivPorts, account)
	fanbox.Register(server, fanboxPorts, fanboxProxy)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, request)
			if err == nil {
				result = observeMediaResult(ctx, request, result)
			}
			return result, err
		}
	})
	return server
}
