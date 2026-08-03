package application

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// LoginService 将浏览器交互同 public SDK 的一次性 LoginSession 连接起来。应用层不再
// 自行生成 PKCE、直连 OAuth 或写入 auth store。
type LoginService struct {
	SDK         SDKService
	Pixiv       *pixivapp.Service
	LoadRuntime func() (config.RuntimeConfig, error)
}

type LoginStart struct {
	session          *pixiv.LoginSession
	AuthorizationURL string
}

type LoginCompleteRequest struct {
	CallbackOrCode string
	UseAfterLogin  bool
}

func (s LoginService) Start(requests ...SDKClientRequest) (LoginStart, error) {
	request := SDKClientRequest{}
	if len(requests) > 0 {
		request = requests[0]
	}
	options := pixiv.LoginOptions{}
	if request.HTTPSProxyOverride != nil {
		httpClient, err := loginProxyHTTPClient(*request.HTTPSProxyOverride)
		if err != nil {
			return LoginStart{}, err
		}
		options.HTTPClient = httpClient
	}
	session, err := pixiv.BeginLogin(options)
	if err != nil {
		return LoginStart{}, err
	}
	return LoginStart{session: session, AuthorizationURL: session.AuthorizationURL()}, nil
}

// AcceptsCallbackURL 是浏览器、loopback 和手工 URL 输入的非消耗性校验入口。它把
// state/verifier 保持在 public SDK 的不透明 LoginSession 内，调用方只能得到布尔结果。
func (s LoginStart) AcceptsCallbackURL(rawURL string) bool {
	if s.session == nil {
		return false
	}
	expectedState := loginStateFromURL(s.AuthorizationURL)
	return loginCallbackAccepts(rawURL, expectedState)
}

func (s LoginService) Complete(ctx context.Context, start LoginStart, request LoginCompleteRequest) (AccountResult, error) {
	if start.session == nil {
		return AccountResult{}, errors.New("login session is not initialized")
	}
	credentials, err := start.session.Complete(ctx, request.CallbackOrCode)
	if err != nil {
		return AccountResult{}, err
	}
	service, err := s.pixivService()
	if err != nil {
		return AccountResult{}, err
	}
	account, err := service.CompleteLogin(ctx, credentials, request.UseAfterLogin)
	if err != nil {
		return AccountResult{}, err
	}
	return accountResultFromPixiv(account), nil
}

func (s LoginService) pixivService() (*pixivapp.Service, error) {
	if s.Pixiv == nil {
		return nil, errors.New("pixiv account service is not configured")
	}
	return s.Pixiv, nil
}

func loginProxyHTTPClient(proxy string) (*http.Client, error) {
	return internalpixiv.HTTPClient(proxy)
}

func loginStateFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("state"))
}

// loginCallbackRequiresState 与 public SDK 的 loginCode 规则保持一致：官方 HTTPS
// callback 与 pixiv://account/login 不要求 state，但若携带 state 则必须匹配。
func loginCallbackRequiresState(parsed *url.URL) bool {
	if parsed == nil {
		return true
	}
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return false
	}
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return true
	}
	return !(strings.EqualFold(parsed.Scheme, callback.Scheme) && strings.EqualFold(parsed.Host, callback.Host) && parsed.Path == callback.Path)
}

// loginCallbackAccepts 非消耗性地校验一个回调 URL 是否属于本会话。
func loginCallbackAccepts(rawURL, expectedState string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return false
	}
	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		return false
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if loginCallbackRequiresState(parsed) {
		if state == "" || state != expectedState {
			return false
		}
	} else if state != "" && state != expectedState {
		return false
	}
	return true
}
