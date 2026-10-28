// Package urlscheme 负责 pixiv:// URL scheme 的平台安装、回调分发与远程登录 hand-off。
//
// 包的组成：[HandleCallback] 分派系统回调，[StartRemoteLogin] 与 [RemoteCallbackSession]
// 管理远程 relay，[Install] 和 [EnsurePersistentIfNeeded] 维护桌面 handler。回调与 relay
// 的私有状态只留在本包；登录命令的上层流程在 internal/cli/auth，远程中继的 HTTP 协议在
// internal/loginrelay。
package urlscheme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
	filelock "github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

const (
	remoteLoginLinkPath     = "/remote-login"
	activeHandoffFilename   = "remote-login-session.json"
	remoteLoginStartPath    = "start"
	remoteLoginCallbackPath = "callback"
)

var errNoActiveRemoteLogin = errors.New("no active remote login handoff")

// remoteLoginHandoff 持有 relay 传输和一次性的本地清理。生产代码经 newRemoteLoginHandoff
// 构造；同包测试可以换成记录型 client 或会失败的清理函数。
type remoteLoginHandoff struct {
	// httpClient 不跟随 relay 重定向：请求体含一次性 proof 和 Pixiv callback，
	// 307/308 不能把它们重放到另一个 origin。
	httpClient *http.Client
	clear      func(activeRemoteLogin) error
}

// handoffHTTPClient 不跟随 relay 重定向：请求体含一次性 proof 和 Pixiv callback，
// 307/308 不能把它们重放到另一个 origin。共享同一个 client 以复用连接。
var handoffHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func newRemoteLoginHandoff() remoteLoginHandoff {
	return remoteLoginHandoff{
		httpClient: handoffHTTPClient,
		clear:      clearActiveRemoteLoginIfMatches,
	}
}

// RemoteLoginStart 承载 session page 交给 desktop handler 的一次性请求，
// 不含 OAuth URL、token 或任何持久设备身份；proof 只绑定这次会话。
type RemoteLoginStart struct {
	Origin    string
	SessionID string
	Proof     string
}

type remoteLoginStartRequest struct {
	Proof string `json:"proof"`
}

type remoteLoginCallbackRequest struct {
	CallbackURL string `json:"callback_url"`
	Proof       string `json:"proof"`
}

// activeRemoteLogin 保存当前 remote login handoff 的本地私有状态，只含
// relay origin、session 与一次性 proof，绝不保存 OAuth 凭证。
type activeRemoteLogin struct {
	Version   int    `json:"version"`
	Origin    string `json:"origin"`
	SessionID string `json:"session_id"`
	Proof     string `json:"proof"`
}

// parseRemoteLoginLink 只接受带完整 proof 的精确内部 deep link，拒绝额外
// 参数，避免 handler 成为任意 origin 的请求转发器。
func parseRemoteLoginLink(raw string) (RemoteLoginStart, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "pixiv") || !strings.EqualFold(parsed.Host, "account") || parsed.Path != remoteLoginLinkPath || parsed.User != nil || parsed.Fragment != "" {
		return RemoteLoginStart{}, errors.New("invalid remote login start link")
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values) != 3 {
		return RemoteLoginStart{}, errors.New("invalid remote login start link")
	}
	for _, key := range []string{"origin", "session", "access"} {
		if len(values[key]) != 1 || strings.TrimSpace(values.Get(key)) == "" {
			return RemoteLoginStart{}, errors.New("invalid remote login start link")
		}
	}
	origin, err := canonicalRelayOrigin(values.Get("origin"))
	if err != nil {
		return RemoteLoginStart{}, errors.New("invalid remote login start link")
	}
	return RemoteLoginStart{Origin: origin, SessionID: values.Get("session"), Proof: values.Get("access")}, nil
}

// StartRemoteLogin 向明确选中的一次性会话领取 OAuth URL，并只保存本次 callback
// 转发所需的最小私有状态。新的 handoff 会原子替换旧状态。
func StartRemoteLogin(ctx context.Context, start RemoteLoginStart) (string, error) {
	return newRemoteLoginHandoff().start(ctx, start)
}

func (h remoteLoginHandoff) start(ctx context.Context, start RemoteLoginStart) (string, error) {
	origin, err := canonicalRelayOrigin(start.Origin)
	if err != nil || strings.TrimSpace(start.SessionID) == "" || strings.TrimSpace(start.Proof) == "" {
		return "", errors.New("invalid remote login start request")
	}
	body, err := json.Marshal(remoteLoginStartRequest{Proof: start.Proof})
	if err != nil {
		return "", err
	}
	endpoint, err := relayEndpointURL(origin, remoteLoginStartPath, start.SessionID)
	if err != nil {
		return "", err
	}
	request, err := newHandoffRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	response, err := h.httpClient.Do(request)
	if err != nil {
		return "", errors.New("could not contact remote Pixiv login relay")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("remote Pixiv login relay rejected login handoff")
	}
	var result loginrelay.RemoteLoginStartResponse
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&result); err != nil {
		return "", errors.New("remote Pixiv login relay returned an invalid sign-in address")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("remote Pixiv login relay returned an invalid sign-in address")
	}
	if err := validateAuthorizationURL(result.AuthorizationURL); err != nil {
		return "", err
	}
	if err := saveActiveRemoteLogin(activeRemoteLogin{Version: 1, Origin: origin, SessionID: start.SessionID, Proof: start.Proof}); err != nil {
		return "", err
	}
	return result.AuthorizationURL, nil
}

// forwardActiveRemoteLoginCallback 只将官方 callback 交给被 remote-login deep link
// 明确启动的会话。服务端接收成功即清理私有 transient state，避免后续 callback
// 被错误重用或转发到旧会话。
func forwardActiveRemoteLoginCallback(ctx context.Context, rawCallbackURL string) (*RemoteCallbackSession, error) {
	return newRemoteLoginHandoff().forward(ctx, rawCallbackURL)
}

func (h remoteLoginHandoff) forward(ctx context.Context, rawCallbackURL string) (*RemoteCallbackSession, error) {
	if !loginrelay.IsAllowedPixivCallbackURL(rawCallbackURL) {
		return nil, errors.New("this Pixiv login link cannot be used for remote sign-in")
	}
	active, err := loadActiveRemoteLogin()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(remoteLoginCallbackRequest{CallbackURL: rawCallbackURL, Proof: active.Proof})
	if err != nil {
		return nil, err
	}
	endpoint, err := relayEndpointURL(active.Origin, remoteLoginCallbackPath, active.SessionID)
	if err != nil {
		return nil, err
	}
	request, err := newHandoffRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := h.httpClient.Do(request)
	if err != nil {
		return nil, errors.New("could not contact remote Pixiv login relay")
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, errors.New("remote Pixiv login relay rejected the login result")
	}
	resultURL := strings.TrimSpace(response.Header.Get(loginrelay.RelayResultURLHeader))
	if err := validateRelayResultURL(active.Origin, resultURL); err != nil {
		_ = response.Body.Close()
		return nil, err
	}
	if err := h.clear(active); err != nil {
		_ = response.Body.Close()
		return nil, errors.New("could not clear active remote login handoff")
	}
	return &RemoteCallbackSession{ResultURL: resultURL, response: response.Body}, nil
}

// ClearRemoteLoginHandoff 仅在本地 active state 仍然对应 start 时删除它。handler
// 成功领取 URL 后若浏览器无法启动，应调用此函数；随后 Pixiv callback 会回到此前
// 的系统 handler，而不会转发给已无法使用的会话。
func ClearRemoteLoginHandoff(start RemoteLoginStart) error {
	return newRemoteLoginHandoff().clearStart(start)
}

func (h remoteLoginHandoff) clearStart(start RemoteLoginStart) error {
	origin, err := canonicalRelayOrigin(start.Origin)
	if err != nil || strings.TrimSpace(start.SessionID) == "" || strings.TrimSpace(start.Proof) == "" {
		return errors.New("invalid remote login handoff")
	}
	if err := h.clear(activeRemoteLogin{
		Version:   1,
		Origin:    origin,
		SessionID: start.SessionID,
		Proof:     start.Proof,
	}); err != nil {
		return errors.New("could not clear active remote login handoff")
	}
	return nil
}

func canonicalRelayOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid remote login relay URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path != "" {
		parsed.Path = path.Clean("/" + parsed.Path)
		if parsed.Path == "/" {
			parsed.Path = ""
		}
	}
	return parsed.String(), nil
}

// validateAuthorizationURL 校验 relay 返回的 OAuth start URL 是否精确匹配官方
// Pixiv app login 契约；错误不包含 URL 细节。
func validateAuthorizationURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), "app-api.pixiv.net") || parsed.Port() != "" || parsed.User != nil || parsed.Fragment != "" || parsed.Path != "/web/v1/login" || parsed.EscapedPath() != "/web/v1/login" {
		return errors.New("remote Pixiv login relay returned an invalid sign-in address")
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values) != 4 || !hasExactQueryValue(values, "client", "pixiv-android") || !hasExactQueryValue(values, "code_challenge_method", "S256") || !hasSingleNonEmptyQueryValue(values, "code_challenge") || !hasSingleNonEmptyQueryValue(values, "state") {
		return errors.New("remote Pixiv login relay returned an invalid sign-in address")
	}
	return nil
}

func hasExactQueryValue(values url.Values, key, expected string) bool {
	return len(values[key]) == 1 && values.Get(key) == expected
}

func hasSingleNonEmptyQueryValue(values url.Values, key string) bool {
	return len(values[key]) == 1 && strings.TrimSpace(values.Get(key)) != ""
}

// activeRemoteLoginPath 返回当前 remote handoff 私有状态文件路径。
func activeRemoteLoginPath() (string, error) {
	return paths.UserDataFile(paths.AppDataDirName, activeHandoffFilename)
}

// saveActiveRemoteLogin 在私有锁保护下写入当前 remote handoff 状态。
func saveActiveRemoteLogin(session activeRemoteLogin) error {
	statePath, err := activeRemoteLoginPath()
	if err != nil {
		return err
	}
	return filelock.WithPrivateLock(context.Background(), statePath, func() error {
		return saveActiveRemoteLoginAt(statePath, session)
	})
}

func saveActiveRemoteLoginAt(statePath string, session activeRemoteLogin) error {
	body, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return secretfile.WritePrivateFile(statePath, body, paths.PrivateFileMode)
}

// loadActiveRemoteLogin 读取当前 remote handoff 状态；没有活动 handoff 时返回
// errNoActiveRemoteLogin。
func loadActiveRemoteLogin() (activeRemoteLogin, error) {
	statePath, err := activeRemoteLoginPath()
	if err != nil {
		return activeRemoteLogin{}, err
	}
	var session activeRemoteLogin
	err = filelock.WithPrivateLock(context.Background(), statePath, func() error {
		var loadErr error
		session, loadErr = loadActiveRemoteLoginAt(statePath)
		return loadErr
	})
	return session, err
}

func loadActiveRemoteLoginAt(statePath string) (activeRemoteLogin, error) {
	body, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return activeRemoteLogin{}, errNoActiveRemoteLogin
	}
	if err != nil {
		return activeRemoteLogin{}, errors.New("could not read active remote login handoff")
	}
	var session activeRemoteLogin
	if err := json.Unmarshal(body, &session); err != nil || session.Version != 1 || strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(session.Proof) == "" {
		return activeRemoteLogin{}, errors.New("active remote login handoff is invalid")
	}
	origin, err := canonicalRelayOrigin(session.Origin)
	if err != nil {
		return activeRemoteLogin{}, errors.New("active remote login handoff is invalid")
	}
	session.Origin = origin
	return session, nil
}

func clearActiveRemoteLoginIfMatches(expected activeRemoteLogin) error {
	statePath, err := activeRemoteLoginPath()
	if err != nil {
		return err
	}
	return filelock.WithPrivateLock(context.Background(), statePath, func() error {
		active, loadErr := loadActiveRemoteLoginAt(statePath)
		if errors.Is(loadErr, errNoActiveRemoteLogin) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if active.Version != expected.Version || active.Origin != expected.Origin || active.SessionID != expected.SessionID || active.Proof != expected.Proof {
			return nil
		}
		if removeErr := os.Remove(statePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		return nil
	})
}
