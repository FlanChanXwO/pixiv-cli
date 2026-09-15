package traversal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCursor struct {
	value string
}

func (c fakeCursor) IsZero() bool   { return c.value == "" }
func (c fakeCursor) String() string { return c.value }

func TestCollectStreamsWithClearsUncommittedResultsBeforeReplay(t *testing.T) {
	firstNext := fakeCursor{value: "first-next"}
	var attempts int
	execute := traversal.Execute[int](func(ctx context.Context, use func(context.Context, int) (bool, error)) error {
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

	result, err := traversal.CollectStreamsWith(
		context.Background(),
		execute,
		pagination.PagePlan{},
		func(client int) []pagination.Stream[int, fakeCursor] {
			return []pagination.Stream[int, fakeCursor]{
				{
					Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
						if client == 1 {
							switch cursor.value {
							case "":
								return []int{1}, firstNext, nil
							case firstNext.value:
								return nil, fakeCursor{}, errors.New("first attempt failed")
							}
						}
						if !cursor.IsZero() {
							t.Fatalf("replay must restart at zero cursor, got %q", cursor.value)
						}
						return []int{2}, fakeCursor{}, nil
					},
					Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
						return fakeCursor{}, nil
					},
				},
			}
		},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{2}, result.Items)
	assert.False(t, result.HasMore)
	assert.Equal(t, 1, result.State.Current)
	assert.Equal(t, []fakeCursor{{}}, result.State.Cursors)
}

func TestCollectStreamsWithFromStartsAtAggregateState(t *testing.T) {
	remainder := fakeCursor{value: "remainder"}
	initial := pagination.StreamState[fakeCursor]{Current: 0, Cursors: []fakeCursor{remainder, {}}}

	result, err := traversal.CollectStreamsWithFrom(
		context.Background(),
		traversal.Execute[struct{}](func(ctx context.Context, use func(context.Context, struct{}) (bool, error)) error {
			committed, err := use(ctx, struct{}{})
			require.False(t, committed)
			return err
		}),
		pagination.PagePlan{},
		initial,
		func(_ struct{}) []pagination.Stream[int, fakeCursor] {
			return []pagination.Stream[int, fakeCursor]{
				{
					Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
						if cursor != remainder {
							t.Fatalf("fetch cursor = %#v, want %#v", cursor, remainder)
						}
						return []int{3}, fakeCursor{}, nil
					},
					Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
						return fakeCursor{}, nil
					},
				},
				{
					Fetch: func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
						if !cursor.IsZero() {
							t.Fatalf("second stream fetch cursor = %#v, want zero", cursor)
						}
						return []int{4}, fakeCursor{}, nil
					},
					Checkpoint: func(fakeCursor, int) (fakeCursor, error) {
						return fakeCursor{}, nil
					},
				},
			}
		},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{3, 4}, result.Items)
	assert.Equal(t, pagination.StreamState[fakeCursor]{Current: 2, Cursors: []fakeCursor{{}, {}}}, result.State)
}
