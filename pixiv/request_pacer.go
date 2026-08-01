package pixiv

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// requestPacer 为同一个 Client operation 串行化请求起始时间。它不设总超时；
// 请求等待仅受调用者 context 控制，零间隔完全绕过节流。
type requestPacer struct {
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

func newRequestPacer(interval time.Duration) *requestPacer {
	if interval <= 0 {
		return nil
	}
	return &requestPacer{interval: interval}
}

func (p *requestPacer) wait(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	now := time.Now()
	start := p.next
	if start.Before(now) {
		start = now
	}
	p.next = start.Add(p.interval)
	p.mu.Unlock()
	delay := time.Until(start)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type pacedRoundTripper struct {
	next  http.RoundTripper
	pacer *requestPacer
}

func (r pacedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := r.pacer.wait(request.Context()); err != nil {
		return nil, err
	}
	return r.next.RoundTrip(request)
}

func withRequestPacer(client *http.Client, pacer *requestPacer) *http.Client {
	if pacer == nil {
		return client
	}
	copy := *client
	next := copy.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	copy.Transport = pacedRoundTripper{next: next, pacer: pacer}
	return &copy
}
