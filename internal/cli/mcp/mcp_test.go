package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHost struct{}

func (testHost) InitMCPAuth(context.Context, bool) (string, error) { return "fixture-owner", nil }

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

func TestNewCommandExposesExplicitHTTPEndpoint(t *testing.T) {
	cmd := NewCommand(testHost{})
	require.Nil(t, cmd.Flags().Lookup("listen-addr"))
	require.NotNil(t, cmd.Flags().Lookup("port"))
	require.NotNil(t, cmd.Flags().Lookup("host"))
	require.NotNil(t, cmd.Flags().Lookup("base-url"))
	require.Contains(t, cmd.Short, "HTTP")
	require.Contains(t, cmd.Short, "stdio")
}

func TestTransportFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--host", "0.0.0.0"}, {"--base-url", "https://mcp.example.com"},
		{"--port", "0"}, {"--port", "65536"}, {"--port", "-1"},
		{"--port", "9000", "--host", "0.0.0.0"},
		{"--port", "9000", "--base-url", "http://public.example.com"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			cmd := NewCommand(testHost{})
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
		})
	}
	for _, args := range [][]string{nil, {"--port", "1270"}, {"--port", "9000"},
		{"--port", "9000", "--host", "::1"},
		{"--port", "9000", "--host", "0.0.0.0", "--base-url", "https://mcp.example.com/prefix/"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			cmd := NewCommand(testHost{})
			cmd.SetArgs(args)
			require.NoError(t, cmd.Execute())
		})
	}
}
