package ugoira

import (
	"context"
	"fmt"
	"image"
	"math"
)

const (
	maxColorSamples        = 1 << 18
	reservoirSeed   uint64 = 0x9e3779b97f4a7c15
)

type rgb [3]byte

type colorStatistics struct {
	samples         []rgb
	opaquePixels    uint64
	hasTransparency bool
	state           uint64
}

func newColorStatistics() *colorStatistics {
	return &colorStatistics{samples: make([]rgb, 0, maxColorSamples), state: reservoirSeed}
}

func (s *colorStatistics) addFrame(ctx context.Context, frame *image.NRGBA) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for y := 0; y < frame.Rect.Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := frame.Pix[y*frame.Stride:][:frame.Rect.Dx()*4]
		for x := 0; x < len(row); x += 4 {
			if row[x+3] == 0 {
				s.hasTransparency = true
				continue
			}
			if s.opaquePixels == math.MaxUint64 {
				return fmt.Errorf("ugoira color statistics overflow")
			}
			s.opaquePixels++
			c := rgb{row[x], row[x+1], row[x+2]}
			if len(s.samples) < maxColorSamples {
				s.samples = append(s.samples, c)
				continue
			}
			// 沿用旧版固定种子；装满之前不推进随机序列，半透明颜色也不加权。
			s.state ^= s.state >> 12
			s.state ^= s.state << 25
			s.state ^= s.state >> 27
			selected := (s.state * 0x2545f4914f6cdd1d) % s.opaquePixels
			if selected < maxColorSamples {
				s.samples[selected] = c
			}
		}
	}
	return ctx.Err()
}

type paletteSamples struct {
	colors []oklab
	counts []uint32
	// K-means 按原样本顺序采样，不能只保留去重后的颜色和权重。
	indices []uint32
}

func (s *colorStatistics) paletteSamples(ctx context.Context) (paletteSamples, error) {
	order := make([]uint32, len(s.samples))
	scratch := make([]uint32, len(s.samples))
	for i := range order {
		if err := ctx.Err(); err != nil {
			return paletteSamples{}, err
		}
		order[i] = uint32(i)
	}
	// 按 B/G/R 稳定分桶，保留 RGB 顺序；两块索引工作区最终一块排序、一块输出。
	for axis := 2; axis >= 0; axis-- {
		if err := ctx.Err(); err != nil {
			return paletteSamples{}, err
		}
		var positions [256]int
		for _, index := range order {
			positions[s.samples[index][axis]]++
		}
		start := 0
		for i, count := range positions {
			positions[i] = start
			start += count
		}
		for _, index := range order {
			value := s.samples[index][axis]
			scratch[positions[value]] = index
			positions[value]++
		}
		order, scratch = scratch, order
	}
	unique := 0
	var previous rgb
	for i, index := range order {
		if err := ctx.Err(); err != nil {
			return paletteSamples{}, err
		}
		c := s.samples[index]
		if i == 0 || c != previous {
			unique++
			previous = c
		}
	}
	out := paletteSamples{colors: make([]oklab, unique), counts: make([]uint32, unique), indices: scratch}
	rank := -1
	for i, index := range order {
		if err := ctx.Err(); err != nil {
			return paletteSamples{}, err
		}
		c := s.samples[index]
		if i == 0 || c != previous {
			rank++
			previous = c
			out.colors[rank] = srgbToOklab(c)
		}
		out.counts[rank]++
		out.indices[index] = uint32(rank)
	}
	if err := ctx.Err(); err != nil {
		return paletteSamples{}, err
	}
	return out, nil
}
