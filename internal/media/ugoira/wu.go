package ugoira

import (
	"context"
	"fmt"
	"math"
)

// Wu 初始化移植自 quantette 0.6.0 的 quantize/wu/{f32,shared}.rs。
// Copyright (c) 2023 Ian Manske，按 MIT 使用；许可见
// third_party/licenses/quantette-0.6.0/LICENSE-MIT。

type wuStats struct {
	count      uint32
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

type wuHistogram [32][16][16]wuStats

func wuBin(c oklab) [3]int {
	// 先按 float32 求范围差和比例，避免 Go 常量折叠改变旧版分箱边界。
	amin, amax := float32(-0.2338874), float32(0.2762164)
	bmin, bmax := float32(-0.31152815), float32(0.19856972)
	return [3]int{
		max(0, min(31, int(c[0]*32))),
		max(0, min(15, int(float32(c[1]-amin)*float32(16/float32(amax-amin))))),
		max(0, min(15, int(float32(c[2]-bmin)*float32(16/float32(bmax-bmin))))),
	}
}

func wuPalette(ctx context.Context, samples paletteSamples, size int) ([]oklab, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 1 || size > 256 {
		return nil, fmt.Errorf("invalid palette size %d", size)
	}
	if len(samples.colors) == 0 {
		return nil, nil
	}
	hist := new(wuHistogram)
	for i, c := range samples.colors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bin := wuBin(c)
		s := &hist[bin[0]][bin[1]][bin[2]]
		n := samples.counts[i]
		s.count += n
		v := [3]float64{float64(c[0]), float64(c[1]), float64(c[2])}
		for j := range v {
			s.components[j] += float64(float64(n) * v[j])
		}
		s.squared += float64(float64(n) * wuSquares(v))
	}
	for r := range hist {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var area [16]wuStats
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
	queue := make(wuHeap, 0, size)
	queue.push(wuCube{max: [3]int{32, 16, 16}, variance: math.Inf(1)})
	for len(queue) < size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cube := queue.pop()
		if !(cube.variance > 0) {
			queue.push(cube)
			break
		}
		first, second, ok := hist.cut(cube)
		if ok {
			first.variance, second.variance = hist.variance(first), hist.variance(second)
			queue.push(first)
			queue.push(second)
		} else {
			cube.variance = 0
			queue.push(cube)
		}
	}
	palette := make([]oklab, len(queue))
	for i, cube := range queue {
		s := hist.volume(cube)
		for j, v := range s.components {
			palette[i][j] = float32(v / float64(s.count))
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
	// 与旧版按 L/a/b 递归相减的顺序一致，不能重排为八项求和。
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
			// 相等时保留第一个维度/切点，与 min_by_key 一致。
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

// 调色板沿用 Rust BinaryHeap 的数组顺序；标准库 heap 的相等元素顺序不同。
// pop 先下沉到底（相等选右子节点），再上浮；push 相等时不交换。
// 这里只存由有限 sRGB 样本计算出的方差。
type wuHeap []wuCube

func (h *wuHeap) push(c wuCube) {
	*h = append(*h, c)
	h.siftUp(len(*h) - 1)
}

func (h wuHeap) siftUp(pos int) {
	c := h[pos]
	for pos > 0 {
		parent := (pos - 1) / 2
		if c.variance <= h[parent].variance {
			break
		}
		h[pos] = h[parent]
		pos = parent
	}
	h[pos] = c
}

func (h *wuHeap) pop() wuCube {
	q := *h
	out, last := q[0], q[len(q)-1]
	q = q[:len(q)-1]
	if len(q) > 0 {
		pos := 0
		for child := 1; child < len(q); child = 2*pos + 1 {
			if child+1 < len(q) && q[child].variance <= q[child+1].variance {
				child++
			}
			q[pos] = q[child]
			pos = child
		}
		q[pos] = last
		q.siftUp(pos)
	}
	*h = q
	return out
}
