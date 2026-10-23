package pixivcmd_test

import (
	"context"
	"errors"
	"testing"

	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestReadReturnsOnlySuccessfulResult(t *testing.T) {
	failure := errors.New("read failed")
	for _, tc := range []struct {
		name     string
		failures int
		succeed  bool
	}{
		{"success", 0, true}, {"replay succeeds", 1, true}, {"final failure", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := pixivcmd.New(nil, func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				for i := 0; i < tc.failures; i++ {
					committed, err := attempt(ctx, nil)
					if committed || !errors.Is(err, failure) {
						t.Fatalf("failed read committed=%v err=%v", committed, err)
					}
				}
				if !tc.succeed {
					return failure
				}
				committed, err := attempt(ctx, nil)
				if committed {
					t.Fatal("read must not commit")
				}
				return err
			})
			result, err := pixivcmd.Read(runner, context.Background(), pixivcmd.Request{}, func(context.Context, *pixiv.Client) ([]int, error) {
				calls++
				if calls <= tc.failures {
					return []int{-calls}, failure
				}
				return []int{42}, nil
			})
			if tc.succeed {
				if err != nil || len(result) != 1 || result[0] != 42 {
					t.Fatalf("result=%v err=%v", result, err)
				}
			} else if !errors.Is(err, failure) || result != nil {
				t.Fatalf("failed result must be zero: %v %v", result, err)
			}
			wantCalls := tc.failures
			if tc.succeed {
				wantCalls++
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want=%d", calls, wantCalls)
			}
		})
	}
}

func TestReadPreservesRequestAndAttemptContext(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "parent")
	attemptCtx, cancel := context.WithCancel(context.WithValue(parent, key{}, "attempt"))
	defer cancel()
	proxy := "http://proxy.example"
	request := pixivcmd.Request{UserID: 123, HTTPSProxyOverride: &proxy}
	client := &pixiv.Client{}
	runner := pixivcmd.New(nil, func(ctx context.Context, got pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		if ctx != parent || got != request {
			t.Fatal("pool input was replaced")
		}
		cancel()
		committed, err := attempt(attemptCtx, client)
		if committed {
			t.Fatal("cancelled read committed")
		}
		return err
	})
	result, err := pixivcmd.Read(runner, parent, request, func(ctx context.Context, got *pixiv.Client) (int, error) {
		if ctx != attemptCtx || got != client {
			t.Fatal("attempt context or client was replaced")
		}
		return 9, ctx.Err()
	})
	if result != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read result=%d err=%v", result, err)
	}
}

func TestReadMissingPortAndPreAttemptCancellation(t *testing.T) {
	var missing pixivcmd.Runner
	result, err := pixivcmd.Read(missing, context.Background(), pixivcmd.Request{}, func(context.Context, *pixiv.Client) (string, error) {
		t.Fatal("nil port invoked callback")
		return "", nil
	})
	if result != "" || err == nil || err.Error() != "pixiv pooled operation is not configured" {
		t.Fatalf("nil port result=%q err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := pixivcmd.New(nil, func(ctx context.Context, _ pixivcmd.Request, _ func(context.Context, *pixiv.Client) (bool, error)) error {
		return ctx.Err()
	})
	result, err = pixivcmd.Read(runner, ctx, pixivcmd.Request{}, func(context.Context, *pixiv.Client) (string, error) {
		t.Fatal("cancelled pool invoked callback")
		return "", nil
	})
	if result != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result=%q err=%v", result, err)
	}
}

func TestWriteMissingPooledPort(t *testing.T) {
	err := (pixivcmd.Runner{}).Write(context.Background(), pixivcmd.Request{}, func(context.Context, *pixiv.Client) error {
		t.Fatal("nil port invoked write callback")
		return nil
	})
	if err == nil || err.Error() != "pixiv pooled operation is not configured" {
		t.Fatalf("missing port error=%v", err)
	}
}

func TestWriteCommitsEveryInvokedAttempt(t *testing.T) {
	failure := errors.New("SDK write failed")
	for _, tc := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", failure}, {"cancelled", context.Canceled}} {
		t.Run(tc.name, func(t *testing.T) {
			parent := context.Background()
			attemptCtx, cancel := context.WithCancel(parent)
			defer cancel()
			if tc.err == context.Canceled {
				cancel()
			}
			proxy := "http://proxy.example"
			request := pixivcmd.Request{UserID: 123, HTTPSProxyOverride: &proxy}
			client := &pixiv.Client{}
			calls := 0
			runner := pixivcmd.New(nil, func(ctx context.Context, got pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				if ctx != parent || got != request {
					t.Fatal("pool input was replaced")
				}
				committed, err := attempt(attemptCtx, client)
				if !committed || !errors.Is(err, tc.err) {
					t.Fatalf("write committed=%v error=%v", committed, err)
				}
				return err
			})
			err := runner.Write(parent, request, func(ctx context.Context, got *pixiv.Client) error {
				calls++
				if ctx != attemptCtx || got != client {
					t.Fatal("attempt context or client was replaced")
				}
				return tc.err
			})
			if calls != 1 || !errors.Is(err, tc.err) {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}
