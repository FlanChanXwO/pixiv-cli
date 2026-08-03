package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// hostRewrite 把某个上游主机映射到测试 mock 服务器；请求路径与查询原样保留。
type hostRewrite struct {
	host string
	base string
}

// hostRewriteTransport 将 Pixiv 固定主机重写到本地 httptest 服务器，使公开 SDK
// 的 OAuth、App API 与资源请求都能在离线测试中命中 mock 端点。inner 为 nil 时
// 使用净 transport，避免宿主代理环境把本地 mock 请求导向外部。
type hostRewriteTransport struct {
	rewrites []hostRewrite
	inner    http.RoundTripper
}

func (rt *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	for _, rewrite := range rt.rewrites {
		if strings.EqualFold(out.URL.Host, rewrite.host) {
			parsed, err := url.Parse(rewrite.base)
			if err != nil {
				return nil, err
			}
			out.URL.Scheme = parsed.Scheme
			out.URL.Host = parsed.Host
			break
		}
	}
	if rt.inner != nil {
		return rt.inner.RoundTrip(out)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport.RoundTrip(out)
}

// rewriteHTTPClient 构造把 OAuth 与 App API 主机重写到同一 mock 服务器的客户端。
func rewriteHTTPClient(_ *testing.T, server *httptest.Server, oauthBase, appAPIBase string) *http.Client {
	return &http.Client{Transport: &hostRewriteTransport{
		rewrites: []hostRewrite{
			{host: "oauth.secure.pixiv.net", base: oauthBase},
			{host: "app-api.pixiv.net", base: appAPIBase},
		},
	}}
}

// rewriteAndProxyHTTPClient 先重写上游主机，再把改写后的请求交给 inner transport
// （例如携带 proxy 与 TLS 配置的测试 transport）。
func rewriteAndProxyHTTPClient(rewrites []hostRewrite, inner http.RoundTripper) *http.Client {
	return &http.Client{Transport: &hostRewriteTransport{rewrites: rewrites, inner: inner}}
}
