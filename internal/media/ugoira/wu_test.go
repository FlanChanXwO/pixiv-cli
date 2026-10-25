package ugoira

import (
	"context"
	"errors"
	"testing"
)

func TestWuPaletteCancellationAndSize(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := wuPalette(ctx, paletteSamples{}, 256); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, size := range []int{0, 257} {
		if _, err := wuPalette(t.Context(), paletteSamples{}, size); err == nil {
			t.Fatalf("accepted palette size %d", size)
		}
	}
}
