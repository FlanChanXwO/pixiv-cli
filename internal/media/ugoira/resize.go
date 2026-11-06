package ugoira

import (
	"context"
	"fmt"
	"image"
)

func scaledDimensions(width, height, maxEdge uint32) (uint32, uint32) {
	if width == 0 || height == 0 {
		return 0, 0
	}
	if maxEdge == 0 || max(width, height) <= maxEdge {
		return width, height
	}
	if width >= height {
		return maxEdge, uint32(max(uint64(height)*uint64(maxEdge)/uint64(width), 1))
	}
	return uint32(max(uint64(width)*uint64(maxEdge)/uint64(height), 1)), maxEdge
}

func resizeFrame(ctx context.Context, src *image.NRGBA, size image.Point) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	width, height := src.Rect.Dx(), src.Rect.Dy()
	if size.X <= 0 || size.Y <= 0 || size.X > width || size.Y > height {
		return nil, fmt.Errorf("invalid resize dimensions %dx%d for %dx%d frame", size.X, size.Y, width, height)
	}
	if size == src.Rect.Size() {
		return src, nil
	}
	dst := image.NewNRGBA(image.Rectangle{Max: size})
	// 取目标像素中心的双线性插值；四通道独立处理，保留透明像素的隐藏 RGB。
	for y := 0; y < size.Y; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sy := max(0, (float64(y)+0.5)*float64(height)/float64(size.Y)-0.5)
		y0 := int(sy)
		y1 := min(y0+1, height-1)
		fy := sy - float64(y0)
		for x := 0; x < size.X; x++ {
			sx := max(0, (float64(x)+0.5)*float64(width)/float64(size.X)-0.5)
			x0 := int(sx)
			x1 := min(x0+1, width-1)
			fx := sx - float64(x0)
			p00 := src.PixOffset(src.Rect.Min.X+x0, src.Rect.Min.Y+y0)
			p10 := src.PixOffset(src.Rect.Min.X+x1, src.Rect.Min.Y+y0)
			p01 := src.PixOffset(src.Rect.Min.X+x0, src.Rect.Min.Y+y1)
			p11 := src.PixOffset(src.Rect.Min.X+x1, src.Rect.Min.Y+y1)
			pixel := dst.Pix[y*dst.Stride+x*4:][:4]
			for c := range pixel {
				top := float64(src.Pix[p00+c])*(1-fx) + float64(src.Pix[p10+c])*fx
				bottom := float64(src.Pix[p01+c])*(1-fx) + float64(src.Pix[p11+c])*fx
				pixel[c] = uint8(top*(1-fy) + bottom*fy + 0.5)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return dst, nil
}
