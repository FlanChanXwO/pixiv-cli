package ugoira

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
)

func TestColorStatistics(t *testing.T) {
	s := newColorStatistics()
	frame := &image.NRGBA{Rect: image.Rect(0, 0, 3, 1), Stride: 12, Pix: []byte{9, 8, 7, 0, 1, 2, 3, 1, 1, 2, 3, 255}}
	if err := s.addFrame(t.Context(), frame); err != nil {
		t.Fatal(err)
	}
	if !s.hasTransparency || s.opaquePixels != 2 || s.histogram[0][0][0].count != 2 {
		t.Fatal("color counts or transparency differ")
	}
	palette, err := globalPalette(t.Context(), s)
	if err != nil || len(palette) != 1 || palette[0] != (rgb{1, 2, 3}) {
		t.Fatalf("palette=%v err=%v", palette, err)
	}
	s = newColorStatistics()
	s.opaquePixels = math.MaxUint64
	if err := s.addFrame(t.Context(), frame); err == nil {
		t.Fatal("accepted counter overflow")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.addFrame(ctx, frame); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
