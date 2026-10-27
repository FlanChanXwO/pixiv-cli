package pixivapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OpenRequest 描述一次资源读取；Method 为空时按 GET 发送，Header 由调用方校验后传入。
type OpenRequest struct {
	URL    string
	Method string
	Header http.Header
}

// Response 保留响应流所有权，调用方负责关闭 Body。
type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// OpenResource 以不带产品凭据的 client 流式读取允许列表内的 URL；注入的 redirect
// 回调改写目标后，最终 URL 仍会再次校验。
func (c *Client) OpenResource(ctx context.Context, request OpenRequest) (*Response, error) {
	if err := c.ValidateResourceURL(request.URL); err != nil {
		return nil, Failure{Kind: FailureResourceForbidden}
	}
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, request.URL, nil)
	if err != nil {
		return nil, Transport(err)
	}
	for key, values := range request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Referer", AppReferer)
	req.Header.Set("User-Agent", AppUserAgent)
	// 复制调用方设置，但去掉可能携带凭据的 Cookie jar。
	httpClient := *c.http
	httpClient.Jar = nil
	originalRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if originalRedirect != nil {
			if err := originalRedirect(next, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			// 保持 net/http.Client 在 CheckRedirect 为空时的默认十跳上限。
			return errors.New("stopped after 10 redirects")
		}
		// caller callback 可能改写 URL，因此必须验证其最终结果。
		if err := c.ValidateResourceURL(next.URL.String()); err != nil {
			return Failure{Kind: FailureResourceForbidden}
		}
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, Transport(err)
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}

// ValidateResourceURL 按 Pixiv 媒体的 scheme、主机与路径策略校验 rawURL。
func (c *Client) ValidateResourceURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid resource URL")
	}
	return validateResourceURL(parsed, c.allowedResourceHosts)
}

// ValidStampURL 按上游 stamp 契约校验绝对 request URI：必须位于官方媒体主机，不接受
// 调用方额外添加的资源主机。
func ValidStampURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(rawURL)
	return err == nil && validateResourceURL(parsed, nil) == nil
}

func validateResourceURL(parsed *url.URL, extra []string) error {
	if parsed.Scheme != "https" || parsed.User != nil {
		return errors.New("resource URL must be https without userinfo")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return errors.New("resource URL has no path")
	}
	host := strings.ToLower(parsed.Hostname())
	if !allowedResourceHost(host, extra) {
		return errors.New("resource host is not allowed")
	}
	return nil
}

var defaultResourceHosts = []string{"i.pximg.net", "s.pximg.net", "i-f.pximg.net"}

func allowedResourceHost(host string, extra []string) bool {
	for _, allowed := range defaultResourceHosts {
		if host == allowed {
			return true
		}
	}
	for _, allowed := range extra {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

// CacheResource 只登记已通过校验的 locator；ref 是公开模型持有的不透明引用。
func (c *Client) CacheResource(ref, rawURL string) error {
	if err := c.ValidateResourceURL(rawURL); err != nil {
		return err
	}
	c.resourceMu.Lock()
	c.resourceURLs[ref] = rawURL
	c.resourceMu.Unlock()
	return nil
}

// CachedResource 返回本运行时为 ref 登记的 locator。
func (c *Client) CachedResource(ref string) (string, bool) {
	c.resourceMu.RLock()
	rawURL, ok := c.resourceURLs[ref]
	c.resourceMu.RUnlock()
	return rawURL, ok
}
