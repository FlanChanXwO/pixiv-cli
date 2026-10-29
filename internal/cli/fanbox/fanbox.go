// Package fanbox 提供 `pixiv fanbox` 命令组、组的持久 --proxy flag 与子命令共用的客户端租约包装。
//
// 子命令同时导入 sdk/fanbox，导入本包时使用 fanboxcli 别名。
package fanbox

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

const (
	proxyFlag   = "proxy"
	noProxyFlag = "no-proxy"
)

// NewCommand 组装 FANBOX 产品组，只固定组的帮助文本、持久 flag 和子命令顺序。
func NewCommand(env environment.Env, children []*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fanbox",
		Short: "Browse and download Pixiv FANBOX content",
		Args:  env.ExactArgs(0, "pixiv fanbox <command>"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	flags := cmd.PersistentFlags()
	flags.String(proxyFlag, "", "native FANBOX proxy URL (HTTP or HTTPS CONNECT)")
	flags.Bool(noProxyFlag, false, "use a direct native FANBOX connection for this command")
	cmd.AddCommand(children...)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Execution{})
	return cmd
}

// ProxyOverride 读取组的持久 --proxy/--no-proxy；两者都给时报错，nil 表示沿用配置。
func ProxyOverride(cmd *cobra.Command) (*string, error) {
	proxy, _ := cmd.Flags().GetString(proxyFlag)
	noProxy, _ := cmd.Flags().GetBool(noProxyFlag)
	return environment.ProxyOverride(cmd, environment.ProxyOptions{Proxy: proxy, NoProxy: noProxy})
}

// Open 为当前命令打开一个 FANBOX 客户端租约；session 延迟解析，构造命令树时不打开账号库。
func Open(ctx context.Context, open func() (*session.Fanbox, error), cmd *cobra.Command) (*lifecycle.Lease[*fanbox.Client], error) {
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return nil, err
	}
	return session.Open(ctx, open, session.OpenRequest{ProxyOverride: proxyOverride})
}

// UseClient 让租约只覆盖一次命令回调；回调和关闭的错误都会保留并合并返回。
func UseClient(cmd *cobra.Command, open func() (*session.Fanbox, error), use func(*fanbox.Client) error) error {
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return err
	}
	return session.Use(cmd.Context(), open, session.OpenRequest{ProxyOverride: proxyOverride}, use)
}
