// Authenticated JSON and media requests, redirect policy, and safe external failures.

package fanboxapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

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

// GetJSONURL performs one authenticated request against a server-provided
// continuation. The continuation is validated against the FANBOX host policy
// before it reaches the transport.
func (s *Client) GetJSONURL(ctx context.Context, endpoint string, target any) error {
	response, err := s.do(ctx, endpoint, requestKindFanbox, true, "application/json, text/plain, */*")
	if err != nil {
		return err
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		err = safeExternalError(ctx, "read FANBOX API response failed", err)
	} else if json.Unmarshal(body, target) != nil {
		err = ErrMalformedResponse
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		err = errors.Join(err, safeExternalError(ctx, "close FANBOX API response failed", closeErr))
	}
	return err
}

// ValidateAPIURL applies the same HTTPS/FANBOX host policy used by the native
// transport. Endpoint families call it before handing server continuations to
// a transport implementation.
func ValidateAPIURL(rawURL string) error {
	_, err := parseAllowedURL(rawURL, requestKindFanbox)
	return err
}

// ValidateMediaURL applies the media host policy without opening a request.
// sdk/fanbox uses it to validate every locator it exposes.
func ValidateMediaURL(rawURL string) error {
	_, err := parseAllowedURL(rawURL, requestKindMedia)
	return err
}

func (s *Client) do(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept string) (*http.Response, error) {
	return s.doWithRequest(ctx, rawURL, kind, includeCookie, accept, http.MethodGet, nil)
}

// doWithRequest adds the small, product-controlled resource request surface to
// the shared FANBOX transport. Callers cannot pass arbitrary headers; the
// public SDK constructs the allowlisted Range/conditional headers before this
// function is reached.
func (s *Client) doWithRequest(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept, method string, headers http.Header) (*http.Response, error) {
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
	s.log.LogAttrs(ctx, slog.LevelDebug, "Replaying request with fresh clearance", logging.OperationAttr(ctx))
	response, err = s.doNativeWithRequest(ctx, rawURL, kind, includeCookie, accept, method, headers)
	if errors.Is(err, ErrChallenge) {
		s.invalidateSolverState()
	}
	return response, err
}

// OpenMedia exposes the narrow conditional resource request surface
// without allowing arbitrary caller headers or credentials.
func (s *Client) OpenMedia(ctx context.Context, mediaURL string, request MediaRequest) (*http.Response, error) {
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

func (s *Client) doNativeWithRequest(ctx context.Context, rawURL string, kind requestKind, includeCookie bool, accept, method string, headers http.Header) (*http.Response, error) {
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
		started := time.Now()
		response, err := client.Do(request)
		attrs := []slog.Attr{
			slog.String(logging.AttrMethod, method),
			slog.String(logging.AttrHost, target.Host),
			slog.String(logging.AttrPath, target.Path),
			slog.Duration(logging.AttrDuration, time.Since(started)),
			logging.OperationAttr(ctx),
		}
		if err != nil {
			s.log.LogAttrs(ctx, slog.LevelDebug, "Request failed", append(attrs, slog.String(logging.AttrError, safeExternalError(ctx, "FANBOX request failed", err).Error()))...)
			return nil, safeExternalError(ctx, "FANBOX request failed", err)
		}
		statusAttrs := append(attrs, slog.Int(logging.AttrStatus, response.StatusCode))
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
			s.log.LogAttrs(ctx, slog.LevelWarn, "Request rejected by upstream", statusAttrs...)
		} else {
			s.log.LogAttrs(ctx, slog.LevelDebug, "Request completed", statusAttrs...)
		}
		// 只有真正的 redirect 状态码（301/302/303/307/308）才跟随 Location；
		// 304 Not Modified 是条件 GET 的正常结果，没有 Location，必须原样返回。
		if isRedirectStatus(response.StatusCode) {
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
		// 304 Not Modified 是条件 GET 的正常结果，原样返回给调用方；其余非 2xx 才报错。
		if response.StatusCode == http.StatusNotModified {
			return response, nil
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, s.classifyError(ctx, response)
		}
		return response, nil
	}
}

// isRedirectStatus 仅识别真正带 Location 的重定向状态码。304 Not Modified 是
// 条件 GET 的正常结果，必须排除，否则会被误当 redirect 并因无 Location 报错。
func isRedirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// classifyError 把非 2xx 响应映射为分类 sentinel；403 依 body/header 区分 challenge。
func (s *Client) classifyError(ctx context.Context, response *http.Response) error {
	status := response.StatusCode
	challengeMarkers := false
	var readErr error
	if status == http.StatusForbidden {
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
			s.log.LogAttrs(ctx, slog.LevelWarn, "Cloudflare challenge detected",
				logging.OperationAttr(ctx), slog.Int(logging.AttrStatus, status))
			return ErrChallenge
		}
		return ErrForbidden
	default:
		return fmt.Errorf("FANBOX request returned HTTP status %d", status)
	}
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
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
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
		return host == DownloadsHost
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
