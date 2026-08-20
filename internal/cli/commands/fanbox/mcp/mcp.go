// Package mcp owns the FANBOX stdio command route. It parses only the proxy
// flags and delegates server construction to the composition root; stdio
// lifecycle belongs to the parent mcpserver package.
package mcp

import (
	"errors"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
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
			// 先完成互斥 flag 校验，避免非法输入触发本地账号或服务初始化。
			proxy, err := deps.ProxyOverride(cmd)
			if err != nil {
				return err
			}
			service, err := data.FanboxService()
			if err != nil {
				return err
			}
			if service == nil {
				return errors.New("fanbox is not available: cannot open the local account store")
			}
			return data.RunMCPServer(cmd, service, proxy)
		},
	}
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.FanboxMCP())
	return cmd
}
