package loginhelper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
)

const (
	relaySessionPath  = "session"
	relayCallbackPath = "callback"
	// RelayResultURLHeader 只承载一次性、无敏感最终页 URL。server 直到 OAuth
	// exchange 完成才结束 callback response；client 在此期间打开结果页。
	RelayResultURLHeader = "X-Pixiv-Relay-Result-URL"
)

type relayCallbackRequest struct {
	CallbackURL string `json:"callback_url"`
}

type relayCallbackCompletion struct {
	Success bool `json:"success"`
}

// RemoteCallbackSession 代表已被 server 接收、但尚未完成 OAuth exchange 的一次
// callback。ResultURL 只显示固定成功/失败页面；它从不包含 Pixiv code、token 或 secret。
type RemoteCallbackSession struct {
	ResultURL string
	response  *http.Response
}

// Complete 等待 server 完成 OAuth exchange。调用方应先打开 ResultURL，再调用它；
// 这样浏览器可以持续显示“Completing login”，最终得到与 server 一致的结果。
func (s *RemoteCallbackSession) Complete() error {
	if s == nil || s.response == nil {
		return errors.New("remote Pixiv login relay session is unavailable")
	}
	defer s.response.Body.Close()
	var completion relayCallbackCompletion
	decoder := json.NewDecoder(s.response.Body)
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
	if s != nil && s.response != nil {
		_ = s.response.Body.Close()
	}
}

// RelayTargetConfig 是 client handler 所需的最小配置。secret 只在内存中用于
// bearer 请求，不能写入 handler manifest、日志或错误文本。
type RelayTargetConfig struct {
	TargetURL string
	Secret    string
}

// ConfiguredRelayTarget 从当前用户的私有配置读取 client relay。它不回显、包装或
// 格式化 secret，避免把 bearer credential 传播到诊断边界。
func ConfiguredRelayTarget() (RelayTargetConfig, error) {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return RelayTargetConfig{}, err
	}
	runtime, err := settings.Runtime()
	if err != nil {
		return RelayTargetConfig{}, err
	}
	target := RelayTargetConfig{TargetURL: runtime.LoginRelayTargetURL, Secret: runtime.LoginRelaySecret}
	if target.TargetURL == "" && target.Secret == "" {
		return RelayTargetConfig{}, ErrNoConfiguredRelay
	}
	if target.TargetURL == "" || target.Secret == "" {
		return RelayTargetConfig{}, fmt.Errorf("%w: both login_relay_target_url and login_relay_secret are required", ErrIncompleteRelayConfig)
	}
	if _, err := relayEndpointURL(target.TargetURL, relayCallbackPath); err != nil {
		return RelayTargetConfig{}, err
	}
	return target, nil
}

// ForwardConfiguredCallback 在没有活跃 loopback bridge 时转发精确白名单 callback。
// 它只在 server 已接收并回传一次性最终页 URL 后返回；调用方随后打开该页并通过
// RemoteCallbackSession.Complete 等待 OAuth exchange 的真实结果。
func ForwardConfiguredCallback(ctx context.Context, rawCallbackURL string) (*RemoteCallbackSession, error) {
	if !IsAllowedPixivCallbackURL(rawCallbackURL) {
		return nil, errors.New("Pixiv callback URL is not allowed for remote relay")
	}
	target, err := ConfiguredRelayTarget()
	if err != nil {
		return nil, err
	}
	sessionURL, err := relayEndpointURL(target.TargetURL, relaySessionPath)
	if err != nil {
		return nil, err
	}
	callbackURL, err := relayEndpointURL(target.TargetURL, relayCallbackPath)
	if err != nil {
		return nil, err
	}
	if err := relaySessionRequest(ctx, sessionURL, target.Secret); err != nil {
		return nil, err
	}
	body, err := json.Marshal(relayCallbackRequest{CallbackURL: rawCallbackURL})
	if err != nil {
		return nil, err
	}
	request, err := newRelayRequest(ctx, http.MethodPost, callbackURL, target.Secret, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, errors.New("could not contact remote Pixiv login relay")
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("remote Pixiv login relay rejected callback (HTTP %d)", response.StatusCode)
	}
	resultURL := strings.TrimSpace(response.Header.Get(RelayResultURLHeader))
	if err := validateRelayResultURL(target.TargetURL, resultURL); err != nil {
		_ = response.Body.Close()
		return nil, err
	}
	return &RemoteCallbackSession{ResultURL: resultURL, response: response}, nil
}

func relaySessionRequest(ctx context.Context, endpoint, secret string) error {
	req, err := newRelayRequest(ctx, http.MethodGet, endpoint, secret, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		// transport error 可能包含 target URL；remote target 是部署细节，且 URL
		// 可带敏感路径，handler 的用户可见错误只保留稳定分类。
		return errors.New("could not contact remote Pixiv login relay")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remote Pixiv login relay rejected callback (HTTP %d)", response.StatusCode)
	}
	return nil
}

func newRelayRequest(ctx context.Context, method, endpoint, secret string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// relayEndpointURL 允许 reverse proxy 将 relay 挂在 URL 前缀下，但拒绝 query、
// fragment、userinfo 与其他 scheme，避免配置值变成请求注入或凭据泄露通道。
func relayEndpointURL(base, suffix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid remote login relay URL")
	}
	parsed.Path = path.Join("/", parsed.Path, suffix)
	return parsed.String(), nil
}

// validateRelayResultURL 确保被打开的浏览器页面仍属于用户配置的 relay base。
// server response 不能借由结果页 header 将 protocol handler 变成任意 URL opener。
func validateRelayResultURL(base, resultURL string) error {
	expectedPrefix, err := relayEndpointURL(base, "result")
	if err != nil {
		return err
	}
	expected, err := url.Parse(expectedPrefix)
	if err != nil {
		return errors.New("invalid remote login relay result URL")
	}
	actual, err := url.Parse(strings.TrimSpace(resultURL))
	if err != nil || actual == nil || actual.Scheme != expected.Scheme || actual.Host != expected.Host || actual.User != nil || actual.RawQuery != "" || actual.Fragment != "" {
		return errors.New("invalid remote login relay result URL")
	}
	prefix := strings.TrimSuffix(expected.Path, "/") + "/"
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
