package update

import (
	"testing"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestAutomaticProxyRejectsConflictingFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("proxy", "", "")
	cmd.Flags().Bool("no-proxy", false, "")
	require.NoError(t, cmd.Flags().Set("proxy", "http://proxy.invalid"))
	require.NoError(t, cmd.Flags().Set("no-proxy", "true"))

	_, err := automaticProxy(cmd, configapp.RuntimeConfig{})

	require.EqualError(t, err, "use either --proxy or --no-proxy, not both")
}
