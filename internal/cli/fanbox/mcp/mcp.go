// Package mcp owns the FANBOX stdio command route. It parses only the proxy
// flags and delegates server construction to the composition root; stdio
// lifecycle belongs to internal/mcpserver/stdio.
package mcp

import (
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/spf13/cobra"
)

// New builds the `pixiv fanbox mcp` route without forwarding through a retired
// controller. The composition root owns mcpserver wiring via RunMCPServer.
func New(data deps.Data) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the FANBOX MCP stdio server",
		Example: "pixiv fanbox mcp",
		Args:    data.RequireExactArgs(0, "pixiv fanbox mcp"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := data.FanboxService()
			if err != nil {
				return err
			}
			if service == nil {
				return errors.New("fanbox is not available: cannot open the local account store")
			}
			proxy, err := proxyOverride(cmd)
			if err != nil {
				return err
			}
			return data.RunMCPServer(cmd, service, proxy)
		},
	}
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.FanboxMCP())
	return cmd
}

func proxyOverride(cmd *cobra.Command) (*string, error) {
	if cmd == nil {
		return nil, nil
	}
	proxyFlag := cmd.Flags().Lookup("proxy")
	noProxyFlag := cmd.Flags().Lookup("no-proxy")
	proxyChanged := proxyFlag != nil && proxyFlag.Changed
	noProxyChanged := noProxyFlag != nil && noProxyFlag.Changed
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && noProxyFlag.Value.String() == "true" {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		value := proxyFlag.Value.String()
		return &value, nil
	}
	return nil, nil
}
