// paging.go 解析 MCP 逻辑分页输入，并把 Pixiv 筛选与账号执行接入共享遍历。

package toolhost

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PaginationOut extends the shared logical page with Pixiv's query-bound batch cursor.
type PaginationOut struct {
	NextCursor string `json:"next_cursor,omitempty"`
	toolrun.PaginationOut
}

// ListPagination wraps the shared page fields for Pixiv cursor-bearing output.
func ListPagination(plan toolrun.ListPlan, limit *int, returned int, more bool) PaginationOut {
	return PaginationOut{PaginationOut: toolrun.ListPagination(plan, limit, returned, more)}
}

// CollectWith 只负责 MCP record filter，逻辑分页由共享 traversal 引擎执行。
// MCP 不自行打开 client、重放账号或解释 opaque cursor。
func CollectWith[T any](ctx context.Context, app *App, plan toolrun.ListPlan, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	return collectWithFrom(ctx, app, plan, sdk.Cursor{}, fetch)
}

// CollectBatchWith 从已有 cursor 读取完整过滤批次；不应用逻辑 limit，
// 因而返回的 SDK cursor 之前不会遗留尚未交付的批次内条目。
func CollectBatchWith[T any](ctx context.Context, app *App, initial sdk.Cursor, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, sdk.Cursor, error) {
	var next sdk.Cursor
	items, _, err := collectWithFrom(ctx, app, toolrun.ListPlan{Page: 1, OneBatch: true}, initial, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		batch, continuation, err := fetch(ctx, client, cursor)
		if err == nil && !continuation.IsZero() && continuation == cursor {
			return nil, sdk.Cursor{}, errors.New("batch continuation cursor repeated")
		}
		next = continuation
		return batch, continuation, err
	})
	if err != nil {
		return nil, sdk.Cursor{}, err
	}
	return items, next, nil
}

// collectWithFrom 统一逻辑分页与批次续读的 execution-attempt 生命周期；
// SDK cursor 的解析与公开 wire envelope 由具体 tool owner 负责。
func collectWithFrom[T any](ctx context.Context, app *App, plan toolrun.ListPlan, initial sdk.Cursor, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	seen := make(map[string]struct{})
	baseExecute := app.ReadExecutor()
	execute := baseExecute
	if baseExecute != nil {
		execute = func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return baseExecute(ctx, func(attemptCtx context.Context, client *pixiv.Client) (bool, error) {
				// safe replay 可能在同一 Execute 调用内切换账号；去重状态必须
				// 与每次 attempt 对齐，不能由 opaque cursor 的数值推断生命周期。
				seen = make(map[string]struct{})
				return attempt(attemptCtx, client)
			})
		}
	}
	result, err := pagination.CollectWithFrom(ctx, execute, pagination.PagePlan{
		Skip: plan.Skip, Limit: plan.Limit, OneBatch: plan.OneBatch,
	}, initial, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, next, err := fetch(ctx, client, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if errors.Is(err, pagination.ErrExecuteNotConfigured) {
		err = sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return result.Items, result.HasMore, err
}

// CollectStreamsWith 为需要多个有序上游流的 MCP read tool 复用同一账号池
// execution 边界与 structured 配置错误语义。聚合结果由 traversal 在每次
// attempt 中重置，失败 attempt 的部分结果不会泄漏到后续安全重放。
func CollectStreamsWith[T any, C pagination.Cursor](ctx context.Context, app *App, plan toolrun.ListPlan, streams func(*pixiv.Client) []pagination.Stream[T, C]) ([]T, bool, error) {
	result, err := pagination.CollectStreamsWith(ctx, app.ReadExecutor(), pagination.PagePlan{
		Skip: plan.Skip, Limit: plan.Limit, OneBatch: plan.OneBatch,
	}, streams)
	if errors.Is(err, pagination.ErrExecuteNotConfigured) {
		err = sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return result.Items, result.HasMore, err
}

// CollectPagesFrom 保持流内过滤语义，默认完整批次才返回可安全续读的 cursor。
func CollectPagesFrom[T any](ctx context.Context, plan toolrun.ListPlan, initial sdk.Cursor, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, sdk.Cursor, error) {
	var continuation sdk.Cursor
	seen := make(map[string]struct{})
	items, result, err := pagination.CollectPagesFrom(ctx, pagination.PagePlan{
		Skip:     plan.Skip,
		Limit:    plan.Limit,
		OneBatch: plan.OneBatch,
	}, initial, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		if plan.OneBatch && !next.IsZero() && next == cursor {
			return nil, sdk.Cursor{}, errors.New("batch continuation cursor repeated")
		}
		continuation = next
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if err != nil {
		return nil, false, sdk.Cursor{}, err
	}
	if !plan.OneBatch {
		continuation = sdk.Cursor{}
	}
	return items, result.HasMore, continuation, nil
}
