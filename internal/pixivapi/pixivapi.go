// Package pixivapi 提供 Pixiv App API 的传输层、媒体 URL 策略与失败分类。
//
// 本包不承载内容语义：端点路由与响应映射由 sdk/pixiv 持有，OAuth token 交换在
// 子包 oauth。
//
// 包的组成：[Client] 发送 App 请求并缓存已校验的资源 locator，由 [New] 构造；
// [Config] 是构造参数，[Failure] 是脱敏失败，[RequiredList] 与 [RequiredObject]
// 解码必需字段；api.go 与 profile.go 声明路由、参数和请求头。
package pixivapi

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// Client 发送 Pixiv App 请求，持有共享 HTTP 生命周期与已校验的资源 locator 缓存。
//
// resourceMu 保护 resourceURLs；只有本 client 创建的 HTTP 连接会被关闭。
type Client struct {
	http                 *http.Client
	ownsHTTP             bool
	accessToken          string
	acceptLanguage       string
	userID               int64
	log                  *slog.Logger
	allowedResourceHosts []string
	resourceMu           sync.RWMutex
	resourceURLs         map[string]string
}

// Config 提供 App API 客户端的构造参数；零值使用默认 HTTP client 和无认证。
type Config struct {
	HTTPClient *http.Client
	// OwnHTTPClient 表示 HTTPClient 由 NewHTTPClient 创建，关闭时只释放这种 client。
	OwnHTTPClient  bool
	ResourceHosts  []string
	AccessToken    string
	AcceptLanguage string
	UserID         int64
	// Logger 记录请求结果与重试决策；nil 时不输出。
	Logger *slog.Logger
}

// New 按 cfg 构造 App API client；零值使用默认 HTTP client 且不带认证。
func New(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	logger := cmp.Or(cfg.Logger, slog.New(slog.DiscardHandler))
	return &Client{
		http:                 httpClient,
		accessToken:          strings.TrimSpace(cfg.AccessToken),
		ownsHTTP:             cfg.OwnHTTPClient,
		allowedResourceHosts: cfg.ResourceHosts,
		resourceURLs:         make(map[string]string),
		acceptLanguage:       strings.TrimSpace(cfg.AcceptLanguage),
		userID:               cfg.UserID,
		log:                  logger,
	}
}

// request 描述一次 App API 请求；do 负责限流重试，send 只发送一次。
type request struct {
	method string
	url    *url.URL
	form   url.Values
	// retryRateLimit 只为读取端点开启：mutation 的结果不确定，不能重放。
	retryRateLimit bool
}

func newRequest(method, rawURL string, query url.Values) (request, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return request{}, err
	}
	if len(query) > 0 {
		// 与旧实现一致：保留 URL 自带的 query，再追加本次参数。
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

// GetJSON 提供给 endpoint family 的窄 transport 能力。route、query、wire
// DTO 与响应校验由具体 endpoint package 拥有；这里仅负责 App API 的请求、
// 静态认证与既有 Retry-After 读取契约。
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	req, err := c.appRequest(http.MethodGet, path, query, nil)
	if err != nil {
		return Transport(err)
	}
	return c.doJSON(ctx, req, out)
}

// GetRaw 提供给需要保留原始响应体的 endpoint family。App API adapter 仍只负责
// 认证头、状态码处理与既有重试契约，正文语义由调用方 family 解释。
func (c *Client) GetRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	req, err := c.appRequest(http.MethodGet, path, query, nil)
	if err != nil {
		return nil, Transport(err)
	}
	return c.doRaw(ctx, req)
}

// PostForm 提供给 mutation endpoint family 的窄 form transport，不重放写入。
// 认证头与状态码由此层处理；具体 route 和字段由 family 持有。
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) error {
	req, err := c.appRequest(http.MethodPost, path, nil, form)
	if err != nil {
		return Transport(err)
	}
	return c.doForm(ctx, req)
}

// PostFormJSON 提供给需要读取 mutation 响应的 endpoint family。成功响应必须
// 包含可解码 JSON；不重放请求，避免把不确定的 mutation 当作安全失败处理。
func (c *Client) PostFormJSON(ctx context.Context, path string, form url.Values, out any) error {
	req, err := c.appRequest(http.MethodPost, path, nil, form)
	if err != nil {
		return Transport(err)
	}
	return c.doJSON(ctx, req, out)
}

// do 统一发送读取请求，并按有效 Retry-After 重试一次；静态 token 不在此刷新。
// 返回的响应 Body 由调用方关闭；被重试的响应在这里关闭。
func (c *Client) do(ctx context.Context, req request) (*http.Response, error) {
	rateRetried := false
	for attempt := 1; ; attempt++ {
		resp, err := c.send(ctx, req, attempt)
		if err != nil {
			return nil, Transport(err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && req.retryRateLimit && !rateRetried {
			if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
				rateRetried = true
				c.log.LogAttrs(ctx, slog.LevelWarn, "Rate limited; retrying after wait",
					logging.OperationAttr(ctx), slog.Duration(logging.AttrWait, wait))
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

// send 发出一次请求并记录结果。form 使用显式 Content-Type；日志只记录 host、path 和状态。
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
		slog.String(logging.AttrMethod, req.method),
		slog.String(logging.AttrHost, req.url.Host),
		slog.String(logging.AttrPath, req.url.Path),
		slog.Duration(logging.AttrDuration, time.Since(start)),
		slog.Int(logging.AttrAttempt, attempt),
		logging.OperationAttr(ctx),
	}
	if err != nil {
		attrs = append(attrs, logging.TransportAttrs(err)...)
		c.log.LogAttrs(ctx, slog.LevelDebug, "Request failed", append(attrs, slog.String(logging.AttrError, Transport(err).Error()))...)
		return nil, err
	}
	attrs = append(attrs, slog.Int(logging.AttrStatus, resp.StatusCode))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		c.log.LogAttrs(ctx, slog.LevelWarn, "Request rejected by upstream", attrs...)
	} else {
		c.log.LogAttrs(ctx, slog.LevelDebug, "Request completed", attrs...)
	}
	return resp, nil
}

func discardBody(resp *http.Response) {
	// 读完再关闭，重试时保留可复用连接。
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// readBody 完整读取响应；读取失败优先，非 2xx 映射为带 Retry-After 的脱敏失败。
func readBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
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

func (c *Client) doRaw(ctx context.Context, req request) ([]byte, error) {
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readBody(resp)
}

// doForm 保留所有 2xx 成功状态；mutation endpoint 不保证返回 JSON body。
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

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		// time.Duration 以 int64 纳秒表示；不能表达的服务端秒数若乘法溢出会
		// 伪装成负等待，必须按无效 Retry-After 保留原始限流错误。
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
