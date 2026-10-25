package pagination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
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

// fakeCursor 证明分页算法只依赖稳定文本与零值语义，不依赖 Pixiv SDK。
type fakeCursor struct {
	value string
}

func (c fakeCursor) IsZero() bool   { return c.value == "" }
func (c fakeCursor) String() string { return c.value }

func TestTraversePagesAcceptsGenericCursor(t *testing.T) {
	next := fakeCursor{value: "next"}
	var visited []fakeCursor
	var values []int

	result, err := pagination.TraversePages(context.Background(), pagination.PagePlan{OneBatch: true}, func(_ context.Context, cursor fakeCursor) ([]int, fakeCursor, error) {
		visited = append(visited, cursor)
		if cursor.IsZero() {
			return []int{1}, next, nil
		}
		return []int{2}, fakeCursor{}, nil
	}, func(items []int) error {
		values = append(values, items...)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []fakeCursor{{}}, visited)
	assert.Equal(t, []int{1}, values)
	assert.Equal(t, pagination.PageResult{Returned: 1, HasMore: true}, result)
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

// 一个逻辑批次必须跨过前导空批次，但批次内的 Skip 只消耗当前批次。
func TestTraversePagesOneBatchAndLimitAcrossEmptyBatches(t *testing.T) {
	empty := testCursor(t, "empty")
	data := testCursor(t, "data")
	tail := testCursor(t, "tail")
	type page struct {
		items []int
		next  sdk.Cursor
	}
	dataPages := map[sdk.Cursor]page{
		{}:    {next: empty},
		empty: {next: data},
		data:  {items: []int{7, 8}, next: tail},
		tail:  {items: []int{9}},
	}
	emptyPages := map[sdk.Cursor]page{
		{}:    {next: empty},
		empty: {},
	}
	for _, test := range []struct {
		name        string
		plan        pagination.PagePlan
		pages       map[sdk.Cursor]page
		wantCursors []string
		wantItems   []int
		want        pagination.PageResult
	}{
		{
			name:        "one batch skips leading empty batches",
			plan:        pagination.PagePlan{OneBatch: true},
			pages:       dataPages,
			wantCursors: []string{"", empty.String(), data.String()},
			wantItems:   []int{7, 8},
			want:        pagination.PageResult{Returned: 2, HasMore: true},
		},
		{
			name:        "one batch stops when only empty batches remain",
			plan:        pagination.PagePlan{OneBatch: true},
			pages:       emptyPages,
			wantCursors: []string{"", empty.String()},
			want:        pagination.PageResult{},
		},
		{
			name:        "limit fills across empty batches",
			plan:        pagination.PagePlan{Limit: 3},
			pages:       dataPages,
			wantCursors: []string{"", empty.String(), data.String(), tail.String()},
			wantItems:   []int{7, 8, 9},
			want:        pagination.PageResult{Returned: 3},
		},
		{
			name:        "one batch after skip stops at first non-empty batch",
			plan:        pagination.PagePlan{Skip: 1, OneBatch: true},
			pages:       dataPages,
			wantCursors: []string{"", empty.String(), data.String()},
			wantItems:   []int{8},
			want:        pagination.PageResult{Returned: 1, HasMore: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cursors []sdk.Cursor
			var got []int
			result, err := pagination.TraversePages(context.Background(), test.plan, func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
				cursors = append(cursors, cursor)
				current := test.pages[cursor]
				return current.items, current.next, nil
			}, func(items []int) error {
				got = append(got, items...)
				return nil
			})

			require.NoError(t, err)
			assert.Equal(t, test.wantCursors, cursorNames(cursors))
			assert.Equal(t, test.wantItems, got)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestTraversePagesRejectsCursorCycleBeforeFetchingItAgain(t *testing.T) {
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

func TestTraversePagesPropagatesFetchAndConsumeErrorsUnchanged(t *testing.T) {
	fetchErr := errors.New("fetch failed")
	consumeErr := errors.New("consume failed")
	next := testCursor(t, "next")
	for _, test := range []struct {
		name    string
		fetch   func(context.Context, sdk.Cursor) ([]int, sdk.Cursor, error)
		consume func([]int) error
		want    error
	}{
		{
			name: "fetch",
			fetch: func(context.Context, sdk.Cursor) ([]int, sdk.Cursor, error) {
				return nil, sdk.Cursor{}, fetchErr
			},
			consume: func([]int) error { return nil },
			want:    fetchErr,
		},
		{
			name: "consume",
			fetch: func(context.Context, sdk.Cursor) ([]int, sdk.Cursor, error) {
				return []int{1}, next, nil
			},
			consume: func([]int) error { return consumeErr },
			want:    consumeErr,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pagination.TraversePages(context.Background(), pagination.PagePlan{}, test.fetch, test.consume)
			require.ErrorIs(t, err, test.want)
		})
	}
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

func TestCollectPagesEmptyAndPartialFailureContracts(t *testing.T) {
	next := testCursor(t, "next")
	fetchErr := errors.New("second page failed")
	for _, test := range []struct {
		name  string
		fetch func(context.Context, sdk.Cursor) ([]int, sdk.Cursor, error)
		want  error
	}{
		{
			name: "empty success returns non-nil slice",
			fetch: func(context.Context, sdk.Cursor) ([]int, sdk.Cursor, error) {
				return nil, sdk.Cursor{}, nil
			},
		},
		{
			name: "failure discards partial items",
			fetch: func(_ context.Context, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
				if cursor.IsZero() {
					return []int{1}, next, nil
				}
				return nil, sdk.Cursor{}, fetchErr
			},
			want: fetchErr,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, result, err := pagination.CollectPagesFrom(context.Background(), pagination.PagePlan{}, sdk.Cursor{}, test.fetch)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				assert.Nil(t, items)
				assert.Equal(t, pagination.PageResult{}, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, items)
			assert.Empty(t, items)
			assert.Equal(t, pagination.PageResult{}, result)
		})
	}
}

func TestCollectFilteredPagesAppliesLimitAfterFiltering(t *testing.T) {
	next := testCursor(t, "next")
	checkpoint := func(sdk.Cursor, int) (sdk.Cursor, error) { return testCursor(t, "checkpoint"), nil }
	for _, test := range []struct {
		name      string
		plan      pagination.PagePlan
		first     []int
		second    []int
		wantItems []int
		wantNext  sdk.Cursor
		want      pagination.PageResult
	}{
		{
			name:      "across batches",
			plan:      pagination.PagePlan{Limit: 2},
			first:     []int{1, 2, 3},
			second:    []int{4},
			wantItems: []int{2, 4},
			want:      pagination.PageResult{Returned: 2},
		},
		{
			name:      "checkpoint when limit stops inside source batch",
			plan:      pagination.PagePlan{Limit: 1},
			first:     []int{2, 4},
			wantItems: []int{2},
			wantNext:  testCursor(t, "checkpoint"),
			want:      pagination.PageResult{Returned: 1, HasMore: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, cursor, result, err := pagination.CollectFilteredPagesFrom(context.Background(), test.plan, sdk.Cursor{}, func(_ context.Context, current sdk.Cursor) ([]int, sdk.Cursor, error) {
				if current.IsZero() {
					return test.first, next, nil
				}
				return test.second, sdk.Cursor{}, nil
			}, func(value int) (bool, error) {
				return value%2 == 0, nil
			}, checkpoint)

			require.NoError(t, err)
			assert.Equal(t, test.wantItems, items)
			assert.Equal(t, test.wantNext, cursor)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestFilteredContinuationResumesFromCheckpoint(t *testing.T) {
	fetch := func(_ context.Context, c fakeCursor) ([]int, fakeCursor, error) {
		switch c.value {
		case "":
			return []int{1, 3}, fakeCursor{value: "batch"}, nil
		case "batch":
			return []int{2, 3, 4, 5, 6}, fakeCursor{}, nil
		case "rest":
			return []int{5, 6}, fakeCursor{}, nil
		default:
			t.Fatalf("unexpected continuation %q", c.value)
			return nil, fakeCursor{}, nil
		}
	}
	checkpoint := func(c fakeCursor, n int) (fakeCursor, error) {
		require.Equal(t, "batch", c.value)
		return fakeCursor{value: "rest"}, nil
	}
	include := func(n int) (bool, error) { return n%2 == 0, nil }

	first, next, result, err := pagination.CollectFilteredPagesFrom(context.Background(), pagination.PagePlan{Skip: 1, Limit: 1, OneBatch: true}, fakeCursor{}, fetch, include, checkpoint)
	require.NoError(t, err)
	require.Equal(t, []int{4}, first)
	require.True(t, result.HasMore)

	rest, next, result, err := pagination.CollectFilteredPagesFrom(context.Background(), pagination.PagePlan{OneBatch: true}, next, fetch, include, checkpoint)
	require.NoError(t, err)
	require.Equal(t, []int{6}, rest)
	require.True(t, next.IsZero())
	require.False(t, result.HasMore)
}

func TestFilteredContinuationCancellationAndFailures(t *testing.T) {
	boom := errors.New("checkpoint failed")
	include := func(int) (bool, error) { return true, nil }
	fetch := func(context.Context, fakeCursor) ([]int, fakeCursor, error) { return []int{1, 2}, fakeCursor{}, nil }
	for _, test := range []struct {
		name     string
		fetch    func(context.Context, fakeCursor) ([]int, fakeCursor, error)
		include  func(int) (bool, error)
		want     error
		wantText string
	}{
		{name: "checkpoint failure", fetch: fetch, include: include, want: boom},
		{name: "predicate failure", fetch: fetch, include: func(int) (bool, error) { return false, boom }, want: boom},
		{
			name: "repeated cursor",
			fetch: func(context.Context, fakeCursor) ([]int, fakeCursor, error) {
				return nil, fakeCursor{value: "same"}, nil
			},
			include:  include,
			wantText: "cursor repeated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkpoint := func(fakeCursor, int) (fakeCursor, error) { return fakeCursor{}, boom }
			if test.name == "repeated cursor" {
				checkpoint = func(c fakeCursor, n int) (fakeCursor, error) { t.Fatal("unexpected checkpoint"); return c, nil }
			}
			items, _, _, err := pagination.CollectFilteredPagesFrom(context.Background(), pagination.PagePlan{Limit: 1}, fakeCursor{}, test.fetch, test.include, checkpoint)
			if test.wantText != "" {
				require.ErrorContains(t, err, test.wantText)
			} else {
				require.ErrorIs(t, err, test.want)
			}
			require.Nil(t, items)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := pagination.CollectFilteredPagesFrom(ctx, pagination.PagePlan{}, fakeCursor{}, fetch, include, func(fakeCursor, int) (fakeCursor, error) { return fakeCursor{value: "unused"}, nil })
	require.ErrorIs(t, err, context.Canceled)
}
