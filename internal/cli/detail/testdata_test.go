package detail

import (
	"context"
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// testDependencies preserves the previous test fields so existing assertions
// stay unchanged while production takes explicit arguments.
type testDependencies struct {
	Input             io.Reader
	Output            io.Writer
	ErrorOutput       io.Writer
	UsageError        func(error) error
	JSONOut           func(*bool) (bool, error)
	Pooled            func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	BuildRequest      func(*cobra.Command, Options) (pixivcmd.Request, error)
	FetchArtwork      func(context.Context, *pixiv.Client, int64) (pixiv.Artwork, error)
	FetchNovel        func(context.Context, *pixiv.Client, int64) (pixiv.Novel, error)
	FetchNovelContent func(context.Context, *pixiv.Client, int64) (pixiv.NovelContent, error)
	FetchUser         func(context.Context, *pixiv.Client, int64) (pixiv.UserDetail, error)
	OutputIsTTY       func() bool
}

type Request = pixivcmd.Request

func newTestCommand(d testDependencies) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return newCommand(env, pixivcmd.Executor(d.Pooled), fetchers{
		artwork:      d.FetchArtwork,
		novel:        d.FetchNovel,
		novelContent: d.FetchNovelContent,
		user:         d.FetchUser,
	}, d.BuildRequest, d.OutputIsTTY)
}
