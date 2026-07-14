package pixiv

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// HTTPClient 是公开 SDK 创建 operation transport 的内部基础设施；它不构造
// Source，也不包含内容、认证或资源 facade。
func HTTPClient(proxyValue string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	if proxyValue == "" {
		return client, nil
	}
	proxyURL, err := url.Parse(proxyValue)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyValue, err)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return client, nil
}
