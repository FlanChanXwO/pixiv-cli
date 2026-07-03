package pixiv

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv/web"
)

type SourceConfig struct {
	RefreshToken       string
	HTTPSProxy         string
	WebFallbackEnabled bool
}

type OAuthConfig struct {
	HTTPSProxy string
}

func NewSource(cfg SourceConfig) (*Source, error) {
	httpClient, err := HTTPClient(cfg.HTTPSProxy)
	if err != nil {
		return nil, err
	}
	app := New(cfg.RefreshToken, WithHTTPClient(httpClient))
	webClient := web.New(web.WithHTTPClient(httpClient))
	return NewSourceFromClients(app, webClient, SourcePolicy{
		RefreshToken:       cfg.RefreshToken,
		WebFallbackEnabled: cfg.WebFallbackEnabled,
	}), nil
}

func NewOAuthClient(cfg OAuthConfig, oauthBase string) (*Client, error) {
	httpClient, err := HTTPClient(cfg.HTTPSProxy)
	if err != nil {
		return nil, err
	}
	return New("", WithHTTPClient(httpClient), WithBaseURLs("", oauthBase)), nil
}

func HTTPClient(proxyValue string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 沿用 Pixiv App API client 既有 60s 网络上限；代理 client 也必须保留同一保护。
	httpClient := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	if proxyValue == "" {
		return httpClient, nil
	}
	proxyURL, err := url.Parse(proxyValue)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyValue, err)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return httpClient, nil
}
