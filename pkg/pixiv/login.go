package pixiv

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

// LoginSession 是同一进程、同一 Client 绑定的一次性 PKCE 会话。其字段均不导出，
// 因而 JSON、fmt 和错误链不会暴露 verifier 或 state。
type LoginSession struct {
	owner            *Client
	authorizationURL string
	verifier         string
	state            string
	used             atomic.Bool
}

// AuthorizationURL 返回调用方应自行打开或交给浏览器的 Pixiv 授权地址。
func (s *LoginSession) AuthorizationURL() string {
	if s == nil {
		return ""
	}
	return s.authorizationURL
}

// StartLogin 创建一次性的程序化登录会话；pkg 不会启动浏览器、loopback server 或 TTY。
func (c *Client) StartLogin() (*LoginSession, error) {
	verifier, challenge, err := oauth.GeneratePKCEPair()
	if err != nil {
		return nil, newError(CodeUpstreamUnavailable, OperationStartLogin, BackendOAuth, true, 0, 0, errors.New("cannot create oauth login session"))
	}
	state, err := oauth.RandomURLToken(32)
	if err != nil {
		return nil, newError(CodeUpstreamUnavailable, OperationStartLogin, BackendOAuth, true, 0, 0, errors.New("cannot create oauth login session"))
	}
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("state", state)
	return &LoginSession{
		owner:            c,
		authorizationURL: strings.TrimRight(c.loginBaseURL(), "/") + "/web/v1/login?" + values.Encode(),
		verifier:         verifier,
		state:            state,
	}, nil
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
	return appapi.DefaultAPIBase
}

// CompleteLogin 校验 callback/state，交换 authorization code 并安全保存账号。
// callbackOrCode 可为裸 code 或回调 URL；任意非官方 callback URL 必须带匹配 state。
func (c *Client) CompleteLogin(ctx context.Context, session *LoginSession, callbackOrCode string, options LoginOptions) (*Account, error) {
	if session == nil || session.owner != c {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login session is not owned by this client"))
	}
	code, err := loginCode(callbackOrCode, session.state)
	if err != nil {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login callback is invalid"))
	}
	if !session.used.CompareAndSwap(false, true) {
		return nil, newError(CodeInvalidArgument, OperationCompleteLogin, "", false, 0, 0, errors.New("login session was already used"))
	}
	state, httpClient, err := c.accountState(OperationCompleteLogin, true)
	if err != nil {
		return nil, err
	}
	oauthClient := oauth.New("", oauth.WithHTTPClient(httpClient), oauth.WithBaseURL(c.oauthBase()))
	token, err := oauthClient.ExchangeAuthorizationCode(ctx, code, session.verifier)
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
	return !(strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Host, "app-api.pixiv.net") && parsed.Path == "/web/v1/users/auth/pixiv/callback")
}
