package ugoira

import "math"

const colorGridSide = 128

// 32³ 父格复用粗候选，4³ 子格只扫描该集合；缓存由本次编码独占。
type colorGrid struct {
	palette       []oklab
	scale, origin [3]float64
	cells         [32 * 32 * 32]*colorGridCell
}

type colorGridCell struct {
	indices     []byte
	coarseCount uint16
	// 子格保存首候选偏移，前一字节存数量，0 表示 256 色。
	fine [64]uint16
}

func newColorGrid(palette []oklab) *colorGrid {
	g := &colorGrid{palette: palette}
	for axis := range 3 {
		low, high := palette[0][axis], palette[0][axis]
		for _, p := range palette {
			low, high = min(low, p[axis]), max(high, p[axis])
		}
		span := float64(high) - float64(low)
		g.scale[axis] = colorGridSide
		if span > 0 {
			fraction, exponent := math.Frexp(span)
			if fraction == 0.5 {
				exponent--
			}
			g.scale[axis] = math.Ldexp(colorGridSide, -exponent)
		}
		g.origin[axis] = math.Floor(float64(low) * g.scale[axis])
	}
	return g
}

func (g *colorGrid) nearest(c oklab) (byte, bool) {
	var bin [3]int
	for axis := range 3 {
		v := float64(c[axis])*g.scale[axis] - g.origin[axis]
		if !(v >= 0 && v < colorGridSide) {
			return 0, false
		}
		bin[axis] = int(v)
	}
	// 二进制幂缩放保留精确边界，先定位 4³ 子格所属的父格。
	key := (bin[0]>>2)*32*32 + (bin[1]>>2)*32 + (bin[2] >> 2)
	cell := g.cells[key]
	if cell == nil {
		cell = new(colorGridCell)
		var low, high, center oklab
		for axis := range 3 {
			start := g.origin[axis] + float64(bin[axis]&^3)
			low[axis] = float32(start / g.scale[axis])
			high[axis] = float32((start + 4) / g.scale[axis])
			center[axis] = float32((start + 2) / g.scale[axis])
		}
		p := g.palette[nearestColor(g.palette, center)]
		var far oklab
		for axis := range 3 {
			far[axis] = max(colorGridAbs(float32(low[axis]-p[axis])), colorGridAbs(float32(high[axis]-p[axis])))
		}
		upper := colorGridSquared(far)
		for i, q := range g.palette {
			var near oklab
			for axis := range 3 {
				near[axis] = max(float32(low[axis]-q[axis]), float32(q[axis]-high[axis]), 0)
			}
			if colorGridSquared(near) <= upper {
				cell.indices = append(cell.indices, byte(i))
			}
		}
		cell.coarseCount = uint16(len(cell.indices))
		g.cells[key] = cell
	}
	slot := (bin[0]&3)*16 + (bin[1]&3)*4 + (bin[2] & 3)
	offset := int(cell.fine[slot])
	if offset == 0 {
		var low, high oklab
		for axis := range 3 {
			low[axis] = float32((g.origin[axis] + float64(bin[axis])) / g.scale[axis])
			high[axis] = float32((g.origin[axis] + float64(bin[axis]+1)) / g.scale[axis])
		}
		// 任一颜色的全子格上界都有效；按查询距离选锚点，减少冷构建运算。
		anchor := cell.indices[0]
		distance := float32(math.Inf(1))
		for _, i := range cell.indices[:cell.coarseCount] {
			p := g.palette[i]
			x, y, z := float32(c[0]-p[0]), float32(c[1]-p[1]), float32(c[2]-p[2])
			d := float32(float32(x*x)+float32(y*y)) + float32(z*z)
			if d < distance {
				anchor, distance = i, d
			}
		}
		p := g.palette[anchor]
		var far oklab
		for axis := range 3 {
			far[axis] = max(colorGridAbs(float32(low[axis]-p[axis])), colorGridAbs(float32(high[axis]-p[axis])))
		}
		upper := colorGridSquared(far)

		var candidates [256]byte
		count := 0
		for _, q := range cell.indices[:cell.coarseCount] {
			var near oklab
			for axis := range 3 {
				near[axis] = max(float32(low[axis]-g.palette[q][axis]), float32(g.palette[q][axis]-high[axis]), 0)
			}
			// 仅排除下界严格超出上界的色，保留原同距候选顺序。
			if colorGridSquared(near) <= upper {
				candidates[count] = q
				count++
			}
		}
		// 每父格最多 256 + 64×257 = 16704 字节，增长后仍可使用 uint16 偏移。
		offset = len(cell.indices) + 1
		cell.indices = append(cell.indices, byte(count))
		cell.indices = append(cell.indices, candidates[:count]...)
		cell.fine[slot] = uint16(offset)
	}
	count := int(cell.indices[offset-1])
	if count == 0 {
		count = 256
	}
	best := byte(0)
	distance := float32(math.Inf(1))
	for _, i := range cell.indices[offset : offset+count] {
		p := g.palette[i]
		x, y, z := float32(c[0]-p[0]), float32(c[1]-p[1]), float32(c[2]-p[2])
		d := float32(float32(x*x)+float32(y*y)) + float32(z*z)
		if d < distance || d == distance && i%8 <= best%8 {
			best, distance = i, d
		}
	}
	return best, true
}

func colorGridSquared(c oklab) float32 {
	return float32(float32(c[0]*c[0])+float32(c[1]*c[1])) + float32(c[2]*c[2])
}

func colorGridAbs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
