package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/session"
)

func TestRunLifecycleAndErrorJoin(t *testing.T) {
	useErr := errors.New("use")
	closeErr := errors.New("close")
	var closeCount int
	err := session.Run(context.Background(), func(ctx context.Context) (string, func() error, error) {
		if ctx == nil {
			t.Fatal("open received nil context")
		}
		return "value", func() error {
			closeCount++
			return closeErr
		}, nil
	}, func(_ context.Context, value string, attempt *session.Attempt) error {
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

func TestRunOpenFailureSkipsUseAndClose(t *testing.T) {
	openErr := errors.New("open")
	var use, closeCount int
	err := session.Run(context.Background(), func(context.Context) (int, func() error, error) {
		return 0, func() error { closeCount++; return nil }, openErr
	}, func(context.Context, int, *session.Attempt) error { use++; return nil })
	if !errors.Is(err, openErr) || use != 0 || closeCount != 0 {
		t.Fatalf("Run error=%v use=%d close=%d", err, use, closeCount)
	}
}

func TestAttemptCommitIsIdempotentAndRaceSafe(t *testing.T) {
	var attempt session.Attempt
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempt.Commit()
			_ = attempt.Committed()
		}()
	}
	wg.Wait()
	if !attempt.Committed() {
		t.Fatal("attempt did not commit")
	}
}
