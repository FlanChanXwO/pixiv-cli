package pixiv

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
)

// HTTPClient 是公开 SDK 创建 operation transport 的内部基础设施；它不构造
// Source，也不包含内容、认证或资源 facade。
func HTTPClient(proxyValue string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport}
	if proxyValue == "" {
		return client, nil
	}
	proxyURL, err := url.Parse(proxyValue)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", uri.ErrInvalidProxy)
	}
	if proxyURL.Host == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https" && proxyURL.Scheme != "socks5" && proxyURL.Scheme != "socks5h") {
		return nil, fmt.Errorf("proxy URL must use http, https, socks5, or socks5h: %w", uri.ErrInvalidProxy)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return client, nil
}
