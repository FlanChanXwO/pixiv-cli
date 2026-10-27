// Package fanboxcmd 提供 FANBOX 子命令共用的 flag、客户端租约和分页执行。
//
// 包的组成：[UseClient] 把客户端租约限制在一次命令回调内，[Run] 与 [Traverse] 执行列表读取。
// [Options] 携带列表 flag；逻辑分页与 JSON 暂存复用
// [github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command]。
package fanboxcmd

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

// ProxyOverride 读取 FANBOX 组的持久 --proxy/--no-proxy；两者都给时报错。
func ProxyOverride(cmd *cobra.Command) (*string, error) {
	proxy, _ := cmd.Flags().GetString("proxy")
	noProxy, _ := cmd.Flags().GetBool("no-proxy")
	return command.ProxyOverride(cmd, command.ProxyOptions{Proxy: proxy, NoProxy: noProxy})
}

// Open 为当前命令打开一个 FANBOX 客户端租约；session 延迟解析，构造命令树时不打开账号库。
func Open(ctx context.Context, open func() (*session.Fanbox, error), cmd *cobra.Command) (*lifecycle.Lease[*fanbox.Client], error) {
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return nil, err
	}
	service, err := open()
	if err != nil {
		return nil, err
	}
	return service.Open(ctx, session.OpenRequest{ProxyOverride: proxyOverride})
}

// UseClient 让租约只覆盖一次命令回调；回调和关闭的错误都会保留并合并返回。
func UseClient(cmd *cobra.Command, open func() (*session.Fanbox, error), use func(*fanbox.Client) error) error {
	lease, err := Open(cmd.Context(), open, cmd)
	if err != nil {
		return err
	}
	useErr := use(lease.Value())
	return errors.Join(useErr, lease.Close())
}
