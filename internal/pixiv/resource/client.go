// Package resource 负责 Pixiv 资源响应的传输与复制。
package resource

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	AppReferer   = "https://app-api.pixiv.net/"
	AppUserAgent = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	WebUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

type Client struct {
	httpClient *http.Client
	referer    string
	userAgent  string
	headers    map[string]string
	// includeErrorBody 保留 Web resource 旧有的非 2xx body 诊断语义；App 仅报告 status。
	includeErrorBody bool
}

func NewApp(httpClient *http.Client) *Client {
	return newClient(httpClient, AppReferer, AppUserAgent)
}

func NewWeb(httpClient *http.Client, webBase string) *Client {
	base := strings.TrimRight(webBase, "/")
	if base == "" {
		base = "https://www.pixiv.net"
	}
	c := newClient(httpClient, base+"/", WebUserAgent)
	c.includeErrorBody = true
	c.headers = map[string]string{
		"Accept":          "application/json,text/plain,*/*",
		"Accept-Language": "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7",
	}
	return c
}

func newClient(httpClient *http.Client, referer, userAgent string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient, referer: referer, userAgent: userAgent}
}

func (c *Client) Download(ctx context.Context, rawURL string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", c.referer)
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
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
