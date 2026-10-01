// Package mcp 注册 Pixiv MCP 命令。
package mcp

import (
	"context"
	"fmt"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/spf13/cobra"
)

// Request 是 mcp 命令解析 flags 后的本地请求值；传输覆写由 root host 桥接。
type Request struct {
	HTTPSProxyOverride  *string
	ListenAddr, BaseURL *string
}

type Host interface {
	RequireExactArgs(int, string) cobra.PositionalArgs
	BindProxyFlags(*cobra.Command, *ProxyOptions)
	ClientRequest(*cobra.Command, ProxyOptions) (Request, error)
	RunMCP(context.Context, Request) error
	InitMCPAuth(context.Context, bool) (string, error)
}

type ProxyOptions struct {
	Proxy   string
	NoProxy bool
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 构造 Pixiv MCP 入口；runtime 与 HTTP lifecycle 由 root host 注入。
func NewCommand(host Host) *cobra.Command {
	var options ProxyOptions
	var listenAddr, baseURL string
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the unified Pixiv/FANBOX MCP Streamable HTTP server",
		Example: "pixiv mcp --listen-addr 127.0.0.1:8080 --base-url http://127.0.0.1:8080",
		Args:    host.RequireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := host.ClientRequest(cmd, options)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("listen-addr") {
				request.ListenAddr = &listenAddr
			}
			if cmd.Flags().Changed("base-url") {
				request.BaseURL = &baseURL
			}
			return host.RunMCP(cmd.Context(), request)
		},
	}
	cmd.Flags().StringVar(&listenAddr, "listen-addr", "", "HTTP listen address (overrides mcp.listen_addr)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Canonical HTTP(S) base URL (overrides mcp.base_url)")
	host.BindProxyFlags(cmd, &options)
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0})
	requirements.Bind(cmd, requirements.PixivMCP())
	auth := &cobra.Command{Use: "auth", Short: "Manage the MCP owner"}
	var reset bool
	init := &cobra.Command{
		Use: "init", Short: "Initialize the MCP owner and print its secret once",
		Args: host.RequireExactArgs(0, "pixiv mcp auth init"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret, err := host.InitMCPAuth(cmd.Context(), reset)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), secret)
			if err != nil {
				return fmt.Errorf("MCP owner saved but secret output failed; run pixiv mcp auth init --reset to recover: %w", err)
			}
			return nil
		},
	}
	// 本地 owner 管理不能继承 server 的启动、账号和日志副作用。
	requirements.Bind(auth, requirements.Execution{})
	pipeline.Bind(init, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0})
	init.Flags().BoolVar(&reset, "reset", false, "Rotate the owner secret and revoke all OAuth grants; keep clients and selected account")
	auth.AddCommand(init)
	cmd.AddCommand(auth)
	return cmd
}
