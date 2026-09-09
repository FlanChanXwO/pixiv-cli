package traversal

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
)

// ErrStreamFactoryNotConfigured 表示调用方没有提供聚合流构造函数。
var ErrStreamFactoryNotConfigured = errors.New("traversal: stream factory is not configured")

// StreamReadResult 是可重放聚合流的逻辑分页结果。State 由产品 owner
// 进一步编码为自己的 opaque cursor；traversal 不解释其中的 cursor。
type StreamReadResult[T any, C pagination.Cursor] struct {
	Items   []T
	State   pagination.StreamState[C]
	HasMore bool
}

// CollectStreamsWith 从所有流的零 cursor 开始，在 execute 的可重放边界内
// 收集一个统一逻辑页。
func CollectStreamsWith[E any, T any, C pagination.Cursor](ctx context.Context, execute Execute[E], plan pagination.PagePlan, streams func(E) []pagination.Stream[T, C]) (StreamReadResult[T, C], error) {
	var initial pagination.StreamState[C]
	return CollectStreamsWithFrom(ctx, execute, plan, initial, streams)
}

// CollectStreamsWithFrom 与 CollectStreamsWith 相同，但从调用方已有的聚合
// 位置开始。每次 execute attempt 都从同一个 initial state 重新收集，失败
// attempt 的部分结果不会混入安全重放的成功结果。
func CollectStreamsWithFrom[E any, T any, C pagination.Cursor](ctx context.Context, execute Execute[E], plan pagination.PagePlan, initial pagination.StreamState[C], streams func(E) []pagination.Stream[T, C]) (StreamReadResult[T, C], error) {
	result := StreamReadResult[T, C]{Items: make([]T, 0), State: initial}
	if execute == nil {
		return result, ErrExecuteNotConfigured
	}
	if streams == nil {
		return result, ErrStreamFactoryNotConfigured
	}

	err := execute(ctx, func(attemptCtx context.Context, client E) (bool, error) {
		result.Items = result.Items[:0]
		result.State = initial
		result.HasMore = false

		items, state, pageResult, err := pagination.CollectStreamsFrom(attemptCtx, plan, streams(client), initial)
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
