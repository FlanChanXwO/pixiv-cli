// http.go 组装 Streamable HTTP、OAuth 与媒体路由，并管理 listener 和写入观测。

package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHTTPHandler 将专用 server 绑定到 ctx 的服务生命周期并组装 HTTP/OAuth 路由。
// owner 必须已初始化；同一 server 不应再次绑定到另一生命周期。
func NewHTTPHandler(ctx context.Context, server *mcp.Server, baseURL string, store auth.Store, login *accounts.Login, media *mediaproxy.Service) (http.Handler, error) {
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
	bindLifecycle(ctx, server)
	protocol := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// 新 revision 的 POST 就是请求生命周期；断线后不再继续无法交付的工具工作。
		PropagateRequestCancellation: true,
		// RequireCanonical 严格校验 Host 后才进入 SDK；反向代理保留公网 Host，不能被 localhost 自动检查误拒。
		DisableLocalhostProtection: true,
		CrossOriginProtection:      protection,
	})
	protected := authorization.RequireBearer(protocol)
	relayBase, err := loginrelay.CanonicalPublicURL(strings.TrimRight(baseURL, "/") + "/pixiv-login")
	if err != nil {
		return nil, err
	}
	relayURL, _ := url.Parse(relayBase)
	return authorization.RequireCanonical(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if media != nil && strings.HasPrefix(r.URL.EscapedPath(), media.Path()) {
			media.ServeHTTP(w, r)
			return
		}
		// ServeMux 会清理双斜线和转义路径，破坏已发布的 canonical resource。
		if r.URL.EscapedPath() == base.EscapedPath()+"/mcp" {
			protected.ServeHTTP(observeHTTPWriter(ctx, r.Context(), w, r.Method), r)
			return
		}
		if login != nil && strings.HasPrefix(r.URL.EscapedPath(), relayURL.EscapedPath()+"/") {
			// Preserve escaped canonical prefixes instead of ServeMux path cleaning.
			request := r.Clone(r.Context())
			request.URL.Path = strings.TrimPrefix(r.URL.Path, relayURL.Path)
			request.URL.RawPath = strings.TrimPrefix(r.URL.EscapedPath(), relayURL.EscapedPath())
			login.ServeHTTP(w, request)
			return
		}
		authorization.ServeHTTP(w, r)
	})), nil
}

// RunHTTP 监听显式地址；取消或 listener 失败时取消并等待在途 handler 退出，不切换端口。
func RunHTTP(ctx context.Context, server *mcp.Server, listenAddr, baseURL string, store auth.Store, out io.Writer, login *accounts.Login, media *mediaproxy.Service) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if login != nil {
		defer login.Close()
	}
	handler, err := NewHTTPHandler(ctx, server, baseURL, store, login, media)
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
	httpServer := &http.Server{Handler: handler,
		// Bound slow header admission without timing out legitimate media/tool work.
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx }}
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

// 只记录 HTTP writer 接受的字节，不代表客户端收齐；保留 SDK framing 与取消行为。
func observeHTTPWriter(lifecycleCtx, requestCtx context.Context, writer http.ResponseWriter, method string) http.ResponseWriter {
	if method != http.MethodPost {
		return writer
	}
	return &observedHTTPWriter{ResponseWriter: writer, lifecycleCtx: lifecycleCtx, requestCtx: requestCtx}
}

type observedHTTPWriter struct {
	http.ResponseWriter
	lifecycleCtx, requestCtx context.Context
}

func (w *observedHTTPWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observedHTTPWriter) Write(p []byte) (int, error) {
	started := time.Now()
	n, err := w.ResponseWriter.Write(p)
	w.observe("mcp_http_write", n, started, err != nil || n != len(p))
	return n, err
}
func (w *observedHTTPWriter) FlushError() error {
	started := time.Now()
	err := http.NewResponseController(w.ResponseWriter).Flush()
	w.observe("mcp_http_flush", 0, started, err != nil)
	return err
}
func (w *observedHTTPWriter) observe(operation string, accepted int, started time.Time, failed bool) {
	attrs := []slog.Attr{
		slog.String(logging.AttrStage, operation),
		slog.Int(logging.AttrCount, accepted),
		slog.Duration(logging.AttrDuration, time.Since(started)),
	}
	reason := ""
	switch {
	case w.lifecycleCtx.Err() != nil:
		reason = "service closed"
	case failed:
		reason = "transport interrupted"
	case w.requestCtx.Err() != nil:
		reason = "request cancelled"
	}
	if reason == "" {
		logging.From(w.lifecycleCtx).LogAttrs(w.lifecycleCtx, slog.LevelDebug, "MCP HTTP step completed", attrs...)
		return
	}
	logging.From(w.lifecycleCtx).LogAttrs(w.lifecycleCtx, slog.LevelWarn, "MCP HTTP step failed",
		append(attrs, slog.String(logging.AttrReason, reason))...)
}
