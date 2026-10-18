package ugoira

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
)

func TestDitherFrameReference(t *testing.T) {
	for _, tc := range readKmeansReference(t).Frames {
		t.Run(tc.Name, func(t *testing.T) {
			if len(tc.Input) != tc.Width*tc.Height*4 || len(tc.Indices) != tc.Width*tc.Height {
				t.Fatal("frame reference length mismatch")
			}
			backing := image.NewNRGBA(image.Rect(-2, -3, tc.Width+5, tc.Height+5))
			frame := backing.SubImage(image.Rect(1, 2, tc.Width+1, tc.Height+2)).(*image.NRGBA)
			for y := range tc.Height {
				copy(frame.Pix[y*frame.Stride:][:tc.Width*4], tc.Input[y*tc.Width*4:][:tc.Width*4])
			}
			before := bytes.Clone(backing.Pix)
			got, err := quantizeFrame(t.Context(), frame, tc.Palette, tc.HasTransparency)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.Indices) {
				for i, want := range tc.Indices {
					if got[i] != want {
						t.Fatalf("pixel (%d,%d): index %d, old %d", i%tc.Width, i/tc.Width, got[i], want)
					}
				}
			}
			if !bytes.Equal(before, backing.Pix) {
				t.Fatal("quantization modified source")
			}
			if tc.HasTransparency {
				for i, index := range got {
					if (tc.Input[i*4+3] == 0) != (index == 0) {
						t.Fatalf("pixel %d alpha %d: index %d", i, tc.Input[i*4+3], index)
					}
				}
			}
		})
	}
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

func TestDitherQuantizerResetsFrameErrors(t *testing.T) {
	palette := []rgb{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}, {0, 0, 255}}
	q, err := newFrameQuantizer(palette, true)
	if err != nil {
		t.Fatal(err)
	}
	grid := q.search.grid
	var first, saved []byte
	for phase := range 4 {
		frame := image.NewNRGBA(image.Rect(0, 0, 37, 19))
		for i := range frame.Pix {
			frame.Pix[i] = byte(i*37 + phase*29)
		}
		got, err := q.quantize(t.Context(), frame)
		want, wantErr := quantizeFrame(t.Context(), frame, palette, true)
		if err != nil || wantErr != nil || !bytes.Equal(got, want) || q.search.grid != grid {
			t.Fatalf("phase=%d: pixels differ=%t err=%v fresh=%v", phase, !bytes.Equal(got, want), err, wantErr)
		}
		if phase == 0 {
			first, saved = got, bytes.Clone(got)
		}
		if !bytes.Equal(first, saved) {
			t.Fatal("next frame overwrote previous indices")
		}
	}
}

func TestDitherReusesConsumedFrameBuffer(t *testing.T) {
	palette := []rgb{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}, {0, 0, 255}}
	q, err := newFrameQuantizer(palette, true)
	if err != nil {
		t.Fatal(err)
	}
	var indices []byte
	for phase, size := range []image.Point{{37, 19}, {1, 1}, {1, 35}, {2, 31}, {3, 17}, {19, 37}, {53, 29}, {37, 19}} {
		frame := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
		for i := range frame.Pix {
			frame.Pix[i] = byte(i*37 + phase*29)
		}
		before := bytes.Clone(frame.Pix)
		previous := indices
		indices, err = q.quantizeInto(t.Context(), frame, previous)
		want, wantErr := quantizeFrame(t.Context(), frame, palette, true)
		if err != nil || wantErr != nil || !bytes.Equal(indices, want) {
			t.Fatalf("phase %d: reused output differs: %v, %v", phase, err, wantErr)
		}
		if cap(previous) >= len(indices) && &previous[:cap(previous)][0] != &indices[0] {
			t.Fatal("consumed indices were not reused")
		}
		if !bytes.Equal(before, frame.Pix) {
			t.Fatal("reused quantization modified source")
		}
	}
	frame := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	for _, checks := range []int{1, 3, 5, 7} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		if got, err := q.quantizeInto(ctx, frame, indices); got != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel after %d checks: %v, %v", checks, got, err)
		}
		got, err := q.quantizeInto(t.Context(), frame, indices)
		want, wantErr := quantizeFrame(t.Context(), frame, palette, true)
		if err != nil || wantErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("state leaked after cancel: %v, %v", err, wantErr)
		}
	}
	empty, err := newFrameQuantizer(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range indices {
		indices[i] = 255
	}
	got, err := empty.quantizeInto(t.Context(), frame, indices)
	if err != nil || !bytes.Equal(got, make([]byte, 35)) || &got[0] != &indices[0] {
		t.Fatalf("empty palette retained old indices: %v, %v", got, err)
	}
}
