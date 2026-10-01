// Package mcpserver 拥有统一 MCP Streamable HTTP transport 与 runtime lifecycle。
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHTTPHandler 将专用 server 绑定到 ctx 的服务生命周期并组装 HTTP/OAuth 路由。
// owner 必须已初始化；同一 server 不应再次绑定到另一生命周期。
func NewHTTPHandler(ctx context.Context, server *mcp.Server, baseURL string, store auth.Store) (http.Handler, error) {
	authorization, err := auth.NewHandler(baseURL, store)
	if err != nil {
		return nil, err
	}
	if _, err := store.Read(ctx); err != nil {
		return nil, err
	}
	base, _ := url.Parse(strings.TrimRight(baseURL, "/"))
	protection := http.NewCrossOriginProtection()
	if err := protection.AddTrustedOrigin(base.Scheme + "://" + base.Host); err != nil {
		return nil, err
	}
	// 旧 revision 的 SDK session 会脱离 HTTP context；服务退出仍必须取消在途工具。
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(requestCtx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			requestCtx, cancel := context.WithCancel(requestCtx)
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			defer cancel()
			return next(requestCtx, method, request)
		}
	})
	protocol := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// 新 revision 的 POST 就是请求生命周期；断线后不再继续无法交付的工具工作。
		PropagateRequestCancellation: true,
		// RequireCanonical 严格校验 Host 后才进入 SDK；反向代理保留公网 Host，不能被 localhost 自动检查误拒。
		DisableLocalhostProtection: true,
		CrossOriginProtection:      protection,
	})
	protected := authorization.RequireBearer(protocol)
	return authorization.RequireCanonical(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ServeMux 会清理双斜线和转义路径，破坏已发布的 canonical resource。
		if r.URL.EscapedPath() == base.EscapedPath()+"/mcp" {
			protected.ServeHTTP(w, r)
			return
		}
		authorization.ServeHTTP(w, r)
	})), nil
}

// RunHTTP 监听显式地址；取消或 listener 失败时取消并等待在途 handler 退出，不切换端口。
func RunHTTP(ctx context.Context, server *mcp.Server, listenAddr, baseURL string, store auth.Store, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	handler, err := NewHTTPHandler(ctx, server, baseURL, store)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	if _, err := fmt.Fprintln(out, "MCP owner initialized; endpoint:", strings.TrimRight(baseURL, "/")+"/mcp"); err != nil {
		return err
	}
	httpServer := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	select {
	case err := <-served:
		// listener 故障不取消调用方 context，但必须结束本服务及旧版 SDK 的在途工作。
		cancel()
		return errors.Join(err, httpServer.Shutdown(context.Background()))
	case <-ctx.Done():
		// BaseContext 已取消业务请求；不用固定超时截断合法的清理。
		shutdownErr := httpServer.Shutdown(context.Background())
		serveErr := <-served
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(ctx.Err(), shutdownErr, serveErr)
	}
}
