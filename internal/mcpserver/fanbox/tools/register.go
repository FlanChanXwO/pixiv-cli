// Package tools 实现 FANBOX 的 MCP 工具：注册、输入 schema、调用与安全输出。
//
// 包的组成：[Register] 一次注册全部 FANBOX 工具，[Account] 与 [SDKPorts] 承载组合根
// 注入的传输覆写和 client lease 端口。用户、帖子与资源三类工具分别在 users.go、
// posts.go、resources.go；结构化输出在 internal/outputs。
package tools

import (
	"context"
	"errors"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 保存每次运行时使用的 FANBOX 传输覆写。
type Account struct{ HTTPSProxyOverride *string }

// SDKPorts 提供 client lease 的打开函数与执行日志。OpenLease 成功返回的 lease 归调用方
// 所有，由调用方关闭。
type SDKPorts struct {
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*fanbox.Client], error)
	Logger    *slog.Logger
}

type fanboxTools struct {
	open    func(context.Context, Account) (*lifecycle.Lease[*fanbox.Client], error)
	account Account
	calls   *toolrun.Scope
}

func (t fanboxTools) openClient(ctx context.Context) (*lifecycle.Lease[*fanbox.Client], error) {
	if t.open == nil {
		return nil, errors.New("fanbox service is not configured")
	}
	return t.open(ctx, t.account)
}

// Register 注册全部 FANBOX 工具；缺少 OpenLease 时工具仍会注册，调用时返回明确错误。
func Register(server *mcp.Server, ports SDKPorts, account Account) {
	t := fanboxTools{open: ports.OpenLease, account: account, calls: toolrun.NewScope(ports.Logger)}
	t.registerCurrentUser(server)
	t.registerCreator(server)
	t.registerCreators(server)
	t.registerCreatorTags(server)
	t.registerCreatorPosts(server)
	t.registerPost(server)
	t.registerFeed(server)
	t.registerResolveURL(server)
	t.registerOpenResource(server)
}
