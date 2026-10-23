package search

import (
	"context"
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// testDependencies keeps the old test-facing fields so assertions stay
// unchanged while production constructors take explicit arguments.
type testDependencies struct {
	Input         io.Reader
	Output        io.Writer
	ErrorOutput   io.Writer
	UsageError    func(error) error
	JSONOut       func(*bool) (bool, error)
	Pooled        func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	ReverseSearch func(context.Context, ReverseSearchRequest) (reversesearch.Response, error)
}

type Request = pixivcmd.Request

func (d testDependencies) env() commands.Env {
	return commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
}

func newTestCommand(d testDependencies) *cobra.Command {
	return New(d.env(), pixivcmd.Executor(d.Pooled), ReverseSearchPort(d.ReverseSearch))
}

func newTestNovelCommand(d testDependencies) *cobra.Command {
	return NewNovel(d.env(), pixivcmd.Executor(d.Pooled), ReverseSearchPort(d.ReverseSearch))
}
