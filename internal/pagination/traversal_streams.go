package pagination

import (
	"context"
)

// StreamReadResult 是可重放聚合流的逻辑分页结果。State 由产品 owner
// 进一步编码为自己的 opaque cursor；traversal 不解释其中的 cursor。
type StreamReadResult[T any, C Cursor] struct {
	Items   []T
	State   StreamState[C]
	HasMore bool
}

// CollectStreamsWith 从所有流的零 cursor 开始，在 execute 的可重放边界内
// 收集一个统一逻辑页。每次 execute attempt 都从头重新收集，失败 attempt
// 的部分结果不会混入安全重放的成功结果。
func CollectStreamsWith[E any, T any, C Cursor](ctx context.Context, execute Execute[E], plan PagePlan, streams func(E) []Stream[T, C]) (StreamReadResult[T, C], error) {
	var initial StreamState[C]
	result := StreamReadResult[T, C]{Items: make([]T, 0), State: initial}
	if execute == nil {
		return result, ErrExecuteNotConfigured
	}

	err := execute(ctx, func(attemptCtx context.Context, client E) (bool, error) {
		result.Items = result.Items[:0]
		result.State = initial
		result.HasMore = false

		items, state, pageResult, err := collectStreamsFrom(attemptCtx, plan, streams(client), initial)
		if err != nil {
			return false, err
		}
		result.Items = items
		result.State = state
		result.HasMore = pageResult.HasMore
		return false, nil
	})
	return result, err
}
