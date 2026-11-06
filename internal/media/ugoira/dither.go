package ugoira

import (
	"context"
	"fmt"
	"image"
	"math"
)

// 蛇形 Floyd–Steinberg 移植自 quantette 0.6.0 的 dither.rs。
// Copyright (c) 2023 Ian Manske，MIT；见 third_party/licenses/quantette-0.6.0/LICENSE-MIT。

// 调色板与误差工作区由一次编码独占。
type frameQuantizer struct {
	errors          [][3]float32
	palette         []rgb
	hasTransparency bool
}

func newFrameQuantizer(palette []rgb, hasTransparency bool) (*frameQuantizer, error) {
	limit := maxGIFPaletteColors
	if hasTransparency {
		limit--
	}
	if len(palette) > limit {
		return nil, fmt.Errorf("palette has %d colors, maximum is %d", len(palette), limit)
	}
	return &frameQuantizer{palette: palette, hasTransparency: hasTransparency}, nil
}

func (q *frameQuantizer) quantize(ctx context.Context, frame *image.NRGBA) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	width, height := frame.Rect.Dx(), frame.Rect.Dy()
	indices := make([]byte, width*height)
	colors := q.palette
	if len(colors) == 0 {
		clear(indices)
		return indices, ctx.Err()
	}
	length := 2 * (width + 2)
	if cap(q.errors) < length {
		q.errors = make([][3]float32, length)
	} else {
		q.errors = q.errors[:length]
		clear(q.errors)
	}
	current, next := q.errors[:width+2], q.errors[width+2:]
	for y := range height {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row := frame.Pix[y*frame.Stride:][:width*4]
		start, end, step := 0, width, 1
		if y%2 != 0 {
			start, end, step = width-1, -1, -1
		}
		for x := start; x != end; x += step {
			p := row[x*4:][:4]
			c := [3]float32{float32(p[0]), float32(p[1]), float32(p[2])}
			for channel := range c {
				c[channel] += current[x+1][channel]
			}
			index := nearestColor(colors, c)
			selected := colors[index]
			forward, downBack, down, downForward := &current[x+1+step], &next[x+1-step], &next[x+1], &next[x+1+step]
			for channel := range c {
				error := c[channel] - float32(selected[channel])
				forward[channel] += float32((7.0 / 16) * error)
				downBack[channel] += float32((3.0 / 16) * error)
				down[channel] += float32((5.0 / 16) * error)
				downForward[channel] += float32((1.0 / 16) * error)
			}
			// 隐藏 RGB 仍参与误差扩散；只有输出索引受 alpha=0 影响。
			if q.hasTransparency {
				if p[3] == 0 {
					index = 0
				} else {
					index++
				}
			}
			indices[y*width+x] = index
		}
		current, next = next, current
		clear(next)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return indices, nil
}

// shortcut: 每像素扫描至多 256 色；实际量化延迟影响调用时再引入查找索引。
func nearestColor(palette []rgb, value [3]float32) byte {
	best := 0
	distance := float32(math.Inf(1))
	for i, c := range palette {
		r, g, b := value[0]-float32(c[0]), value[1]-float32(c[1]), value[2]-float32(c[2])
		d := r*r + g*g + b*b
		if d < distance {
			best, distance = i, d
		}
	}
	return byte(best)
}
