package download

import (
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/spf13/cobra"
)

type testData struct {
	Reader    io.Reader
	Writer    io.Writer
	WrapUsage func(error) error
	Service   func() (*session.Fanbox, error)
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Reader, Out: d.Writer, Err: io.Discard, WrapUsage: d.WrapUsage}
	return New(env, d.Service, func() (string, error) { return "", nil })
}
