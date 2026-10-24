// The auth controller and its group-wide runtime wiring.

package auth

import (
	"context"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/pkg/browser"
)

// controller owns the auth command tree and the system boundaries its leaves
// use. newController fills the production dependencies; same-package tests
// replace individual fields.
type controller struct {
	env     command.Env
	support Support
	in      io.Reader
	out     io.Writer
	errOut  io.Writer

	openBrowser       func(string) error
	installURLHandler func(context.Context, string) (func(), error)
	ensureURLHandler  func(context.Context) error
	clearHandoff      func(loginhelper.RemoteLoginStart) error
}

func newController(env command.Env, support Support) controller {
	return controller{
		env:               env,
		support:           support,
		in:                env.In,
		out:               env.Out,
		errOut:            env.Err,
		openBrowser:       browser.OpenURL,
		installURLHandler: loginhelper.Install,
		ensureURLHandler:  loginhelper.EnsurePersistentIfNeeded,
		clearHandoff:      loginhelper.ClearRemoteLoginHandoff,
	}
}

type runtimeServices struct {
	Account     AccountService
	Login       pixivaccount.LoginService
	LoadRuntime func() (settings.RuntimeConfig, error)
	err         error
}
