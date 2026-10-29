package account

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

// LoginRequest 保存 login 的本地请求值；连接策略由调用方转换为 public SDK
// 的 LoginOptions，业务服务不读取配置或创建 proxy transport。
type LoginRequest struct {
	Options pixiv.LoginOptions
}

// LoginStart 保存一次浏览器登录的不透明 SDK session 与授权 URL。
type LoginStart struct {
	session          *pixiv.LoginSession
	AuthorizationURL string
}

// LoginCompleteRequest 携带回调 URL 或授权码，以及保存成功后是否切换默认账号。
type LoginCompleteRequest struct {
	CallbackOrCode string
	UseAfterLogin  bool
	// PreserveDefault 保持现有默认账号；保存成功但读取摘要失败时，返回的身份仍指向已
	// 持久化的账号。
	PreserveDefault bool
}

// Start 开始一次浏览器登录并返回授权 URL；返回的 session 由 Complete 消费，失败时
// 不保留任何状态。
func (s LoginService) Start(request LoginRequest) (LoginStart, error) {
	session, err := pixiv.BeginLogin(request.Options)
	if err != nil {
		return LoginStart{}, err
	}
	return LoginStart{session: session, AuthorizationURL: session.AuthorizationURL()}, nil
}

// AcceptsCallbackURL 在不消耗 session 的前提下校验浏览器、loopback 与手工 URL；它把
// state/verifier 保持在 public SDK 的不透明 LoginSession 内，调用方只能得到布尔结果。
func (s LoginStart) AcceptsCallbackURL(rawURL string) bool {
	return s.session != nil && s.session.AcceptsCallbackURL(rawURL)
}

// Complete 用回调 URL 或授权码兑换凭据并保存账号，随后释放该登录 session 的 HTTP
// 传输。PreserveDefault 下保存已成功但读取摘要失败时，错误与已持久化的身份一起返回；
// 其他失败不返回账号。
func (s LoginService) Complete(ctx context.Context, start LoginStart, request LoginCompleteRequest) (AccountSummary, error) {
	if start.session == nil {
		return AccountSummary{}, errors.New("login session is not initialized")
	}
	// 登录只执行一次；这里只释放该 SDK session 自己创建的 HTTP 传输。
	defer start.session.CloseIdleConnections()
	credentials, err := start.session.Complete(ctx, request.CallbackOrCode)
	if err != nil {
		return AccountSummary{}, err
	}
	return s.Pixiv.completeLogin(ctx, credentials, request.UseAfterLogin, request.PreserveDefault)
}
