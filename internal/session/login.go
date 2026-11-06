package session

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
)

// LoginService 将浏览器交互同 public SDK 的一次性 LoginSession 连接起来。应用层不再
// 自行生成 PKCE、直连 OAuth 或写入 auth store。
type LoginService struct {
	Pixiv *Accounts
}

// LoginCompleteRequest 将登录凭据保存与调用方的默认账号选择分开编排。
type LoginCompleteRequest struct {
	CallbackOrCode  string
	UseAfterLogin   bool
	PreserveDefault bool
}

// Start 开始账号 owner 的一次性登录，网络选项由调用方提供。
func (s LoginService) Start(request account.LoginRequest) (account.LoginStart, error) {
	return (account.LoginService{Pixiv: s.Pixiv.store}).Start(request)
}

// Complete 用回调 URL 或授权码兑换凭据并保存账号，随后释放该登录 session 的 HTTP
// 传输。PreserveDefault 下保存已成功但读取摘要失败时，错误与已持久化的身份一起返回；
// 其他失败不返回账号。
func (s LoginService) Complete(ctx context.Context, start account.LoginStart, request LoginCompleteRequest) (AccountSummary, error) {
	saved, err := (account.LoginService{Pixiv: s.Pixiv.store}).Complete(ctx, start, account.LoginCompleteRequest{CallbackOrCode: request.CallbackOrCode})
	if err != nil {
		return accountSummary(saved, false, false), err
	}
	return s.Pixiv.completeLogin(ctx, saved, request.UseAfterLogin, request.PreserveDefault)
}
