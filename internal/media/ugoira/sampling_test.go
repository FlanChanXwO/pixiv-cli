package ugoira

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"math"
	"slices"
	"testing"
)

func TestColorSamplingReference(t *testing.T) {
	ref := readQuantizationReference(t).Reservoir
	// 非零原点、行尾 padding，多个 frame 保持原始像素顺序。
	backing := image.NewNRGBA(image.Rect(-2, -3, 4098, 3))
	frame := backing.SubImage(image.Rect(1, 0, 4094, 1)).(*image.NRGBA)
	s := newColorStatistics()
	for start := 0; start < ref.Pixels; start += frame.Rect.Dx() {
		n := min(frame.Rect.Dx(), ref.Pixels-start)
		part := frame.SubImage(image.Rect(1, 0, 1+n, 1)).(*image.NRGBA)
		for j := range n {
			i := start + j
			a := byte(255)
			if i%13 == 0 {
				a = 0
			} else if i%7 == 0 {
				a = 1
			}
			copy(part.Pix[j*4:][:4], []byte{byte(i * 37), byte(i >> 3), byte(i*17 + i/251), a})
		}
		if err := s.addFrame(t.Context(), part); err != nil {
			t.Fatal(err)
		}
		if len(s.samples) > maxColorSamples || cap(s.samples) != maxColorSamples {
			t.Fatal("reservoir grew beyond fixed allocation")
		}
	}
	hash := sha256.New()
	for _, c := range s.samples {
		_, _ = hash.Write(c[:])
	}
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != ref.SHA256 {
		t.Fatalf("sample bytes hash %s, old %s", got, ref.SHA256)
	}
	if s.opaquePixels != ref.OpaquePixels || s.state != ref.State || s.hasTransparency != ref.HasTransparency || len(s.samples) != ref.Length {
		t.Fatalf("reservoir state: count=%d state=%x transparent=%t length=%d", s.opaquePixels, s.state, s.hasTransparency, len(s.samples))
	}
}

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

func TestPaletteSamplesReference(t *testing.T) {
	for _, ref := range readQuantizationReference(t).Cases {
		t.Run(ref.Name, func(t *testing.T) {
			s := newColorStatistics()
			s.samples = append(s.samples, ref.Input...)
			out, err := s.paletteSamples(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(out.counts, ref.Counts) || !slices.Equal(out.indices, ref.Indices) || len(out.colors) != len(ref.Colors) || len(ref.Bits) != len(ref.Colors) {
				t.Fatal("dedup weights, sample indices or length differ from old quantette")
			}
			for i, c := range out.colors {
				if oklabBits(c) != ref.Bits[i] || oklabToSRGB(c) != ref.Colors[i] {
					t.Fatalf("unique color %d: got %x, old %x", i, oklabBits(c), ref.Bits[i])
				}
			}
			if !slices.Equal(s.samples, ref.Input) {
				t.Fatal("dedup modified reservoir")
			}
		})
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

func TestPaletteSamplesFullReservoirIndices(t *testing.T) {
	s := newColorStatistics()
	for i := range maxColorSamples {
		s.samples = append(s.samples, rgb{byte(i * 37), byte(i >> 8), byte(i >> 16)})
	}
	out, err := s.paletteSamples(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.colors) != maxColorSamples || len(out.counts) != maxColorSamples || len(out.indices) != maxColorSamples {
		t.Fatal("unique reservoir lost colors or indices")
	}
	for i, c := range s.samples {
		wantColor := rgb{byte(i * 37), byte(i >> 8), byte(i >> 16)}
		// 每个 R 有 1024 色，每个 G 有四个 B；覆盖超过 uint16 的索引。
		index := uint32(wantColor[0])*1024 + uint32(wantColor[1])*4 + uint32(wantColor[2])
		if c != wantColor || out.indices[i] != index || out.counts[index] != 1 || oklabBits(out.colors[index]) != oklabBits(srgbToOklab(c)) {
			t.Fatalf("sample %d: color=%v index=%d, want color=%v index=%d", i, c, out.indices[i], wantColor, index)
		}
	}
	again, err := s.paletteSamples(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.colors, again.colors) || !slices.Equal(out.counts, again.counts) || !slices.Equal(out.indices, again.indices) {
		t.Fatal("deduplication did not preserve the reservoir or previous result")
	}
}

func BenchmarkPaletteSamples(b *testing.B) {
	for _, unique := range []bool{false, true} {
		name := "repeated"
		if unique {
			name = "unique"
		}
		b.Run(name, func(b *testing.B) {
			s := newColorStatistics()
			for i := range maxColorSamples {
				c := rgb{byte(i), 0, 0}
				if unique {
					c = rgb{byte(i * 37), byte(i >> 8), byte(i >> 16)}
				}
				s.samples = append(s.samples, c)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := s.paletteSamples(b.Context()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPaletteSamplesByteBoundaries(t *testing.T) {
	input := []rgb{{1, 0, 255}, {0, 255, 255}, {0, 1, 0}, {255, 255, 255}, {0, 0, 255}, {255, 0, 0}, {1, 0, 0}, {0, 0, 0}, {0, 0, 255}, {255, 255, 255}}
	colors := []rgb{{0, 0, 0}, {0, 0, 255}, {0, 1, 0}, {0, 255, 255}, {1, 0, 0}, {1, 0, 255}, {255, 0, 0}, {255, 255, 255}}
	s := newColorStatistics()
	s.samples = append(s.samples, input...)
	got, err := s.paletteSamples(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.colors) != len(colors) || !slices.Equal(got.indices, []uint32{5, 3, 2, 7, 1, 6, 4, 0, 1, 7}) || !slices.Equal(got.counts, []uint32{1, 2, 1, 1, 1, 1, 1, 2}) {
		t.Fatalf("RGB byte ordering: counts=%v indices=%v", got.counts, got.indices)
	}
	for i, c := range colors {
		if oklabBits(got.colors[i]) != oklabBits(srgbToOklab(c)) {
			t.Fatalf("color %d differs", i)
		}
	}
	if !slices.Equal(s.samples, input) {
		t.Fatal("sorting mutated reservoir")
	}
}
