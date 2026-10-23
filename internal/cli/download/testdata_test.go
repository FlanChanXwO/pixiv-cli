package download_test

import (
	"context"
	"io"

	download "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type testDownloadData struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	Runtime     func() (download.Runtime, error)
	Download    func() downloader.DownloadService
	Pooled      func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

func newTestDownloadCommand(d testDownloadData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError}
	return download.New(env, pixivcmd.Executor(d.Pooled), d.Runtime, d.Download)
}
