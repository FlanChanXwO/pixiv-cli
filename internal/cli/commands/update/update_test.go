package update

import (
	"bytes"
	"io"
	"testing"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	updateapp "github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHost struct {
	out *bytes.Buffer
	err *bytes.Buffer
}

func (h testHost) Output() io.Writer      { return h.out }
func (h testHost) ErrorOutput() io.Writer { return h.err }
func (h testHost) PrintJSON(any) error    { return nil }
func (h testHost) RequireExactArgs(count int, usage string) cobra.PositionalArgs {
	return cobra.ExactArgs(count)
}
func (h testHost) LoadUpdateRuntimeConfig() (configapp.RuntimeConfig, error) {
	return configapp.RuntimeConfig{}, nil
}
func (h testHost) NewUpdateCoordinator(string, io.Writer, io.Writer) (*updateapp.UpdateCoordinator, error) {
	return nil, nil
}

func TestNewCommandPreservesUpdateFlags(t *testing.T) {
	cmd := NewCommand(testHost{out: &bytes.Buffer{}, err: &bytes.Buffer{}})

	require.Equal(t, "update", cmd.Name())
	for _, name := range []string{"check", "prerelease", "json", "proxy"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), name)
	}
}
