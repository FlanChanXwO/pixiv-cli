package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account/loginrelay"
)

// WaitForHandoffRelayLoginCode 在独立 server 上运行一次性 remote Pixiv login
// relay 会话，并把会话页 URL 写入 errOut。errOut 只接收会话 URL，绝不含 OAuth
// 凭证；loginURL 不写入任何输出。该函数是 accountLogin 的 relay 分支与聚焦
// handoff 测试共用的入口。
func WaitForHandoffRelayLoginCode(ctx context.Context, errOut io.Writer, opts RelayServerOptions, acceptsCallback CallbackURLAccepter, loginURL string) (string, func(bool), func(), error) {
	return controller{errOut: errOut}.waitForHandoffRelayLoginCode(ctx, opts, acceptsCallback, loginURL)
}

// waitForHandoffRelayLoginCode 在 server 上运行一次性会话。会话 proof 只存在
// session page 的操作入口与 desktop 的短暂私有状态中，终端不输出 Pixiv OAuth URL。
func (a controller) waitForHandoffRelayLoginCode(ctx context.Context, opts RelayServerOptions, acceptsCallback CallbackURLAccepter, loginURL string) (string, func(bool), func(), error) {
	publicURL, err := loginrelay.CanonicalPublicURL(opts.PublicURL)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	listen := opts.Listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", opts.ListenAddr)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	session, err := loginrelay.New(ctx, publicURL, loginURL, acceptsCallback)
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	if opts.ContextWaiterExited != nil {
		go func() { <-session.Done; opts.ContextWaiterExited() }()
	}
	server := &http.Server{Handler: session.Handler}
	serveErr := make(chan error, 1)
	go func() {
		var serveErrValue error
		if opts.TLSCertFile != "" {
			serveErrValue = server.ServeTLS(listener, opts.TLSCertFile, opts.TLSKeyFile)
		} else {
			serveErrValue = server.Serve(listener)
		}
		if errors.Is(serveErrValue, http.ErrServerClosed) {
			serveErrValue = nil
		}
		serveErr <- serveErrValue
	}()
	var cleanupOnce sync.Once
	closeServer := func(graceful bool) {
		cleanupOnce.Do(func() {
			session.Stop()
			if graceful {
				_ = server.Shutdown(context.Background())
				return
			}
			// 父 context 取消表示本次登录会话已经被明确放弃。这里不能继续使用
			// Shutdown 等待 active handler 变 idle，否则 race runner 上仍在收尾的
			// HTTP handler 会反过来阻塞取消路径；Close 会立即终止这些连接。
			_ = server.Close()
		})
	}
	cleanup := func() { closeServer(true) }
	abort := func() { closeServer(false) }
	fmt.Fprintf(a.errOut, "Remote Pixiv login relay is listening on %s.\n", listener.Addr().String())
	fmt.Fprintf(a.errOut, "Open remote Pixiv login session:\n%s\n", session.URL)

	select {
	case callback := <-session.Callback:
		// callback 已由本次会话接收，后续由 notifyFinal 负责结果页；不再需要一个
		// 仅等待父 context 的 goroutine，否则成功登录会一直保留至进程结束。
		session.Stop()
		return callback, session.Complete, cleanup, nil
	case err := <-serveErr:
		cleanup()
		if err != nil {
			// ServeTLS 的底层错误会包含 PEM 的绝对路径，不能越过 CLI 错误边界。
			return "", func(bool) {}, cleanup, errors.New("remote login relay server failed; verify its listener and TLS configuration")
		}
		return "", func(bool) {}, cleanup, errors.New("remote login relay stopped before sign-in completed")
	case <-ctx.Done():
		abort()
		return "", func(bool) {}, cleanup, ctx.Err()
	}
}
