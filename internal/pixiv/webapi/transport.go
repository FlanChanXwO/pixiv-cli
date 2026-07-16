package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

// EnvelopeError 保留内部兼容名称；message 不得越过 Web adapter。
type EnvelopeError = protocol.Failure

// IllustPagesError 保留匿名详情流程中 pages 子阶段，供 facade 精确标注 operation。
type IllustPagesError struct {
	err error
}

func (e *IllustPagesError) Error() string { return fmt.Sprintf("web illust pages: %v", e.err) }
func (e *IllustPagesError) Unwrap() error { return e.err }

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	rawURL, err := c.webURL(path, query)
	if err != nil {
		return protocol.Transport(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return protocol.Transport(err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return protocol.Transport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// reader 失败可能来自底层 transport；不能让其 URL 或代理诊断跨越
		// Web adapter，统一转换为 protocol 的脱敏失败。
		return protocol.Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return protocol.HTTPStatus(resp.StatusCode)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return protocol.MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return protocol.MalformedResponse()
	}
	return nil
}

func (c *Client) webURL(path string, query url.Values) (string, error) {
	base, err := url.Parse(c.webBase)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(rel)
	if len(query) > 0 {
		resolved.RawQuery = query.Encode()
	}
	return resolved.String(), nil
}

func (c *Client) setHeaders(req *http.Request) {
	for key, value := range protocol.WebHeaders() {
		if key == "User-Agent" && c.userAgent != "" {
			value = c.userAgent
		}
		req.Header.Set(key, value)
	}
}

type APIError = protocol.Failure

func webEnvelopeError(string) error {
	return protocol.UpstreamRejected()
}
