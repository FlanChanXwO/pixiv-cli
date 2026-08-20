package lifecycle_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
)

func TestRunLifecycleAndErrorJoin(t *testing.T) {
	useErr := errors.New("use")
	closeErr := errors.New("close")
	var closeCount int

	err := lifecycle.Run(context.Background(), func(ctx context.Context) (*lifecycle.Lease[string], error) {
		if ctx == nil {
			t.Fatal("open received nil context")
		}
		return lifecycle.NewLease("value", func() error {
			closeCount++
			return closeErr
		}), nil
	}, func(_ context.Context, value string, attempt *lifecycle.Attempt) error {
		if value != "value" {
			t.Fatalf("value = %q", value)
		}
		attempt.Commit()
		return useErr
	})

	if !errors.Is(err, useErr) || !errors.Is(err, closeErr) || closeCount != 1 {
		t.Fatalf("Run error=%v closeCount=%d", err, closeCount)
	}
}

func TestRunOpenFailureClosesReturnedLeaseAndJoinsErrors(t *testing.T) {
	openErr := errors.New("open")
	closeErr := errors.New("close")
	var use, closeCount int

	err := lifecycle.Run(context.Background(), func(context.Context) (*lifecycle.Lease[int], error) {
		return lifecycle.NewLease(0, func() error {
			closeCount++
			return closeErr
		}), openErr
	}, func(context.Context, int, *lifecycle.Attempt) error {
		use++
		return nil
	})

	if !errors.Is(err, openErr) || !errors.Is(err, closeErr) || use != 0 || closeCount != 1 {
		t.Fatalf("Run error=%v use=%d close=%d", err, use, closeCount)
	}
}

func TestRunClosesLeaseWhenUsePanics(t *testing.T) {
	panicValue := "use panic"
	var closeCount int
	var recovered any

	func() {
		defer func() {
			recovered = recover()
		}()

		lifecycle.Run(context.Background(), func(context.Context) (*lifecycle.Lease[int], error) {
			return lifecycle.NewLease(1, func() error {
				closeCount++
				return nil
			}), nil
		}, func(context.Context, int, *lifecycle.Attempt) error {
			panic(panicValue)
		})
	}()

	if recovered != panicValue {
		t.Fatalf("recovered = %#v, want %#v", recovered, panicValue)
	}
	if closeCount != 1 {
		t.Fatalf("close count = %d, want 1", closeCount)
	}
}

func TestLeaseCloseIsIdempotentAndRaceSafe(t *testing.T) {
	closeErr := errors.New("close")
	var closeCount int
	lease := lifecycle.NewLease("value", func() error {
		closeCount++
		return closeErr
	})

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if err := lease.Close(); !errors.Is(err, closeErr) {
				t.Errorf("Close error = %v", err)
			}
		})
	}
	wg.Wait()

	if closeCount != 1 {
		t.Fatalf("close count = %d, want 1", closeCount)
	}
}

func TestRunPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := lifecycle.Run(ctx, func(openCtx context.Context) (*lifecycle.Lease[struct{}], error) {
		return lifecycle.NewLease(struct{}{}, nil), nil
	}, func(useCtx context.Context, _ struct{}, _ *lifecycle.Attempt) error {
		cancel()
		select {
		case <-useCtx.Done():
			return useCtx.Err()
		default:
			return errors.New("child context was not canceled")
		}
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}

func TestRunRejectsMissingInputs(t *testing.T) {
	var nilContext context.Context
	if !errors.Is(lifecycle.Run[string](nilContext, nil, nil), lifecycle.ErrNilContext) {
		t.Fatal("nil context was not rejected")
	}
	if !errors.Is(lifecycle.Run[string](context.Background(), nil, nil), lifecycle.ErrNilOpen) {
		t.Fatal("nil open was not rejected")
	}
	open := lifecycle.Open[string](func(context.Context) (*lifecycle.Lease[string], error) {
		return lifecycle.NewLease("value", nil), nil
	})
	if !errors.Is(lifecycle.Run(context.Background(), open, nil), lifecycle.ErrNilUse) {
		t.Fatal("nil use was not rejected")
	}
}
