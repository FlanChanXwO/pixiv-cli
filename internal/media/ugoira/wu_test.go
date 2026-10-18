package ugoira

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestWuPaletteReference(t *testing.T) {
	for _, ref := range readQuantizationReference(t).Cases {
		for _, want := range ref.Palettes {
			t.Run(fmt.Sprintf("%s/%d", ref.Name, want.Size), func(t *testing.T) {
				s := newColorStatistics()
				s.samples = append(s.samples, ref.Input...)
				samples, err := s.paletteSamples(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				got, err := wuPalette(t.Context(), samples, want.Size)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != len(want.Bits) || len(want.RGB) != len(got) {
					t.Fatalf("palette length %d, old %d", len(got), len(want.Bits))
				}
				for i, c := range got {
					if oklabBits(c) != want.Bits[i] {
						t.Fatalf("palette[%d] bits %x, old %x", i, oklabBits(c), want.Bits[i])
					}
					if rgb := oklabToSRGB(c); rgb != want.RGB[i] {
						t.Fatalf("palette[%d] RGB %v, old %v", i, rgb, want.RGB[i])
					}
				}
			})
		}
	}
}

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

func TestWuPaletteCancellationDuringWork(t *testing.T) {
	s := newColorStatistics()
	for _, ref := range readQuantizationReference(t).Cases {
		if ref.Name == "many" {
			s.samples = append(s.samples, ref.Input...)
		}
	}
	samples, err := s.paletteSamples(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		checks int
	}{
		{"histogram", 2}, {"moments", len(samples.colors) + 2},
		{"first-cut", len(samples.colors) + 34}, {"later-cut", len(samples.colors) + 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &resizeCancelContext{Context: t.Context(), remaining: tc.checks}
			if got, err := wuPalette(ctx, samples, 256); got != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled palette length %d, err %v", len(got), err)
			}
		})
	}
}
