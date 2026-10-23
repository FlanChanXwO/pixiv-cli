package download

import (
	"bytes"
	"errors"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRejectsMissingSourceBeforeResolvingServices(t *testing.T) {
	serviceCalls := 0
	cmd := New(deps.Data{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
		ServiceFactory: func() (*session.Fanbox, error) {
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
