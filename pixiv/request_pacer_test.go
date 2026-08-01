package pixiv

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestPacerHonorsContextWhileWaiting(t *testing.T) {
	pacer := newRequestPacer(time.Hour)
	if err := pacer.wait(context.Background()); err != nil {
		t.Fatalf("first request wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pacer.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request wait = %v, want context.Canceled", err)
	}
}

func TestRequestPacerSchedulesRequestStartsFromOneSharedClock(t *testing.T) {
	interval := 15 * time.Millisecond
	pacer := newRequestPacer(interval)
	started := time.Now()
	if err := pacer.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pacer.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < interval {
		t.Fatalf("second request started after %s, want at least %s", elapsed, interval)
	}
}
