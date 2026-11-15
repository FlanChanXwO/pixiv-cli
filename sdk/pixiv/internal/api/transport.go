package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// pacingTransport shares one request interval across App, OAuth, and media requests.
// The mutex orders request starts; cancellation releases a waiting caller.
type pacingTransport struct {
	inner    http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

// RoundTrip waits for the shared interval or request cancellation before sending.
func (p *pacingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := p.interval - time.Since(p.last)
	if wait > 0 {
		slog.LogAttrs(req.Context(), slog.LevelDebug, "Request pacing waited",
			slog.String(AttrSDKOperation, ""), slog.Duration(AttrWait, wait))
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

// CloseIdleConnections releases idle sockets through the wrapped transport.
func (p *pacingTransport) CloseIdleConnections() {
	if closer, ok := p.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// NewHTTPClient preserves injected settings and ownership. The second result is
// true only for a newly created client whose idle sockets the SDK may close.
func NewHTTPClient(injected *http.Client, interval time.Duration) (*http.Client, bool) {
	if injected == nil {
		var transport http.RoundTripper = http.DefaultTransport.(*http.Transport).Clone()
		if interval > 0 {
			transport = &pacingTransport{inner: transport, interval: interval}
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
	return &http.Client{Transport: &pacingTransport{inner: transport, interval: interval}, Jar: injected.Jar, Timeout: injected.Timeout, CheckRedirect: injected.CheckRedirect}, false
}
