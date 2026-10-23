package pagination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type traversalCursor struct {
	value string
}

func (c traversalCursor) IsZero() bool   { return c.value == "" }
func (c traversalCursor) String() string { return c.value }

func TestCollectStreamsWithClearsUncommittedResultsBeforeReplay(t *testing.T) {
	firstNext := traversalCursor{value: "first-next"}
	var attempts int
	execute := pagination.Execute[int](func(ctx context.Context, use func(context.Context, int) (bool, error)) error {
		attempts++
		committed, err := use(ctx, attempts)
		require.False(t, committed)
		if attempts == 1 {
			require.Error(t, err)
			committed, err = use(ctx, 2)
			require.False(t, committed)
		}
		return err
	})

	result, err := pagination.CollectStreamsWith(
		context.Background(),
		execute,
		pagination.PagePlan{},
		func(client int) []pagination.Stream[int, traversalCursor] {
			return []pagination.Stream[int, traversalCursor]{
				{
					Fetch: func(_ context.Context, cursor traversalCursor) ([]int, traversalCursor, error) {
						if client == 1 {
							switch cursor.value {
							case "":
								return []int{1}, firstNext, nil
							case firstNext.value:
								return nil, traversalCursor{}, errors.New("first attempt failed")
							}
						}
						if !cursor.IsZero() {
							t.Fatalf("replay must restart at zero cursor, got %q", cursor.value)
						}
						return []int{2}, traversalCursor{}, nil
					},
					Checkpoint: func(traversalCursor, int) (traversalCursor, error) {
						return traversalCursor{}, nil
					},
				},
			}
		},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{2}, result.Items)
	assert.False(t, result.HasMore)
	assert.Equal(t, 1, result.State.Current)
	assert.Equal(t, []traversalCursor{{}}, result.State.Cursors)
}

func TestCollectStreamsWithFromStartsAtAggregateState(t *testing.T) {
	remainder := traversalCursor{value: "remainder"}
	initial := pagination.StreamState[traversalCursor]{Current: 0, Cursors: []traversalCursor{remainder, {}}}

	result, err := pagination.CollectStreamsWithFrom(
		context.Background(),
		pagination.Execute[struct{}](func(ctx context.Context, use func(context.Context, struct{}) (bool, error)) error {
			committed, err := use(ctx, struct{}{})
			require.False(t, committed)
			return err
		}),
		pagination.PagePlan{},
		initial,
		func(_ struct{}) []pagination.Stream[int, traversalCursor] {
			return []pagination.Stream[int, traversalCursor]{
				{
					Fetch: func(_ context.Context, cursor traversalCursor) ([]int, traversalCursor, error) {
						if cursor != remainder {
							t.Fatalf("fetch cursor = %#v, want %#v", cursor, remainder)
						}
						return []int{3}, traversalCursor{}, nil
					},
					Checkpoint: func(traversalCursor, int) (traversalCursor, error) {
						return traversalCursor{}, nil
					},
				},
				{
					Fetch: func(_ context.Context, cursor traversalCursor) ([]int, traversalCursor, error) {
						if !cursor.IsZero() {
							t.Fatalf("second stream fetch cursor = %#v, want zero", cursor)
						}
						return []int{4}, traversalCursor{}, nil
					},
					Checkpoint: func(traversalCursor, int) (traversalCursor, error) {
						return traversalCursor{}, nil
					},
				},
			}
		},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{3, 4}, result.Items)
	assert.Equal(t, pagination.StreamState[traversalCursor]{Current: 2, Cursors: []traversalCursor{{}, {}}}, result.State)
}
