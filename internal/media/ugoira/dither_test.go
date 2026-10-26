package ugoira

import (
	"context"
	"errors"
	"image"
	"testing"
)

func quantizeFrame(ctx context.Context, frame *image.NRGBA, palette []rgb, hasTransparency bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q, err := newFrameQuantizer(palette, hasTransparency)
	if err != nil {
		return nil, err
	}
	return q.quantize(ctx, frame)
}

func (q *frameQuantizer) quantize(ctx context.Context, frame *image.NRGBA) ([]byte, error) {
	return q.quantizeInto(ctx, frame, nil)
}

func TestDitherCancellationAndPaletteSize(t *testing.T) {
	frame := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	palette := []rgb{{0, 0, 0}, {255, 255, 255}}
	for _, checks := range []int{1, 3, 5, 7} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		if got, err := quantizeFrame(ctx, frame, palette, true); got != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel after %d checks: %v, %v", checks, got, err)
		}
	}
	for _, tc := range []struct {
		size         int
		transparency bool
	}{{257, false}, {256, true}} {
		if _, err := quantizeFrame(t.Context(), frame, make([]rgb, tc.size), tc.transparency); err == nil {
			t.Fatalf("accepted %d colors, transparent=%t", tc.size, tc.transparency)
		}
	}
}
