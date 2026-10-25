package ugoira

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestColorGridCellBoundaries(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	palette := make([]oklab, 256)
	for i := range palette {
		palette[i] = srgbToOklab(rgb{byte(rng.Uint32()), byte(rng.Uint32()), byte(rng.Uint32())})
	}
	g := newColorGrid(palette)
	inside := 0
	for axis := range 3 {
		for edge := 0; edge <= colorGridSide; edge++ {
			var c oklab
			for k := range 3 {
				c[k] = float32((g.origin[k] + colorGridSide/2 + 0.5) / g.scale[k])
			}
			boundary := float32((g.origin[axis] + float64(edge)) / g.scale[axis])
			for _, value := range []float32{math.Nextafter32(boundary, float32(math.Inf(-1))), boundary, math.Nextafter32(boundary, float32(math.Inf(1)))} {
				c[axis] = value
				if got, ok := g.nearest(c); ok {
					inside++
					if want := nearestColor(palette, c); got != want {
						t.Fatalf("axis=%d edge=%d query=%v: index %d, want %d", axis, edge, c, got, want)
					}
				}
			}
		}
	}
	if inside < (colorGridSide-2)*9 {
		t.Fatalf("only %d boundary queries used grid", inside)
	}
	for _, value := range []float32{float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN())} {
		if _, ok := g.nearest(oklab{value, 0, 0}); ok {
			t.Fatalf("nonfinite query %v entered grid", value)
		}
	}
}

func TestColorGridMatchesScanForDegeneratePalettes(t *testing.T) {
	for _, palette := range [][]oklab{
		{{0, 0, 0}},
		make([]oklab, 256),
		{{0, 0, 0}, {math.SmallestNonzeroFloat32, 0, 0}, {2 * math.SmallestNonzeroFloat32, 0, 0}},
		{{math.MaxFloat32, 0, 0}, {math.Nextafter32(math.MaxFloat32, 0), 0, 0}},
	} {
		g := newColorGrid(palette)
		inside := 0
		for _, c := range palette {
			if got, ok := g.nearest(c); ok {
				inside++
				if got != nearestColor(palette, c) {
					t.Fatalf("palette=%v query=%v: index %d, want %d", palette, c, got, nearestColor(palette, c))
				}
			}
		}
		if inside == 0 {
			t.Fatalf("degenerate palette %v never used grid", palette)
		}
	}
}
