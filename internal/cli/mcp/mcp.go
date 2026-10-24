// Package mcp 注册 Pixiv MCP 命令。
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	mcpauth "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
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
	var hostAddr, baseURL string
	var port int
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run unified Pixiv/FANBOX MCP over stdio (default) or HTTP (--port)",
		Example: "pixiv mcp\npixiv mcp --port 1270\npixiv mcp --port 1270 --base-url https://mcp.example.com",
		Args:    host.RequireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := host.ClientRequest(cmd, options)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("port") {
				if cmd.Flags().Changed("host") || cmd.Flags().Changed("base-url") {
					return errors.New("--host and --base-url require --port")
				}
			} else {
				listen, base, err := endpoint(hostAddr, port, baseURL, cmd.Flags().Changed("base-url"))
				if err != nil {
					return err
				}
				request.ListenAddr, request.BaseURL = &listen, &base
			}
			return host.RunMCP(cmd.Context(), request)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "Enable HTTP on this port (1-65535); omitted uses stdio")
	cmd.Flags().StringVar(&hostAddr, "host", "127.0.0.1", "HTTP listen host; requires --port")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Canonical public URL; requires --port and HTTPS outside loopback")

	host.BindProxyFlags(cmd, &options)
	input.Bind(cmd, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0})
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
	input.Bind(init, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0})
	init.Flags().BoolVar(&reset, "reset", false, "Rotate the owner secret and revoke all OAuth grants; keep clients and selected account")
	auth.AddCommand(init)
	cmd.AddCommand(auth)
	return cmd
}

// endpoint derives the public origin only from explicit startup values, never request headers.
func endpoint(host string, port int, rawBase string, explicitBase bool) (string, string, error) {
	if port < 1 || port > 65535 {
		return "", "", errors.New("--port must be an integer from 1 to 65535")
	}
	if host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, "/?#@[]\\") || (net.ParseIP(host) == nil && strings.Contains(host, ":")) {
		return "", "", errors.New("--host must be a hostname or unbracketed IP address")
	}
	listen := net.JoinHostPort(host, strconv.Itoa(port))
	if _, err := url.Parse("http://" + listen); err != nil {
		return "", "", errors.New("invalid --host")
	}
	if !explicitBase {
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", "", errors.New("non-loopback --host requires --base-url")
		}
		rawBase = "http://" + listen
	}
	base, err := mcpauth.ParseBaseURL(rawBase)
	if err != nil {
		return "", "", err
	}
	base.Scheme = strings.ToLower(base.Scheme)
	base.Host = strings.ToLower(base.Host)
	ip := net.ParseIP(base.Hostname())
	if base.Scheme != "https" && base.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", "", errors.New("non-loopback --base-url requires HTTPS")
	}
	return listen, base.String(), nil
}
