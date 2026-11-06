package ugoira

import (
	"context"
	"fmt"
	"math"
)

// Wu 方差分割改写自 quantette 0.6.0 的 quantize/wu/{f32,shared}.rs。
// Copyright (c) 2023 Ian Manske，按 MIT 使用；许可见
// third_party/licenses/quantette-0.6.0/LICENSE-MIT。

const (
	wuHistogramBits = 5
	wuHistogramBins = 1 << wuHistogramBits
)

type wuStats struct {
	count      uint64
	components [3]float64
	squared    float64
}

func (s wuStats) add(t wuStats) wuStats {
	return wuStats{s.count + t.count, [3]float64{s.components[0] + t.components[0], s.components[1] + t.components[1], s.components[2] + t.components[2]}, s.squared + t.squared}
}

func (s wuStats) sub(t wuStats) wuStats {
	return wuStats{s.count - t.count, [3]float64{s.components[0] - t.components[0], s.components[1] - t.components[1], s.components[2] - t.components[2]}, s.squared - t.squared}
}

func wuSquares(c [3]float64) float64 {
	var sum float64
	for _, v := range c {
		sum += float64(v * v)
	}
	return sum
}

type wuCube struct {
	min, max [3]int
	variance float64
}

type wuHistogram [wuHistogramBins][wuHistogramBins][wuHistogramBins]wuStats

func wuPalette(ctx context.Context, hist *wuHistogram, size int) ([]rgb, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 1 || size > maxGIFPaletteColors {
		return nil, fmt.Errorf("invalid palette size %d", size)
	}
	for r := range hist {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var area [wuHistogramBins]wuStats
		for g := range hist[r] {
			var line wuStats
			for b := range hist[r][g] {
				line = line.add(hist[r][g][b])
				area[b] = area[b].add(line)
				hist[r][g][b] = area[b]
				if r > 0 {
					hist[r][g][b] = hist[r-1][g][b].add(area[b])
				}
			}
		}
	}
	cubes := []wuCube{{max: [3]int{wuHistogramBins, wuHistogramBins, wuHistogramBins}}}
	if hist.volume(cubes[0]).count == 0 {
		return nil, nil
	}
	cubes[0].variance = hist.variance(cubes[0])
	for len(cubes) < size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		best := 0
		for i := range cubes {
			if cubes[i].variance > cubes[best].variance {
				best = i
			}
		}
		if cubes[best].variance <= 0 {
			break
		}
		first, second, ok := hist.cut(cubes[best])
		if !ok {
			cubes[best].variance = 0
			continue
		}
		first.variance, second.variance = hist.variance(first), hist.variance(second)
		cubes[best] = first
		cubes = append(cubes, second)
	}
	palette := make([]rgb, len(cubes))
	for i, cube := range cubes {
		s := hist.volume(cube)
		for j, v := range s.components {
			palette[i][j] = byte(v/float64(s.count) + 0.5)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return palette, nil
}

func (h *wuHistogram) plane(r int, c wuCube) wuStats {
	upper := h[r][c.max[1]-1][c.max[2]-1]
	if c.min[2] > 0 {
		upper = upper.sub(h[r][c.max[1]-1][c.min[2]-1])
	}
	if c.min[1] == 0 {
		return upper
	}
	lower := h[r][c.min[1]-1][c.max[2]-1]
	if c.min[2] > 0 {
		lower = lower.sub(h[r][c.min[1]-1][c.min[2]-1])
	}
	return upper.sub(lower)
}

func (h *wuHistogram) volume(c wuCube) wuStats {
	upper := h.plane(c.max[0]-1, c)
	if c.min[0] == 0 {
		return upper
	}
	return upper.sub(h.plane(c.min[0]-1, c))
}

func (h *wuHistogram) volumeAt(c wuCube, dim, bin int) wuStats {
	if bin == 0 {
		return wuStats{}
	}
	c.min[dim], c.max[dim] = 0, bin
	return h.volume(c)
}

func (h *wuHistogram) variance(c wuCube) float64 {
	if c.max[0]-c.min[0] == 1 && c.max[1]-c.min[1] == 1 && c.max[2]-c.min[2] == 1 {
		return 0
	}
	s := h.volume(c)
	return s.squared - wuSquares(s.components)/float64(s.count)
}

func (h *wuHistogram) cut(c wuCube) (wuCube, wuCube, bool) {
	total := h.volume(c)
	best, dim, cut := math.Inf(1), -1, 0
	for d := range 3 {
		base := h.volumeAt(c, d, c.min[d])
		for bin := c.min[d] + 1; bin < c.max[d]; bin++ {
			upper := h.volumeAt(c, d, bin).sub(base)
			lower := total.sub(upper)
			if upper.count == 0 || lower.count == 0 {
				continue
			}
			value := -(wuSquares(upper.components)/float64(upper.count) + wuSquares(lower.components)/float64(lower.count))
			// 相等时保留第一个维度和切点，保持输出可复现。
			if value < best {
				best, dim, cut = value, d, bin
			}
		}
	}
	if dim < 0 {
		return wuCube{}, wuCube{}, false
	}
	first, second := c, c
	first.max[dim], second.min[dim] = cut, cut
	return first, second, true
}
