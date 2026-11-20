// Package mcp 提供 pixiv mcp 命令，启动统一的 Pixiv/FANBOX MCP server。
//
// 包的组成：[New] 组装命令树，[Request] 记录监听地址与代理覆盖。
// stdio 与 HTTP runtime 由 [github.com/FlanChanXwO/pixiv-cli/internal/mcpserver] 实现。
package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/spf13/cobra"
)

// Request 保存 mcp 命令解析 flags 后的本地请求值。
type Request struct {
	HTTPSProxyOverride  *string
	ListenAddr, BaseURL *string
}

// New 构造 Pixiv MCP 入口；run 启动 stdio 或 HTTP runtime，initAuth 初始化 HTTP owner。
func New(env environment.Env, run func(context.Context, Request) error, initAuth func(context.Context, bool) (string, error)) *cobra.Command {
	var options environment.ProxyOptions
	var hostAddr, baseURL string
	var port int
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "help.run_unified_pixiv_fanbox_mcp_over_stdio_default",
		Example: "pixiv mcp\npixiv mcp --port 1270\npixiv mcp --port 1270 --base-url https://mcp.example.com",
		Args:    env.ExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			proxyOverride, err := environment.ProxyOverride(cmd, options)
			if err != nil {
				return err
			}
			request := Request{HTTPSProxyOverride: proxyOverride}
			if !cmd.Flags().Changed("port") {
				if cmd.Flags().Changed("host") || cmd.Flags().Changed("base-url") {
					return i18n.Errorf("error.host_and_base_url_require_port")
				}
			} else {
				listen, base, err := endpoint(hostAddr, port, baseURL, cmd.Flags().Changed("base-url"))
				if err != nil {
					return err
				}
				request.ListenAddr, request.BaseURL = &listen, &base
			}
			return run(cmd.Context(), request)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "flag.enable_http_on_this_port_1_65535_omitted")
	cmd.Flags().StringVar(&hostAddr, "host", "127.0.0.1", "flag.http_listen_host_requires_port")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "flag.canonical_public_url_requires_port_and_https_outside")

	env.BindProxyFlags(cmd, &options)
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0})
	environment.Bind(cmd, environment.PixivMCP())
	auth := &cobra.Command{Use: "auth", Short: "help.manage_the_mcp_owner"}
	var reset bool
	init := &cobra.Command{
		Use: "init", Short: "help.initialize_the_mcp_owner_and_print_its_secret",
		Args: env.ExactArgs(0, "pixiv mcp auth init"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret, err := initAuth(cmd.Context(), reset)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), secret)
			if err != nil {
				return i18n.Errorf("error.mcp_owner_saved_but_secret_output_failed_run", err)
			}
			return nil
		},
	}
	// 本地 owner 管理不能继承 server 的启动、账号和日志副作用。
	environment.Bind(auth, environment.Execution{})
	environment.BindInput(init, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0})
	init.Flags().BoolVar(&reset, "reset", false, "flag.rotate_the_owner_secret_and_revoke_all_oauth")
	auth.AddCommand(init)
	cmd.AddCommand(auth)
	return cmd
}

// endpoint 只从显式启动参数推导公开 origin，避免 Host 等请求头影响回调地址。
func endpoint(host string, port int, rawBase string, explicitBase bool) (string, string, error) {
	if port < 1 || port > 65535 {
		return "", "", i18n.Errorf("error.port_must_be_an_integer_from_1_to")
	}
	if host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, "/?#@[]\\") || (net.ParseIP(host) == nil && strings.Contains(host, ":")) {
		return "", "", i18n.Errorf("error.host_must_be_a_hostname_or_unbracketed_ip")
	}
	listen := net.JoinHostPort(host, strconv.Itoa(port))
	if _, err := url.Parse("http://" + listen); err != nil {
		return "", "", i18n.Errorf("error.invalid_host")
	}
	if !explicitBase {
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", "", i18n.Errorf("error.non_loopback_host_requires_base_url")
		}
		rawBase = "http://" + listen
	}
	base, err := auth.ParseBaseURL(rawBase)
	if err != nil {
		return "", "", err
	}
	base.Scheme = strings.ToLower(base.Scheme)
	base.Host = strings.ToLower(base.Host)
	ip := net.ParseIP(base.Hostname())
	if base.Scheme != "https" && base.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", "", i18n.Errorf("error.non_loopback_base_url_requires_https")
	}
	return listen, base.String(), nil
}
