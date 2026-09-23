package listing

import (
	"context"
	"io"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestRunnerUsesNarrowExecutorPort(t *testing.T) {
	called := false
	executor := Executor(func(_ context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		called = true
		_, err := attempt(context.Background(), nil)
		return err
	})

	runner := New(io.Discard, executor)
	execute := runner.Executor(Request{})
	if execute == nil {
		t.Fatal("expected pooled executor")
	}
	if err := execute(context.Background(), func(_ context.Context, _ *pixiv.Client) (bool, error) { return false, nil }); err != nil {
		t.Fatalf("pooled executor failed: %v", err)
	}
	if !called {
		t.Fatal("pooled port was not called")
	}
}

// TestRunPooledIllustListObservesAlreadyFetchedArtworks 锁定 Task 15 契约：
// observer 只消费命令已经取得的 Artwork，且不会触发额外抓取。
func TestRunPooledIllustListObservesAlreadyFetchedArtworks(t *testing.T) {
	fetches := 0
	fetched := []pixiv.Artwork{{ID: 7001, PageCount: 1}, {ID: 7002, PageCount: 2}}
	executor := Executor(func(_ context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		_, err := attempt(context.Background(), nil)
		return err
	})
	var observed []pixiv.Artwork
	runner := New(io.Discard, executor).WithObserver(func(items []pixiv.Artwork) {
		observed = append(observed, items...)
	})
	printed := 0
	err := runner.RunPooledIllustList(context.Background(), Request{}, Plan{oneBatch: true}, false, false, "artworks",
		func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			fetches++
			return fetched, sdk.Cursor{}, nil
		},
		func(items []pixiv.Artwork, _ int) error { printed += len(items); return nil })
	if err != nil {
		t.Fatalf("RunPooledIllustList: %v", err)
	}
	if len(observed) != 2 || observed[0].ID != 7001 || observed[1].ID != 7002 {
		t.Fatalf("observer saw %+v, want the fetched artworks", observed)
	}
	if printed != 2 {
		t.Fatalf("printed = %d, want the fetched artworks unchanged", printed)
	}
	if fetches != 1 {
		t.Fatalf("fetch calls = %d, want exactly one fetch with no observation-triggered refetch", fetches)
	}
}

// TestRunPooledIllustListWithoutObserverStaysNoop 保证未接线时行为与之前完全一致。
func TestRunPooledIllustListWithoutObserverStaysNoop(t *testing.T) {
	executor := Executor(func(_ context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		_, err := attempt(context.Background(), nil)
		return err
	})
	printed := 0
	err := New(io.Discard, executor).RunPooledIllustList(context.Background(), Request{}, Plan{oneBatch: true}, false, false, "artworks",
		func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			return []pixiv.Artwork{{ID: 1, PageCount: 1}}, sdk.Cursor{}, nil
		},
		func(items []pixiv.Artwork, _ int) error { printed += len(items); return nil })
	if err != nil || printed != 1 {
		t.Fatalf("no-observer run: printed=%d err=%v", printed, err)
	}
}
