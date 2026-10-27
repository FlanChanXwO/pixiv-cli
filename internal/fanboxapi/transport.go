package fanboxapi

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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

func validateProxyURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(raw)
	if err != nil || proxyURL.Hostname() == "" ||
		(proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.User != nil {
		return nil, fmt.Errorf("%w: FANBOX proxy URL is invalid", ErrInvalidOption)
	}
	return proxyURL, nil
}

func normalizeFlareSolverrOptions(options *FlareSolverrOptions) (*FlareSolverrOptions, error) {
	if options == nil {
		return nil, nil
	}
	serviceURL, err := url.Parse(options.URL)
	if err != nil || serviceURL.Hostname() == "" ||
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
		if err != nil || proxy.Hostname() == "" || proxy.User != nil ||
			(proxy.Scheme != "http" && proxy.Scheme != "socks4" && proxy.Scheme != "socks5") ||
			(proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.Fragment != "" || proxy.ForceQuery {
			return nil, fmt.Errorf("%w: FlareSolverr upstream proxy URL is invalid", ErrInvalidOption)
		}
	}
	return &FlareSolverrOptions{URL: serviceURL.String(), ProxyURL: options.ProxyURL}, nil
}

func newBrowserTransport(proxyURL *url.URL) (*browserTransport, error) {
	noIdleTimeout := time.Duration(0)
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithNotFollowRedirects(),
		// 上下文取消负责中断请求；这里显式关闭依赖库默认的 30 秒总 deadline。
		tlsclient.WithTimeoutSeconds(0),
		// 这不是请求 timeout：空闲连接由 Client.CloseIdleConnections 显式释放，
		// 因而不会让正常的大媒体流因无依据的时间阈值中断。
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{IdleConnTimeout: &noIdleTimeout}),
	}
	if proxyURL != nil {
		// tls-client 的内置 CONNECT dialer 会将 timeout=0 转化为当前时刻的
		// deadline，导致 HTTP proxy 立刻超时；这里由自定义、可取消的 dialer
		// 接管。不能同时设置 WithProxyUrl 和 factory，故 URL 由闭包保存。
		options = append(options, tlsclient.WithProxyDialerFactory(newProxyDialerFactory(proxyURL)))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &browserTransport{client: client}, nil
}

// newProxyDialerFactory 为 tls-client 提供仅支持 HTTP(S) CONNECT 的代理拨号器。
// 它不设置固定 deadline：调用方 context 取消时主动关闭当前连接，确保 CONNECT 写入、
// 响应读取及 HTTPS proxy TLS 握手均能及时结束。
func newProxyDialerFactory(proxyURL *url.URL) tlsclient.ProxyDialerFactory {
	if proxyURL.Port() == "" {
		defaultPort := "80"
		if proxyURL.Scheme == "https" {
			defaultPort = "443"
		}
		proxyURL.Host = net.JoinHostPort(proxyURL.Hostname(), defaultPort)
	}
	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, connectHeaders fhttp.Header, _ tlsclient.Logger) (proxy.ContextDialer, error) {
		headers := make(http.Header, len(connectHeaders))
		for key, values := range connectHeaders {
			headers[key] = append([]string(nil), values...)
		}
		return &proxyDialer{
			proxyURL: proxyURL,
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
		return nil, proxyContextError(ctx)
	}
	stopWatching := closeProxyConnOnCancel(ctx, conn)
	defer stopWatching()

	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: address},
		Host:   address,
		Header: d.headers.Clone(),
	}
	if err := request.Write(conn); err != nil {
		_ = conn.Close()
		return nil, proxyContextError(ctx)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		_ = conn.Close()
		return nil, proxyContextError(ctx)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, errors.New("FANBOX proxy CONNECT failed")
	}
	return conn, nil
}

func proxyContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("FANBOX proxy connection failed")
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

func (t *browserTransport) RoundTrip(request *http.Request) (*http.Response, error) {
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
	t.client.CloseIdleConnections()
}
