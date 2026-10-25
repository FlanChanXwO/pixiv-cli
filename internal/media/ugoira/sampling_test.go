package ugoira

import (
	"context"
	"errors"
	"image"
	"math"
	"slices"
	"testing"
)

func TestColorSamplingTransparency(t *testing.T) {
	s := newColorStatistics()
	frame := &image.NRGBA{Rect: image.Rect(0, 0, 3, 1), Stride: 12, Pix: []byte{9, 8, 7, 0, 1, 2, 3, 1, 1, 2, 3, 255}}
	if err := s.addFrame(t.Context(), frame); err != nil {
		t.Fatal(err)
	}
	if !s.hasTransparency || s.opaquePixels != 2 || !slices.Equal(s.samples, []rgb{{1, 2, 3}, {1, 2, 3}}) || s.state != reservoirSeed {
		t.Fatalf("unexpected statistics: %+v", s)
	}
	frame.Pix[7], frame.Pix[11] = 0, 0
	s = newColorStatistics()
	if err := s.addFrame(t.Context(), frame); err != nil {
		t.Fatal(err)
	}
	out, err := s.paletteSamples(t.Context())
	if err != nil || len(out.colors) != 0 || len(out.indices) != 0 || !s.hasTransparency || s.opaquePixels != 0 || s.state != reservoirSeed {
		t.Fatalf("transparent frame: %+v, %v", out, err)
	}
}

func TestColorSamplingOverflowAndCancellation(t *testing.T) {
	frame := &image.NRGBA{Rect: image.Rect(0, 0, 1, 1), Stride: 4, Pix: []byte{1, 2, 3, 255}}
	s := newColorStatistics()
	s.opaquePixels = math.MaxUint64
	if err := s.addFrame(t.Context(), frame); err == nil {
		t.Fatal("expected counter overflow")
	}
	if s.opaquePixels != math.MaxUint64 || len(s.samples) != 0 {
		t.Fatal("overflow mutated statistics")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.addFrame(ctx, frame); !errors.Is(err, context.Canceled) {
		t.Fatalf("sampling cancellation: %v", err)
	}
	if _, err := s.paletteSamples(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("dedup cancellation: %v", err)
	}
}

func TestColorSamplingCancellationDuringWork(t *testing.T) {
	frame := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for i := 3; i < len(frame.Pix); i += 4 {
		frame.Pix[i] = 255
	}
	s := newColorStatistics()
	ctx := &resizeCancelContext{Context: t.Context(), remaining: 3}
	if err := s.addFrame(ctx, frame); !errors.Is(err, context.Canceled) {
		t.Fatalf("row cancellation: %v", err)
	}
	if s.opaquePixels != 8 || len(s.samples) != 8 {
		t.Fatalf("sampling continued after cancellation: %d pixels", s.opaquePixels)
	}

	s.samples = []rgb{{0, 0, 0}, {255, 0, 0}, {0, 255, 0}, {0, 0, 0}}
	// 覆盖索引初始化、三轮分桶、去重、映射和返回前的取消。
	for _, checks := range []int{2, 5, 6, 7, 9, 13, 16} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		got, err := s.paletteSamples(ctx)
		if !errors.Is(err, context.Canceled) || got.colors != nil || got.counts != nil || got.indices != nil {
			t.Fatalf("cancel after %d checks: partial result %+v, err %v", checks, got, err)
		}
	}
}
