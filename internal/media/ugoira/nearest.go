package ugoira

import (
	"math"
	"slices"
)

// 最近颜色同距规则沿用 quantette 0.6.0 的 color_map/nearest_neighbor.rs。
// Copyright (c) 2023 Ian Manske，MIT；见 third_party/licenses/quantette-0.6.0/LICENSE-MIT。

func nearestColor(palette []oklab, c oklab) byte {
	best := 0
	distance := float32(math.Inf(1))
	for i, p := range palette {
		x, y, z := float32(c[0]-p[0]), float32(c[1]-p[1]), float32(c[2]-p[2])
		d := float32(float32(x*x)+float32(y*y)) + float32(z*z)
		// SIMD 先在每个 lane 内取最后一个同距 chunk，再取最小的 lane。
		if d < distance || d == distance && i%8 <= best%8 {
			best, distance = i, d
		}
	}
	return byte(best)
}

// 查询期间调色板固定；格内扫描精确候选，批内或格外使用最宽轴树。
// 只裁掉三轴距离下界之和严格大于最优值的子树，不近似距离或丢弃同距色。
type colorSearch struct {
	nodes     []colorNode
	palette   []oklab
	last      byte
	query     oklab
	haveQuery bool
	grid      *colorGrid
}

type colorNode struct {
	color oklab
	index byte
	axis  byte
}

func newColorSearch(palette []oklab) colorSearch {
	s := newColorTree(palette)
	if len(palette) > 0 {
		s.grid = newColorGrid(palette)
	}
	return s
}

// minibatch 中心在批间更新，只为当前批建立精确树，不分配固定调色板的栅格。
func newColorTree(palette []oklab) colorSearch {
	var s colorSearch
	s.resetTree(palette)
	return s
}

func (s *colorSearch) resetTree(palette []oklab) {
	nodes := s.nodes
	if cap(nodes) < len(palette) {
		nodes = make([]colorNode, len(palette))
	}
	// 中心更新后清空查询状态，但复用节点，避免每批产生新工作区垃圾。
	*s = colorSearch{nodes: nodes[:len(palette)], palette: palette}
	for i, c := range palette {
		s.nodes[i] = colorNode{color: c, index: byte(i)}
	}
	var build func(int, int)
	build = func(lo, hi int) {
		// 小块顺序扫描，避免每个颜色都递归；8 对齐原同距规则的 lane 宽度。
		if hi-lo <= 8 {
			return
		}
		lower, upper := s.nodes[lo].color, s.nodes[lo].color
		for _, node := range s.nodes[lo+1 : hi] {
			for channel, value := range node.color {
				lower[channel] = min(lower[channel], value)
				upper[channel] = max(upper[channel], value)
			}
		}
		axis := 0
		for channel := 1; channel < 3; channel++ {
			if upper[channel]-lower[channel] > upper[axis]-lower[axis] {
				axis = channel
			}
		}
		nodes := s.nodes[lo:hi]
		slices.SortFunc(nodes, func(a, b colorNode) int {
			if a.color[axis] < b.color[axis] {
				return -1
			}
			if a.color[axis] > b.color[axis] {
				return 1
			}
			return 0
		})
		mid := (lo + hi) / 2
		s.nodes[mid].axis = byte(axis)
		build(lo, mid)
		build(mid+1, hi)
	}
	build(0, len(s.nodes))
}

func (s *colorSearch) nearest(c oklab) byte {
	if len(s.palette) == 0 {
		return 0
	}
	// 平涂区域会重复完全相同的查询；只复用完整结果，不合并相近坐标。
	if s.haveQuery && c == s.query {
		return s.last
	}
	if s.grid != nil {
		if index, ok := s.grid.nearest(c); ok {
			s.last, s.query, s.haveQuery = index, c, true
			return index
		}
	}
	// 批内或栅格外查询使用原精确树；上一索引只初始化上界。
	best := s.last
	p := s.palette[best]
	x, y, z := float32(c[0]-p[0]), float32(c[1]-p[1]), float32(c[2]-p[2])
	distance := float32(float32(x*x)+float32(y*y)) + float32(z*z)
	var bounds [3]float32
	var visit func(int, int)
	visit = func(lo, hi int) {
		if hi-lo <= 8 {
			for _, node := range s.nodes[lo:hi] {
				x, y, z := float32(c[0]-node.color[0]), float32(c[1]-node.color[1]), float32(c[2]-node.color[2])
				d := float32(float32(x*x)+float32(y*y)) + float32(z*z)
				if d < distance || d == distance && (node.index%8 < best%8 || node.index%8 == best%8 && node.index > best) {
					best, distance = node.index, d
				}
			}
			return
		}
		mid := (lo + hi) / 2
		node := s.nodes[mid]
		x, y, z := float32(c[0]-node.color[0]), float32(c[1]-node.color[1]), float32(c[2]-node.color[2])
		d := float32(float32(x*x)+float32(y*y)) + float32(z*z)
		if d < distance || d == distance && (node.index%8 < best%8 || node.index%8 == best%8 && node.index > best) {
			best, distance = node.index, d
		}
		delta := float32(c[node.axis] - node.color[node.axis])
		if delta <= 0 {
			visit(lo, mid)
		} else {
			visit(mid+1, hi)
		}
		// 保留祖先分割给出的约束，并按原距离的 float32 加法顺序求下界。
		old := bounds[node.axis]
		bounds[node.axis] = max(old, float32(delta*delta))
		lower := float32(bounds[0]+bounds[1]) + bounds[2]
		if lower <= distance {
			if delta <= 0 {
				visit(mid+1, hi)
			} else {
				visit(lo, mid)
			}
		}
		bounds[node.axis] = old
	}
	visit(0, len(s.nodes))
	s.last, s.query, s.haveQuery = best, c, true
	return best
}
