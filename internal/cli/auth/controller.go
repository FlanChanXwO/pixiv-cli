// The auth controller and its group-wide runtime wiring.

package auth

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// controller owns the auth command tree and its execution environment.
type controller struct {
	env     command.Env
	support Support
}

func newController(env command.Env, support Support) controller {
	return controller{env: env, support: support}
}

type runtimeServices struct {
	Account     AccountService
	Login       pixivaccount.LoginService
	LoadRuntime func() (settings.RuntimeConfig, error)
}

// services opens the account and login backing for one auth leaf.
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
