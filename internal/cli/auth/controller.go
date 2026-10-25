// The auth controller and its group-wide runtime wiring.

package auth

import (
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// controller owns the auth command tree and its execution environment.
type controller struct {
	env     command.Env
	support Support
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
}

func newController(env command.Env, support Support) controller {
	return controller{
		env:     env,
		support: support,
		in:      env.In,
		out:     env.Out,
		errOut:  env.Err,
	}
}

type runtimeServices struct {
	Account     AccountService
	Login       pixivaccount.LoginService
	LoadRuntime func() (settings.RuntimeConfig, error)
	err         error
}
