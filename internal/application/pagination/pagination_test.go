package pagination_test

import (
	"context"
	"errors"
	"testing"

	pagination "github.com/FlanChanXwO/pixiv-cli/internal/application/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCursor 构造一个可比较的 opaque cursor。
func testCursor(t *testing.T, name string) sdk.Cursor {
	t.Helper()
	c, err := sdk.NewCursor("test", "op", 1, "hash", []byte(name))
	require.NoError(t, err)
	return c
}

func cursorNames(cursors []sdk.Cursor) []string {
	names := make([]string, len(cursors))
	for i, cursor := range cursors {
		names[i] = cursor.String()
	}
	return names
}

func TestTraversePagesWithZeroLimitReadsEveryPage(t *testing.T) {
	second := testCursor(t, "second")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:     {items: []int{1, 2}, next: second},
		second: {items: []int{3}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"", second.String()}, cursorNames(cursors))
	assert.Equal(t, []int{1, 2, 3}, got)
	assert.Equal(t, pagination.PageResult{Returned: 3, HasMore: false}, result)
}

func TestTraversePagesSkipsAcrossEmptyAndWholeBatches(t *testing.T) {
	first := testCursor(t, "first")
	second := testCursor(t, "second")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:     {next: first},
		first:  {items: []int{1, 2}, next: second},
		second: {items: []int{3, 4}},
	}
	var got []int

	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{Skip: 3}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int{4}, got)
	assert.Equal(t, pagination.PageResult{Returned: 1}, result)
}

func TestTraversePagesTruncatesInsideBatchAndReportsMore(t *testing.T) {
	var got []int
	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{Limit: 2}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return []int{1, 2, 3}, sdk.Cursor{}, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, got)
	assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesAtExactLimitUsesNextCursorForHasMore(t *testing.T) {
	nextCursor := testCursor(t, "next")
	for _, test := range []struct {
		name string
		next sdk.Cursor
		more bool
	}{
		{name: "next cursor", next: nextCursor, more: true},
		{name: "end of results", more: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{Limit: 2}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
				return []int{1, 2}, test.next, nil
			}, func([]int) error { return nil })

			require.NoError(t, err)
			assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: test.more}, result)
		})
	}
}

func TestTraversePagesOneBatchSkipsLeadingEmptyBatches(t *testing.T) {
	empty2 := testCursor(t, "empty2")
	data := testCursor(t, "data")
	later := testCursor(t, "later")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:     {next: empty2},
		empty2: {next: data},
		data:   {items: []int{7, 8}, next: later},
		later:  {items: []int{9}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"", empty2.String(), data.String()}, cursorNames(cursors))
	assert.Equal(t, []int{7, 8}, got)
	assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesOneBatchEndsWhenOnlyEmptyBatchesRemain(t *testing.T) {
	empty := testCursor(t, "empty")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:    {next: empty},
		empty: {},
	}
	var cursors []sdk.Cursor
	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func([]int) error { return nil })

	require.NoError(t, err)
	assert.Equal(t, []string{"", empty.String()}, cursorNames(cursors))
	assert.Equal(t, pagination.PageResult{Returned: 0, HasMore: false}, result)
}

func TestTraversePagesLimitFillsAcrossEmptyBatches(t *testing.T) {
	empty := testCursor(t, "empty")
	more := testCursor(t, "more")
	tail := testCursor(t, "tail")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:    {items: []int{1}, next: empty},
		empty: {next: more},
		more:  {items: []int{2, 3}, next: tail},
		tail:  {items: []int{4}},
	}
	var cursors []sdk.Cursor
	var got []int
	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{Limit: 3}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"", empty.String(), more.String()}, cursorNames(cursors))
	assert.Equal(t, []int{1, 2, 3}, got)
	assert.Equal(t, pagination.PageResult{Returned: 3, HasMore: true}, result)
}

func TestTraversePagesOneBatchStopsAtFirstNonEmptyBatchAfterSkip(t *testing.T) {
	empty := testCursor(t, "empty")
	target := testCursor(t, "target")
	later := testCursor(t, "later")
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		{}:     {items: []int{1, 2}, next: empty},
		empty:  {next: target},
		target: {items: []int{3, 4}, next: later},
		later:  {items: []int{5}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{Skip: 2, OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"", empty.String(), target.String()}, cursorNames(cursors))
	assert.Equal(t, []int{3, 4}, got)
	assert.Equal(t, pagination.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesRejectsRepeatedCursorBeforeFetchingItAgain(t *testing.T) {
	repeat := testCursor(t, "repeat")
	unexpectedFetch := errors.New("repeated cursor was fetched")
	var cursors []sdk.Cursor

	_, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		if len(cursors) > 2 {
			return nil, sdk.Cursor{}, unexpectedFetch
		}
		return nil, repeat, nil
	}, func([]int) error { return nil })

	require.EqualError(t, err, `pagination cursor repeated: `+repeat.String())
	assert.Equal(t, []string{"", repeat.String()}, cursorNames(cursors))
}

func TestTraversePagesRejectsLongerCursorCycle(t *testing.T) {
	a := testCursor(t, "A")
	b := testCursor(t, "B")
	next := map[sdk.Cursor]sdk.Cursor{{}: a, a: b, b: a}
	var cursors []sdk.Cursor
	_, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		return nil, next[cursor], nil
	}, func([]int) error { return nil })

	require.EqualError(t, err, `pagination cursor repeated: `+a.String())
	assert.Equal(t, []string{"", a.String(), b.String()}, cursorNames(cursors))
}

func TestTraversePagesRejectsNegativePlanValuesBeforeFetch(t *testing.T) {
	for _, test := range []struct {
		name string
		plan pagination.PagePlan
		want string
	}{
		{name: "skip", plan: pagination.PagePlan{Skip: -1}, want: "page skip must be zero or positive"},
		{name: "limit", plan: pagination.PagePlan{Limit: -1}, want: "page limit must be zero or positive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fetchCalled := false
			_, err := pagination.TraversePages(context.Background(), test.plan, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
				fetchCalled = true
				return nil, sdk.Cursor{}, nil
			}, func([]int) error { return nil })

			require.EqualError(t, err, test.want)
			assert.False(t, fetchCalled)
		})
	}
}

func TestTraversePagesReturnsFetchErrorUnchanged(t *testing.T) {
	fetchErr := errors.New("fetch failed")
	_, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return nil, sdk.Cursor{}, fetchErr
	}, func([]int) error { return nil })

	require.ErrorIs(t, err, fetchErr)
}

func TestTraversePagesReturnsConsumeErrorUnchanged(t *testing.T) {
	next := testCursor(t, "next")
	consumeErr := errors.New("consume failed")
	_, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return []int{1}, next, nil
	}, func([]int) error { return consumeErr })

	require.ErrorIs(t, err, consumeErr)
}

func TestTraversePagesPassesCallerContextToEveryFetch(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "caller")
	next := testCursor(t, "next")
	var values []any

	_, err := pagination.TraversePages(ctx, pagination.PagePlan{}, func(fetchCtx context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		values = append(values, fetchCtx.Value(contextKey{}))
		if cursor.IsZero() {
			return nil, next, nil
		}
		return nil, sdk.Cursor{}, nil
	}, func([]int) error { return nil })

	require.NoError(t, err)
	assert.Equal(t, []any{"caller", "caller"}, values)
}

func TestCollectPagesReturnsNonNilEmptySliceOnSuccess(t *testing.T) {
	items, result, err := pagination.CollectPages(context.Background(), pagination.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return nil, sdk.Cursor{}, nil
	})

	require.NoError(t, err)
	require.NotNil(t, items)
	assert.Empty(t, items)
	assert.Equal(t, pagination.PageResult{}, result)
}

func TestCollectPagesDiscardsPartialItemsOnError(t *testing.T) {
	next := testCursor(t, "next")
	fetchErr := errors.New("second page failed")
	items, result, err := pagination.CollectPages(context.Background(), pagination.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		if cursor.IsZero() {
			return []int{1}, next, nil
		}
		return nil, sdk.Cursor{}, fetchErr
	})

	require.ErrorIs(t, err, fetchErr)
	assert.Nil(t, items)
	assert.Equal(t, pagination.PageResult{}, result)
}
