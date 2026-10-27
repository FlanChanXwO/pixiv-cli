// Package mcpserver 把 Pixiv 与 FANBOX 的工具注册到同一个 MCP server，并提供 stdio、
// Streamable HTTP、OAuth 入站保护和服务生命周期。
//
// 包的组成：[New] 注册两个产品的工具，[RunStdio] 与 [RunHTTP] 分别启动两种传输，
// [NewHTTPHandler] 只组装可挂到既有 listener 的 handler。两个产品共享协议 server，
// SDK、账号、OAuth 授权状态和请求生命周期保持独立。
package mcpserver

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New 返回已经注册两个产品工具的 server；调用方仍持有各自的 SDK 端口和账号值。
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

// bindLifecycle 让服务退出时取消在途请求；每次调用自行释放打开的资源。
func bindLifecycle(ctx context.Context, server *mcp.Server) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(requestCtx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			requestCtx, cancel := context.WithCancel(requestCtx)
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			defer cancel()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			result, err := next(requestCtx, method, request)
			if err == nil {
				result = observeMediaResult(ctx, request, result)
			}
			return result, err
		}
	})
}
