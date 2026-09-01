package ascii2d

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

const (
	// defaultUserAgent 与 tls-client 的 Chrome_146 profile 保持同一主版本。
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	browserAccept    = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	browserEncoding  = "gzip, deflate, br"
	browserLanguage  = "en-US,en;q=0.9"
)

var chromiumUserAgentPatterns = []struct {
	pattern *regexp.Regexp
	brand   string
}{
	{pattern: regexp.MustCompile(`(?:^|[\s(;])EdgiOS/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])EdgA/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Edg/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])OPR/([0-9]+)`), brand: "Opera"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])CriOS/([0-9]+)`), brand: "Google Chrome"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Chrome/([0-9]+)`), brand: "Google Chrome"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Chromium/([0-9]+)`), brand: "Chromium"},
}

// clientHints 保存从 UA 安全解析出的 Chromium hints。Enabled=false 时，调用方必须
// 完全省略 sec-ch-ua*，避免向非 Chromium UA 发送相互矛盾的浏览器声明。
type clientHints struct {
	enabled  bool
	value    string
	mobile   string
	platform string
}

func normalizeUserAgent(raw string) (string, clientHints, error) {
	if raw == "" {
		raw = defaultUserAgent
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return "", clientHints{}, reversesearch.NewError(reversesearch.CodeInvalidRequest, "ascii2d user-agent contains invalid header characters", nil)
	}
	return raw, parseClientHints(raw), nil
}

func parseClientHints(userAgent string) clientHints {
	var (
		version string
		brand   string
	)
	for _, candidate := range chromiumUserAgentPatterns {
		matches := candidate.pattern.FindStringSubmatch(userAgent)
		if len(matches) == 2 {
			version = matches[1]
			brand = candidate.brand
			break
		}
	}
	if version == "" {
		return clientHints{}
	}

	platform := userAgentPlatform(userAgent)
	mobile := "?0"
	if platform == "Android" || platform == "iOS" {
		mobile = "?1"
	}
	brands := []string{
		`"Not(A:Brand";v="24"`,
		fmt.Sprintf(`"Chromium";v="%s"`, version),
	}
	if brand != "Chromium" {
		brands = append(brands, fmt.Sprintf(`"%s";v="%s"`, brand, version))
	}
	return clientHints{
		enabled:  true,
		value:    strings.Join(brands, ", "),
		mobile:   mobile,
		platform: platform,
	}
}

func userAgentPlatform(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "Android"):
		return "Android"
	case strings.Contains(userAgent, "iPhone"), strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "iPod"):
		return "iOS"
	case strings.Contains(userAgent, "Windows"):
		return "Windows"
	case strings.Contains(userAgent, "Macintosh"):
		return "macOS"
	case strings.Contains(userAgent, "Linux"):
		return "Linux"
	default:
		return ""
	}
}

func (c *Client) applyNavigationHeaders(request *http.Request, fetchSite, referer string) {
	header := make(http.Header)
	c.applyClientHints(header)
	header.Set("Upgrade-Insecure-Requests", "1")
	header.Set("User-Agent", c.userAgent)
	header.Set("Accept", browserAccept)
	if referer != "" {
		header.Set("Referer", referer)
	}
	header.Set("Sec-Fetch-Site", fetchSite)
	header.Set("Sec-Fetch-Mode", "navigate")
	header.Set("Sec-Fetch-User", "?1")
	header.Set("Sec-Fetch-Dest", "document")
	header.Set("Accept-Encoding", browserEncoding)
	header.Set("Accept-Language", browserLanguage)
	request.Header = header
}

func (c *Client) applyFormHeaders(request *http.Request) {
	header := make(http.Header)
	c.applyClientHints(header)
	header.Set("User-Agent", c.userAgent)
	header.Set("Accept", browserAccept)
	header.Set("Origin", c.originURL())
	header.Set("Referer", c.homeURL())
	header.Set("Sec-Fetch-Site", "same-origin")
	header.Set("Sec-Fetch-Mode", "navigate")
	header.Set("Sec-Fetch-User", "?1")
	header.Set("Sec-Fetch-Dest", "document")
	header.Set("Accept-Encoding", browserEncoding)
	header.Set("Accept-Language", browserLanguage)
	request.Header = header
}

func (c *Client) applyClientHints(header http.Header) {
	if !c.hints.enabled {
		return
	}
	header.Set("Sec-CH-UA", c.hints.value)
	header.Set("Sec-CH-UA-Mobile", c.hints.mobile)
	if c.hints.platform != "" {
		header.Set("Sec-CH-UA-Platform", fmt.Sprintf(`"%s"`, c.hints.platform))
	}
}

// browserHeaderOrder is kept out of net/http.Header: the fhttp magic keys are
// invalid ordinary HTTP header names and must never reach an injected standard
// transport. browserTransport adds them only after converting the request.
func browserHeaderOrder(header http.Header) []string {
	order := make([]string, 0, 15)
	if header.Get("Sec-CH-UA") != "" {
		order = append(order, "sec-ch-ua", "sec-ch-ua-mobile")
		if header.Get("Sec-CH-UA-Platform") != "" {
			order = append(order, "sec-ch-ua-platform")
		}
	}
	if header.Get("Upgrade-Insecure-Requests") != "" {
		order = append(order, "upgrade-insecure-requests")
	}
	order = append(order, "user-agent", "cookie", "accept")
	if header.Get("Origin") != "" {
		order = append(order, "origin")
	}
	if header.Get("Referer") != "" {
		order = append(order, "referer")
	}
	order = append(order,
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"accept-encoding",
		"accept-language",
	)
	if header.Get("Content-Type") != "" {
		order = append(order, "content-type")
	}
	return order
}

func (c *Client) originURL() string {
	origin := *c.endpoint
	origin.Path = ""
	origin.RawPath = ""
	origin.RawQuery = ""
	origin.Fragment = ""
	origin.RawFragment = ""
	origin.ForceQuery = false
	return origin.String()
}

func (c *Client) homeURL() string {
	home := *c.endpoint
	home.RawQuery = ""
	home.Fragment = ""
	home.RawFragment = ""
	home.ForceQuery = false
	if home.Path == "" {
		home.Path = "/"
	}
	return home.String()
}

// browserTransport 将标准 RoundTripper 调用桥接到 tls-client 的 Chrome_146 profile。
type browserTransport struct{ client tlsclient.HttpClient }

var _ http.RoundTripper = (*browserTransport)(nil)

func newBrowserTransport(proxyURL string) (*browserTransport, error) {
	noIdleTimeout := time.Duration(0)
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithNotFollowRedirects(),
		// 上下文取消负责中断请求；这里显式关闭依赖库默认的 30 秒总 deadline。
		tlsclient.WithTimeoutSeconds(0),
		// 这不是请求 timeout：空闲连接由调用方的生命周期关闭，避免正常的大媒体流
		// 因无依据的时间阈值中断。
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{IdleConnTimeout: &noIdleTimeout}),
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed == nil {
			return nil, errors.New("ascii2d proxy URL is invalid")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			// tls-client 的内置 CONNECT dialer 会把 timeout=0 转成当前时刻的
			// deadline；HTTP(S) 代理改用不设固定 deadline 的 context dialer。
			options = append(options, tlsclient.WithProxyDialerFactory(newProxyDialerFactory(proxyURL)))
		default:
			options = append(options, tlsclient.WithProxyUrl(proxyURL))
		}
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &browserTransport{client: client}, nil
}

func (t *browserTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("ascii2d request is invalid")
	}
	if t == nil || t.client == nil {
		return nil, errors.New("ascii2d TLS transport is not configured")
	}
	frequest, err := fhttp.NewRequestWithContext(request.Context(), request.Method, request.URL.String(), request.Body)
	if err != nil {
		return nil, err
	}
	frequest.Header = make(fhttp.Header, len(request.Header))
	for key, values := range request.Header {
		frequest.Header[key] = append([]string(nil), values...)
	}
	frequest.Header[fhttp.HeaderOrderKey] = browserHeaderOrder(request.Header)
	frequest.Header[fhttp.PHeaderOrderKey] = []string{":method", ":authority", ":scheme", ":path"}
	frequest.ContentLength = request.ContentLength
	frequest.Host = request.Host
	response, err := t.client.Do(frequest)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("ascii2d TLS client returned no response")
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
		Header:           cloneHeader(response.Header),
		Request:          request,
	}
	return result, nil
}

func (t *browserTransport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}

func cloneHeader(header fhttp.Header) http.Header {
	cloned := make(http.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func validateProxyURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", reversesearch.NewError(reversesearch.CodeInvalidRequest, "ascii2d proxy URL is invalid", nil)
	}
	return parsed.String(), nil
}

func newProxyDialerFactory(proxyURL string) tlsclient.ProxyDialerFactory {
	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, connectHeaders fhttp.Header, _ tlsclient.Logger) (proxy.ContextDialer, error) {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed == nil || parsed.Hostname() == "" {
			return nil, errors.New("ascii2d proxy URL is invalid")
		}
		if parsed.Port() == "" {
			defaultPort := "80"
			if strings.EqualFold(parsed.Scheme, "https") {
				defaultPort = "443"
			}
			parsed.Host = net.JoinHostPort(parsed.Hostname(), defaultPort)
		}
		headers := cloneHeader(connectHeaders)
		if parsed.User != nil {
			password, _ := parsed.User.Password()
			headerValue := base64.StdEncoding.EncodeToString([]byte(parsed.User.Username() + ":" + password))
			headers.Set("Proxy-Authorization", "Basic "+headerValue)
		}
		return &proxyDialer{
			proxyURL: parsed,
			dialer:   net.Dialer{LocalAddr: localAddr},
			headers:  headers,
		}, nil
	}
}

type proxyDialer struct {
	proxyURL *url.URL
	dialer   net.Dialer
	headers  http.Header
}

func (d *proxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil || d.proxyURL == nil {
		return nil, errors.New("ascii2d proxy dialer is not configured")
	}

	var (
		conn net.Conn
		err  error
	)
	switch strings.ToLower(d.proxyURL.Scheme) {
	case "http":
		conn, err = d.dialer.DialContext(ctx, network, d.proxyURL.Host)
	case "https":
		tlsDialer := tls.Dialer{
			NetDialer: &d.dialer,
			Config:    &tls.Config{ServerName: d.proxyURL.Hostname()},
		}
		conn, err = tlsDialer.DialContext(ctx, network, d.proxyURL.Host)
	default:
		return nil, errors.New("ascii2d proxy URL is invalid")
	}
	if err != nil {
		return nil, proxyContextError(ctx)
	}
	stopWatching := closeProxyConnOnCancel(ctx, conn)
	defer stopWatching()

	request := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: address},
		Host:   address,
		Header: d.headers.Clone(),
	}).WithContext(ctx)
	if err := request.Write(conn); err != nil {
		_ = conn.Close()
		return nil, proxyContextError(ctx)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		_ = conn.Close()
		return nil, proxyContextError(ctx)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, errors.New("ascii2d proxy CONNECT failed")
	}
	return conn, nil
}

func proxyContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("ascii2d proxy connection failed")
}

func closeProxyConnOnCancel(ctx context.Context, conn net.Conn) func() {
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
