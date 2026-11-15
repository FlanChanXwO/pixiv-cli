package toolhost

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// PaginationOut 在共享逻辑页字段之外增加 Pixiv 查询绑定的批次 cursor。
type PaginationOut struct {
	NextCursor string `json:"next_cursor,omitempty"`
	pagination.PaginationOut
}

// ListPagination 生成不含 next_cursor 的分页；cursor 由调用方确认续读可用后再填。
func ListPagination(plan pagination.PagePlan, limit *int, returned int, more bool) PaginationOut {
	return PaginationOut{PaginationOut: pagination.ListPagination(plan, limit, returned, more)}
}

func collectFrom[E any, T any, C pagination.Cursor](ctx context.Context, baseExecute pagination.Execute[E], plan pagination.PagePlan, initial C, fetch func(context.Context, E, C) ([]T, C, error)) ([]T, bool, error) {
	var zero C
	seen := make(map[string]struct{})
	execute := baseExecute
	if baseExecute != nil {
		execute = func(ctx context.Context, attempt func(context.Context, E) (bool, error)) error {
			return baseExecute(ctx, func(attemptCtx context.Context, client E) (bool, error) {
				// 安全重放可能在同一次 Execute 内切换账号，因此去重状态随 attempt 重置。
				seen = make(map[string]struct{})
				return attempt(attemptCtx, client)
			})
		}
	}
	result, err := pagination.CollectWithFrom(ctx, execute, pagination.PagePlan{
		Skip: plan.Skip, Limit: plan.Limit, OneBatch: plan.OneBatch,
	}, initial, func(ctx context.Context, client E, cursor C) ([]T, C, error) {
		items, next, err := fetch(ctx, client, cursor)
		if err != nil {
			return nil, zero, err
		}
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if errors.Is(err, pagination.ErrExecuteNotConfigured) {
		err = &sdk.Error{Operation: "PagedRead", Reason: sdk.LocalStateError, Message: "sdk pooled operation is not configured"}
	}
	return result.Items, result.HasMore, err
}

// CollectPagesFrom 保留流内过滤语义；只有完整批次结束才返回可供续读的 cursor。
func CollectPagesFrom[T any, C pagination.Cursor](ctx context.Context, plan pagination.PagePlan, initial C, fetch func(context.Context, C) ([]T, C, error)) ([]T, bool, C, error) {
	var zero, continuation C
	seen := make(map[string]struct{})
	items, result, err := pagination.CollectPagesFrom(ctx, pagination.PagePlan{
		Skip:     plan.Skip,
		Limit:    plan.Limit,
		OneBatch: plan.OneBatch,
	}, initial, func(ctx context.Context, cursor C) ([]T, C, error) {
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, zero, err
		}
		if plan.OneBatch && !next.IsZero() && next.String() == cursor.String() {
			return nil, zero, errors.New("batch continuation cursor repeated")
		}
		continuation = next
		return filters.FilterPage(ctx, items, seen), next, nil
	})
	if err != nil {
		return nil, false, zero, err
	}
	if !plan.OneBatch {
		continuation = zero
	}
	return items, result.HasMore, continuation, nil
}
