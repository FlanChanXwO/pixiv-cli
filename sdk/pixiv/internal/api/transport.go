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
	cooldown time.Time
}

// RoundTrip waits for the shared interval or request cancellation before sending.
func (p *pacingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		p.mu.Lock()
		next := p.last.Add(p.interval)
		if p.cooldown.After(next) {
			next = p.cooldown
		}
		wait := time.Until(next)
		if wait <= 0 {
			p.last = time.Now()
			p.mu.Unlock()
			break
		}
		p.mu.Unlock()
		slog.LogAttrs(req.Context(), slog.LevelDebug, "Request pacing waited",
			slog.String(AttrSDKOperation, ""), slog.Duration(AttrWait, wait))
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	resp, err := p.inner.RoundTrip(req)
	if err == nil && resp.StatusCode == http.StatusTooManyRequests {
		if wait, ok := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			p.mu.Lock()
			until := time.Now().Add(wait)
			if until.After(p.cooldown) {
				p.cooldown = until
			}
			p.mu.Unlock()
		}
	}
	return resp, err
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
