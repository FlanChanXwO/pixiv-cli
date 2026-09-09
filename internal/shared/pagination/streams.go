package pagination

import (
	"context"
	"errors"
	"fmt"
)

// Stream 描述一个保持自身 upstream 顺序的实体流。多个 Stream 会按切片顺序
// 连接成一个逻辑序列；Include、Skip 和 Limit 都在连接序列上统一生效。
// Checkpoint 必须把源批次的输入 cursor 和已消费位置编码为可继续的 cursor，
// 以便逻辑页在批内截断时不会丢失余项。
type Stream[T any, C Cursor] struct {
	Fetch      func(context.Context, C) ([]T, C, error)
	Include    func(T) (bool, error)
	Checkpoint func(C, int) (C, error)
}

// StreamState 是聚合流的可恢复位置。Current 指向下一次读取的流，Cursors
// 保存每个流各自的 continuation；已完成的流使用零 cursor。它只承载共享
// 算法需要的结构，不负责产品 cursor 的编码或 binding。
type StreamState[C Cursor] struct {
	Current int
	Cursors []C
}

// CollectStreams 从所有流的零 cursor 开始，按顺序收集一个统一逻辑页。
func CollectStreams[T any, C Cursor](ctx context.Context, plan PagePlan, streams []Stream[T, C]) ([]T, StreamState[C], PageResult, error) {
	var initial StreamState[C]
	return CollectStreamsFrom(ctx, plan, streams, initial)
}

// CollectStreamsFrom 从调用方提供的聚合位置开始收集一个统一逻辑页。
//
// 当 Limit 在源批次内部截断时，返回的 StreamState 会保留当前流的
// Checkpoint cursor；当当前流结束而仍有后续流时，位置推进到下一流的零
// cursor。任一流失败都会丢弃整个逻辑页的部分结果。
func CollectStreamsFrom[T any, C Cursor](ctx context.Context, plan PagePlan, streams []Stream[T, C], initial StreamState[C]) ([]T, StreamState[C], PageResult, error) {
	var zeroState StreamState[C]
	if plan.Skip < 0 {
		return nil, zeroState, PageResult{}, errors.New("page skip must be zero or positive")
	}
	if plan.Limit < 0 {
		return nil, zeroState, PageResult{}, errors.New("page limit must be zero or positive")
	}
	if initial.Current < 0 || initial.Current > len(streams) {
		return nil, zeroState, PageResult{}, errors.New("stream state current index is out of range")
	}
	if len(initial.Cursors) > 0 && len(initial.Cursors) != len(streams) {
		return nil, zeroState, PageResult{}, errors.New("stream state cursor count does not match stream count")
	}
	for index, stream := range streams {
		if stream.Fetch == nil {
			return nil, zeroState, PageResult{}, fmt.Errorf("stream %d fetch is required", index)
		}
		if stream.Checkpoint == nil {
			return nil, zeroState, PageResult{}, fmt.Errorf("stream %d checkpoint is required", index)
		}
	}

	cursors := append([]C(nil), initial.Cursors...)
	if len(cursors) == 0 {
		cursors = make([]C, len(streams))
	}
	state := StreamState[C]{Current: initial.Current, Cursors: cursors}
	items := make([]T, 0)
	result := PageResult{}
	skip := plan.Skip
	seekingOffset := skip > 0

	for state.Current < len(streams) {
		streamIndex := state.Current
		stream := streams[streamIndex]
		cursor := state.Cursors[streamIndex]
		seen := make(map[string]struct{})

		for {
			if err := ctx.Err(); err != nil {
				return nil, zeroState, PageResult{}, err
			}
			if _, exists := seen[cursor.String()]; exists {
				return nil, zeroState, PageResult{}, fmt.Errorf("pagination stream %d cursor repeated: %s", streamIndex, cursor.String())
			}
			seen[cursor.String()] = struct{}{}

			batch, next, err := stream.Fetch(ctx, cursor)
			if err != nil {
				return nil, zeroState, PageResult{}, err
			}
			matched := make([]T, 0, len(batch))
			positions := make([]int, 0, len(batch))
			for index, value := range batch {
				keep := true
				if stream.Include != nil {
					keep, err = stream.Include(value)
					if err != nil {
						return nil, zeroState, PageResult{}, err
					}
				}
				if keep {
					matched = append(matched, value)
					positions = append(positions, index+1)
				}
			}

			if skip >= len(matched) {
				skip -= len(matched)
				matched = nil
				positions = nil
			} else if skip > 0 {
				matched = matched[skip:]
				positions = positions[skip:]
				skip = 0
			}
			if seekingOffset && skip == 0 && len(matched) > 0 {
				seekingOffset = false
			}
			batchReturned := len(matched) > 0

			if plan.Limit > 0 {
				remaining := plan.Limit - result.Returned
				if len(matched) > remaining {
					checkpoint, err := stream.Checkpoint(cursor, positions[remaining-1])
					if err != nil {
						return nil, zeroState, PageResult{}, err
					}
					if checkpoint.IsZero() {
						return nil, zeroState, PageResult{}, errors.New("stream checkpoint cursor must not be zero")
					}
					matched = matched[:remaining]
					state.Cursors[streamIndex] = checkpoint
					result.HasMore = true
				}
			}

			if len(matched) > 0 {
				items = append(items, matched...)
				result.Returned += len(matched)
			}

			if plan.Limit > 0 && result.Returned >= plan.Limit {
				if result.HasMore {
					state.Current = streamIndex
					return items, state, result, nil
				}
				state.Cursors[streamIndex] = next
				if !next.IsZero() {
					state.Current = streamIndex
					result.HasMore = true
					return items, state, result, nil
				}
				state.Current = streamIndex + 1
				result.HasMore = state.Current < len(streams)
				return items, state, result, nil
			}

			// OneBatch 只在当前流找到第一个有匹配内容的源批次后停止；
			// 没有匹配内容的流会自然推进到下一个流，不为填充结果额外取批。
			if plan.OneBatch && !seekingOffset && batchReturned {
				state.Cursors[streamIndex] = next
				if next.IsZero() {
					state.Current = streamIndex + 1
					result.HasMore = state.Current < len(streams)
				} else {
					state.Current = streamIndex
					result.HasMore = true
				}
				return items, state, result, nil
			}

			if next.IsZero() {
				state.Cursors[streamIndex] = next
				state.Current = streamIndex + 1
				break
			}
			state.Cursors[streamIndex] = next
			cursor = next
		}
	}

	return items, state, result, nil
}
