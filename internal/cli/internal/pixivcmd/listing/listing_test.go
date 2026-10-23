package listing

import (
	"context"
	"io"
	"testing"

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
