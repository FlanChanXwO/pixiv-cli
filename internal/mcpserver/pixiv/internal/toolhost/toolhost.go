// Package toolhost 持有 Pixiv MCP 的 SDK 端口、单次调用账号与执行边界。
// 分页输入和遍历适配在 paging.go；工具注册由 tools 包负责。
package toolhost

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account 是 MCP 请求的本地值；只携带传输覆写与账号选择，不持有
// client 或凭据。
type Account struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// LocalDownloads is injected only for local filesystem delivery.
type LocalDownloads struct {
	Service                                   downloader.DownloadService
	Path, FilenameTemplate, DirectoryTemplate string
}

// SDKPorts 是 MCP 对 Pixiv 会话的窄端口：打开独立认证快照、在账号
// 池重放边界内执行用例。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts struct {
	Downloads *LocalDownloads
	Media     *mediaproxy.Service
	// Accounts shares the owner-local manager used by production ResolveAccount.
	Accounts accounts.Selection
	// Login is shared with the HTTP relay and owned by the server lifecycle.
	Login *accounts.Login
	// ResolveAccount 固定一次业务调用的本地账号选择；不读取或刷新上游凭据。
	ResolveAccount func(context.Context, Account) (Account, error)
	// shortcut: CLI still initializes Open; remove this adapter with that initializer.
	Open      func(Account) (*pixiv.Client, error)
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*pixiv.Client], error)
	Execute   func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error
	// Read 用于 HTTP 只读会话；stdio 留空，读取通过 Execute 的账号池执行。
	Read func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error
	// ReverseSearch 是启动时注入的反向搜图能力与配置快照。Searcher 的
	// HTTP client、代理和凭据均由 composition root 构造，不从 MCP input 传入。
	ReverseSearch ReverseSearchPorts
	// Logger 是执行边界的日志入口；nil 时不输出。
	Logger *slog.Logger
}

// ReverseSearchPorts 是 reverse_search tool 使用的窄端口与启动时配置快照。
// Provider 为空时由 tool 使用 saucenao 作为配置默认值；PixivOnly 的 false
// 是合法配置，因此不能用零值推断是否启用。
type ReverseSearchPorts struct {
	Searcher  reversesearch.Searcher
	Provider  reversesearch.Provider
	PixivOnly bool
}

// App 是 Pixiv MCP 应用。
type App struct {
	sdk            SDKPorts
	sdkAccount     Account
	logger         *slog.Logger
	requestCounter atomic.Uint64
}

// NewApp 构造独立的 Pixiv SDK 与账号运行时。
func NewApp(ports SDKPorts, account Account) *App {
	return &App{sdk: ports, sdkAccount: account, logger: cmp.Or(ports.Logger, slog.New(slog.DiscardHandler))}
}

// SDKPorts 返回注入的 SDK 端口。
func (a *App) SDKPorts() SDKPorts { return a.sdk }

// AccountSelection 返回共享的账号选择；不打开 SDK client。
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

// AddTool 统一保留注册入口；失败结果直接由各 handler 的 CallToolResult 表达。
// wrapper 增加 diagnostics scope 和惰性账号快照，不改变 handler 的输入、
// structured result、isError 或错误返回。
func AddTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if app.sdk.Media != nil {
		gallery.Attach(tool, app.sdk.Media)
	}
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		requestID := app.requestCounter.Add(1)
		ctx = logging.WithCall(ctx, app.logger, tool.Name, strconv.FormatUint(requestID, 10))
		ctx = context.WithValue(ctx, accountSnapshotKey{}, &accountSnapshot{app: app})
		result, output, err := handler(ctx, request, input)
		if err != nil || (result != nil && result.IsError) {
			app.logger.ErrorContext(ctx, "Tool failed", slog.String(logging.AttrReason, "tool failed"))
		}
		return result, output, err
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

// Read 与分页读取共用账号快照和传输对应的执行策略。
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

// Write 在账号池重放边界内执行一次 mutation；提交与重放由会话端口拥有。
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

// OpenClient 打开一次由会话端口管理的独立 SDK snapshot，并返回
// 显式 Lease。调用方必须关闭 Lease；底层 gate 与 client 释放由注入端口拥有。
func (a *App) OpenClient(ctx context.Context) (*lifecycle.Lease[*pixiv.Client], error) {
	openLease := a.sdk.OpenLease
	if openLease == nil && a.sdk.Open != nil {
		openLease = func(_ context.Context, account Account) (*lifecycle.Lease[*pixiv.Client], error) {
			client, err := a.sdk.Open(account)
			if err != nil {
				return nil, err
			}
			return lifecycle.NewLease(client, func() error {
				client.CloseIdleConnections()
				return nil
			}), nil
		}
	}
	if openLease == nil {
		return nil, errors.New("pixiv sdk is not configured")
	}
	account, err := a.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	return openLease(ctx, account)
}

// ResolveAccount returns the fixed account for this tool invocation.
func (a *App) ResolveAccount(ctx context.Context) (Account, error) { return a.resolveAccount(ctx) }

// CurrentUserID 返回当前 client snapshot 的身份 UID；未携带账号时报错，
// 不静默回退到默认账号语义。
func (a *App) CurrentUserID(ctx context.Context) (int64, error) {
	return Read(a, ctx, func(ctx context.Context, client *pixiv.Client) (int64, error) {
		if id := client.UserID(); id > 0 {
			return id, nil
		}
		return 0, errors.New("cannot determine current user id")
	})
}

// ResolveUserID 解析 MCP 用户引用：显式正数直接返回；缺省时回退到当前认证
// 账号，负值显式报错，绝不静默混含缺省与非法请求。
func ResolveUserID(app *App, ctx context.Context, requested int64) (int64, error) {
	if requested > 0 {
		return requested, nil
	}
	if requested < 0 {
		return 0, errors.New("user_id must be positive when provided")
	}
	return app.CurrentUserID(ctx)
}

// Artwork uses grant-scoped Gallery metadata when production read sessions are available.
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
