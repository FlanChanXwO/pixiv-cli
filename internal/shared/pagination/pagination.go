// Package pagination 提供与产品、账号和输出无关的逻辑分页算法。
package pagination

import (
	"context"
	"errors"
	"fmt"
)

// Cursor 是分页算法所需的最小 continuation 契约。实现必须让零值表示没有
// continuation，并提供稳定的文本形式以检测循环；算法不会解码或重建 cursor。
type Cursor interface {
	IsZero() bool
	String() string
}

// PagePlan 描述逻辑分页请求。Limit 为 0 时遍历全部结果。
type PagePlan struct {
	Skip     int
	Limit    int
	OneBatch bool
}

// PageResult 描述逻辑分页实际返回的条数以及上游是否仍有内容。
type PageResult struct {
	Returned int
	HasMore  bool
}

// TraversePages 从 Cursor 的零值开始遍历。Cursor 的具体产品类型由 fetch
// 推导，分页包不依赖任何 SDK 或传输实现。
func TraversePages[T any, C Cursor](ctx context.Context, plan PagePlan, fetch func(context.Context, C) ([]T, C, error), consume func([]T) error) (PageResult, error) {
	var initial C
	return TraversePagesFrom(ctx, plan, initial, fetch, consume)
}

// TraversePagesFrom 从调用方已有的 opaque cursor 开始遍历。
func TraversePagesFrom[T any, C Cursor](ctx context.Context, plan PagePlan, initial C, fetch func(context.Context, C) ([]T, C, error), consume func([]T) error) (PageResult, error) {
	var result PageResult
	if plan.Skip < 0 {
		return result, errors.New("page skip must be zero or positive")
	}
	if plan.Limit < 0 {
		return result, errors.New("page limit must be zero or positive")
	}
	cursor := initial
	seen := make(map[string]struct{})
	skip := plan.Skip
	seekingOffset := skip > 0
	for {
		// 稳定文本而非实现身份用于判重，支持不透明的值或指针 cursor。
		if _, exists := seen[cursor.String()]; exists {
			return result, fmt.Errorf("pagination cursor repeated: %s", cursor.String())
		}
		seen[cursor.String()] = struct{}{}
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return result, err
		}
		if skip >= len(items) {
			skip -= len(items)
			items = nil
		} else if skip > 0 {
			items = items[skip:]
			skip = 0
		}
		if seekingOffset && skip == 0 && len(items) > 0 {
			seekingOffset = false
		}
		if plan.Limit > 0 {
			remaining := plan.Limit - result.Returned
			if len(items) > remaining {
				items = items[:remaining]
				result.HasMore = true
			}
		}
		if len(items) > 0 {
			if err := consume(items); err != nil {
				return result, err
			}
			result.Returned += len(items)
		}
		if plan.Limit > 0 && result.Returned >= plan.Limit {
			if !result.HasMore {
				result.HasMore = !next.IsZero()
			}
			return result, nil
		}
		// OneBatch 表示一个逻辑批次：前导空上游批次不能提前结束分页。
		if plan.OneBatch && !seekingOffset {
			if result.Returned > 0 || next.IsZero() {
				result.HasMore = !next.IsZero()
				return result, nil
			}
		}
		if next.IsZero() {
			return result, nil
		}
		cursor = next
	}
}

// CollectPages 使用共享遍历引擎收集结果；失败时丢弃已经收集的部分结果。
func CollectPages[T any, C Cursor](ctx context.Context, plan PagePlan, fetch func(context.Context, C) ([]T, C, error)) ([]T, PageResult, error) {
	var initial C
	return CollectPagesFrom(ctx, plan, initial, fetch)
}

// CollectPagesFrom 保留调用方初始 cursor 的 operation/query binding，并在失败
// 时丢弃已经收集的部分结果。
func CollectPagesFrom[T any, C Cursor](ctx context.Context, plan PagePlan, initial C, fetch func(context.Context, C) ([]T, C, error)) ([]T, PageResult, error) {
	items := make([]T, 0)
	result, err := TraversePagesFrom(ctx, plan, initial, fetch, func(page []T) error {
		items = append(items, page...)
		return nil
	})
	if err != nil {
		return nil, PageResult{}, err
	}
	return items, result, nil
}

// CollectFilteredPagesFrom 对上游批次逐项筛选，再应用 Skip、Limit 和 OneBatch。
// 不能先把 Limit 传给普通 TraversePages，否则被筛掉的候选会占用逻辑页额度。
func CollectFilteredPagesFrom[T any, C Cursor](ctx context.Context, plan PagePlan, initial C, fetch func(context.Context, C) ([]T, C, error), include func(T) (bool, error)) ([]T, C, PageResult, error) {
	var zero C
	if plan.Skip < 0 {
		return nil, zero, PageResult{}, errors.New("page skip must be zero or positive")
	}
	if plan.Limit < 0 {
		return nil, zero, PageResult{}, errors.New("page limit must be zero or positive")
	}
	if include == nil {
		return nil, zero, PageResult{}, errors.New("filtered page predicate is required")
	}

	items := make([]T, 0)
	var result PageResult
	cursor := initial
	skip := plan.Skip
	seekingOffset := skip > 0
	seen := make(map[string]struct{})
	for {
		if _, exists := seen[cursor.String()]; exists {
			return nil, zero, PageResult{}, fmt.Errorf("pagination cursor repeated: %s", cursor.String())
		}
		seen[cursor.String()] = struct{}{}

		batch, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, zero, PageResult{}, err
		}
		matched := make([]T, 0, len(batch))
		for _, value := range batch {
			keep, err := include(value)
			if err != nil {
				return nil, zero, PageResult{}, err
			}
			if keep {
				matched = append(matched, value)
			}
		}

		if skip >= len(matched) {
			skip -= len(matched)
			matched = nil
		} else if skip > 0 {
			matched = matched[skip:]
			skip = 0
		}
		if seekingOffset && skip == 0 && len(matched) > 0 {
			seekingOffset = false
		}
		if plan.Limit > 0 {
			remaining := plan.Limit - result.Returned
			if len(matched) > remaining {
				matched = matched[:remaining]
				result.HasMore = true
			}
		}
		if len(matched) > 0 {
			items = append(items, matched...)
			result.Returned += len(matched)
		}
		if plan.Limit > 0 && result.Returned >= plan.Limit {
			if !result.HasMore {
				result.HasMore = !next.IsZero()
			}
			return items, next, result, nil
		}
		if plan.OneBatch && !seekingOffset {
			if result.Returned > 0 || next.IsZero() {
				result.HasMore = !next.IsZero()
				return items, next, result, nil
			}
		}
		if next.IsZero() {
			return items, zero, result, nil
		}
		cursor = next
	}
}
