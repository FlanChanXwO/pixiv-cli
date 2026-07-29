// Package resource 负责 Pixiv 资源响应的传输与复制。
package resource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

const (
	AppReferer   = protocol.AppReferer
	AppUserAgent = protocol.AppUserAgent
	WebUserAgent = protocol.WebUserAgent
)

type Client struct {
	httpClient *http.Client
	referer    string
	userAgent  string
	headers    map[string]string
	// includeErrorBody 保留 Web resource 旧有的非 2xx body 诊断语义；App 仅报告 status。
	includeErrorBody bool
}

// OpenRequest 描述一次已由上层 policy 限定的资源读取。
type OpenRequest struct {
	URL            string
	Header         http.Header
	Validate       func(string) error
	DisableCookies bool
}

// Response 保留响应流所有权，调用方负责关闭 Body。
type Response struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       io.ReadCloser
}

func NewApp(httpClient *http.Client) *Client {
	return newClient(httpClient, AppReferer, AppUserAgent)
}

func NewWeb(httpClient *http.Client, webBase string) *Client {
	base := strings.TrimRight(webBase, "/")
	if base == "" {
		base = protocol.WebAPIBase
	}
	c := newClient(httpClient, base+"/", WebUserAgent)
	c.includeErrorBody = true
	c.headers = protocol.WebHeaders()
	delete(c.headers, "User-Agent")
	return c
}

func newClient(httpClient *http.Client, referer, userAgent string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{httpClient: httpClient, referer: referer, userAgent: userAgent}
}

func (c *Client) Download(ctx context.Context, rawURL string, dst io.Writer) error {
	resp, err := c.Open(ctx, OpenRequest{URL: rawURL})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if c.includeErrorBody {
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return fmt.Errorf("download failed: %s: read error body: %w", resp.Status, readErr)
			}
			if len(bytes.TrimSpace(body)) != 0 {
				return fmt.Errorf("download failed: %s: %s", resp.Status, string(body))
			}
		}
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

// Open 执行不预读 body 的资源请求。Validate 在首个 URL 与每次 redirect 前调用。
func (c *Client) Open(ctx context.Context, request OpenRequest) (*Response, error) {
	if request.Validate != nil {
		if err := request.Validate(request.URL); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return nil, protocol.Transport(err)
	}
	for key, values := range request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Referer", c.referer)
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	// 不修改调用方 Client；public SDK 可显式禁用 Jar，legacy 下载则保持原语义。
	httpClient := *c.httpClient
	if request.DisableCookies {
		httpClient.Jar = nil
	}
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
		if request.Validate != nil {
			if err := request.Validate(next.URL.String()); err != nil {
				return err
			}
		}
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, protocol.Transport(err)
	}
	return &Response{StatusCode: resp.StatusCode, Status: resp.Status, Header: resp.Header, Body: resp.Body}, nil
}
