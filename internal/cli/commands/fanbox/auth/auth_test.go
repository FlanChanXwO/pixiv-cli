package auth

import (
	"bytes"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
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

func commandNames(cmd interface{ Commands() []*cobra.Command }) []string {
	commands := cmd.Commands()
	names := make([]string, 0, len(commands))
	for _, child := range commands {
		names = append(names, child.Name())
	}
	return names
}
