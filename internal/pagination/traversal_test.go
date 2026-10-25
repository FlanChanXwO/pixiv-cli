package pagination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestTraverseWithPreservesCommitBoundaryAndPagePlan(t *testing.T) {
	next := traversalTestCursor(t, "next")
	var cursors []sdk.Cursor
	var received []int
	var beginCalls int

	_, err := pagination.TraverseWith(
		context.Background(),
		onceExecute(t, struct{}{}, true),
		pagination.PagePlan{Skip: 1, Limit: 1},
		func() error {
			beginCalls++
			return nil
		},
		func(_ context.Context, _ struct{}, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
			cursors = append(cursors, cursor)
			if cursor.IsZero() {
				return []int{1}, next, nil
			}
			return []int{2}, sdk.Cursor{}, nil
		},
		func(items []int) (bool, error) {
			received = append(received, items...)
			return len(items) > 0, nil
		},
	)

	if err != nil {
		t.Fatalf("TraverseWith error = %v", err)
	}
	if len(cursors) != 2 || !cursors[0].IsZero() || cursors[1] != next {
		t.Fatalf("cursors = %#v", cursors)
	}
	if len(received) != 1 || received[0] != 2 {
		t.Fatalf("received = %#v", received)
	}
	if beginCalls != 1 {
		t.Fatalf("begin calls = %d, want 1", beginCalls)
	}
}

func TestTraverseWithReportsUnconfiguredExecute(t *testing.T) {
	_, err := pagination.TraverseWith(context.Background(), nil, pagination.PagePlan{}, nil,
		func(context.Context, struct{}, sdk.Cursor) ([]int, sdk.Cursor, error) {
			return nil, sdk.Cursor{}, nil
		}, func([]int) (bool, error) {
			return false, nil
		})
	if !errors.Is(err, pagination.ErrExecuteNotConfigured) {
		t.Fatalf("error = %v", err)
	}
}

func TestCollectWithClearsUncommittedResultsBeforeReplay(t *testing.T) {
	firstNext := traversalTestCursor(t, "first-next")
	var fetchCalls int
	execute := pagination.Execute[struct{}](func(ctx context.Context, use func(context.Context, struct{}) (bool, error)) error {
		committed, err := use(ctx, struct{}{})
		if !committed {
			if err == nil {
				t.Fatal("first attempt must fail")
			}
			_, retryErr := use(ctx, struct{}{})
			return retryErr
		}
		return err
	})

	result, err := pagination.CollectWithFrom(context.Background(), execute, pagination.PagePlan{}, sdk.Cursor{}, func(_ context.Context, _ struct{}, cursor sdk.Cursor) ([]int, sdk.Cursor, error) {
		fetchCalls++
		if fetchCalls == 1 {
			if cursor.IsZero() {
				return []int{1}, firstNext, nil
			}
			t.Fatal("first fetch should start at zero cursor")
		}
		if fetchCalls == 2 {
			if cursor.IsZero() {
				t.Fatal("second fetch should continue from first cursor")
			}
			return nil, sdk.Cursor{}, errors.New("first attempt failed")
		}
		return []int{2}, sdk.Cursor{}, nil
	})

	if err != nil {
		t.Fatalf("CollectWith error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0] != 2 || result.HasMore {
		t.Fatalf("result = %#v", result)
	}
}

func onceExecute[C any](t *testing.T, client C, wantCommitted bool) pagination.Execute[C] {
	t.Helper()
	return func(ctx context.Context, use func(context.Context, C) (bool, error)) error {
		committed, err := use(ctx, client)
		if committed != wantCommitted {
			t.Errorf("committed = %v, want %v", committed, wantCommitted)
		}
		return err
	}
}

func traversalTestCursor(t *testing.T, payload string) sdk.Cursor {
	t.Helper()
	cursor, err := sdk.NewCursor("test", "Traverse", 1, "traversal-test", []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return cursor
}
