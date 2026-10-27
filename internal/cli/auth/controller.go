package auth

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// controller 保存 auth 命令树共用的执行环境和延迟打开的账号、登录服务。
type controller struct {
	env     command.Env
	support Support
}

func newController(env command.Env, support Support) controller {
	return controller{env: env, support: support}
}

type runtimeServices struct {
	Account AccountService
	Login   pixivaccount.LoginService
	// LoadRuntime 每次调用读取最新配置；登录与检查需要当前代理设置。
	LoadRuntime func() (settings.RuntimeConfig, error)
}

// services 为单个 auth 叶子打开账号与登录服务；构造命令树时不调用，避免提前打开数据库。
func (a controller) services() (runtimeServices, error) {
	account, err := a.support.Account()
	if err != nil {
		return runtimeServices{}, err
	}
	login, err := a.support.Login()
	if err != nil {
		return runtimeServices{}, err
	}
	return runtimeServices{Account: account, Login: login, LoadRuntime: a.support.LoadRuntime}, nil
}
