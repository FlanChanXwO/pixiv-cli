package pagination

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// PagePlan 描述与传输协议无关的逻辑分页请求。Limit 为 0 时遍历全部结果。
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

// TraversePages 跟随 SDK opaque cursor，并把每批结果交给 consume。
func TraversePages[T any](ctx context.Context, plan PagePlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) (PageResult, error) {
	var result PageResult
	if plan.Skip < 0 {
		return result, errors.New("page skip must be zero or positive")
	}
	if plan.Limit < 0 {
		return result, errors.New("page limit must be zero or positive")
	}
	var cursor sdk.Cursor
	seen := make(map[string]struct{})
	skip := plan.Skip
	seekingOffset := skip > 0
	for {
		// 按 cursor 的文本编码判断重复，避免不透明 Cursor 的指针身份差异漏判。
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
		// OneBatch 表示“一个逻辑批次”：本地筛选后的连续空上游批次要跳过，
		// 直到拿到首个非空逻辑结果或真正结束；不得在空批上提前停。
		if plan.OneBatch && !seekingOffset {
			if result.Returned > 0 || next.IsZero() {
				result.HasMore = !next.IsZero()
				return result, nil
			}
			// 空批且仍有上游：继续补拉。
		}
		if next.IsZero() {
			return result, nil
		}
		cursor = next
	}
}

// CollectPages 使用共享遍历引擎收集结果；失败时丢弃已经收集的部分结果。
func CollectPages[T any](ctx context.Context, plan PagePlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, PageResult, error) {
	items := make([]T, 0)
	result, err := TraversePages(ctx, plan, fetch, func(page []T) error {
		items = append(items, page...)
		return nil
	})
	if err != nil {
		return nil, PageResult{}, err
	}
	return items, result, nil
}
