package application

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// LoginService 将浏览器交互同 public SDK 的一次性 LoginSession 连接起来。应用层不再
// 自行生成 PKCE、直连 OAuth 或写入 auth store。
type LoginService struct {
	SDK         SDKService
	LoadRuntime func() (config.RuntimeConfig, error)
}

type LoginStart struct {
	client           SDKClient
	session          *sdk.LoginSession
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
	client, err := s.SDK.Client(request)
	if err != nil {
		return LoginStart{}, err
	}
	session, err := client.StartLogin()
	if err != nil {
		return LoginStart{}, err
	}
	return LoginStart{client: client, session: session, AuthorizationURL: session.AuthorizationURL()}, nil
}

// AcceptsCallbackURL 是浏览器、loopback 和手工 URL 输入的非消耗性校验入口。它把
// state/verifier 保持在 public SDK 的不透明 LoginSession 内，调用方只能得到布尔结果。
func (s LoginStart) AcceptsCallbackURL(rawURL string) bool {
	return s.session != nil && s.session.AcceptsCallbackURL(rawURL)
}

func (s LoginService) Complete(ctx context.Context, start LoginStart, request LoginCompleteRequest) (AccountResult, error) {
	if start.client == nil || start.session == nil {
		return AccountResult{}, errors.New("login session is not initialized")
	}
	account, err := start.client.CompleteLogin(ctx, start.session, request.CallbackOrCode, sdk.LoginOptions{UseAsDefault: request.UseAfterLogin})
	if err != nil {
		return AccountResult{}, err
	}
	return sdkAccountResult(*account), nil
}
