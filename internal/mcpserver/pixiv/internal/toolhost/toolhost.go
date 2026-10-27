// Package toolhost 是 Pixiv MCP 的执行宿主：它持有 SDK 窄端口，把一次工具调用绑定到
// 固定账号，并在该账号的池重放边界内执行读、写和 client lease 操作。
//
// 包的组成：[App] 与 [NewApp] 是调用宿主，[SDKPorts]、[Account] 与
// [ReverseSearchPorts] 由组合根注入，[AddTool] 注册工具并附加诊断与惰性账号快照。
// [Read]、[Write] 与 [OpenClient] 是三种执行入口；分页适配在 paging.go，cursor
// 封装在 cursor.go，具体工具在 tools 包。
package toolhost

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 携带一次 MCP 请求使用的 UID 与传输覆写；client 与凭据由执行端口持有。
type Account struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// LocalDownloads 只在本地 stdio 交付下载文件时注入，路径模板由组合根决定。
type LocalDownloads struct {
	Service                                   downloader.DownloadService
	Path, FilenameTemplate, DirectoryTemplate string
}

// SDKPorts 让 MCP 打开独立认证快照，并在账号池的重放边界内执行操作。实现由组合根
// 注入；MCP 通过这组函数取用依赖。
type SDKPorts struct {
	Downloads *LocalDownloads
	Media     *mediaproxy.Service
	// Accounts 供 ResolveAccount 读取本地账号选择，所有连接器共享。
	Accounts accounts.Selection
	// Login 与 HTTP 登录接力共享，生命周期归 server 所有。
	Login *accounts.Login
	// ResolveAccount 固定一次业务调用使用的本地账号；不读取或刷新上游凭据。
	ResolveAccount func(context.Context, Account) (Account, error)
	OpenLease      func(context.Context, Account) (*lifecycle.Lease[*pixiv.Client], error)
	Execute        func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error
	// Read 服务 HTTP 的只读会话；stdio 下留空，读取走 Execute 的账号池。
	Read func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error
	// ReverseSearch 保存启动时注入的反向搜图能力与配置快照。它的 HTTP client、代理和
	// 凭据由组合根构造，MCP 输入无法覆盖。
	ReverseSearch ReverseSearchPorts
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

// ReadExecutor 固定调用账号；HTTP 使用只读会话，stdio 使用账号池执行端口。
func (a *App) ReadExecutor() pagination.Execute[*pixiv.Client] {
	if a.sdk.Execute == nil && a.sdk.Read == nil {
		return nil
	}
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		account, err := a.resolveAccount(ctx)
		if err != nil {
			return err
		}
		if a.sdk.Read != nil {
			return a.sdk.Read(ctx, account, attempt)
		}
		return a.sdk.Execute(ctx, account, attempt)
	}
}

// Read 在固定账号的执行边界内运行一次读操作，复用分页读取的账号快照。
func Read[T any](a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	execute := a.ReadExecutor()
	if execute == nil {
		return zero, sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	var result T
	err := execute(ctx, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Write 在账号池重放边界内执行一次写操作；是否提交与重放由会话端口决定。
func Write(a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) error) error {
	if a.sdk.Execute == nil {
		return sdk.NewError("pixiv", "Write", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	account, err := a.resolveAccount(ctx)
	if err != nil {
		return err
	}
	return a.sdk.Execute(ctx, account, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// OpenClient 打开一次由会话端口管理的独立 SDK snapshot 并返回 Lease。调用方必须关闭
// 返回的 Lease；底层 gate 与 client 的释放由注入端口负责。
func (a *App) OpenClient(ctx context.Context) (*lifecycle.Lease[*pixiv.Client], error) {
	if a.sdk.OpenLease == nil {
		return nil, errors.New("pixiv sdk is not configured")
	}
	account, err := a.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	return a.sdk.OpenLease(ctx, account)
}

// ResolveAccount 返回本次工具调用固定的账号；同一调用内多次调用结果一致。
func (a *App) ResolveAccount(ctx context.Context) (Account, error) { return a.resolveAccount(ctx) }

// CurrentUserID 返回当前 client snapshot 的身份 UID；取不到时报错，不回退到默认账号。
func (a *App) CurrentUserID(ctx context.Context) (int64, error) {
	return Read(a, ctx, func(ctx context.Context, client *pixiv.Client) (int64, error) {
		if id := client.UserID(); id > 0 {
			return id, nil
		}
		return 0, errors.New("cannot determine current user id")
	})
}

// ResolveUserID 解析 MCP 用户引用：正数原样返回，零回退到当前认证账号，负数报错。
func ResolveUserID(app *App, ctx context.Context, requested int64) (int64, error) {
	if requested > 0 {
		return requested, nil
	}
	if requested < 0 {
		return 0, errors.New("user_id must be positive when provided")
	}
	return app.CurrentUserID(ctx)
}

// Artwork 在有生产读会话时使用带 grant 范围的 Gallery 缓存，否则回退为一次普通 SDK
// 读取。
func (a *App) Artwork(ctx context.Context, id int64) (pixiv.Artwork, error) {
	if a.sdk.Media.HasReadSessions() {
		account, err := a.resolveAccount(ctx)
		if err != nil {
			return pixiv.Artwork{}, err
		}
		return a.sdk.Media.Artwork(ctx, account.UserID, id)
	}
	return Read(a, ctx, func(ctx context.Context, c *pixiv.Client) (pixiv.Artwork, error) {
		return c.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
	})
}
