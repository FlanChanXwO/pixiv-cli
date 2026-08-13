package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/go-resty/resty/v2"
)

const (
	DefaultAPIBase      = protocol.AppAPIBase
	DefaultUserAgent    = protocol.AppUserAgent
	DefaultAppOS        = protocol.AppOS
	DefaultAppOSVersion = protocol.AppOSVersion
	DefaultAppVersion   = protocol.AppVersion
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

type Client struct {
	restyClient    *resty.Client
	apiBase        string
	session        Session
	acceptLanguage string
	userID         int64
	// disableRetryAfterRetry 只由上层明确需要观察首个 429 的调度器启用。
	// 默认仍遵守既有的有效 Retry-After 单次重试契约。
	disableRetryAfterRetry bool
}

// Session 是 App 内容 API 仅需知道的最小认证边界。
type Session interface {
	AccessToken() string
	Refresh(context.Context) error
}

// GetJSON 提供给 endpoint family 的窄 transport 能力。route、query、wire
// DTO 与响应校验由具体 endpoint package 拥有；这里仅负责 App API 的请求、
// 认证刷新与既有 Retry-After 读取契约。
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.getJSONWithRetry(ctx, path, query, out)
}

// GetRaw 提供给需要保留原始响应体的 endpoint family。App API adapter 仍只负责
// 认证刷新、状态码处理与既有重试契约，正文语义由调用方 family 解释。
func (c *Client) GetRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.getRawWithRetry(ctx, path, query)
}

// PostForm 提供给 mutation endpoint family 的窄 form transport。重试与认证
// 刷新仍由 App API adapter 统一执行；具体 route 和字段由 family 持有。
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) error {
	return c.postFormWithRetry(ctx, path, form)
}

type staticSession struct{ token string }

func (s staticSession) AccessToken() string { return s.token }
func (staticSession) Refresh(context.Context) error {
	return errors.New("access token cannot be refreshed")
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.restyClient = resty.NewWithClient(httpClient)
		}
	}
}

func WithBaseURL(apiBase string) Option {
	return func(c *Client) {
		if apiBase != "" {
			c.apiBase = strings.TrimRight(apiBase, "/")
		}
	}
}

func WithSession(session Session) Option {
	return func(c *Client) { c.session = session }
}

// WithAccessToken 注入已取得的 access token，供无需刷新流程的 SDK 调用复用 App API。
func WithAccessToken(token string) Option {
	return func(c *Client) {
		c.session = staticSession{token: strings.TrimSpace(token)}
	}
}

// WithAcceptLanguage 注入语言协商头；空值不设置。
func WithAcceptLanguage(language string) Option {
	return func(c *Client) {
		c.acceptLanguage = strings.TrimSpace(language)
	}
}

// WithUserID 注入经过验证的当前账号 ID，供需要 X-User-Id 的 App endpoint（如小说内容）使用。
func WithUserID(userID int64) Option {
	return func(c *Client) { c.userID = userID }
}

// WithDisableRetryAfterRetry 使读取请求把首个有效 Retry-After 限流直接交给调用方。
// 它不改变认证刷新重试，也不会让 mutation 被重放。
func WithDisableRetryAfterRetry() Option {
	return func(c *Client) { c.disableRetryAfterRetry = true }
}

func New(opts ...Option) *Client {
	c := &Client{
		restyClient: resty.New(),
		apiBase:     DefaultAPIBase,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.restyClient == nil {
		c.restyClient = resty.New()
	}
	return c
}

type requestOptions struct {
	Headers map[string]string
	Query   url.Values
}

func (c *Client) getJSONWithRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSONWithAuthRetry(ctx, path, query, out)
	if c.disableRetryAfterRetry {
		return err
	}
	retryAfter, shouldRetry := retryAfterForRateLimit(err)
	if !shouldRetry {
		return err
	}
	// 产品契约只允许依据服务端有效 Retry-After 重试一次，避免对读取端点进行猜测性重放。
	if err := waitForRetryAfter(ctx, retryAfter); err != nil {
		return protocol.Transport(err)
	}
	return c.getJSONWithAuthRetry(ctx, path, query, out)
}

func (c *Client) getJSONWithAuthRetry(ctx context.Context, path string, query url.Values, out any) error {
	err := c.getJSON(ctx, path, query, out)
	if !isAuthAPIResponse(err) {
		return err
	}
	if c.session == nil {
		return err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		// 原始刷新失败可能携带 OAuth 传输细节；保留已脱敏的认证状态失败。
		return err
	}
	return c.getJSON(ctx, path, query, out)
}

func retryAfterForRateLimit(err error) (time.Duration, bool) {
	var failure protocol.Failure
	if !errors.As(err, &failure) || failure.Kind != protocol.FailureHTTPStatus || failure.StatusCode != http.StatusTooManyRequests || !failure.HasRetryAfter {
		return 0, false
	}
	return failure.RetryAfter, true
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

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, c.apiBase+path, requestOptions{
		Headers: c.apiHeaders(),
		Query:   query,
	}, out)
}

func (c *Client) postFormWithRetry(ctx context.Context, path string, form url.Values) error {
	err := c.postForm(ctx, path, form)
	if !isAuthAPIResponse(err) {
		return err
	}
	if c.session == nil {
		return err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		return err
	}
	return c.postForm(ctx, path, form)
}

// isAuthAPIResponse 仅识别明确的认证 HTTP 状态，避免响应正文中的词汇触发 mutation 重放。
func isAuthAPIResponse(err error) bool {
	var apiErr protocol.Failure
	return errors.As(err, &apiErr) && apiErr.Kind == protocol.FailureHTTPStatus && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) error {
	return c.doForm(ctx, http.MethodPost, c.apiBase+path, requestOptions{Headers: c.apiHeaders()}, form)
}

// getRawWithRetry 与 JSON transport 共享认证刷新和有效 Retry-After 重试。
func (c *Client) getRawWithRetry(ctx context.Context, path string, query url.Values) ([]byte, error) {
	body, err := c.getRawWithAuthRetry(ctx, path, query)
	if c.disableRetryAfterRetry {
		return body, err
	}
	retryAfter, shouldRetry := retryAfterForRateLimit(err)
	if !shouldRetry {
		return body, err
	}
	if err := waitForRetryAfter(ctx, retryAfter); err != nil {
		return nil, protocol.Transport(err)
	}
	return c.getRawWithAuthRetry(ctx, path, query)
}

func (c *Client) getRawWithAuthRetry(ctx context.Context, path string, query url.Values) ([]byte, error) {
	body, err := c.getRaw(ctx, path, query)
	if !isAuthAPIResponse(err) {
		return body, err
	}
	if c.session == nil {
		return body, err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		return body, err
	}
	return c.getRaw(ctx, path, query)
}

func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	req := c.restyClient.R().SetContext(ctx)
	req.SetHeaders(c.apiHeaders())
	if len(query) > 0 {
		req.SetQueryParamsFromValues(query)
	}
	resp, err := req.Execute(http.MethodGet, c.apiBase+path)
	if err != nil {
		return nil, protocol.Transport(err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header().Get("Retry-After"), time.Now())
		return nil, protocol.HTTPStatusWithRetryAfter(resp.StatusCode(), retryAfter, present)
	}
	return resp.Body(), nil
}

func (c *Client) apiHeaders() map[string]string {
	token := ""
	if c.session != nil {
		token = c.session.AccessToken()
	}
	headers := protocol.AppHeaders(token)
	if c.acceptLanguage != "" {
		headers["Accept-Language"] = c.acceptLanguage
	}
	if c.userID > 0 {
		headers["X-User-Id"] = strconv.FormatInt(c.userID, 10)
	}
	return headers
}

func (c *Client) doJSON(ctx context.Context, method, rawURL string, opts requestOptions, out any) error {
	req := c.restyClient.R().SetContext(ctx)
	if len(opts.Headers) > 0 {
		req.SetHeaders(opts.Headers)
	}
	if len(opts.Query) > 0 {
		req.SetQueryParamsFromValues(opts.Query)
	}
	resp, err := req.Execute(method, rawURL)
	if err != nil {
		return protocol.Transport(err)
	}
	body := resp.Body()
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header().Get("Retry-After"), time.Now())
		return protocol.HTTPStatusWithRetryAfter(resp.StatusCode(), retryAfter, present)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return protocol.MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return protocol.MalformedResponse()
	}
	return nil
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

// doForm 保留所有 2xx 成功状态；mutation endpoint 不保证返回 JSON body。
func (c *Client) doForm(ctx context.Context, method, rawURL string, opts requestOptions, form url.Values) error {
	req := c.restyClient.R().SetContext(ctx).SetFormDataFromValues(form)
	if len(opts.Headers) > 0 {
		req.SetHeaders(opts.Headers)
	}
	resp, err := req.Execute(method, rawURL)
	if err != nil {
		return protocol.Transport(err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return protocol.HTTPStatus(resp.StatusCode())
	}
	return nil
}

// APIError 保留内部兼容名称；实际失败统一由 protocol.Failure 脱敏表示。
type APIError = protocol.Failure
