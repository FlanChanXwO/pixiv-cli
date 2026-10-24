package post

import (
	"bytes"
	"errors"
	"io"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatorsRejectsInvalidKindBeforeResolvingClient(t *testing.T) {
	serviceCalls := 0
	data := testData{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
		Service: func() (*session.Fanbox, error) {
			serviceCalls++
			return nil, errors.New("service must not be resolved")
		},
	}
	root := &cobra.Command{Use: "fanbox", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newTestCommands(data)...)
	root.SetArgs([]string{"creators", "--kind", "invalid"})

	err := root.Execute()

	require.EqualError(t, err, "kind must be one of: supporting, following")
	assert.Zero(t, serviceCalls)
}

type testData struct {
	Reader    io.Reader
	Writer    io.Writer
	WrapUsage func(error) error
	Service   func() (*session.Fanbox, error)
}

func newTestCommands(d testData) []*cobra.Command {
	env := commands.Env{In: d.Reader, Out: d.Writer, Err: io.Discard, WrapUsage: d.WrapUsage}
	return Commands(env, d.Service)
}
