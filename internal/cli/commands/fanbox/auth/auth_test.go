package auth

import (
	"bytes"
	"errors"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPreservesAuthCommandSurface(t *testing.T) {
	cmd := New(deps.Data{
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
			data := deps.Data{
				Reader:    bytes.NewReader(nil),
				Writer:    &bytes.Buffer{},
				WrapUsage: func(err error) error { return err },
				AccountServiceFactory: func() (*fanboxapp.Service, error) {
					opened = true
					return nil, errors.New("account store must not be opened")
				},
			}
			root := deps.New(data, deps.CommandSet{Auth: New(data)})
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
