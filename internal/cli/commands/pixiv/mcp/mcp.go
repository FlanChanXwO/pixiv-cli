// Package mcp 注册 Pixiv MCP 命令。
package mcp

import (
	"context"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/spf13/cobra"
)

// Request 是 mcp 命令解析 flags 后的本地请求值；传输覆写由 root host 桥接。
type Request struct {
	HTTPSProxyOverride *string
}

type Host interface {
	RequireExactArgs(int, string) cobra.PositionalArgs
	BindProxyFlags(*cobra.Command, *ProxyOptions)
	ClientRequest(*cobra.Command, ProxyOptions) (Request, error)
	RunMCP(context.Context, Request) error
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
			return host.RunMCP(cmd.Context(), request)
		},
	}
	host.BindProxyFlags(cmd, &options)
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0})
	requirements.Bind(cmd, requirements.PixivMCP())
	return cmd
}
