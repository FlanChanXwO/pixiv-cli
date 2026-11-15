// Package api owns Pixiv App transport, media URL policy, and sanitized failures.
//
// [Client] sends requests using [Config]; sdk/pixiv owns endpoint response mapping
// and credential refresh. The oauth subpackage exchanges tokens. Routes and
// headers live in routes.go and profile.go; [RequiredList] and [RequiredObject]
// track required response fields.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/netdiagnostic"
)

// Client sends requests with fixed credentials over an SDK-managed HTTP client.
type Client struct {
	http                 *http.Client
	accessToken          string
	acceptLanguage       string
	userID               int64
	allowedResourceHosts []string
}

// Config configures App transport; zero values use an unauthenticated HTTP client.
type Config struct {
	HTTPClient     *http.Client
	ResourceHosts  []string
	AccessToken    string
	AcceptLanguage string
	UserID         int64
}

// New constructs transport without I/O or taking ownership of the HTTP client.
func New(config Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		http:                 httpClient,
		accessToken:          strings.TrimSpace(config.AccessToken),
		allowedResourceHosts: config.ResourceHosts,
		acceptLanguage:       strings.TrimSpace(config.AcceptLanguage),
		userID:               config.UserID,
	}
}

// request holds one App request; do handles rate limits and send issues one attempt.
type request struct {
	method string
	url    *url.URL
	form   url.Values
	// retryRateLimit applies only to reads because uncertain mutations must not replay.
	retryRateLimit bool
}

func newRequest(method, rawURL string, query url.Values) (request, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return request{}, err
	}
	if len(query) > 0 {
		// Preserve any query embedded in the URL before appending endpoint parameters.
		if target.RawQuery == "" {
			target.RawQuery = query.Encode()
		} else {
			target.RawQuery += "&" + query.Encode()
		}
	}
	return request{method: method, url: target, retryRateLimit: method == http.MethodGet}, nil
}

func (c *Client) appRequest(method, path string, query, form url.Values) (request, error) {
	req, err := newRequest(method, AppAPIBase+path, query)
	if err != nil {
		return request{}, err
	}
	req.form = form
	return req, nil
}

// GetJSON sends an authenticated read and decodes JSON with one Retry-After replay.
// The endpoint caller owns its route, wire response, and semantic validation.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	req, err := c.appRequest(http.MethodGet, path, query, nil)
	if err != nil {
		return Transport(err)
	}
	return c.doJSON(ctx, req, out)
}

// PostForm sends a mutation once; successful responses may omit a body.
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) error {
	req, err := c.appRequest(http.MethodPost, path, nil, form)
	if err != nil {
		return Transport(err)
	}
	return c.doForm(ctx, req)
}

// PostFormJSON sends a mutation once and requires a decodable JSON response.
func (c *Client) PostFormJSON(ctx context.Context, path string, form url.Values, out any) error {
	req, err := c.appRequest(http.MethodPost, path, nil, form)
	if err != nil {
		return Transport(err)
	}
	return c.doJSON(ctx, req, out)
}

// do retries a read once on a valid Retry-After and closes the rejected response.
// The caller owns the returned body; credential refresh belongs to sdk/pixiv.
func (c *Client) do(ctx context.Context, req request) (*http.Response, error) {
	rateRetried := false
	for attempt := 1; ; attempt++ {
		resp, err := c.send(ctx, req, attempt)
		if err != nil {
			return nil, Transport(err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && req.retryRateLimit && !rateRetried {
			if wait, ok := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
				rateRetried = true
				slog.LogAttrs(ctx, slog.LevelWarn, "Rate limited; retrying after wait",
					slog.String(AttrSDKOperation, ""), slog.Duration(AttrWait, wait))
				discardBody(resp)
				if err := waitForRetryAfter(ctx, wait); err != nil {
					return nil, Transport(err)
				}
				continue
			}
		}
		return resp, nil
	}
}

// send logs only safe request metadata and typed transport diagnostics.
func (c *Client) send(ctx context.Context, req request, attempt int) (*http.Response, error) {
	var body io.Reader
	if len(req.form) > 0 {
		body = strings.NewReader(req.form.Encode())
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.method, req.url.String(), body)
	if err != nil {
		return nil, err
	}
	for name, value := range c.apiHeaders() {
		httpReq.Header.Set(name, value)
	}
	if len(req.form) > 0 {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	start := time.Now()
	resp, err := c.http.Do(httpReq)
	attrs := []slog.Attr{
		slog.String(AttrMethod, req.method),
		slog.String(AttrHost, req.url.Host),
		slog.String(AttrPath, req.url.Path),
		slog.Duration(AttrDuration, time.Since(start)),
		slog.Int(AttrAttempt, attempt),
		slog.String(AttrSDKOperation, ""),
	}
	if err != nil {
		attrs = append(attrs, netdiagnostic.Attrs(err)...)
		slog.LogAttrs(ctx, slog.LevelDebug, "Request failed", append(attrs, slog.String(AttrError, Transport(err).Error()))...)
		return nil, err
	}
	attrs = append(attrs, slog.Int(AttrStatus, resp.StatusCode))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		slog.LogAttrs(ctx, slog.LevelWarn, "Request rejected by upstream", attrs...)
	} else {
		slog.LogAttrs(ctx, slog.LevelDebug, "Request completed", attrs...)
	}
	return resp, nil
}

func discardBody(resp *http.Response) {
	// Drain before closing so a retry can reuse the connection.
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// readBody prioritizes read errors and preserves Retry-After on HTTP failures.
func readBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter, present := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return nil, HTTPStatusWithRetryAfter(resp.StatusCode, retryAfter, present)
	}
	return body, nil
}

func (c *Client) doJSON(ctx context.Context, req request, out any) error {
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return MalformedResponse()
	}
	return nil
}

// doForm accepts every 2xx because mutation endpoints may omit JSON.
func (c *Client) doForm(ctx context.Context, req request) error {
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	_, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return Transport(readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HTTPStatus(resp.StatusCode)
	}
	return nil
}

func waitForRetryAfter(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) apiHeaders() map[string]string {
	headers := AppHeaders(c.accessToken)
	if c.acceptLanguage != "" {
		headers["Accept-Language"] = c.acceptLanguage
	}
	if c.userID > 0 {
		headers["X-User-Id"] = strconv.FormatInt(c.userID, 10)
	}
	return headers
}

// ParseRetryAfter accepts seconds or HTTP dates; the boolean distinguishes a
// valid zero wait from a missing or invalid header.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Reject seconds that would overflow Duration and disguise rate limiting as a negative wait.
		const maxDurationSeconds = int64(1<<63-1) / int64(time.Second)
		if seconds < 0 || seconds > maxDurationSeconds {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	duration := when.Sub(now)
	if duration < 0 {
		duration = 0
	}
	return duration, true
}
