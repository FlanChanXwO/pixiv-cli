package ugoira

import (
	"context"
	"fmt"
	"image"
)

// 蛇形 Floyd–Steinberg 移植自 quantette 0.6.0 的 dither.rs。
// Copyright (c) 2023 Ian Manske，MIT；见 third_party/licenses/quantette-0.6.0/LICENSE-MIT。

func quantizeFrame(ctx context.Context, frame *image.NRGBA, palette []rgb, hasTransparency bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q, err := newFrameQuantizer(palette, hasTransparency)
	if err != nil {
		return nil, err
	}
	return q.quantize(ctx, frame)
}

// 调色板、搜索单元与误差工作区由一次编码独占。
type frameQuantizer struct {
	errors          []oklab
	search          colorSearch
	hasTransparency bool
}

func newFrameQuantizer(palette []rgb, hasTransparency bool) (*frameQuantizer, error) {
	limit := 256
	if hasTransparency {
		limit = 255
	}
	if len(palette) > limit {
		return nil, fmt.Errorf("palette has %d colors, maximum is %d", len(palette), limit)
	}
	colors := make([]oklab, len(palette))
	for i, c := range palette {
		colors[i] = srgbToOklab(c)
	}
	return &frameQuantizer{search: newColorSearch(colors), hasTransparency: hasTransparency}, nil
}

func (q *frameQuantizer) quantize(ctx context.Context, frame *image.NRGBA) ([]byte, error) {
	return q.quantizeInto(ctx, frame, nil)
}

// 只允许调用方在消费完上一帧后交回索引缓冲。
func (q *frameQuantizer) quantizeInto(ctx context.Context, frame *image.NRGBA, indices []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	width, height := frame.Rect.Dx(), frame.Rect.Dy()
	if cap(indices) < width*height {
		indices = make([]byte, width*height)
	} else {
		indices = indices[:width*height]
	}
	colors := q.search.palette
	if len(colors) == 0 {
		clear(indices)
		return indices, ctx.Err()
	}
	length := 2 * (width + 2)
	if cap(q.errors) < length {
		q.errors = make([]oklab, length)
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
			c := srgbToOklab(rgb{p[0], p[1], p[2]})
			for channel := range c {
				c[channel] += current[x+1][channel]
			}
			index := q.search.nearest(c)
			selected := colors[index]
			forward, downBack, down, downForward := &current[x+1+step], &next[x+1-step], &next[x+1], &next[x+1+step]
			for channel := range c {
				error := float32(0.1) * float32(c[channel]-selected[channel])
				forward[channel] += float32((7.0 / 16) * error)
				downBack[channel] += float32((3.0 / 16) * error)
				down[channel] += float32((5.0 / 16) * error)
				downForward[channel] = float32((1.0 / 16) * error)
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
		next[1], next[width] = oklab{}, oklab{}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return indices, nil
}
