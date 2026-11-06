package ugoira

import (
	"context"
	"fmt"
	"image"
	"math"
)

type rgb [3]byte

type colorStatistics struct {
	histogram       wuHistogram
	opaquePixels    uint64
	hasTransparency bool
}

func newColorStatistics() *colorStatistics { return new(colorStatistics) }

func (s *colorStatistics) addFrame(ctx context.Context, frame *image.NRGBA) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := frame.Pix[frame.PixOffset(frame.Rect.Min.X, y):][:frame.Rect.Dx()*4]
		for x := 0; x < len(row); x += 4 {
			if row[x+3] == 0 {
				s.hasTransparency = true
				continue
			}
			if s.opaquePixels == math.MaxUint64 {
				return fmt.Errorf("ugoira color statistics overflow")
			}
			s.opaquePixels++
			bin := &s.histogram[row[x]>>3][row[x+1]>>3][row[x+2]>>3]
			bin.count++
			for c := range bin.components {
				v := float64(row[x+c])
				bin.components[c] += v
				bin.squared += v * v
			}
		}
	}
	return ctx.Err()
}

func globalPalette(ctx context.Context, statistics *colorStatistics) ([]rgb, error) {
	size := 256
	if statistics.hasTransparency {
		size--
	}
	return wuPalette(ctx, &statistics.histogram, size)
}
