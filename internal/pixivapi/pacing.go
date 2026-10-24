package pixivapi

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// PacingTransport 把最小请求间隔放到传输层，让 App API、OAuth 与资源请求共享同一节奏。
type PacingTransport struct {
	Inner    http.RoundTripper
	Interval time.Duration
	Logger   *slog.Logger
	mu       sync.Mutex
	last     time.Time
}

func (p *PacingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := p.Interval - time.Since(p.last)
	if wait > 0 {
		if p.Logger != nil {
			p.Logger.LogAttrs(req.Context(), slog.LevelDebug, "Request pacing waited",
				logging.OperationAttr(req.Context()), slog.Duration(logging.AttrWait, wait))
		}
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
	return p.Inner.RoundTrip(req)
}

func (p *PacingTransport) CloseIdleConnections() {
	if closer, ok := p.Inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
