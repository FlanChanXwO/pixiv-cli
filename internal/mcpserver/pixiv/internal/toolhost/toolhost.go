// Package toolhost 是 Pixiv MCP 的执行宿主：它持有 SDK 窄端口，在账号池重放边界内
// 执行 stdio 读写，并固定 HTTP 调用与 client lease 的账号。
//
// 包的组成：[App] 与 [NewApp] 是调用宿主，[SDKPorts]、[Account] 与
// [ReverseSearchPorts] 由组合根注入，[AddTool] 注册工具并附加诊断与惰性账号快照。
// [ReadSDK]、[WriteSDK]、[App.SDKReadExecutor] 与 [App.OpenSDKClient] 提供领域入口。
// 分页适配在 paging.go 与 sdk_paging.go，
// cursor 封装在 cursor.go，具体工具在 tools 包。
package toolhost

import (
	"context"
	"log/slog"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 携带一次 MCP 请求使用的 UID 与传输覆写；client 与凭据由执行端口持有。
type Account struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// LocalDownloads 只在本地 stdio 交付下载文件时注入，路径模板由组合根决定。
type LocalDownloads struct {
	Path, FilenameTemplate, DirectoryTemplate string
}

// SDKPorts 让 MCP 打开独立认证快照，并在账号池的重放边界内执行操作。实现由组合根
// 注入；MCP 通过这组函数取用依赖。
type SDKPorts struct {
	// ExecuteSDK 在账号池边界内执行 SDK 操作。
	ExecuteSDK   SDKExecutor
	OpenSDKLease SDKLease
	// ResolveSDKAccount 保留 stdio 的池选择；HTTP 沿用 ResolveAccount 的固定快照。
	ResolveSDKAccount func(context.Context, Account) (Account, error)
	Downloads         *LocalDownloads
	Media             *mediaproxy.Service
	// Accounts 供 ResolveAccount 读取本地账号选择，所有连接器共享。
	Accounts accounts.Selection
	// Login 与 HTTP 登录接力共享，生命周期归 server 所有。
	Login *accounts.Login
	// ResolveAccount 固定一次业务调用使用的本地账号；不读取或刷新上游凭据。
	ResolveAccount func(context.Context, Account) (Account, error)
	// ReverseSearch 保存启动时注入的反向搜图能力与配置快照。它的 HTTP client、代理和
	// 凭据由组合根构造，MCP 输入无法覆盖。
	ReverseSearch ReverseSearchPorts
	// VectorSearch 延迟解析账号，让配置与输入失败先于账号读取。
	VectorSearch func(context.Context, vectorindex.SearchInput, string, func(context.Context) (Account, error)) (vectorindex.SearchResponse, error)
	// Logger 接收执行边界日志；nil 时工具保持静默。
	Logger *slog.Logger
}

// ReverseSearchPorts 保存 reverse_search 工具的窄端口与启动时配置快照。
// Provider 为空时工具采用 saucenao；PixivOnly 的两个取值都是有效配置。
type ReverseSearchPorts struct {
	Searcher  reversesearch.Searcher
	Provider  reversesearch.Provider
	PixivOnly bool
}

// App 保存一次 MCP 运行时的 SDK 端口、初始账号与调用诊断。
type App struct {
	sdk        SDKPorts
	sdkAccount Account
	calls      *toolrun.Scope
}

// NewApp 用注入的端口构造 App；同一个 App 的工具共享调用计数器。
func NewApp(ports SDKPorts, account Account) *App {
	return &App{sdk: ports, sdkAccount: account, calls: toolrun.NewScope(ports.Logger)}
}

// SDKPorts 返回注入的 SDK 端口原值。
func (a *App) SDKPorts() SDKPorts { return a.sdk }

// AccountSelection 返回共享的账号选择器，不会打开 SDK client。
func (a *App) AccountSelection() accounts.Selection { return a.sdk.Accounts }

// ReverseSearchPorts 返回启动时注入的反向搜图端口与配置。
func (a *App) ReverseSearchPorts() ReverseSearchPorts { return a.sdk.ReverseSearch }

type accountSnapshotKey struct{}

type accountSnapshot struct {
	app     *App
	once    sync.Once
	account Account
	err     error
}

// resolveAccount 在首次 SDK 访问时取本地选择，避免无账号工具被选择错误阻断。
// 同一 tool 的后续 SDK 操作与并行子操作复用结果（包括错误），切换只影响后续调用。
func (a *App) resolveAccount(ctx context.Context) (Account, error) {
	if a.sdk.ResolveAccount == nil {
		return a.sdkAccount, nil
	}
	if snapshot, ok := ctx.Value(accountSnapshotKey{}).(*accountSnapshot); ok && snapshot.app == a {
		snapshot.once.Do(func() { snapshot.account, snapshot.err = a.sdk.ResolveAccount(ctx, a.sdkAccount) })
		return snapshot.account, snapshot.err
	}
	return a.sdk.ResolveAccount(ctx, a.sdkAccount)
}

// AddTool 注册一个 Pixiv 工具，并附加诊断 scope 与惰性账号快照。handler 的输入、
// structured result、isError 与错误返回都原样传回 MCP SDK。
func AddTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if app.sdk.Media != nil {
		gallery.Attach(tool, app.sdk.Media)
	}
	toolrun.AddTool(app.calls, server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		ctx = context.WithValue(ctx, accountSnapshotKey{}, &accountSnapshot{app: app})
		return handler(ctx, request, input)
	})
}

// ResolveAccount 返回本次工具调用固定的账号；同一调用内多次调用结果一致。
func (a *App) ResolveAccount(ctx context.Context) (Account, error) { return a.resolveAccount(ctx) }

// VectorAccount 保留 stdio 池选择与 HTTP 固定账号，在向量配置校验后才调用。
func (a *App) VectorAccount(ctx context.Context) (Account, error) { return a.resolveSDKAccount(ctx) }
