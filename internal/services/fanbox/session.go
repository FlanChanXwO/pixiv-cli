package fanbox

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

const (
	webBaseURL = "https://www.fanbox.cc/"
	apiBaseURL = "https://api.fanbox.cc/api/v1/"
	userAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
)

// 分类 sentinel 错误；公开 SDK 层依据它们做进一步分类。
var (
	// ErrForbidden 表示 FANBOX 拒绝访问（非 Cloudflare challenge）。
	ErrForbidden = errors.New("fanbox: forbidden")
	// ErrChallenge 表示 403 响应体或 header 表明触发了 Cloudflare challenge。
	ErrChallenge = errors.New("fanbox: challenge required")
	// ErrNotAuthenticated 表示会话缺失或过期（401）。
	ErrNotAuthenticated = errors.New("fanbox: not authenticated")
)

// Session 是带 FANBOX Cookie 的只读会话。Cookie 不会作为导出字段或日志数据暴露。
type Session struct {
	httpClient *http.Client
	proxyURL   string
	cookie     string
}

// String 只返回会话类型，避免诊断格式化泄露认证 Cookie。
func (Session) String() string { return "FANBOX Session" }

// GoString 保护 %#v 格式化路径，避免它展开 Session 私有字段。
func (Session) GoString() string { return "fanbox.Session{}" }

// Option 调整 Session 构造的非敏感选项。
type Option func(*Session)

// WithHTTPClient 注入标准 *http.Client；client 必须携带显式 transport，绝不静默
// 降级到标准 net/http TLS 指纹。
func WithHTTPClient(client *http.Client) Option {
	return func(s *Session) { s.httpClient = client }
}

// WithProxyURL 使用 HTTP(S) CONNECT 代理构造 tls-client Chrome 指纹 transport。
// 校验失败不回显原始 URL，避免 userinfo 出现在错误或日志中。
func WithProxyURL(raw string) Option {
	return func(s *Session) { s.proxyURL = raw }
}

// NewSession 创建 FANBOX 只读会话。默认使用 tls-client Chrome_146 指纹的生产
// transport；注入 WithHTTPClient 时使用调用方提供的标准 transport。
func NewSession(cookieValue string, opts ...Option) (*Session, error) {
	options := &Session{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	if options.httpClient != nil {
		return NewSessionWithHTTPClient(cookieValue, options.httpClient)
	}
	proxyURL, err := validateProxyURL(options.proxyURL)
	if err != nil {
		return nil, err
	}
	transport, err := newChromeTransport(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create FANBOX TLS transport: %w", err)
	}
	return newSession(cookieValue, &http.Client{Transport: transport})
}

// NewSessionWithHTTPClient 仅供受控测试或上层显式注入 transport；client 必须有 transport。
func NewSessionWithHTTPClient(cookieHeader string, client *http.Client) (*Session, error) {
	if client == nil || client.Transport == nil {
		return nil, errors.New("FANBOX injected HTTP client requires an explicit transport")
	}
	return newSession(cookieHeader, client)
}

func newSession(cookieHeader string, client *http.Client) (*Session, error) {
	cookie, err := NormalizeCookieHeader(cookieHeader)
	if err != nil {
		return nil, err
	}
	return &Session{httpClient: client, cookie: cookie}, nil
}

// CloseIdleConnections 释放会话 transport 持有的空闲连接。它不设置请求 deadline；
// 长期使用的调用方可在会话不再需要时显式调用，避免无请求期间的 FD 长期占用。
func (s *Session) CloseIdleConnections() {
	if s == nil || s.httpClient == nil {
		return
	}
	s.httpClient.CloseIdleConnections()
}

type requestKind uint8

const (
	requestKindFanbox requestKind = iota
	requestKindMedia
)

func (s *Session) do(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept string) (*http.Response, error) {
	if s == nil || s.httpClient == nil || s.httpClient.Transport == nil {
		return nil, errors.New("FANBOX session has no HTTP transport")
	}
	visited := make(map[string]struct{})
	redirected := false
	for {
		target, err := parseAllowedURL(rawURL, kind)
		if err != nil {
			return nil, err
		}
		canonical := target.String()
		if _, exists := visited[canonical]; exists {
			return nil, errors.New("FANBOX redirect loop detected")
		}
		visited[canonical] = struct{}{}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, canonical, nil)
		if err != nil {
			return nil, errors.New("build FANBOX request")
		}
		request.Header.Set("Origin", "https://www.fanbox.cc")
		request.Header.Set("Referer", webBaseURL)
		request.Header.Set("User-Agent", userAgent)
		request.Header.Set("Accept", accept)
		// Cookie 只用于首个认证请求。一旦服务端发出 redirect，之后的手动跟随请求
		// 即使仍指向 www/api FANBOX 也绝不带 Cookie。
		if includeCookie && !redirected && fanboxCookieHost(target.Hostname()) {
			request.Header.Set("Cookie", s.cookie)
		}

		// 禁用 Client 的默认十跳 redirect 行为；redirect 由此循环逐跳验证而无猜测上限。
		client := *s.httpClient
		client.Jar = nil
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		response, err := client.Do(request)
		if err != nil {
			return nil, safeExternalError(ctx, "FANBOX request failed", err)
		}
		if response == nil {
			return nil, errors.New("FANBOX request returned no response")
		}
		if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
			location := response.Header.Get("Location")
			if err := closeResponseBody(ctx, response); err != nil {
				return nil, err
			}
			if strings.TrimSpace(location) == "" {
				return nil, errors.New("FANBOX redirect has no location")
			}
			next, err := target.Parse(location)
			if err != nil {
				return nil, errors.New("FANBOX redirect location is invalid")
			}
			rawURL = next.String()
			redirected = true
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, s.classifyError(ctx, response)
		}
		return response, nil
	}
}

// classifyError 把非 2xx 响应映射为分类 sentinel；403 依 body/header 区分 challenge。
func (s *Session) classifyError(ctx context.Context, response *http.Response) error {
	status := response.StatusCode
	var challengeMarkers string
	if status == http.StatusForbidden && response.Body != nil {
		// 只读取有限字节判定 challenge marker；内容永不进入返回的错误。
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		if readErr == nil {
			challengeMarkers = string(data)
		}
	}
	if err := closeResponseBody(ctx, response); err != nil {
		return err
	}
	switch status {
	case http.StatusUnauthorized:
		return ErrNotAuthenticated
	case http.StatusForbidden:
		if challengeDetected(challengeMarkers, response.Header) {
			return ErrChallenge
		}
		return ErrForbidden
	default:
		return fmt.Errorf("FANBOX request returned HTTP status %d", status)
	}
}

// challengeDetected 依据 Cloudflare 的 cf-chl/cf_chl/challenge body marker 或
// Cf-Mitigated: challenge header 判定 challenge；FANBOX 本身在 Cloudflare 之后，
// 因此不把普通的 Server: cloudflare 当作 challenge 证据。
func challengeDetected(body string, header http.Header) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "cf-chl") || strings.Contains(lower, "cf_chl") || strings.Contains(lower, "challenge") {
		return true
	}
	for _, value := range header.Values("Cf-Mitigated") {
		if strings.Contains(strings.ToLower(value), "challenge") {
			return true
		}
	}
	return false
}

func closeResponseBody(ctx context.Context, response *http.Response) error {
	if response == nil || response.Body == nil {
		return errors.New("FANBOX response has no body")
	}
	if err := response.Body.Close(); err != nil {
		return safeExternalError(ctx, "close FANBOX response failed", err)
	}
	return nil
}

// safeExternalError 不包装 transport/body 的原始错误，避免 URL、Cookie 或响应内容
// 经 url.Error 或上游实现进入 Error/Unwrap 链。上下文取消仍保留可识别的标准 error。
func safeExternalError(ctx context.Context, message string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New(message)
}

func parseAllowedURL(rawURL string, kind requestKind) (*url.URL, error) {
	target, err := url.Parse(rawURL)
	if err != nil || target == nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
		return nil, errors.New("FANBOX URL is not an allowed HTTPS URL")
	}
	host := strings.ToLower(target.Hostname())
	switch kind {
	case requestKindFanbox:
		if !fanboxHost(host) {
			return nil, errors.New("FANBOX URL host is not allowed")
		}
	case requestKindMedia:
		if !mediaHost(host) {
			return nil, errors.New("FANBOX media URL host is not allowed")
		}
	default:
		return nil, errors.New("FANBOX request kind is invalid")
	}
	return target, nil
}

func fanboxHost(host string) bool {
	return host == "fanbox.cc" || strings.HasSuffix(host, ".fanbox.cc")
}

func fanboxCookieHost(host string) bool {
	host = strings.ToLower(host)
	return host == "www.fanbox.cc" || host == "api.fanbox.cc"
}

func mediaHost(host string) bool {
	if fanboxHost(host) {
		return true
	}
	return host == "i.pximg.net" || strings.HasSuffix(host, ".pximg.net") ||
		host == "fanbox.pixiv.net" || strings.HasSuffix(host, ".fanbox.pixiv.net")
}

// chromeTransport 将标准 RoundTripper 调用桥接到 tls-client，生产路径固定 Chrome_146。
type chromeTransport struct{ client tlsclient.HttpClient }

var _ http.RoundTripper = (*chromeTransport)(nil)

func validateProxyURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	proxyURL, err := url.Parse(raw)
	if err != nil || proxyURL == nil || proxyURL.Hostname() == "" ||
		(proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.User != nil {
		return "", errors.New("FANBOX proxy URL is invalid")
	}
	return proxyURL.String(), nil
}

func newChromeTransport(proxyURL string) (*chromeTransport, error) {
	noIdleTimeout := time.Duration(0)
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithNotFollowRedirects(),
		// 上下文取消负责中断请求；这里显式关闭依赖库默认的 30 秒总 deadline。
		tlsclient.WithTimeoutSeconds(0),
		// 这不是请求 timeout：空闲连接由 Session.CloseIdleConnections 显式释放，
		// 因而不会让正常的大媒体流因无依据的时间阈值中断。
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{IdleConnTimeout: &noIdleTimeout}),
	}
	if proxyURL != "" {
		// tls-client 的内置 CONNECT dialer 会将 timeout=0 转化为当前时刻的
		// deadline，导致 HTTP proxy 立刻超时；这里由自定义、可取消的 dialer
		// 接管。不能同时设置 WithProxyUrl 和 factory，故 URL 由闭包保存。
		options = append(options, tlsclient.WithProxyDialerFactory(newFanboxProxyDialerFactory(proxyURL)))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &chromeTransport{client: client}, nil
}

// newFanboxProxyDialerFactory 为 tls-client 提供仅支持 HTTP(S) CONNECT 的代理拨号器。
// 它不设置固定 deadline：调用方 context 取消时主动关闭当前连接，确保 CONNECT 写入、
// 响应读取及 HTTPS proxy TLS 握手均能及时结束。
func newFanboxProxyDialerFactory(proxyURL string) tlsclient.ProxyDialerFactory {
	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, connectHeaders fhttp.Header, _ tlsclient.Logger) (proxy.ContextDialer, error) {
		validated, err := validateProxyURL(proxyURL)
		if err != nil {
			return nil, err
		}
		parsed, err := url.Parse(validated)
		if err != nil || parsed == nil {
			return nil, errors.New("FANBOX proxy URL is invalid")
		}
		if parsed.Port() == "" {
			defaultPort := "80"
			if parsed.Scheme == "https" {
				defaultPort = "443"
			}
			parsed.Host = net.JoinHostPort(parsed.Hostname(), defaultPort)
		}
		headers := make(http.Header, len(connectHeaders))
		for key, values := range connectHeaders {
			headers[key] = append([]string(nil), values...)
		}
		return &fanboxProxyDialer{
			proxyURL: parsed,
			dialer:   net.Dialer{LocalAddr: localAddr},
			headers:  headers,
		}, nil
	}
}

type fanboxProxyDialer struct {
	proxyURL *url.URL
	dialer   net.Dialer
	headers  http.Header
}

func (d *fanboxProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil || d.proxyURL == nil {
		return nil, errors.New("FANBOX proxy dialer is not configured")
	}

	var (
		conn net.Conn
		err  error
	)
	switch d.proxyURL.Scheme {
	case "http":
		conn, err = d.dialer.DialContext(ctx, network, d.proxyURL.Host)
	case "https":
		tlsDialer := tls.Dialer{
			NetDialer: &d.dialer,
			Config:    &tls.Config{ServerName: d.proxyURL.Hostname()},
		}
		conn, err = tlsDialer.DialContext(ctx, network, d.proxyURL.Host)
	default:
		return nil, errors.New("FANBOX proxy URL is invalid")
	}
	if err != nil {
		return nil, fanboxProxyContextError(ctx)
	}
	stopWatching := closeFanboxProxyConnOnCancel(ctx, conn)
	defer stopWatching()

	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: address},
		Host:   address,
		Header: d.headers.Clone(),
	}
	if err := request.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fanboxProxyContextError(ctx)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		_ = conn.Close()
		return nil, fanboxProxyContextError(ctx)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, errors.New("FANBOX proxy CONNECT failed")
	}
	return conn, nil
}

func fanboxProxyContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("FANBOX proxy connection failed")
}

func closeFanboxProxyConnOnCancel(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (t *chromeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("FANBOX request is invalid")
	}
	frequest, err := fhttp.NewRequestWithContext(request.Context(), request.Method, request.URL.String(), request.Body)
	if err != nil {
		return nil, err
	}
	frequest.Header = make(fhttp.Header, len(request.Header))
	for key, values := range request.Header {
		for _, value := range values {
			frequest.Header.Add(key, value)
		}
	}
	frequest.ContentLength = request.ContentLength
	frequest.Host = request.Host
	response, err := t.client.Do(frequest)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("FANBOX TLS client returned no response")
	}
	result := &http.Response{
		Status:           response.Status,
		StatusCode:       response.StatusCode,
		Proto:            response.Proto,
		ProtoMajor:       response.ProtoMajor,
		ProtoMinor:       response.ProtoMinor,
		Body:             response.Body,
		ContentLength:    response.ContentLength,
		TransferEncoding: append([]string(nil), response.TransferEncoding...),
		Close:            response.Close,
		Uncompressed:     response.Uncompressed,
		Header:           make(http.Header, len(response.Header)),
		Request:          request,
	}
	for key, values := range response.Header {
		result.Header[key] = append([]string(nil), values...)
	}
	return result, nil
}

func (t *chromeTransport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}
