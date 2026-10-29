package api

import (
	"cmp"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// pacingTransport 把最小请求间隔放到传输层，让 App API、OAuth 与资源请求共享同一节奏。
type pacingTransport struct {
	inner    http.RoundTripper
	interval time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	last     time.Time
}

func (p *pacingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := p.interval - time.Since(p.last)
	if wait > 0 {
		p.log.LogAttrs(req.Context(), slog.LevelDebug, "Request pacing waited",
			logging.OperationAttr(req.Context()), slog.Duration(logging.AttrWait, wait))
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	p.last = time.Now()
	p.mu.Unlock()
	return p.inner.RoundTrip(req)
}

func (p *pacingTransport) CloseIdleConnections() {
	if closer, ok := p.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// NewHTTPClient 保留注入 client 的生命周期与设置。第二个返回值报告所有权：只有新建的
// client 可以由 SDK 关闭。
func NewHTTPClient(injected *http.Client, interval time.Duration, logger *slog.Logger) (*http.Client, bool) {
	logger = cmp.Or(logger, slog.New(slog.DiscardHandler))
	if injected == nil {
		var transport http.RoundTripper = http.DefaultTransport.(*http.Transport).Clone()
		if interval > 0 {
			transport = &pacingTransport{inner: transport, interval: interval, log: logger}
		}
		return &http.Client{Transport: transport}, true
	}
	if interval <= 0 {
		return injected, false
	}
	transport := injected.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{Transport: &pacingTransport{inner: transport, interval: interval, log: logger}, Jar: injected.Jar, Timeout: injected.Timeout, CheckRedirect: injected.CheckRedirect}, false
}

// CloseIdleConnections 只释放本运行时创建的 client 连接。
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.http.CloseIdleConnections()
	}
}
