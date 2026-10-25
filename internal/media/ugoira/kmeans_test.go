package ugoira

import (
	"context"
	"errors"
	"testing"
)

func TestKmeansEmptyAndCancellation(t *testing.T) {
	s := newColorStatistics()
	s.hasTransparency = true
	got, err := globalPalette(t.Context(), s)
	if err != nil || len(got) != 0 {
		t.Fatalf("all-transparent palette: %v, %v", got, err)
	}
	samples := paletteSamples{colors: []oklab{{0, 0, 0}, {1, 0, 0}}, indices: make([]uint32, 4097)}
	for i := range samples.indices {
		samples.indices[i] = uint32(i % 2)
	}
	// 包含第一批归属/更新及第二批；取消后不得把未完成中心当成有效调色板。
	for _, checks := range []int{1, 3, 4100, 8194} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		if err := refinePalette(ctx, samples, []oklab{{0, 0, 0}, {1, 0, 0}}); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel after %d checks: %v", checks, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := globalPalette(ctx, s); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("global cancelled: %v, %v", got, err)
	}
}
