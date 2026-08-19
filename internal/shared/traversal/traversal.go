// Package traversal 提供可由调用方重放的通用逻辑分页遍历。
//
// 本包只处理 execute 生命周期、分页计划和 opaque cursor，不负责账号选择、
// 网络策略、过滤语义或任何具体产品的错误映射。
package traversal

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ErrExecuteNotConfigured 表示调用方没有提供可重放的 execute 函数。
// 产品 owner 可将它映射为自己的稳定错误类型。
var ErrExecuteNotConfigured = errors.New("traversal: execute function is not configured")

// Execute 在调用方提供的可重放边界内使用一次资源。
// committed=true 表示本次尝试已经越过交付或副作用边界。
type Execute[C any] func(context.Context, func(context.Context, C) (committed bool, err error)) error

// PagedReadResult 是逻辑分页结果；opaque cursor 不离开 traversal。
type PagedReadResult[T any] struct {
	Items   []T
	HasMore bool
}

// TraverseWith 在 execute 提供的重放边界内执行一次逻辑分页遍历。
//
// begin 会在每次 execute 尝试开始时调用。consume 返回 committed=true 时，
// execute owner 可以据此禁止后续重放。分页引擎仍由 shared/pagination 负责
// Skip、Limit、OneBatch、cursor 循环检测和 HasMore 语义。返回的 PageResult
// 携带引擎权威的 Returned 与 HasMore，包括上游批次内被 Limit 截断但上游
// cursor 已为空的情形——此时仍有未返回的条目，HasMore 必须为 true。
func TraverseWith[C any, T any](
	ctx context.Context,
	execute Execute[C],
	plan pagination.PagePlan,
	begin func() error,
	fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error),
	consume func([]T) (committed bool, err error),
) (pagination.PageResult, error) {
	if execute == nil {
		return pagination.PageResult{}, ErrExecuteNotConfigured
	}

	var pageResult pagination.PageResult
	err := execute(ctx, func(ctx context.Context, client C) (bool, error) {
		if begin != nil {
			if err := begin(); err != nil {
				return false, err
			}
		}

		committed := false
		var err error
		pageResult, err = pagination.TraversePagesFrom(ctx, plan, sdk.Cursor{}, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
			return fetch(ctx, client, cursor)
		}, func(items []T) error {
			published, err := consume(items)
			committed = committed || published
			return err
		})
		return committed, err
	})
	return pageResult, err
}

// CollectWith 在 execute 提供的重放边界内收集逻辑分页结果。
// 每次新尝试开始时都会清空前一次未提交的结果，避免重放时混入不同资源
// 的数据；失败时返回当前尝试的结果和原始错误。
func CollectWith[C any, T any](
	ctx context.Context,
	execute Execute[C],
	plan pagination.PagePlan,
	fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error),
) (PagedReadResult[T], error) {
	result := PagedReadResult[T]{Items: make([]T, 0)}

	pageResult, err := TraverseWith(ctx, execute, plan, func() error {
		result.Items = result.Items[:0]
		result.HasMore = false
		return nil
	}, func(ctx context.Context, client C, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, cursorNext, err := fetch(ctx, client, cursor)
		return items, cursorNext, err
	}, func(items []T) (bool, error) {
		result.Items = append(result.Items, items...)
		return false, nil
	})
	result.HasMore = pageResult.HasMore
	return result, err
}
