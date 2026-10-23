// The auth controller and its group-wide runtime wiring.

package auth

import (
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

type controller struct {
	env     commands.Env
	support Support
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
}

type runtimeServices struct {
	Account     AccountService
	Login       pixivaccount.LoginService
	LoadRuntime func() (config.RuntimeConfig, error)
	err         error
}
