package session

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
)

// LoginService 在账号 owner 完成一次性登录后，编排调用方的默认账号选择。
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
// 传输。保存后读取失败时，错误与已持久化的身份一起返回。
func (s LoginService) Complete(ctx context.Context, start account.LoginStart, request LoginCompleteRequest) (AccountSummary, error) {
	saved, err := (account.LoginService{Pixiv: s.Pixiv.store}).Complete(ctx, start, account.LoginCompleteRequest{CallbackOrCode: request.CallbackOrCode})
	if err != nil {
		return accountSummary(saved, false, false), err
	}
	return s.Pixiv.completeLogin(ctx, saved, request.UseAfterLogin, request.PreserveDefault)
}
