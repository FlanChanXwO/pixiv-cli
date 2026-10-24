// Package fanbox 聚合 FANBOX MCP tool 注册。它只负责把 tool packages 注册到
// server；具体 input/output/schema/adapter 与业务逻辑归各 tool package；HTTP transport
// 与服务生命周期由父包提供。
package fanbox

import (
	"context"
	"errors"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account carries only per-request transport overrides into the MCP SDK port.
type Account struct {
	HTTPSProxyOverride *string
}

// SDKPorts is the narrow FANBOX capability injected by the composition root.
// The MCP server does not know how accounts are stored or rotated.
type SDKPorts struct {
	// Open is retained as a raw-client adapter for existing embeddings. New
	// composition roots should inject OpenLease.
	Open      func(context.Context, Account) (*fanboxsdk.Client, error)
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*fanboxsdk.Client], error)
	// Logger 是执行边界的日志入口；nil 时不输出。
	Logger *slog.Logger
}

// Register 将 FANBOX tools 注册到共享 server；proxyOverride 只作用于 FANBOX 原生连接。
func Register(server *mcp.Server, ports SDKPorts, proxyOverride *string) {
	runtimePorts := toolhost.SDKPorts{Logger: ports.Logger, Open: func(ctx context.Context, account toolhost.Account) (*fanboxsdk.Client, error) {
		if ports.Open == nil {
			return nil, errors.New("fanbox SDK ports are not configured")
		}
		return ports.Open(ctx, Account{HTTPSProxyOverride: account.HTTPSProxyOverride})
	}}
	if ports.OpenLease != nil {
		runtimePorts.OpenLease = func(ctx context.Context, account toolhost.Account) (*lifecycle.Lease[*fanboxsdk.Client], error) {
			return ports.OpenLease(ctx, Account{HTTPSProxyOverride: account.HTTPSProxyOverride})
		}
	}
	if ports.Open == nil {
		runtimePorts.Open = nil
	}
	app := toolhost.NewApp(runtimePorts, toolhost.Account{HTTPSProxyOverride: proxyOverride})
	tools.Register(app, server)
}
