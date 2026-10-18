package ugoira

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestNearestColorReference(t *testing.T) {
	for _, tc := range readKmeansReference(t).Nearest {
		colors := make([]oklab, len(tc.Palette))
		for i, c := range tc.Palette {
			colors[i] = srgbToOklab(c)
		}
		search := newColorSearch(colors)
		for i, c := range tc.Input {
			if got := nearestColor(colors, srgbToOklab(c)); got != tc.Indices[i] {
				t.Fatalf("%d colors input %d: index %d, old %d", len(colors), i, got, tc.Indices[i])
			}
			if got := search.nearest(srgbToOklab(c)); got != tc.Indices[i] {
				t.Fatalf("%d colors input %d: tree index %d, old %d", len(colors), i, got, tc.Indices[i])
			}
		}
	}
}

func TestColorSearchPreservesNearest(t *testing.T) {
	rng := rand.New(rand.NewPCG(0, 1))
	for size := 1; size <= 256; size++ {
		palette := make([]oklab, size)
		for i := range palette {
			for channel := range palette[i] {
				palette[i][channel] = rng.Float32()*2 - 1
			}
			if i > 0 && i%7 == 0 {
				palette[i] = palette[0]
			}
		}
		original := slices.Clone(palette)
		search := newColorSearch(palette)
		tree := newColorTree(palette)
		if tree.grid != nil {
			t.Fatal("batch tree allocated a persistent grid")
		}
		for i := range 128 {
			c := oklab{rng.Float32()*4 - 2, rng.Float32()*4 - 2, rng.Float32()*4 - 2}
			if i%4 == 0 && i < size {
				c = palette[i]
			}
			if got, want := search.nearest(c), nearestColor(palette, c); got != want {
				t.Fatalf("size %d query %v: index %d, scan %d", size, c, got, want)
			}
			if got, want := tree.nearest(c), nearestColor(palette, c); got != want {
				t.Fatalf("size %d query %v: batch tree %d, scan %d", size, c, got, want)
			}
		}
		if !slices.Equal(original, palette) {
			t.Fatal("search reordered caller palette")
		}
	}
	// 同距结果不由树遍历顺序决定：最小 lane、该 lane 的最后一个 chunk。
	palette := make([]oklab, 256)
	equalSearch := newColorSearch(palette)
	if got := equalSearch.nearest(oklab{}); got != 248 {
		t.Fatalf("equal palette: %d", got)
	}
	for _, c := range []oklab{{math.SmallestNonzeroFloat32, 0, 0}, {math.Nextafter32(0.5, 1), 0, 0}, {0.5, 0, 0}} {
		palette = []oklab{{0, 0, 0}, {1, 0, 0}, {0.5, 1, 0}, {0.5, -1, 0}}
		boundarySearch := newColorSearch(palette)
		if got, want := boundarySearch.nearest(c), nearestColor(palette, c); got != want {
			t.Fatalf("boundary %v: %d, scan %d", c, got, want)
		}
	}
}

func TestColorSearchBoxBoundary(t *testing.T) {
	palette := make([]oklab, 32)
	for i := range palette {
		for axis := range 3 {
			palette[i][axis] = float32((i>>axis)&1) - 0.5
		}
	}
	search := newColorSearch(palette)
	if got := search.nearest(oklab{}); got != 24 {
		t.Fatalf("box-center tie: index %d, want 24", got)
	}
	values := []float32{-1, math.Nextafter32(-0.5, 0), 0, 0.5, math.Nextafter32(0.5, 1), 1}
	for _, x := range values {
		for _, y := range values {
			for _, z := range values {
				c := oklab{x, y, z}
				if got, want := search.nearest(c), nearestColor(palette, c); got != want {
					t.Fatalf("boundary %v: index %d, scan %d", c, got, want)
				}
			}
		}
	}
}

func TestColorTreeResetReusesNodes(t *testing.T) {
	palette := make([]oklab, 256)
	for i := range palette {
		palette[i][0] = float32(i) / 256
	}
	tree := newColorTree(palette)
	backing := &tree.nodes[0]
	if got := tree.nearest(oklab{}); got != 0 {
		t.Fatalf("initial query: %d", got)
	}
	slices.Reverse(palette)
	tree.resetTree(palette)
	if &tree.nodes[0] != backing || tree.haveQuery || tree.grid != nil {
		t.Fatal("tree reset lost storage or retained old query state")
	}
	if got := tree.nearest(oklab{}); got != 255 {
		t.Fatalf("query after center update: %d, want 255", got)
	}
	tree.resetTree(nil)
	if got := tree.nearest(oklab{}); got != 0 {
		t.Fatalf("empty tree: %d", got)
	}
}

func TestColorSearchAllPalettePoints(t *testing.T) {
	for _, size := range []int{9, 17, 31, 63, 127, 255, 256} {
		palette := make([]oklab, size)
		for i := range palette {
			palette[i] = oklab{float32(i) / 256, float32(i%7) / 16, -float32(i%11) / 16}
		}
		search := newColorSearch(palette)
		for _, reverse := range []bool{false, true} {
			for i := range size {
				if reverse {
					i = size - 1 - i
				}
				if got := search.nearest(palette[i]); got != byte(i) {
					t.Fatalf("size=%d point=%d reverse=%t: index %d", size, i, reverse, got)
				}
			}
		}
	}
}

func TestColorSearchRepeatedQuery(t *testing.T) {
	palette := make([]oklab, 32)
	for i := range palette {
		palette[i] = oklab{float32(i&1) - 0.5, float32((i>>1)&1) - 0.5, float32((i>>2)&1) - 0.5}
	}
	search := newColorSearch(palette)
	if got := search.nearest(oklab{}); got != 24 {
		t.Fatalf("first zero query: index %d, want 24", got)
	}
	for _, c := range []oklab{{}, {-1, 0, 0}, {-1, 0, 0}, {1, 1, 1}, {math.SmallestNonzeroFloat32, 0, 0}, {math.Nextafter32(0.5, 1), 0, 0}, {}} {
		want := nearestColor(palette, c)
		for repeat := range 64 {
			if got := search.nearest(c); got != want {
				t.Fatalf("query=%v repeat=%d: index %d, want %d", c, repeat, got, want)
			}
		}
	}
}

func BenchmarkColorSearch(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 2))
	palette := make([]oklab, 256)
	for i := range palette {
		palette[i] = srgbToOklab(rgb{byte(rng.Uint32()), byte(rng.Uint32()), byte(rng.Uint32())})
	}
	queries := make([]oklab, 4096)
	for i := range queries {
		queries[i] = oklab{rng.Float32(), rng.Float32()*0.7 - 0.35, rng.Float32()*0.7 - 0.35}
	}
	search := newColorSearch(palette)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		search.nearest(queries[i%len(queries)])
		i++
	}
}

func BenchmarkColorSearchRepeated(b *testing.B) {
	search := newColorSearch(make([]oklab, 256))
	b.ReportAllocs()
	for b.Loop() {
		search.nearest(oklab{})
	}
}
