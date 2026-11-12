package account

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// LoginService 将浏览器交互同 SDK 的一次性 LoginSession 连接起来。应用层不再
// 自行生成 PKCE、直连 OAuth 或写入 auth store。
type LoginService struct {
	Pixiv *Store
}

// LoginRequest 保存 login 的本地请求值；连接策略由调用方转换为 SDK
// 的 Options，业务服务不读取配置或创建 proxy transport。
type LoginRequest struct {
	Options pixiv.Options
}

// LoginStart 保存一次浏览器登录的不透明 SDK session 与授权 URL。
type LoginStart struct {
	session          *pixiv.LoginSession
	AuthorizationURL string
}

// LoginCompleteRequest 携带本次登录的回调 URL 或授权码。
type LoginCompleteRequest struct {
	CallbackOrCode string
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
// state/verifier 保持在 SDK 的不透明 LoginSession 内，调用方只能得到布尔结果。
func (s LoginStart) AcceptsCallbackURL(rawURL string) bool {
	return s.session != nil && s.session.AcceptsCallbackURL(rawURL)
}

// Complete 兑换凭据并持久化；保存后读取失败时仍返回已保存的身份，供调用方报告部分成功。
func (s LoginService) Complete(ctx context.Context, start LoginStart, request LoginCompleteRequest) (Pixiv, error) {
	if start.session == nil {
		return Pixiv{}, errors.New("login session is not initialized")
	}
	// 登录只执行一次；这里只释放该 SDK session 自己创建的 HTTP 传输。
	defer start.session.CloseIdleConnections()
	credentials, err := start.session.Complete(ctx, request.CallbackOrCode)
	if err != nil {
		return Pixiv{}, err
	}
	return s.Pixiv.savePixivLogin(ctx, credentials)
}
