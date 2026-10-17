package ugoira

import (
	"math"
	"math/rand/v2"
	"slices"
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

func TestColorGridDegenerateAxes(t *testing.T) {
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

func TestColorGridCellStorage(t *testing.T) {
	for _, size := range []int{255, 256} {
		g := newColorGrid(make([]oklab, size))
		query := func(x, y, z int) oklab {
			var c oklab
			for axis, bin := range []int{x, y, z} {
				c[axis] = float32((g.origin[axis] + float64(bin) + 0.5) / g.scale[axis])
			}
			return c
		}
		first := query(0, 0, 0)
		for x := range 4 {
			for y := range 4 {
				for z := range 4 {
					if got, ok := g.nearest(query(x, y, z)); !ok || got != 248 {
						t.Fatalf("size=%d cell=(%d,%d,%d): index=%d grid=%t", size, x, y, z, got, ok)
					}
				}
			}
		}
		cell := g.cells[0]
		if cell == nil || int(cell.coarseCount) != size || len(cell.indices) != size+64*(size+1) {
			t.Fatalf("size=%d: parent storage %+v", size, cell)
		}
		for slot, offset := range cell.fine {
			if offset == 0 || cell.indices[offset-1] != byte(size) {
				t.Fatalf("size=%d slot=%d: offset=%d", size, slot, offset)
			}
			for i, index := range cell.indices[int(offset) : int(offset)+size] {
				if int(index) != i {
					t.Fatalf("size=%d slot=%d: candidate %d=%d", size, slot, i, index)
				}
			}
		}
		snapshot := slices.Clone(cell.indices)
		if got, ok := g.nearest(query(4, 0, 0)); !ok || got != 248 {
			t.Fatalf("next parent: %d %t", got, ok)
		}
		if next := g.cells[32*32]; next == nil || next == cell || !slices.Equal(cell.indices, snapshot) {
			t.Fatal("parent cells share or overwrite candidate storage")
		}
		if got, ok := g.nearest(first); !ok || got != 248 {
			t.Fatalf("arena growth changed first child: %d %t", got, ok)
		}
	}
}

func TestColorGridCachedBoundsAcrossFirstQueries(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 19))
	palette := make([]oklab, 256)
	for i := range palette {
		palette[i] = srgbToOklab(rgb{byte(rng.Uint32()), byte(rng.Uint32()), byte(rng.Uint32())})
	}
	geometry := newColorGrid(palette)
	for _, seed := range []int{13, 91, 207} {
		var bin [3]int
		var values [3][3]float32
		for axis := range 3 {
			center := float32(palette[seed][axis]+palette[seed+1][axis]) / 2
			bin[axis] = int(float64(center)*geometry.scale[axis] - geometry.origin[axis])
			low := float32((geometry.origin[axis] + float64(bin[axis])) / geometry.scale[axis])
			high := float32((geometry.origin[axis] + float64(bin[axis]+1)) / geometry.scale[axis])
			values[axis] = [3]float32{low, float32(low+high) / 2, math.Nextafter32(high, low)}
		}
		var queries []oklab
		for x := range 3 {
			for y := range 3 {
				for z := range 3 {
					queries = append(queries, oklab{values[0][x], values[1][y], values[2][z]})
				}
			}
		}
		key := (bin[0]>>2)*32*32 + (bin[1]>>2)*32 + (bin[2] >> 2)
		slot := (bin[0]&3)*16 + (bin[1]&3)*4 + (bin[2] & 3)
		for first := range queries {
			g := newColorGrid(palette)
			if got, ok := g.nearest(queries[first]); !ok || got != nearestColor(palette, queries[first]) {
				t.Fatalf("seed=%d first=%d: index=%d grid=%t", seed, first, got, ok)
			}
			cell := g.cells[key]
			if cell == nil || cell.fine[slot] == 0 {
				t.Fatal("query did not populate the target cell")
			}
			count := len(cell.indices)
			// 不同的首查询构建一次缓存，再用全盒各点复查；不能只对首查询正确。
			for _, c := range queries {
				if got, ok := g.nearest(c); !ok || got != nearestColor(palette, c) {
					t.Fatalf("seed=%d first=%d query=%v: index=%d grid=%t", seed, first, c, got, ok)
				}
			}
			if len(cell.indices) != count {
				t.Fatal("queries left the cached child cell")
			}
		}
	}
}

func TestColorGridWholeCellUpperBound(t *testing.T) {
	palette := []oklab{{0, -1, -1}, {1, 1, 1}, {0.501953125, -0.001, 0}, {0.505859375, -0.002, 0}}
	queries := []oklab{{0.501953125, 0, 0}, {0.505859375, 0, 0}}
	for _, first := range queries {
		g := newColorGrid(palette)
		g.nearest(first)
		// 两个近色都在查询子格外；首点距离不能替代整个子格的上界。
		for _, c := range queries {
			got, ok := g.nearest(c)
			if want := nearestColor(palette, c); !ok || got != want {
				t.Fatalf("first=%v query=%v: index=%d want=%d grid=%t", first, c, got, want, ok)
			}
		}
	}
}
