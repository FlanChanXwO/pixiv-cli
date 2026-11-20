package config

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

// errInvalidProxy 标记代理配置错误，包装它的错误统一保留这段稳定分类文本。
var errInvalidProxy = errors.New("invalid proxy configuration")

// HTTPClient 根据代理配置创建独立的 HTTP transport；空值表示直连。
func HTTPClient(proxyValue string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport}
	if proxyValue == "" {
		return client, nil
	}
	proxyURL, err := url.Parse(proxyValue)
	if err != nil {
		return nil, i18n.Errorf("error.parse_proxy_url", errInvalidProxy)
	}
	if proxyURL.Host == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https" && proxyURL.Scheme != "socks5" && proxyURL.Scheme != "socks5h") {
		return nil, i18n.Errorf("error.proxy_url_must_use_http_https_socks5_or", errInvalidProxy)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return client, nil
}
