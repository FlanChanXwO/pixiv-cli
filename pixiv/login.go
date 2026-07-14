package pixiv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

// LoginSession 是同一进程、同一 Client 绑定的一次性 PKCE 会话。复制这个
// 小 handle 仍共享同一个会话状态与 one-time gate。
type LoginSession struct {
	state *loginSessionState
}

type loginSessionState struct {
	owner            *Client
	authorizationURL string
	verifier         string
	state            string
	used             atomic.Bool
}

// AuthorizationURL 返回调用方应自行打开或交给浏览器的 Pixiv 授权地址。
func (s *LoginSession) AuthorizationURL() string {
	if s == nil || s.state == nil {
		return ""
	}
	return s.state.authorizationURL
}

// AcceptsCallbackURL 判断浏览器捕获的 URL 是否携带属于本会话的有效 authorization
// code。它不返回 code、state 或 verifier，也不标记会话已使用；实际交换仍必须由
// CompleteLogin 完成并再次校验。裸 code 不是 URL，不能由 watcher 通过本方法接受。
func (s *LoginSession) AcceptsCallbackURL(rawURL string) bool {
	if s == nil || s.state == nil {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return false
	}
	_, err = loginCode(rawURL, s.state.state)
	return err == nil
}

// BuildLoginAuthorizationURL 返回官方 App OAuth 登录地址。它供只负责浏览器
// 交互的 adapter 复用；PKCE verifier 和 state 的生成、校验及 code exchange
// 仍分别由调用方或 LoginSession 负责。
func BuildLoginAuthorizationURL(challenge, state string) string {
	return buildLoginAuthorizationURL(protocol.AppAPIBase, challenge, state)
}

// OAuthCallbackURLPrefix 返回官方 HTTPS callback 的可扫描 URL 前缀。浏览器
// adapter 可据此定位候选 URL，但仍必须使用 IsOfficialOAuthCallbackURL 校验。
func OAuthCallbackURLPrefix() string { return protocol.OAuthRedirectURI + "?" }

// IsOfficialOAuthCallbackURL 判断 URL 是否为 Pixiv App OAuth 的官方 HTTPS
// callback；它不解析或暴露 authorization code、state 等 query 内容。
func IsOfficialOAuthCallbackURL(rawURL string) bool {
	return isOfficialAppOAuthURL(rawURL, callbackPath())
}

// IsOfficialOAuthStartURL 判断 post-redirect 的 return_to 是否指向本次 App
// OAuth start 路由。调用方仍需独立校验它携带的 PKCE challenge。
func IsOfficialOAuthStartURL(rawURL string) bool {
	return isOfficialAppOAuthURL(rawURL, protocol.AppOAuthStart)
}

// String、GoString 和 Format 均不暴露 URL、state 或 PKCE verifier；授权 URL
// 只能通过 AuthorizationURL 这个显式能力取得。
func (LoginSession) String() string   { return "pixiv.LoginSession{}" }
func (LoginSession) GoString() string { return "pixiv.LoginSession{}" }
func (LoginSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "pixiv.LoginSession{}")
}

// StartLogin 创建一次性的程序化登录会话；pkg 不会启动浏览器、loopback server 或 TTY。
func (c *Client) StartLogin() (out *LoginSession, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationStartLogin, started, err, 0, 0) }()
	verifier, challenge, err := oauth.GeneratePKCEPair()
	if err != nil {
		return nil, newError(CodeUpstreamUnavailable, OperationStartLogin, BackendOAuth, true, 0, 0, errors.New("cannot create oauth login session"))
	}
	state, err := oauth.RandomURLToken(32)
	if err != nil {
		return nil, newError(CodeUpstreamUnavailable, OperationStartLogin, BackendOAuth, true, 0, 0, errors.New("cannot create oauth login session"))
	}
	return &LoginSession{state: &loginSessionState{
		owner:            c,
		authorizationURL: buildLoginAuthorizationURL(c.loginBaseURL(), challenge, state),
		verifier:         verifier,
		state:            state,
	}}, nil
}

func buildLoginAuthorizationURL(baseURL, challenge, state string) string {
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("state", state)
	return strings.TrimRight(baseURL, "/") + protocol.AppLogin + "?" + values.Encode()
}

func isOfficialAppOAuthURL(rawURL, expectedPath string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, callback.Scheme) && strings.EqualFold(parsed.Host, callback.Host) && parsed.Path == expectedPath
}

func callbackPath() string {
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return ""
	}
	return callback.Path
}

func (c *Client) loginBaseURL() string {
	if c.defaults != nil && strings.TrimSpace(c.defaults.options.AppAPIBaseURL) != "" {
		return c.defaults.options.AppAPIBaseURL
	}
	if c.defaults == nil && c.app != nil {
		// appapi base is private. Options are retained only by OpenDefault, so
		// direct custom AppAPIBaseURL is kept separately by NewClient below.
		return c.appAPIBaseURL
	}
	return protocol.AppAPIBase
}

// CompleteLogin 校验 callback/state，交换 authorization code 并安全保存账号。
// callbackOrCode 可为裸 code 或回调 URL；任意非官方 callback URL 必须带匹配 state。
func (c *Client) CompleteLogin(ctx context.Context, session *LoginSession, callbackOrCode string, options LoginOptions) (out *Account, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationCompleteLogin, started, err, 0, 0) }()
	if session == nil || session.state == nil || session.state.owner != c {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login session is not owned by this client"))
	}
	code, err := loginCode(callbackOrCode, session.state.state)
	if err != nil {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login callback is invalid"))
	}
	if !session.state.used.CompareAndSwap(false, true) {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login session was already used"))
	}
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	state, httpClient, err := c.accountState(OperationCompleteLogin, true)
	if err != nil {
		return nil, err
	}
	oauthClient := oauth.New("", oauth.WithHTTPClient(httpClient), oauth.WithBaseURL(c.oauthBase()))
	token, err := oauthClient.ExchangeAuthorizationCode(ctx, code, session.state.verifier)
	if err != nil {
		return nil, mapOAuthError(err, OperationCompleteLogin)
	}
	if token.UserID <= 0 || strings.TrimSpace(token.RefreshToken) == "" {
		return nil, newError(CodeMalformedUpstreamResponse, OperationCompleteLogin, BackendOAuth, false, 0, 0, errors.New("oauth response did not include account identity"))
	}
	account := auth.Account{UserID: token.UserID, Username: strings.TrimSpace(token.Username), RefreshToken: token.RefreshToken}
	state.store.Upsert(account)
	if options.UseAsDefault || state.store.DefaultUserID == 0 {
		state.store.DefaultUserID = account.UserID
	}
	if err := auth.SaveAuthStore(state.authPath, state.store); err != nil {
		return nil, localSnapshotError(OperationCompleteLogin, err)
	}
	result := publicAccount(account, state.store.DefaultUserID)
	return &result, nil
}

func loginCode(input, expectedState string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty input")
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" {
		// A caller that has already extracted code passes it directly. PKCE still
		// binds it to this one-time session; state validation applies to callbacks.
		if strings.ContainsAny(input, "?#&") {
			return "", errors.New("malformed callback")
		}
		return input, nil
	}
	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		return "", errors.New("missing code")
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if loginCallbackRequiresState(parsed) {
		if state == "" || state != expectedState {
			return "", errors.New("state mismatch")
		}
	} else if state != "" && state != expectedState {
		return "", errors.New("state mismatch")
	}
	return code, nil
}

func loginCallbackRequiresState(parsed *url.URL) bool {
	if parsed == nil {
		return true
	}
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return false
	}
	// 与 OAuth client 共用 protocol 中的官方 callback，避免登录校验和实际
	// authorization-code exchange 在上游地址调整时分叉。
	return !IsOfficialOAuthCallbackURL(parsed.String())
}
