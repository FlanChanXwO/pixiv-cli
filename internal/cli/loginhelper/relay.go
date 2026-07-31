package loginhelper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
)

const (
	// RelayResultURLHeader 只承载一次性、无敏感最终页 URL。server 直到 OAuth
	// exchange 完成才结束 callback response；client 在此期间打开结果页。
	RelayResultURLHeader = "X-Pixiv-Relay-Result-URL"
)

type relayCallbackCompletion struct {
	Success bool `json:"success"`
}

// RemoteCallbackSession 代表已被 server 接收、但尚未完成 OAuth exchange 的一次
// callback。ResultURL 只显示固定成功/失败页面；它从不包含 Pixiv code 或 token。
type RemoteCallbackSession struct {
	ResultURL string
	response  io.ReadCloser
	onClose   func()
	closeOnce sync.Once
}

// Complete 等待 server 完成 OAuth exchange。调用方应先打开 ResultURL，再调用它；
// 这样浏览器可以持续显示“Completing login”，最终得到与 server 一致的结果。
func (s *RemoteCallbackSession) Complete() error {
	if s == nil || s.response == nil {
		return errors.New("remote Pixiv login relay session is unavailable")
	}
	defer s.close()
	var completion relayCallbackCompletion
	decoder := json.NewDecoder(s.response)
	if err := decoder.Decode(&completion); err != nil {
		return errors.New("remote Pixiv login relay did not return a final result")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("remote Pixiv login relay returned an invalid final result")
	}
	if !completion.Success {
		return errors.New("remote Pixiv login relay reported that login failed")
	}
	return nil
}

// Abort 仅在浏览器无法打开最终页时释放仍在 server 上等待的 callback 请求。
func (s *RemoteCallbackSession) Abort() {
	if s != nil {
		s.close()
	}
}

func (s *RemoteCallbackSession) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.response != nil {
			_ = s.response.Close()
		}
		if s.onClose != nil {
			s.onClose()
		}
	})
}

// relayEndpointURL 允许 reverse proxy 将 relay 挂在 URL 前缀下，但拒绝 query、
// fragment、userinfo 与其他 scheme，避免 capability 变成请求注入通道。
func relayEndpointURL(base, suffix, sessionID string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.TrimSpace(sessionID) == "" {
		return "", errors.New("invalid remote login relay URL")
	}
	parsed.Path = path.Join("/", parsed.Path, suffix, sessionID)
	return parsed.String(), nil
}

// validateRelayResultURL 确保 server response 的最终页仍属于本次 relay origin。
func validateRelayResultURL(base, resultURL string) error {
	expected, err := canonicalRelayOrigin(base)
	if err != nil {
		return err
	}
	parsedBase, _ := url.Parse(expected)
	actual, err := url.Parse(strings.TrimSpace(resultURL))
	if err != nil || actual == nil || actual.Scheme != parsedBase.Scheme || actual.Host != parsedBase.Host || actual.User != nil || actual.RawQuery != "" || actual.Fragment != "" {
		return errors.New("invalid remote login relay result URL")
	}
	prefix := path.Join("/", parsedBase.Path, "result") + "/"
	if !strings.HasPrefix(actual.Path, prefix) {
		return errors.New("invalid remote login relay result URL")
	}
	resultID := strings.TrimPrefix(actual.Path, prefix)
	if resultID == "" || path.Base(resultID) != resultID {
		return errors.New("invalid remote login relay result URL")
	}
	if _, err := base64.RawURLEncoding.DecodeString(resultID); err != nil {
		return errors.New("invalid remote login relay result URL")
	}
	return nil
}

// newHandoffRequest 保持请求构造集中，确保 callback 永远不会落到 URL query 中。
func newHandoffRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}
