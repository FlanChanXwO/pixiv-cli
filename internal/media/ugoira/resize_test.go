package ugoira

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
)

func TestScaledDimensions(t *testing.T) {
	for _, tc := range []struct {
		width, height, maxEdge uint32
		wantWidth, wantHeight  uint32
	}{
		{0, 7, 3, 0, 0}, {7, 0, 3, 0, 0},
		{1920, 1080, 0, 1920, 1080}, {13, 9, 13, 13, 9}, {9, 13, 99, 9, 13},
		{1920, 1080, 600, 600, 337}, {1080, 1920, 600, 337, 600},
		{13, 9, 7, 7, 4}, {9, 13, 7, 4, 7}, {13, 13, 7, 7, 7},
		{1920, 1080, 1, 1, 1}, {1, 61, 7, 1, 7}, {61, 1, 7, 7, 1},
		{math.MaxUint32, 1, 1, 1, 1}, {1, math.MaxUint32, 1, 1, 1},
		{math.MaxUint32, math.MaxUint32 - 1, math.MaxUint32 - 1, math.MaxUint32 - 1, math.MaxUint32 - 2},
		{math.MaxUint32 - 1, math.MaxUint32, math.MaxUint32 - 1, math.MaxUint32 - 2, math.MaxUint32 - 1},
	} {
		w, h := scaledDimensions(tc.width, tc.height, tc.maxEdge)
		if w != tc.wantWidth || h != tc.wantHeight {
			t.Errorf("%dx%d maxEdge=%d: got %dx%d, want %dx%d", tc.width, tc.height, tc.maxEdge, w, h, tc.wantWidth, tc.wantHeight)
		}
	}
}

func TestResizeFrameInvalidSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for _, size := range []image.Point{{0, 1}, {1, 0}, {-1, 1}, {4, 2}, {3, 3}} {
		if _, err := resizeFrame(t.Context(), src, size); err == nil {
			t.Errorf("invalid size %v accepted", size)
		}
	}
	wide := &image.NRGBA{Rect: image.Rect(0, 0, math.MaxInt/16+1, 1)}
	if _, err := resizeFrame(t.Context(), wide, image.Pt(1, 1)); err == nil {
		t.Fatal("float32 row size overflow accepted")
	}
}

func TestResizeFrameCancellation(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 512, 256))
	// 在权重生成、重采样及下一行开始时取消，避免依赖机器速度或 sleep。
	for _, checks := range []int{1, 8, 3060, 3070, 3200, 3330} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		if got, err := resizeFrame(ctx, src, image.Pt(256, 128)); got != nil || !errors.Is(err, context.Canceled) {
			t.Errorf("cancel after %d checks: frame=%v, error=%v", checks, got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resizeFrame(ctx, src, src.Rect.Size()); !errors.Is(err, context.Canceled) {
		t.Fatalf("unchanged dimensions ignored cancellation: %v", err)
	}
}

type resizeCancelContext struct {
	context.Context
	remaining int
}

func (c *resizeCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return c.Context.Err()
}
