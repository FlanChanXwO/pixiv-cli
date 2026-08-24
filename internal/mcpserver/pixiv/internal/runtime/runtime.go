// Package runtime 持有 Pixiv MCP 各 tool 共享的窄运行时：App 状态、SDK ports、
// 工具注册 wrapper、paged read/write 与 record filter。每个 tool 一个 package，
// 仅依赖这里的窄端口；product 聚合包只负责注册。
package runtime

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DownloadManager 是下载器在 MCP 侧的窄能力接口。
type DownloadManager interface {
	SetDownloadPath(string) error
	Download(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error)
}

// Account 是 MCP 请求的本地值；只携带传输覆写与账号选择，不持有
// client 或凭据。
type Account struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// SDKPorts 是 MCP 对 Pixiv services Facade 的窄端口：打开独立认证快照、在账号
// 池重放边界内执行用例。composition root 注入实现；MCP 不持有 service locator。
type SDKPorts struct {
	// Open is the raw-client compatibility adapter for existing embeddings. New
	// composition roots should inject OpenLease so the services Facade owns the
	// complete client lifecycle.
	Open      func(Account) (*pixiv.Client, error)
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*pixiv.Client], error)
	Execute   func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error
	// ReverseSearch 是启动时注入的反向搜图能力与配置快照。Searcher 的
	// HTTP client、代理和凭据均由 composition root 构造，不从 MCP input 传入。
	ReverseSearch ReverseSearchPorts
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
	downloads DownloadManager
	// newDownloads 在每个 SDK client 的稳定认证 snapshot 上创建下载器。
	// 固定 downloads 仅保留给未注入 SDK 的嵌入测试兼容路径。
	newDownloads   func(*pixiv.Client) DownloadManager
	sdk            SDKPorts
	sdkAccount     Account
	requestCounter atomic.Uint64
}

// NewApp 构造带下载器、下载器工厂、SDK ports 与账号的 Pixiv MCP 应用。
func NewApp(downloads DownloadManager, newDownloads func(*pixiv.Client) DownloadManager, ports SDKPorts, account Account) *App {
	return &App{downloads: downloads, newDownloads: newDownloads, sdk: ports, sdkAccount: account}
}

// Downloads 返回固定下载器（仅嵌入测试兼容路径使用）。
func (a *App) Downloads() DownloadManager { return a.downloads }

// NewDownloads 返回 snapshot-scoped 下载器工厂。
func (a *App) NewDownloads() func(*pixiv.Client) DownloadManager { return a.newDownloads }

// SDKPorts 返回注入的 SDK 端口。
func (a *App) SDKPorts() SDKPorts { return a.sdk }

// ReverseSearchPorts 返回启动时注入的反向搜图端口与配置。
func (a *App) ReverseSearchPorts() ReverseSearchPorts {
	if a == nil {
		return ReverseSearchPorts{}
	}
	return a.sdk.ReverseSearch
}

// Account 返回注入的账号请求值。
func (a *App) Account() Account { return a.sdkAccount }

// AddTool 统一保留注册入口；失败结果直接由各 handler 的 CallToolResult 表达。
// wrapper 只增加 stderr diagnostics scope，不改变 handler 的输入、structured
// result、isError 或错误返回。
func AddTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		requestID := app.requestCounter.Add(1)
		scoped := diagnostics.WithChildScope(ctx, diagnostics.ModulePixivMCPServer, requestID)
		startedAt := time.Now()
		diagnostics.Emit(scoped, diagnostics.Event{Kind: diagnostics.EventStarted, Operation: "tool " + tool.Name})
		result, output, err := handler(scoped, request, input)
		if err != nil || (result != nil && result.IsError) {
			diagnostics.Emit(scoped, diagnostics.Event{
				Kind:      diagnostics.EventFailed,
				Operation: "tool " + tool.Name,
				Reason:    diagnostics.ReasonToolFailed,
				Duration:  time.Since(startedAt),
			})
		} else {
			diagnostics.Emit(scoped, diagnostics.Event{
				Kind:      diagnostics.EventCompleted,
				Operation: "tool " + tool.Name,
				Duration:  time.Since(startedAt),
			})
		}
		return result, output, err
	})
}

// PageLimitIn 以指针区分未传 limit（兼容旧的“单上游批次”）和显式 limit=0（读取全部）。
// Cursor 始终是 SDK 内部细节，不出现在 MCP tool 输入或输出中。
type PageLimitIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

// ListPlan 是解析后的逻辑分页计划。
type ListPlan struct {
	Page     int
	Limit    int
	OneBatch bool
	Skip     int
}

// PaginationOut 是列表 tool 的稳定分页输出。
type PaginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

// ParseListPlan 解析分页输入；nil limit 表示单上游批次。
func ParseListPlan(input PageLimitIn) (ListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return ListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return ListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return ListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return ListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return ListPlan{Page: *input.Page, Limit: *input.Limit, Skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return ListPlan{Page: 1, Limit: -1, OneBatch: true}, nil
	}
	return ListPlan{Page: 1, Limit: *input.Limit}, nil
}

// ListPagination 构造分页输出。
func ListPagination(plan ListPlan, limit *int, returned int, hasMore bool) PaginationOut {
	out := PaginationOut{Page: plan.Page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.Page < math.MaxInt {
		next := plan.Page + 1
		out.NextPage = &next
	}
	return out
}

// Execute 将 MCP 当前请求接入 services Facade 提供的重放边界；逻辑分页和
// cursor 处理由 internal/shared/traversal 与 pagination 统一拥有。
func (a *App) Execute() traversal.Execute[*pixiv.Client] {
	if a == nil || a.sdk.Execute == nil {
		return nil
	}
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.sdk.Execute(ctx, a.sdkAccount, attempt)
	}
}

// Read 在账号池重放边界内执行一次只读用例；MCP 只在成功后拿到结果。
func Read[T any](a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	if a == nil || a.sdk.Execute == nil {
		return zero, sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	var result T
	err := a.sdk.Execute(ctx, a.sdkAccount, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Write 在账号池重放边界内执行一次 mutation；提交边界由 services Facade 语义决定。
func Write(a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) error) error {
	if a == nil || a.sdk.Execute == nil {
		return sdk.NewError("pixiv", "Write", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return a.sdk.Execute(ctx, a.sdkAccount, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// CollectWith 只负责 MCP record filter，逻辑分页由共享 traversal 引擎执行。
// MCP 不自行打开 client、重放账号或解释 opaque cursor。
func CollectWith[T any](ctx context.Context, app *App, plan ListPlan, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	seen := make(map[string]struct{})
	result, err := traversal.CollectWith(ctx, app.Execute(), pagination.PagePlan{
		Skip: plan.Skip, Limit: max(0, plan.Limit), OneBatch: plan.OneBatch,
	}, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		if cursor.IsZero() {
			seen = make(map[string]struct{})
		}
		items, next, err := fetch(ctx, client, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if errors.Is(err, traversal.ErrExecuteNotConfigured) {
		err = sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return result.Items, result.HasMore, err
}

// CollectPages 仅把 MCP 的兼容 sentinel 映射到共享分页语义；成功空结果
// 仍保持 non-nil slice，失败时共享 collector 会丢弃部分结果。
func CollectPages[T any](ctx context.Context, plan ListPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	limit := max(0, plan.Limit)
	seen := make(map[string]struct{})
	items, result, err := pagination.CollectPages(ctx, pagination.PagePlan{
		Skip:     plan.Skip,
		Limit:    limit,
		OneBatch: plan.OneBatch,
	}, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if err != nil {
		return nil, false, err
	}
	return items, result.HasMore, nil
}

// OpenClient 打开一次由 services Facade 管理的独立 SDK snapshot，并返回
// 显式 Lease。调用方必须关闭 Lease；底层 gate 与 client 释放由注入端口拥有。
func (a *App) OpenClient(ctx context.Context) (*lifecycle.Lease[*pixiv.Client], error) {
	if a == nil {
		return nil, errors.New("pixiv sdk is not configured")
	}
	openLease := a.sdk.OpenLease
	if openLease == nil && a.sdk.Open != nil {
		openLease = func(_ context.Context, account Account) (*lifecycle.Lease[*pixiv.Client], error) {
			client, err := a.sdk.Open(account)
			if err != nil {
				return nil, err
			}
			if client == nil {
				return nil, lifecycle.ErrNilLease
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
	lease, err := openLease(ctx, a.sdkAccount)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, lifecycle.ErrNilLease
	}
	return lease, nil
}

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
