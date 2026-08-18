package protocol

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

const (
	// WebBaseURL and APIBaseURL are fixed FANBOX protocol endpoints. Endpoint
	// families use them only to construct their own routes.
	WebBaseURL = "https://www.fanbox.cc/"
	APIBaseURL = "https://api.fanbox.cc/"
	// PostListPageLimit is the explicit upstream parameter used by FANBOX post
	// list routes. It is not a local result cap; continuation remains owned by
	// server-provided URLs.
	PostListPageLimit = 10
	// Chrome 146 is the profile that passed the current authorized FANBOX native
	// target probe. The UA is only a header baseline; it is not a bypass guarantee.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0"
)

// 分类 sentinel 错误；公开 SDK 层依据它们做进一步分类。
var (
	// ErrForbidden 表示 FANBOX 拒绝访问（非 Cloudflare challenge）。
	ErrForbidden = errors.New("fanbox: forbidden")
	// ErrChallenge 表示 403 响应体或 header 表明触发了 Cloudflare challenge。
	ErrChallenge = errors.New("fanbox: challenge required")
	// ErrNotAuthenticated 表示会话缺失或过期（401）。
	ErrNotAuthenticated = errors.New("fanbox: not authenticated")
	// ErrInvalidOption identifies a malformed explicit FANBOX connection option.
	// The public SDK maps it to InvalidArgument without echoing the value.
	ErrInvalidOption = errors.New("fanbox: invalid option")
)

// Session 是带 FANBOX Cookie 的只读会话。Cookie 不会作为导出字段或日志数据暴露。
type Session struct {
	httpClient   *http.Client
	proxyURL     string
	userAgent    string
	cookie       string
	flareSolverr *FlareSolverrOptions

	solverMu         sync.Mutex
	solverState      *solverState
	solverActive     *solverCall
	solverHTTPClient *http.Client // deterministic test seam; production remains direct.
}

// FlareSolverrOptions carries the two independent addresses used by the
// optional challenge recovery path. Solver state is kept on one Session only.
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// SessionOptions contains explicit FANBOX native and solver configuration.
// The solver is inactive when FlareSolverr is nil.
type SessionOptions struct {
	HTTPClient *http.Client
	// SolverHTTPClient optionally injects the FlareSolverr control transport.
	// It is an explicit transport dependency, not a fallback; production uses
	// a client created for the configured solver URL when this is nil.
	SolverHTTPClient *http.Client
	ProxyURL         string
	UserAgent        string
	FlareSolverr     *FlareSolverrOptions
}

// String 只返回会话类型，避免诊断格式化泄露认证 Cookie。
func (*Session) String() string { return "FANBOX Session" }

// GoString 保护 %#v 格式化路径，避免它展开 Session 私有字段。
func (*Session) GoString() string { return "fanbox.Session{}" }

// Option 调整 Session 构造的非敏感选项。
type Option func(*Session)

// WithHTTPClient 注入标准 *http.Client；client 必须携带显式 transport，绝不静默
// 降级到标准 net/http TLS 指纹。
func WithHTTPClient(client *http.Client) Option {
	return func(s *Session) { s.httpClient = client }
}

// WithProxyURL 使用 HTTP(S) CONNECT 代理构造 tls-client Chrome_146 指纹 transport。
// 校验失败不回显原始 URL，避免 userinfo 出现在错误或日志中。
func WithProxyURL(raw string) Option {
	return func(s *Session) { s.proxyURL = raw }
}

// WithUserAgent overrides only the native FANBOX HTTP User-Agent header.
func WithUserAgent(value string) Option {
	return func(s *Session) { s.userAgent = value }
}

// WithFlareSolverr enables the explicit challenge recovery service.
func WithFlareSolverr(options FlareSolverrOptions) Option {
	return func(s *Session) { s.flareSolverr = &options }
}

// NewSession 创建 FANBOX 只读会话。默认使用 tls-client Chrome_146 指纹的生产
// transport；注入 WithHTTPClient 时使用调用方提供的标准 transport。
func NewSession(cookieValue string, opts ...Option) (*Session, error) {
	configured := &Session{}
	for _, opt := range opts {
		if opt != nil {
			opt(configured)
		}
	}
	return NewSessionWithOptions(cookieValue, SessionOptions{
		HTTPClient:   configured.httpClient,
		ProxyURL:     configured.proxyURL,
		UserAgent:    configured.userAgent,
		FlareSolverr: configured.flareSolverr,
	})
}

// NewSessionWithOptions creates a session with explicit native and solver
// options. It performs no network I/O.
func NewSessionWithOptions(cookieHeader string, options SessionOptions) (*Session, error) {
	validatedAgent, err := validateUserAgent(options.UserAgent)
	if err != nil {
		return nil, err
	}
	proxyURL, err := validateProxyURL(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	flareSolverr, err := normalizeFlareSolverrOptions(options.FlareSolverr)
	if err != nil {
		return nil, err
	}
	if options.HTTPClient != nil {
		if options.HTTPClient.Transport == nil {
			return nil, errors.New("FANBOX injected HTTP client requires an explicit transport")
		}
		return newSession(cookieHeader, options.HTTPClient, options.SolverHTTPClient, proxyURL, validatedAgent, flareSolverr)
	}
	transport, err := newBrowserTransport(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create FANBOX TLS transport: %w", err)
	}
	return newSession(cookieHeader, &http.Client{Transport: transport}, options.SolverHTTPClient, proxyURL, validatedAgent, flareSolverr)
}

// NewSessionWithHTTPClient 仅供受控测试或上层显式注入 transport；client 必须有 transport。
func NewSessionWithHTTPClient(cookieHeader string, client *http.Client) (*Session, error) {
	return NewSessionWithOptions(cookieHeader, SessionOptions{HTTPClient: client})
}

func newSession(cookieHeader string, client, solverClient *http.Client, proxyURL, agent string, flareSolverr *FlareSolverrOptions) (*Session, error) {
	cookie, err := NormalizeCookieHeader(cookieHeader)
	if err != nil {
		return nil, err
	}
	return &Session{httpClient: client, proxyURL: proxyURL, userAgent: agent, cookie: cookie, flareSolverr: flareSolverr, solverHTTPClient: solverClient}, nil
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

// MediaRequest is the controlled request surface for FANBOX resource reads.
// It deliberately contains only the method and conditional headers that the
// public resource contract permits; callers cannot inject arbitrary headers or
// credentials into the product transport.
type MediaRequest struct {
	Method          string
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

// GetJSON performs one authenticated FANBOX JSON request for an endpoint
// family. Route and wire decoding remain owned by the endpoint package.
func (s *Session) GetJSON(ctx context.Context, endpoint string, target any) error {
	response, err := s.do(ctx, endpoint, requestKindFanbox, true, "application/json, text/plain, */*")
	if err != nil {
		return err
	}
	if response.Body == nil {
		return errors.New("FANBOX API response has no body")
	}
	decodeErr := json.NewDecoder(response.Body).Decode(target)
	closeErr := response.Body.Close()
	if decodeErr != nil {
		decodeFailure := safeExternalError(ctx, "decode FANBOX API response", decodeErr)
		if closeErr != nil {
			return errors.Join(decodeFailure, safeExternalError(ctx, "close FANBOX API response failed", closeErr))
		}
		return decodeFailure
	}
	if closeErr != nil {
		return safeExternalError(ctx, "close FANBOX API response failed", closeErr)
	}
	return nil
}

// ValidateAPIURL applies the same HTTPS/FANBOX host policy used by the native
// transport. Endpoint families call it before handing server continuations to
// a transport implementation.
func ValidateAPIURL(rawURL string) error {
	_, err := parseAllowedURL(rawURL, requestKindFanbox)
	return err
}

// ValidateMediaURL applies the media host policy without opening a request.
// The resource endpoint uses it to keep locator validation beside the product
// transport policy.
func ValidateMediaURL(rawURL string) error {
	_, err := parseAllowedURL(rawURL, requestKindMedia)
	return err
}

func (s *Session) do(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept string) (*http.Response, error) {
	return s.doWithRequest(ctx, rawURL, kind, includeCookie, accept, http.MethodGet, nil)
}

// doWithRequest adds the small, product-controlled resource request surface to
// the shared FANBOX transport. Callers cannot pass arbitrary headers; the
// public SDK constructs the allowlisted Range/conditional headers before this
// function is reached.
func (s *Session) doWithRequest(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept, method string, headers http.Header) (*http.Response, error) {
	if s == nil {
		return nil, errors.New("FANBOX session has no HTTP transport")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, errors.New("FANBOX resource request method is invalid")
	}
	response, err := s.doNativeWithRequest(ctx, rawURL, kind, includeCookie, accept, method, headers)
	if !errors.Is(err, ErrChallenge) || s.flareSolverr == nil {
		return response, err
	}
	// A challenge after cached clearance invalidates only this Client's in-memory
	// state. The original request is allowed one fresh solve and one native
	// replay; a second challenge is returned to the caller.
	s.invalidateSolverState()
	if _, solveErr := s.waitForSolver(ctx); solveErr != nil {
		return nil, solveErr
	}
	diagnostics.Emit(ctx, diagnostics.Event{
		Module:    diagnostics.ModuleFanboxNetwork,
		Kind:      diagnostics.EventReplay,
		Operation: "request",
		Route:     "native transport",
	})
	response, err = s.doNativeWithRequest(ctx, rawURL, kind, includeCookie, accept, method, headers)
	if errors.Is(err, ErrChallenge) {
		s.invalidateSolverState()
	}
	return response, err
}

// OpenMedia opens one allowlisted media URL. Only the downloads FANBOX host
// receives the session cookie on the first request; redirects are handled by
// the transport policy above.
func (s *Session) OpenMedia(ctx context.Context, mediaURL string) (*http.Response, error) {
	return s.OpenMediaWithRequest(ctx, mediaURL, MediaRequest{Method: http.MethodGet})
}

// OpenMediaWithRequest exposes the narrow conditional resource request surface
// without allowing arbitrary caller headers or credentials.
func (s *Session) OpenMediaWithRequest(ctx context.Context, mediaURL string, request MediaRequest) (*http.Response, error) {
	header := make(http.Header)
	for _, field := range []struct{ name, value string }{
		{"Range", request.Range},
		{"If-None-Match", request.IfNoneMatch},
		{"If-Modified-Since", request.IfModifiedSince},
		{"If-Range", request.IfRange},
	} {
		if field.value != "" {
			header.Set(field.name, field.value)
		}
	}
	response, err := s.doWithRequest(ctx, mediaURL, requestKindMedia, true, "*/*", request.Method, header)
	if err != nil {
		return nil, err
	}
	if response.Body == nil {
		return nil, errors.New("FANBOX media response has no body")
	}
	response.Body = &safeMediaReadCloser{ctx: ctx, body: response.Body}
	return response, nil
}

// safeMediaReadCloser maps external stream failures to safe errors while
// preserving byte-for-byte media streaming and explicit close semantics.
type safeMediaReadCloser struct {
	ctx  context.Context
	body io.ReadCloser
}

func (r *safeMediaReadCloser) Read(body []byte) (int, error) {
	count, err := r.body.Read(body)
	if err == nil || err == io.EOF {
		return count, err
	}
	return count, safeExternalError(r.ctx, "read FANBOX media failed", err)
}

func (r *safeMediaReadCloser) Close() error {
	if err := r.body.Close(); err != nil {
		return safeExternalError(r.ctx, "close FANBOX media failed", err)
	}
	return nil
}

func (s *Session) doNativeWithRequest(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept, method string, headers http.Header) (*http.Response, error) {
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

		request, err := http.NewRequestWithContext(ctx, method, canonical, nil)
		if err != nil {
			return nil, errors.New("build FANBOX request")
		}
		request.Header.Set("Origin", "https://www.fanbox.cc")
		request.Header.Set("Referer", WebBaseURL)
		agent, clearance := s.nativeState()
		request.Header.Set("User-Agent", agent)
		request.Header.Set("Accept", accept)
		for key, values := range headers {
			for _, value := range values {
				request.Header.Add(key, value)
			}
		}
		// Cookie 只用于首个认证请求。一旦服务端发出 redirect，之后的手动跟随请求
		// 即使仍指向 www/api FANBOX 也绝不带 Cookie。
		if includeCookie && !redirected && credentialCookieAllowed(kind, target.Hostname()) {
			request.Header.Set("Cookie", nativeCookieHeader(s.cookie, clearance))
		}

		// 禁用 Client 的默认十跳 redirect 行为；redirect 由此循环逐跳验证而无猜测上限。
		client := *s.httpClient
		client.Jar = nil
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		response, err := client.Do(request)
		if err != nil {
			diagnostics.Emit(ctx, diagnostics.Event{
				Module:    diagnostics.ModuleFanboxNetwork,
				Kind:      diagnostics.EventFailed,
				Operation: "network request",
				Route:     fanboxDiagnosticRoute(kind),
			})
			return nil, safeExternalError(ctx, "FANBOX request failed", err)
		}
		if response == nil {
			return nil, errors.New("FANBOX request returned no response")
		}
		diagnostics.Emit(ctx, diagnostics.Event{
			Module:    diagnostics.ModuleFanboxNetwork,
			Kind:      diagnostics.EventNetworkRequest,
			Operation: fanboxDiagnosticOperation(kind),
			Resource:  target.EscapedPath(),
			Route:     fanboxDiagnosticRoute(kind),
			Proxy:     s.proxyURL,
			UserAgent: agent,
			Status:    response.StatusCode,
		})
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
	challengeMarkers := false
	var readErr error
	if status == http.StatusForbidden && response.Body != nil {
		// 流式扫描并丢弃整个响应体；没有任意字节截断，也不把 body 保留到错误或日志。
		challengeMarkers, readErr = scanChallengeBody(response.Body)
	}
	if err := closeResponseBody(ctx, response); err != nil {
		return err
	}
	if readErr != nil {
		return safeExternalError(ctx, "read FANBOX error response failed", readErr)
	}
	switch status {
	case http.StatusUnauthorized:
		return ErrNotAuthenticated
	case http.StatusForbidden:
		if challengeMarkers || challengeHeaderDetected(response.Header) || challengeHTMLResponse(response.Header) {
			diagnostics.Emit(ctx, diagnostics.Event{
				Module:    diagnostics.ModuleFanboxNetwork,
				Kind:      diagnostics.EventChallenge,
				Operation: "request",
				Status:    status,
			})
			return ErrChallenge
		}
		return ErrForbidden
	default:
		return fmt.Errorf("FANBOX request returned HTTP status %d", status)
	}
}

func fanboxDiagnosticOperation(kind requestKind) string {
	if kind == requestKindMedia {
		return "retrieving media"
	}
	return "retrieving"
}

func fanboxDiagnosticRoute(kind requestKind) string {
	if kind == requestKindMedia {
		return "media transport"
	}
	return "native transport"
}

func scanChallengeBody(body io.Reader) (bool, error) {
	// 只识别 Cloudflare 明确使用的 body marker；普通 JSON 403 即使业务字段
	// 恰好包含 challenge 一词，也必须继续按普通 Forbidden 分类。
	markers := []string{"cf-chl", "cf_chl"}
	maxMarkerLength := 0
	for _, marker := range markers {
		if len(marker) > maxMarkerLength {
			maxMarkerLength = len(marker)
		}
	}
	buffer := make([]byte, 32*1024)
	tail := ""
	found := false
	for {
		count, err := body.Read(buffer)
		if count > 0 {
			combined := tail + strings.ToLower(string(buffer[:count]))
			for _, marker := range markers {
				if strings.Contains(combined, marker) {
					found = true
				}
			}
			if len(combined) >= maxMarkerLength-1 {
				tail = combined[len(combined)-(maxMarkerLength-1):]
			} else {
				tail = combined
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return found, nil
			}
			return false, err
		}
	}
}

func challengeHeaderDetected(header http.Header) bool {
	for _, value := range header.Values("Cf-Mitigated") {
		if strings.Contains(strings.ToLower(value), "challenge") {
			return true
		}
	}
	return false
}

func challengeHTMLResponse(header http.Header) bool {
	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "text/html") {
		return false
	}
	if strings.Contains(strings.ToLower(header.Get("Server")), "cloudflare") {
		return true
	}
	return header.Get("Cf-Ray") != ""
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

func credentialCookieAllowed(kind requestKind, host string) bool {
	host = strings.ToLower(host)
	switch kind {
	case requestKindFanbox:
		return fanboxCookieHost(host)
	case requestKindMedia:
		// The recorded attachment contract requires the same session on
		// downloads.fanbox.cc, but never on Pixiv/CDN or third-party hosts.
		return host == "downloads.fanbox.cc"
	default:
		return false
	}
}

func mediaHost(host string) bool {
	if fanboxHost(host) {
		return true
	}
	return host == "i.pximg.net" || strings.HasSuffix(host, ".pximg.net") ||
		host == "fanbox.pixiv.net" || strings.HasSuffix(host, ".fanbox.pixiv.net")
}

// browserTransport 将标准 RoundTripper 调用桥接到 tls-client 的 Chrome_146 profile。
type browserTransport struct{ client tlsclient.HttpClient }

var _ http.RoundTripper = (*browserTransport)(nil)

func validateUserAgent(raw string) (string, error) {
	if raw == "" {
		return userAgent, nil
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return "", fmt.Errorf("%w: FANBOX User-Agent contains an invalid header character", ErrInvalidOption)
	}
	return raw, nil
}

func validateProxyURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	proxyURL, err := url.Parse(raw)
	if err != nil || proxyURL == nil || proxyURL.Hostname() == "" ||
		(proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.User != nil {
		return "", fmt.Errorf("%w: FANBOX proxy URL is invalid", ErrInvalidOption)
	}
	return proxyURL.String(), nil
}

func normalizeFlareSolverrOptions(options *FlareSolverrOptions) (*FlareSolverrOptions, error) {
	if options == nil {
		return nil, nil
	}
	serviceURL, err := url.Parse(options.URL)
	if err != nil || serviceURL == nil || serviceURL.Hostname() == "" ||
		(serviceURL.Scheme != "http" && serviceURL.Scheme != "https") || serviceURL.User != nil ||
		serviceURL.Path != "" && serviceURL.Path != "/" || serviceURL.RawQuery != "" || serviceURL.Fragment != "" || serviceURL.ForceQuery {
		return nil, fmt.Errorf("%w: FlareSolverr service URL is invalid", ErrInvalidOption)
	}
	serviceURL.Path = ""
	serviceURL.RawPath = ""
	serviceURL.RawQuery = ""
	serviceURL.Fragment = ""
	serviceURL.ForceQuery = false
	if options.ProxyURL != "" {
		proxy, err := url.Parse(options.ProxyURL)
		if err != nil || proxy == nil || proxy.Hostname() == "" || proxy.User != nil ||
			(proxy.Scheme != "http" && proxy.Scheme != "socks4" && proxy.Scheme != "socks5") ||
			(proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.Fragment != "" || proxy.ForceQuery {
			return nil, fmt.Errorf("%w: FlareSolverr upstream proxy URL is invalid", ErrInvalidOption)
		}
	}
	return &FlareSolverrOptions{URL: serviceURL.String(), ProxyURL: options.ProxyURL}, nil
}

func newBrowserTransport(proxyURL string) (*browserTransport, error) {
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
	return &browserTransport{client: client}, nil
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

func (t *browserTransport) RoundTrip(request *http.Request) (*http.Response, error) {
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

func (t *browserTransport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}
