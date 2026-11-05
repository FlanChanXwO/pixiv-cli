package config

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
		return nil, fmt.Errorf("parse proxy URL: %w", errInvalidProxy)
	}
	if proxyURL.Host == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https" && proxyURL.Scheme != "socks5" && proxyURL.Scheme != "socks5h") {
		return nil, fmt.Errorf("proxy URL must use http, https, socks5, or socks5h: %w", errInvalidProxy)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return client, nil
}
