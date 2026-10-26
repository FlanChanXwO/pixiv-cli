// HTTP construction, shared request pacing, and owned connection release.

package pixivapi

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

// NewHTTPClient preserves an injected client's lifetime and settings. The bool
// reports ownership; only a newly created client may be closed by the SDK.
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

// CloseIdleConnections releases only a client created for this runtime.
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.http.CloseIdleConnections()
	}
}
