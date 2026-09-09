package pagination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectStreamsAppliesOneLogicalBudgetAcrossStreams(t *testing.T) {
	stream0Next := fakeCursor{value: "stream-0-next"}
	stream1Remainder := fakeCursor{value: "stream-1-remainder"}
	streams := []pagination.Stream[int, fakeCursor]{
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				switch cursor.value {
				case "":
					return []int{1, 2}, stream0Next, nil
				case stream0Next.value:
					return []int{3}, fakeCursor{}, nil
				default:
					t.Fatalf("unexpected stream 0 cursor %q", cursor.value)
					return nil, fakeCursor{}, nil
				}
			},
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("stream 0 must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				switch cursor.value {
				case "":
					return []int{4, 5}, fakeCursor{}, nil
				case stream1Remainder.value:
					return []int{5}, fakeCursor{}, nil
				default:
					t.Fatalf("unexpected stream 1 cursor %q", cursor.value)
					return nil, fakeCursor{}, nil
				}
			},
			Checkpoint: func(cursor fakeCursor, consumed int) (fakeCursor, error) {
				require.True(t, cursor.IsZero())
				require.Equal(t, 1, consumed)
				return stream1Remainder, nil
			},
		},
	}

	items, state, result, err := pagination.CollectStreamsFrom(
		context.Background(),
		pagination.PagePlan{Skip: 1, Limit: 3},
		streams,
		pagination.StreamState[fakeCursor]{},
	)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3, 4}, items)
	assert.Equal(t, pagination.PageResult{Returned: 3, HasMore: true}, result)
	assert.Equal(t, 1, state.Current)
	assert.Equal(t, []fakeCursor{{}, stream1Remainder}, state.Cursors)

	remainder, finalState, finalResult, err := pagination.CollectStreamsFrom(
		context.Background(),
		pagination.PagePlan{},
		streams,
		state,
	)
	require.NoError(t, err)
	assert.Equal(t, []int{5}, remainder)
	assert.Equal(t, pagination.StreamState[fakeCursor]{Current: 2, Cursors: []fakeCursor{{}, {}}}, finalState)
	assert.Equal(t, pagination.PageResult{Returned: 1}, finalResult)
}

func TestCollectStreamsOneBatchSkipsEmptyStreamsWithoutFetchingAhead(t *testing.T) {
	var stream0Calls, stream1Calls int
	stream1Next := fakeCursor{value: "stream-1-next"}
	streams := []pagination.Stream[int, fakeCursor]{
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				stream0Calls++
				if !cursor.IsZero() {
					t.Fatalf("unexpected stream 0 cursor %q", cursor.value)
				}
				return []int{}, fakeCursor{}, nil
			},
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("empty stream must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				stream1Calls++
				if !cursor.IsZero() {
					t.Fatalf("unexpected stream 1 cursor %q", cursor.value)
				}
				return []int{7, 8}, stream1Next, nil
			},
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("one-batch result must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
	}

	items, state, result, err := pagination.CollectStreams(
		context.Background(),
		pagination.PagePlan{OneBatch: true},
		streams,
	)
	require.NoError(t, err)
	assert.Equal(t, []int{7, 8}, items)
	assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: true}, result)
	assert.Equal(t, pagination.StreamState[fakeCursor]{Current: 1, Cursors: []fakeCursor{{}, stream1Next}}, state)
	assert.Equal(t, 1, stream0Calls)
	assert.Equal(t, 1, stream1Calls)
}

func TestCollectStreamsFiltersBeforeGlobalSkipAndLimit(t *testing.T) {
	streams := []pagination.Stream[int, fakeCursor]{
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				if !cursor.IsZero() {
					t.Fatalf("unexpected stream 0 cursor %q", cursor.value)
				}
				return []int{1, 2}, fakeCursor{}, nil
			},
			Include: func(value int) (bool, error) { return value%2 == 0, nil },
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("stream 0 must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				if !cursor.IsZero() {
					t.Fatalf("unexpected stream 1 cursor %q", cursor.value)
				}
				return []int{3, 4}, fakeCursor{}, nil
			},
			Include: func(value int) (bool, error) { return value%2 == 0, nil },
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("stream 1 must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
	}

	items, state, result, err := pagination.CollectStreamsFrom(
		context.Background(),
		pagination.PagePlan{Skip: 1, Limit: 1},
		streams,
		pagination.StreamState[fakeCursor]{},
	)
	require.NoError(t, err)
	assert.Equal(t, []int{4}, items)
	assert.Equal(t, pagination.PageResult{Returned: 1}, result)
	assert.Equal(t, 2, state.Current)
	assert.Equal(t, []fakeCursor{{}, {}}, state.Cursors)
}

func TestCollectStreamsFailureDiscardsEveryPartialStream(t *testing.T) {
	boom := errors.New("second stream failed")
	streams := []pagination.Stream[int, fakeCursor]{
		{
			Fetch: func(_ context.Context, _ fakeCursor) ([]int, fakeCursor, error) {
				return []int{1}, fakeCursor{}, nil
			},
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				t.Fatal("stream 0 must not be checkpointed")
				return fakeCursor{}, nil
			},
		},
		{
			Fetch: func(_ context.Context, _ fakeCursor) ([]int, fakeCursor, error) {
				return nil, fakeCursor{}, boom
			},
			Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
				return fakeCursor{}, nil
			},
		},
	}

	items, state, result, err := pagination.CollectStreams(
		context.Background(),
		pagination.PagePlan{},
		streams,
	)
	require.ErrorIs(t, err, boom)
	assert.Nil(t, items)
	assert.Equal(t, pagination.StreamState[fakeCursor]{}, state)
	assert.Equal(t, pagination.PageResult{}, result)
}

func TestCollectStreamsCheckpointsLastBatchRemainder(t *testing.T) {
	remainder := fakeCursor{value: "remainder"}
	streams := []pagination.Stream[int, fakeCursor]{
		{
			Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
				switch cursor.value {
				case "":
					return []int{1, 2, 3}, fakeCursor{}, nil
				case remainder.value:
					return []int{3}, fakeCursor{}, nil
				default:
					t.Fatalf("unexpected cursor %q", cursor.value)
					return nil, fakeCursor{}, nil
				}
			},
			Checkpoint: func(cursor fakeCursor, consumed int) (fakeCursor, error) {
				require.True(t, cursor.IsZero())
				require.Equal(t, 2, consumed)
				return remainder, nil
			},
		},
	}

	items, state, result, err := pagination.CollectStreams(
		context.Background(),
		pagination.PagePlan{Limit: 2},
		streams,
	)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, items)
	assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: true}, result)
	assert.Equal(t, pagination.StreamState[fakeCursor]{Current: 0, Cursors: []fakeCursor{remainder}}, state)

	items, state, result, err = pagination.CollectStreamsFrom(context.Background(), pagination.PagePlan{}, streams, state)
	require.NoError(t, err)
	assert.Equal(t, []int{3}, items)
	assert.Equal(t, pagination.StreamState[fakeCursor]{Current: 1, Cursors: []fakeCursor{{}}}, state)
	assert.Equal(t, pagination.PageResult{Returned: 1}, result)
}
