package ugoira

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
)

type kmeansReference struct {
	Words   []uint64
	Uniform []struct {
		N       uint32
		Indices []uint32
	}
	Nearest []struct {
		Palette, Input []rgb
		Indices        []byte
	}
	Cases []struct {
		Name, Base   string
		Length, Size int
		Bits         [][3]uint32
		RGB          []rgb
	}
	Frames []struct {
		Name            string
		Width, Height   int
		HasTransparency bool `json:"has_transparency"`
		Input, Indices  []byte
		Palette         []rgb
	}
}

func readKmeansReference(t *testing.T) kmeansReference {
	t.Helper()
	data, err := os.ReadFile("testdata/kmeans-dither.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref kmeansReference
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 8 || len(ref.Words) != 16 || len(ref.Uniform) != 8 || len(ref.Nearest) != 7 || len(ref.Frames) != 9 {
		t.Fatal("incomplete old K-means/dither reference")
	}
	return ref
}

func TestKmeansReference(t *testing.T) {
	base := readQuantizationReference(t)
	for _, tc := range readKmeansReference(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			var input []rgb
			for _, c := range base.Cases {
				if c.Name == tc.Base {
					input = c.Input
				}
			}
			if len(input) == 0 {
				t.Fatal("missing input reference")
			}
			s := newColorStatistics()
			s.hasTransparency = tc.Size == 255
			for i := range tc.Length {
				s.samples = append(s.samples, input[i%len(input)])
			}
			samples, err := s.paletteSamples(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			centers, err := wuPalette(t.Context(), samples, tc.Size)
			if err != nil {
				t.Fatal(err)
			}
			if err := refinePalette(t.Context(), samples, centers); err != nil {
				t.Fatal(err)
			}
			if len(centers) != len(tc.Bits) || len(centers) != len(tc.RGB) {
				t.Fatal("palette reference length mismatch")
			}
			for i, c := range centers {
				if oklabBits(c) != tc.Bits[i] {
					t.Fatalf("center %d bits: %x, old %x", i, oklabBits(c), tc.Bits[i])
				}
				if got := oklabToSRGB(c); got != tc.RGB[i] {
					t.Fatalf("center %d RGB: %v, old %v", i, got, tc.RGB[i])
				}
			}
			if tc.Name == "many-255" || tc.Name == "many-256" {
				got, err := globalPalette(t.Context(), s)
				if err != nil || !slices.Equal(got, tc.RGB) {
					t.Fatalf("global palette differs from old pipeline: %v", err)
				}
			}
		})
	}
}

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

func BenchmarkRefinePalette(b *testing.B) {
	samples := paletteSamples{colors: make([]oklab, 1024), indices: make([]uint32, maxColorSamples)}
	for i := range samples.colors {
		samples.colors[i] = srgbToOklab(rgb{byte(i), byte(i >> 3), byte(i * 37)})
	}
	for i := range samples.indices {
		samples.indices[i] = uint32(i % len(samples.colors))
	}
	initial := slices.Clone(samples.colors[:256])
	centroids := make([]oklab, len(initial))
	b.ReportAllocs()
	for b.Loop() {
		copy(centroids, initial)
		if err := refinePalette(context.Background(), samples, centroids); err != nil {
			b.Fatal(err)
		}
	}
}
