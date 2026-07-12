package pixiv

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/webapi"
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
	auth := oauth.New(cfg.RefreshToken, oauth.WithHTTPClient(httpClient))
	app := appapi.New(appapi.WithHTTPClient(httpClient), appapi.WithSession(auth))
	webClient := webapi.New(webapi.WithHTTPClient(httpClient))
	return NewSourceFromClients(app, webClient, auth, resource.NewApp(httpClient), resource.NewWeb(httpClient, webapi.DefaultWebBase), SourcePolicy{
		RefreshToken:       cfg.RefreshToken,
		WebFallbackEnabled: cfg.WebFallbackEnabled,
	}), nil
}

func NewOAuthClient(cfg OAuthConfig, oauthBase string) (*oauth.Client, error) {
	httpClient, err := HTTPClient(cfg.HTTPSProxy)
	if err != nil {
		return nil, err
	}
	return oauth.New("", oauth.WithHTTPClient(httpClient), oauth.WithBaseURL(oauthBase)), nil
}

func HTTPClient(proxyValue string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Runtime config 已经合并了环境变量；空代理值必须显式禁用 DefaultTransport 的 ProxyFromEnvironment。
	transport.Proxy = nil
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
