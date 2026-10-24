package auth

import (
	"bytes"
	"errors"
	"io"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPreservesAuthCommandSurface(t *testing.T) {
	cmd := newTestCommand(testData{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
	})

	require.Equal(t, "auth", cmd.Name())
	assert.Equal(t, []string{"import", "list", "remove", "status", "use"}, commandNames(cmd))
}

func TestAuthReadCommandsRejectProxyConflictBeforeOpeningStore(t *testing.T) {
	for _, name := range []string{"list", "status"} {
		t.Run(name, func(t *testing.T) {
			opened := false
			data := testData{
				Reader:    bytes.NewReader(nil),
				Writer:    &bytes.Buffer{},
				WrapUsage: func(err error) error { return err },
				Accounts: func() (*fanboxaccount.Service, error) {
					opened = true
					return nil, errors.New("account store must not be opened")
				},
			}
			root := &cobra.Command{Use: "fanbox"}
			root.PersistentFlags().String("proxy", "", "")
			root.PersistentFlags().Bool("no-proxy", false, "")
			root.AddCommand(newTestCommand(data))
			root.SetArgs([]string{"auth", name, "--proxy", "http://proxy.invalid", "--no-proxy"})

			err := root.Execute()

			require.EqualError(t, err, "use either --proxy or --no-proxy, not both")
			require.False(t, opened)
		})
	}
}

func commandNames(cmd interface{ Commands() []*cobra.Command }) []string {
	commands := cmd.Commands()
	names := make([]string, 0, len(commands))
	for _, child := range commands {
		names = append(names, child.Name())
	}
	return names
}

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
