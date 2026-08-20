// Package runtime 持有 FANBOX MCP 各 tool 共享的窄运行时：App 状态、工具注册
// wrapper、client 打开与统一 result 摘要。每个 tool 一个 package，仅依赖这里的
// 窄端口；product 聚合包只负责注册。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account carries only request-scoped transport overrides.
type Account struct {
	HTTPSProxyOverride *string
}

// SDKPorts is the runtime's narrow dependency on the public FANBOX SDK.
type SDKPorts struct {
	// Open is retained as a raw-client adapter for existing embeddings. New
	// composition roots should inject OpenLease so the services Facade owns the
	// complete client lifecycle.
	Open      func(context.Context, Account) (*fanbox.Client, error)
	OpenLease func(context.Context, Account) (*lifecycle.Lease[*fanbox.Client], error)
}

// App 是 FANBOX MCP 应用，持有 SDK port 与命令级代理覆写。
type App struct {
	sdk            SDKPorts
	account        Account
	requestCounter atomic.Uint64
}

// NewApp 构造带 SDK port 与请求覆写的 FANBOX MCP 应用。
func NewApp(sdk SDKPorts, account Account) *App {
	return &App{sdk: sdk, account: account}
}

// OpenClient 为一次 tool 调用建立独立 client snapshot，并返回显式 Lease。
// tool 必须关闭 Lease，不得直接管理 SDK client 的底层连接。
func (a *App) OpenClient(ctx context.Context) (*lifecycle.Lease[*fanbox.Client], error) {
	if a == nil {
		return nil, errors.New("fanbox service is not configured")
	}
	openLease := a.sdk.OpenLease
	if openLease == nil && a.sdk.Open != nil {
		openLease = func(ctx context.Context, account Account) (*lifecycle.Lease[*fanbox.Client], error) {
			client, err := a.sdk.Open(ctx, account)
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
		return nil, errors.New("fanbox service is not configured")
	}
	lease, err := openLease(ctx, a.account)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, lifecycle.ErrNilLease
	}
	return lease, nil
}

// AddTool 统一保留注册入口；失败结果直接由各 handler 的 CallToolResult 表达。
// wrapper 只增加 stderr diagnostics scope，不改变 handler 的输入、structured
// result、isError 或错误返回。
func AddTool[In, Out any](app *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		requestID := app.requestCounter.Add(1)
		scoped := diagnostics.WithChildScope(ctx, diagnostics.ModuleFanboxMCPServer, requestID)
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

// Result 统一 MCP tool 的文本摘要；完整实体只存在于 structured output。
func Result[Out any](out Out, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = "OK"
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// ListIn 与 pixiv MCP 的 pageLimitIn 语义一致；Cursor 始终是 SDK 内部细节，
// 不出现在 MCP tool 输入或输出中。
type ListIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

// ListPlan 是解析后的分页计划。
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
func ParseListPlan(input ListIn) (ListPlan, error) {
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

// CollectPages 跟随 sdk.Cursor 收集分页结果；失败时丢弃部分结果。
func CollectPages[T any](ctx context.Context, plan ListPlan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) ([]T, bool, error) {
	limit := max(0, plan.Limit)
	items := make([]T, 0)
	cursor := sdk.Cursor{}
	seen := make(map[string]struct{})
	skip := plan.Skip
	returned := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		key := cursor.String()
		if _, exists := seen[key]; exists {
			return nil, false, errors.New("pagination cursor repeated")
		}
		seen[key] = struct{}{}
		page, err := fetch(ctx, cursor)
		if err != nil {
			return nil, false, err
		}
		batch := page.Items
		if skip >= len(batch) {
			skip -= len(batch)
			batch = nil
		} else if skip > 0 {
			batch = batch[skip:]
			skip = 0
		}
		// truncated reports whether this upstream batch still holds unreturned
		// items after the logical limit is applied. When it does, has_more must
		// be true even if the upstream cursor is zero, otherwise clients see
		// has_more=false with unreturned items and no next_page (finding #17).
		truncated := false
		if limit > 0 {
			remaining := limit - returned
			if len(batch) > remaining {
				batch = batch[:remaining]
				truncated = true
			}
		}
		items = append(items, batch...)
		returned += len(batch)
		if limit > 0 && returned >= limit {
			return items, truncated || !page.Next.IsZero(), nil
		}
		if plan.OneBatch && (returned > 0 || page.Next.IsZero()) {
			return items, truncated || !page.Next.IsZero(), nil
		}
		if page.Next.IsZero() {
			return items, truncated, nil
		}
		cursor = page.Next
	}
}

// ResourceOut 是资源引用的安全输出形态。
type ResourceOut struct {
	Ref                 string `json:"ref"`
	RequiresCredentials bool   `json:"requires_credentials,omitempty"`
}

// ResourceOutFrom 把 opaque Resource 转为安全输出。
func ResourceOutFrom(res sdk.Resource) *ResourceOut {
	dto := sdk.ToResourceDTO(res)
	if dto == nil {
		return nil
	}
	return &ResourceOut{Ref: dto.Ref, RequiresCredentials: dto.RequiresCredentials}
}

// PostAssetOut 是帖子资产的安全输出形态。
type PostAssetOut struct {
	ID        string       `json:"id"`
	Kind      string       `json:"kind"`
	Name      string       `json:"name,omitempty"`
	Resource  *ResourceOut `json:"resource,omitempty"`
	Thumbnail *ResourceOut `json:"thumbnail,omitempty"`
}

// PostOut 是 FANBOX 帖子的安全输出形态。
type PostOut struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	PublishedAt   string         `json:"published_at"`
	CreatorID     string         `json:"creator_id"`
	FeeRequired   int            `json:"fee_required,omitempty"`
	IsRestricted  bool           `json:"is_restricted"`
	IsPinned      bool           `json:"is_pinned,omitempty"`
	RestrictedFor int            `json:"restricted_for,omitempty"`
	CommentCount  int            `json:"comment_count,omitempty"`
	Assets        []PostAssetOut `json:"assets"`
}

// PostOutFrom 把 SDK post 转为安全输出。
func PostOutFrom(post fanbox.Post) PostOut {
	published := ""
	if !post.PublishedAt.IsZero() {
		published = post.PublishedAt.UTC().Format(time.RFC3339)
	}
	out := PostOut{
		ID:            post.ID,
		Title:         post.Title,
		PublishedAt:   published,
		CreatorID:     post.CreatorID,
		FeeRequired:   post.FeeRequired,
		IsRestricted:  post.IsRestricted,
		IsPinned:      post.IsPinned,
		RestrictedFor: post.RestrictedFor,
		CommentCount:  post.CommentCount,
		Assets:        []PostAssetOut{},
	}
	if post.Body == nil {
		return out
	}
	for _, asset := range post.Body.Assets {
		out.Assets = append(out.Assets, PostAssetOut{
			ID:        asset.ID,
			Kind:      string(asset.Kind),
			Name:      asset.Name,
			Resource:  ResourceOutFrom(asset.Resource),
			Thumbnail: ResourceOutFrom(asset.Thumbnail.Resource),
		})
	}
	return out
}

// PostsOut 是帖子列表 tool 的输出 envelope。
type PostsOut struct {
	Posts      []PostOut     `json:"posts"`
	Pagination PaginationOut `json:"pagination"`
}

// PostList 打开 client 后统一收集帖子分页并输出。
func PostList(ctx context.Context, app *App, client *fanbox.Client, limit *int, plan ListPlan, fetch func(context.Context, *fanbox.Client, sdk.Cursor) (sdk.Page[fanbox.Post], error)) (*mcp.CallToolResult, PostsOut, error) {
	out := PostsOut{Posts: []PostOut{}}
	items, hasMore, err := CollectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return fetch(ctx, client, cursor)
	})
	if err != nil {
		return Result(out, true, "Error: "+err.Error()), out, nil
	}
	for _, post := range items {
		out.Posts = append(out.Posts, PostOutFrom(post))
	}
	out.Pagination = ListPagination(plan, limit, len(out.Posts), hasMore)
	return Result(out, false, fmt.Sprintf("Retrieved %d posts.", len(out.Posts))), out, nil
}
