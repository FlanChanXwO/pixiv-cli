package auth

import (
	"io"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/spf13/cobra"
)

type testData struct {
	Reader    io.Reader
	Writer    io.Writer
	WrapUsage func(error) error
	Accounts  func() (*fanboxaccount.Service, error)
	Browser   fanboxcmd.BrowserProvider
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Reader, Out: d.Writer, Err: io.Discard, WrapUsage: d.WrapUsage}
	return New(env, d.Accounts, d.Browser)
}
