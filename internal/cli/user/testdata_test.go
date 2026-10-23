package user

import (
	"context"
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type testData struct {
	Input      io.Reader
	Output     io.Writer
	UsageError func(error) error
	JSONOut    func(*bool) (bool, error)
	Pooled     func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	Follow     func() *cobra.Command
}

func newTestUserCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return New(env, pixivcmd.New(d.Output, pixivcmd.Executor(d.Pooled)), d.Follow)
}
