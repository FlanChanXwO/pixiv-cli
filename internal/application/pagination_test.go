package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraversePagesWithZeroLimitReadsEveryPage(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":       {items: []int{1, 2}, next: "second"},
		"second": {items: []int{3}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := application.TraversePages(context.Background(), application.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []sdk.Cursor{"", "second"}, cursors)
	assert.Equal(t, []int{1, 2, 3}, got)
	assert.Equal(t, application.PageResult{Returned: 3, HasMore: false}, result)
}

func TestTraversePagesSkipsAcrossEmptyAndWholeBatches(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":       {next: "first"},
		"first":  {items: []int{1, 2}, next: "second"},
		"second": {items: []int{3, 4}},
	}
	var got []int

	result, err := application.TraversePages(context.Background(), application.PagePlan{Skip: 3}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int{4}, got)
	assert.Equal(t, application.PageResult{Returned: 1}, result)
}

func TestTraversePagesTruncatesInsideBatchAndReportsMore(t *testing.T) {
	var got []int
	result, err := application.TraversePages(context.Background(), application.PagePlan{Limit: 2}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return []int{1, 2, 3}, "", nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, got)
	assert.Equal(t, application.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesAtExactLimitUsesNextCursorForHasMore(t *testing.T) {
	for _, test := range []struct {
		name string
		next sdk.Cursor
		more bool
	}{
		{name: "next cursor", next: "next", more: true},
		{name: "end of results", more: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := application.TraversePages(context.Background(), application.PagePlan{Limit: 2}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
				return []int{1, 2}, test.next, nil
			}, func([]int) error { return nil })

			require.NoError(t, err)
			assert.Equal(t, application.PageResult{Returned: 2, HasMore: test.more}, result)
		})
	}
}

func TestTraversePagesOneBatchSkipsLeadingEmptyBatches(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":       {next: "empty2"},
		"empty2": {next: "data"},
		"data":   {items: []int{7, 8}, next: "later"},
		"later":  {items: []int{9}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := application.TraversePages(context.Background(), application.PagePlan{OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []sdk.Cursor{"", "empty2", "data"}, cursors)
	assert.Equal(t, []int{7, 8}, got)
	assert.Equal(t, application.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesOneBatchEndsWhenOnlyEmptyBatchesRemain(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":      {next: "empty"},
		"empty": {},
	}
	var cursors []sdk.Cursor
	result, err := application.TraversePages(context.Background(), application.PagePlan{OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func([]int) error { return nil })

	require.NoError(t, err)
	assert.Equal(t, []sdk.Cursor{"", "empty"}, cursors)
	assert.Equal(t, application.PageResult{Returned: 0, HasMore: false}, result)
}

func TestTraversePagesLimitFillsAcrossEmptyBatches(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":      {items: []int{1}, next: "empty"},
		"empty": {next: "more"},
		"more":  {items: []int{2, 3}, next: "tail"},
		"tail":  {items: []int{4}},
	}
	var cursors []sdk.Cursor
	var got []int
	result, err := application.TraversePages(context.Background(), application.PagePlan{Limit: 3}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []sdk.Cursor{"", "empty", "more"}, cursors)
	assert.Equal(t, []int{1, 2, 3}, got)
	assert.Equal(t, application.PageResult{Returned: 3, HasMore: true}, result)
}

func TestTraversePagesOneBatchStopsAtFirstNonEmptyBatchAfterSkip(t *testing.T) {
	pages := map[sdk.Cursor]struct {
		items []int
		next  sdk.Cursor
	}{
		"":       {items: []int{1, 2}, next: "empty"},
		"empty":  {next: "target"},
		"target": {items: []int{3, 4}, next: "later"},
		"later":  {items: []int{5}},
	}
	var cursors []sdk.Cursor
	var got []int

	result, err := application.TraversePages(context.Background(), application.PagePlan{Skip: 2, OneBatch: true}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		page := pages[cursor]
		return page.items, page.next, nil
	}, func(items []int) error {
		got = append(got, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []sdk.Cursor{"", "empty", "target"}, cursors)
	assert.Equal(t, []int{3, 4}, got)
	assert.Equal(t, application.PageResult{Returned: 2, HasMore: true}, result)
}

func TestTraversePagesRejectsRepeatedCursorBeforeFetchingItAgain(t *testing.T) {
	unexpectedFetch := errors.New("repeated cursor was fetched")
	var cursors []sdk.Cursor

	_, err := application.TraversePages(context.Background(), application.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		if len(cursors) > 2 {
			return nil, "", unexpectedFetch
		}
		if cursor == "" {
			return nil, "repeat", nil
		}
		return nil, "repeat", nil
	}, func([]int) error { return nil })

	require.EqualError(t, err, `pagination cursor repeated: "repeat"`)
	assert.Equal(t, []sdk.Cursor{"", "repeat"}, cursors)
}

func TestTraversePagesRejectsLongerCursorCycle(t *testing.T) {
	next := map[sdk.Cursor]sdk.Cursor{"": "A", "A": "B", "B": "A"}
	var cursors []sdk.Cursor
	_, err := application.TraversePages(context.Background(), application.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		cursors = append(cursors, cursor)
		return nil, next[cursor], nil
	}, func([]int) error { return nil })

	require.EqualError(t, err, `pagination cursor repeated: "A"`)
	assert.Equal(t, []sdk.Cursor{"", "A", "B"}, cursors)
}

func TestTraversePagesRejectsNegativePlanValuesBeforeFetch(t *testing.T) {
	for _, test := range []struct {
		name string
		plan application.PagePlan
		want string
	}{
		{name: "skip", plan: application.PagePlan{Skip: -1}, want: "page skip must be zero or positive"},
		{name: "limit", plan: application.PagePlan{Limit: -1}, want: "page limit must be zero or positive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fetchCalled := false
			_, err := application.TraversePages(context.Background(), test.plan, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
				fetchCalled = true
				return nil, "", nil
			}, func([]int) error { return nil })

			require.EqualError(t, err, test.want)
			assert.False(t, fetchCalled)
		})
	}
}

func TestTraversePagesReturnsFetchErrorUnchanged(t *testing.T) {
	fetchErr := errors.New("fetch failed")
	_, err := application.TraversePages(context.Background(), application.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return nil, "", fetchErr
	}, func([]int) error { return nil })

	require.ErrorIs(t, err, fetchErr)
}

func TestTraversePagesReturnsConsumeErrorUnchanged(t *testing.T) {
	consumeErr := errors.New("consume failed")
	_, err := application.TraversePages(context.Background(), application.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return []int{1}, "next", nil
	}, func([]int) error { return consumeErr })

	require.ErrorIs(t, err, consumeErr)
}

func TestTraversePagesPassesCallerContextToEveryFetch(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "caller")
	var values []any

	_, err := application.TraversePages(ctx, application.PagePlan{}, func(fetchCtx context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		values = append(values, fetchCtx.Value(contextKey{}))
		if cursor == "" {
			return nil, "next", nil
		}
		return nil, "", nil
	}, func([]int) error { return nil })

	require.NoError(t, err)
	assert.Equal(t, []any{"caller", "caller"}, values)
}

func TestCollectPagesReturnsNonNilEmptySliceOnSuccess(t *testing.T) {
	items, result, err := application.CollectPages(context.Background(), application.PagePlan{}, func(_ context.Context, _ sdk.Cursor) ([]int, sdk.Cursor, error) {
		return nil, "", nil
	})

	require.NoError(t, err)
	require.NotNil(t, items)
	assert.Empty(t, items)
	assert.Equal(t, application.PageResult{}, result)
}

func TestCollectPagesDiscardsPartialItemsOnError(t *testing.T) {
	fetchErr := errors.New("second page failed")
	items, result, err := application.CollectPages(context.Background(), application.PagePlan{}, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		if cursor == "" {
			return []int{1}, "next", nil
		}
		return nil, "", fetchErr
	})

	require.ErrorIs(t, err, fetchErr)
	assert.Nil(t, items)
	assert.Equal(t, application.PageResult{}, result)
}
