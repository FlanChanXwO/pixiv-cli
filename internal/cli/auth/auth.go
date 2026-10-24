// Package auth 注册 Pixiv auth 命令；具体 OAuth/login helper 也归本目录所有。
package auth

import (
	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/spf13/cobra"
)

// New 构造完整 Pixiv auth 命令组；账号、登录、bundle 和交互都保留在该 owner。
func New(env commands.Env, support Support) *cobra.Command {
	return newController(env, support).newAccountCommand()
}

// Support is the account and login backing shared by the auth leaves. Shared
// command support comes from commands.Env; prompts are methods on Env.
type Support struct {
	Account     func() (AccountService, error)
	Login       func() (pixivaccount.LoginService, error)
	LoadRuntime func() (config.RuntimeConfig, error)
	WriteBundle func(string, []byte, bool) error
}
