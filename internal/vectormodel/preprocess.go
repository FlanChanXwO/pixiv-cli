package vectormodel

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
)

type sampleWeight struct {
	start  int
	values []float64
}

func weights(source, target int) []sampleWeight {
	result := make([]sampleWeight, target)
	scale := float64(source) / float64(target)
	support := math.Max(scale, 1)
	for i := range result {
		center := (float64(i) + 0.5) * scale
		first := max(0, int(center-support+0.5))
		last := min(source, int(center+support+0.5))
		row := sampleWeight{start: first}
		sum := 0.0
		for x := first; x < last; x++ {
			w := math.Max(0, 1-math.Abs((float64(x)+0.5-center)/support))
			row.values = append(row.values, w)
			sum += w
		}
		for j := range row.values {
			row.values[j] /= sum
		}
		result[i] = row
	}
	return result
}
func pixels(ctx context.Context, data []byte, size int) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Join(image.ErrFormat, err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	horizontal := make([]byte, size*h*3)
	wx, wy := weights(w, size), weights(h, size)
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x, row := range wx {
			var sums [3]float64
			for j, a := range row.values {
				c := rgbPixel(img, bounds.Min.X+row.start+j, bounds.Min.Y+y)
				sums[0] += float64(c.R) * a
				sums[1] += float64(c.G) * a
				sums[2] += float64(c.B) * a
			}
			for c := range 3 {
				horizontal[(y*size+x)*3+c] = byte(math.Round(sums[c]))
			}
		}
	}
	out := make([]float32, 3*size*size)
	for y, row := range wy {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < size; x++ {
			for c := range 3 {
				sum := 0.0
				for j, a := range row.values {
					sum += float64(horizontal[((row.start+j)*size+x)*3+c]) * a
				}
				value := math.Round(sum)
				out[c*size*size+y*size+x] = float32(value/255*2 - 1)
			}
		}
	}
	return out, nil
}

// JPEG 的色度先双线性上采样，再转 RGB，以匹配参考处理器。
func rgbPixel(img image.Image, x, y int) color.NRGBA {
	jpeg, ok := img.(*image.YCbCr)
	if !ok {
		return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	}
	hx, hy := 1, 1
	switch jpeg.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		hx = 2
	case image.YCbCrSubsampleRatio420:
		hx, hy = 2, 2
	case image.YCbCrSubsampleRatio440:
		hy = 2
	case image.YCbCrSubsampleRatio411:
		hx = 4
	case image.YCbCrSubsampleRatio410:
		hx, hy = 4, 2
	}
	bounds := jpeg.Bounds()
	cx := (float64(x-bounds.Min.X)+0.5)/float64(hx) - 0.5
	cy := (float64(y-bounds.Min.Y)+0.5)/float64(hy) - 0.5
	ix, iy := int(math.Floor(cx)), int(math.Floor(cy))
	tx, ty := cx-math.Floor(cx), cy-math.Floor(cy)
	cw, ch := (bounds.Dx()+hx-1)/hx, (bounds.Dy()+hy-1)/hy
	chroma := func(plane []byte) byte {
		sum := 0.0
		for dy := 0; dy < 2; dy++ {
			wy := 1 - ty
			if dy == 1 {
				wy = ty
			}
			for dx := 0; dx < 2; dx++ {
				wx := 1 - tx
				if dx == 1 {
					wx = tx
				}
				offset := jpeg.COffset(bounds.Min.X+min(max(ix+dx, 0), cw-1)*hx, bounds.Min.Y+min(max(iy+dy, 0), ch-1)*hy)
				sum += float64(plane[offset]) * wx * wy
			}
		}
		return byte(math.Round(sum))
	}
	r, g, b := color.YCbCrToRGB(jpeg.Y[jpeg.YOffset(x, y)], chroma(jpeg.Cb), chroma(jpeg.Cr))
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}
