// Package mcp 注册 Pixiv MCP 命令。
package mcp

import (
	"context"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/spf13/cobra"
)

type Host interface {
	RequireExactArgs(int, string) cobra.PositionalArgs
	BindProxyFlags(*cobra.Command, *ProxyOptions)
	ClientRequest(*cobra.Command, ProxyOptions) (application.ClientRequest, error)
	RunMCP(context.Context, *string, *time.Duration) error
}

type ProxyOptions struct {
	Proxy   string
	NoProxy bool
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 构造 Pixiv MCP 入口；runtime 与 stdio lifecycle 由 root host 注入。
func NewCommand(host Host) *cobra.Command {
	var options ProxyOptions
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP stdio server",
		Example: "pixiv mcp",
		Args:    host.RequireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := host.ClientRequest(cmd, options)
			if err != nil {
				return err
			}
			return host.RunMCP(cmd.Context(), request.HTTPSProxyOverride, request.RequestIntervalOverride)
		},
	}
	host.BindProxyFlags(cmd, &options)
	return cmd
}
