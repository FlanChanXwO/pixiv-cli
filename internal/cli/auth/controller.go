package auth

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
)

// controller 保存 auth 命令树共用的执行环境和延迟打开账号、登录服务的端口。
type controller struct {
	env     environment.Env
	support Support
}

func newController(env environment.Env, support Support) controller {
	return controller{env: env, support: support}
}

// accountService 给单个 auth 叶子打开账号适配层；构造命令树时不调用，避免提前打开数据库。
// 运行时配置只经 Support.LoadRuntime 注入，适配层不另持有来源。
func (a controller) accountService() (AccountService, error) {
	service, err := a.support.Account()
	if err != nil {
		return AccountService{}, err
	}
	return AccountService{Pixiv: service, LoadRuntime: a.support.LoadRuntime}, nil
}

// loginService 打开浏览器登录使用的账号登录服务。
func (a controller) loginService() (account.LoginService, error) {
	return a.support.Login()
}
