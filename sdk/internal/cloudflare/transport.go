package cloudflare

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

const (
	headerProxyAuthorization = "Proxy-Authorization"
	headerClientAgent        = "Sec-CH-UA"
	headerClientMobile       = "Sec-CH-UA-Mobile"
	headerClientPlatform     = "Sec-CH-UA-Platform"
	headerUpgradeInsecure    = "Upgrade-Insecure-Requests"
	headerUserAgent          = "User-Agent"
	headerCookie             = "Cookie"
	headerOrigin             = "Origin"
	headerReferer            = "Referer"
	headerFetchSite          = "Sec-Fetch-Site"
	headerFetchMode          = "Sec-Fetch-Mode"
	headerFetchUser          = "Sec-Fetch-User"
	headerFetchDestination   = "Sec-Fetch-Dest"
	headerAcceptEncoding     = "Accept-Encoding"
	headerAcceptLanguage     = "Accept-Language"
	defaultHTTPPort          = "80"
	defaultHTTPSPort         = "443"
)

func newBrowserTransport(proxyURL *url.URL) (tlsclient.HttpClient, error) {
	noIdleTimeout := time.Duration(0)
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithNotFollowRedirects(),
		// Context cancellation owns request duration, not tls-client's default 30s deadline.
		tlsclient.WithTimeoutSeconds(0),
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{IdleConnTimeout: &noIdleTimeout}),
	}
	if proxyURL != nil {
		switch proxyURL.Scheme {
		case "http", "https":
			// The built-in CONNECT dialer turns timeout=0 into an immediate deadline.
			options = append(options, tlsclient.WithProxyDialerFactory(newProxyDialerFactory(proxyURL)))
		default:
			options = append(options, tlsclient.WithProxyUrl(proxyURL.String()))
		}
	}
	return tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
}

// RoundTrip sends one native request with the Chrome 146 TLS profile. It never
// invokes the solver or follows redirects; the outer http.Client owns redirects.
// Cancellation follows request.Context, as required by http.RoundTripper.
// Header ordering metadata stays in fhttp and is never added to request.Header.
func (c *Client) RoundTrip(request *http.Request) (*http.Response, error) {
	converted, err := fhttp.NewRequestWithContext(request.Context(), request.Method, request.URL.String(), request.Body)
	if err != nil {
		return nil, failure(operationTransport, sdk.InvalidArgument, "invalid native request", err)
	}
	converted.Header = make(fhttp.Header, len(request.Header)+2)
	for key, values := range request.Header {
		converted.Header[key] = append([]string(nil), values...)
	}
	converted.Header[fhttp.HeaderOrderKey] = browserHeaderOrder(request.Header)
	converted.Header[fhttp.PHeaderOrderKey] = []string{":method", ":authority", ":scheme", ":path"}
	converted.ContentLength, converted.Host = request.ContentLength, request.Host
	response, err := c.native.Do(converted)
	if err != nil {
		return nil, failure(operationTransport, sdk.UpstreamUnavailable, "native request failed", err)
	}
	return &http.Response{
		Status: response.Status, StatusCode: response.StatusCode,
		Proto: response.Proto, ProtoMajor: response.ProtoMajor, ProtoMinor: response.ProtoMinor,
		Body: response.Body, ContentLength: response.ContentLength,
		TransferEncoding: response.TransferEncoding, Close: response.Close,
		Uncompressed: response.Uncompressed, Header: http.Header(response.Header),
		Request: request,
	}, nil
}

func browserHeaderOrder(header http.Header) []string {
	var order []string
	for _, key := range []string{
		headerClientAgent, headerClientMobile, headerClientPlatform, headerUpgradeInsecure,
		headerUserAgent, headerCookie, headerAccept, headerOrigin, headerReferer, headerFetchSite,
		headerFetchMode, headerFetchUser, headerFetchDestination, headerAcceptEncoding, headerAcceptLanguage, headerContentType,
	} {
		if header.Get(key) != "" {
			order = append(order, strings.ToLower(key))
		}
	}
	return order
}

func newProxyDialerFactory(proxyURL *url.URL) tlsclient.ProxyDialerFactory {
	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, connectHeaders fhttp.Header, _ tlsclient.Logger) (proxy.ContextDialer, error) {
		parsed := *proxyURL
		if parsed.Port() == "" {
			port := defaultHTTPPort
			if parsed.Scheme == "https" {
				port = defaultHTTPSPort
			}
			parsed.Host = net.JoinHostPort(parsed.Hostname(), port)
		}
		headers := http.Header(connectHeaders).Clone()
		if parsed.User != nil {
			if headers == nil {
				headers = make(http.Header)
			}
			password, _ := parsed.User.Password()
			value := base64.StdEncoding.EncodeToString([]byte(parsed.User.Username() + ":" + password))
			headers.Set(headerProxyAuthorization, "Basic "+value)
		}
		return &proxyDialer{endpoint: &parsed, dialer: net.Dialer{LocalAddr: localAddr}, headers: headers}, nil
	}
}

type proxyDialer struct {
	endpoint *url.URL
	dialer   net.Dialer
	headers  http.Header
}

// DialContext binds proxy dialing and the CONNECT exchange to ctx cancellation.
func (d *proxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var conn net.Conn
	var err error
	if d.endpoint.Scheme == "https" {
		dialer := tls.Dialer{NetDialer: &d.dialer, Config: &tls.Config{ServerName: d.endpoint.Hostname()}}
		conn, err = dialer.DialContext(ctx, network, d.endpoint.Host)
	} else {
		conn, err = d.dialer.DialContext(ctx, network, d.endpoint.Host)
	}
	if err != nil {
		return nil, err
	}
	// Close interrupts CONNECT writes/reads too, which net.Conn does not bind to ctx.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Host: address}, Host: address, Header: d.headers.Clone()}
	if err := request.Write(conn); err != nil {
		return nil, closeFailedProxy(ctx, conn, err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		return nil, closeFailedProxy(ctx, conn, err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, closeFailedProxy(ctx, conn, errors.New("proxy CONNECT failed"))
	}
	return conn, nil
}

func closeFailedProxy(ctx context.Context, conn net.Conn, cause error) error {
	err := errors.Join(cause, conn.Close())
	if ctx.Err() != nil {
		err = errors.Join(ctx.Err(), err)
	}
	return err
}
