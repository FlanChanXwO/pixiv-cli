package pixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// LoginService 将浏览器交互同 public SDK 的一次性 LoginSession 连接起来。应用层不再
// 自行生成 PKCE、直连 OAuth 或写入 auth store。
type LoginService struct {
	Pixiv *Service
}

// LoginRequest 是 login 的本地请求值；连接策略由调用方转换为 public SDK
// 的 LoginOptions，业务服务不读取配置或创建 proxy transport。
type LoginRequest struct {
	Options pixiv.LoginOptions
}

type LoginStart struct {
	session          *pixiv.LoginSession
	AuthorizationURL string
}

type LoginCompleteRequest struct {
	CallbackOrCode string
	UseAfterLogin  bool
}

func (s LoginService) Start(requests ...LoginRequest) (LoginStart, error) {
	request := LoginRequest{}
	if len(requests) > 0 {
		request = requests[0]
	}
	session, err := pixiv.BeginLogin(request.Options)
	if err != nil {
		return LoginStart{}, err
	}
	return LoginStart{session: session, AuthorizationURL: session.AuthorizationURL()}, nil
}

// AcceptsCallbackURL 是浏览器、loopback 和手工 URL 输入的非消耗性校验入口。它把
// state/verifier 保持在 public SDK 的不透明 LoginSession 内，调用方只能得到布尔结果。
func (s LoginStart) AcceptsCallbackURL(rawURL string) bool {
	return s.session != nil && s.session.AcceptsCallbackURL(rawURL)
}

func (s LoginService) Complete(ctx context.Context, start LoginStart, request LoginCompleteRequest) (AccountSummary, error) {
	if start.session == nil {
		return AccountSummary{}, errors.New("login session is not initialized")
	}
	credentials, err := start.session.Complete(ctx, request.CallbackOrCode)
	if err != nil {
		return AccountSummary{}, err
	}
	service, err := s.pixivService()
	if err != nil {
		return AccountSummary{}, err
	}
	account, err := service.CompleteLogin(ctx, credentials, request.UseAfterLogin)
	if err != nil {
		return AccountSummary{}, err
	}
	return account, nil
}

func (s LoginService) pixivService() (*Service, error) {
	if s.Pixiv == nil {
		return nil, errors.New("pixiv account service is not configured")
	}
	return s.Pixiv, nil
}
