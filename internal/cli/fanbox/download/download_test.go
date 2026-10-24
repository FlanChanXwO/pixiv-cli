package download

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

func TestCommandRejectsMissingSourceBeforeResolvingServices(t *testing.T) {
	serviceCalls := 0
	cmd := newTestCommand(testData{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
		Service: func() (*session.Fanbox, error) {
			serviceCalls++
			return nil, errors.New("service must not be resolved")
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)

	err := cmd.Execute()

	require.EqualError(t, err, "usage: pixiv fanbox download SOURCE...")
	assert.Zero(t, serviceCalls)
}

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
