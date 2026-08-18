package mcp

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHost struct{}

func (testHost) RequireExactArgs(count int, usage string) cobra.PositionalArgs {
	return cobra.ExactArgs(count)
}
func (testHost) BindProxyFlags(cmd *cobra.Command, options *ProxyOptions) {
	cmd.Flags().StringVar(&options.Proxy, "proxy", "", "")
	cmd.Flags().BoolVar(&options.NoProxy, "no-proxy", false, "")
}
func (testHost) ClientRequest(*cobra.Command, ProxyOptions) (Request, error) {
	return Request{}, nil
}
func (testHost) RunMCP(context.Context, Request) error { return nil }

func TestNewCommandPreservesMCPSurface(t *testing.T) {
	cmd := NewCommand(testHost{})

	require.Equal(t, "mcp", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("proxy"))
	assert.NotNil(t, cmd.Flags().Lookup("no-proxy"))
}
