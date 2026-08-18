package post

import (
	"bytes"
	"errors"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatorsRejectsInvalidKindBeforeResolvingClient(t *testing.T) {
	serviceCalls := 0
	data := deps.Data{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
		ServiceFactory: func() (*fanboxapp.Facade, error) {
			serviceCalls++
			return nil, errors.New("service must not be resolved")
		},
	}
	root := &cobra.Command{Use: "fanbox", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(Commands(data)...)
	root.SetArgs([]string{"creators", "--kind", "invalid"})

	err := root.Execute()

	require.EqualError(t, err, "kind must be one of: supporting, following")
	assert.Zero(t, serviceCalls)
}
