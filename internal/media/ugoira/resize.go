package ugoira

import (
	"context"
	"fmt"
	"image"
	"math"
)

// Lanczos3 移植自 image 0.25.10 的 imageops/sample.rs（The image-rs Developers）。
// 按 MIT 使用；许可正文见 third_party/licenses/image-0.25.10/LICENSE-MIT。

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
	if width > math.MaxInt/16 {
		return nil, fmt.Errorf("image width %d overflows float32 resize row", width)
	}

	xWeights := make([]resizeWeights, size.X)
	for x := range xWeights {
		weights, err := lanczosWeights(ctx, width, size.X, x, nil)
		if err != nil {
			return nil, err
		}
		xWeights[x] = weights
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	// 保持先纵后横的 float32 运算，只留一行未夹取的中间值；四通道均不预乘 alpha。
	row := make([]float32, width*4)
	var yWeights resizeWeights
	for y := 0; y < size.Y; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		weights, err := lanczosWeights(ctx, height, size.Y, y, yWeights.values[:0])
		if err != nil {
			return nil, err
		}
		yWeights = weights
		clear(row)
		for i, weight := range yWeights.values {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			pixels := src.Pix[(yWeights.start+i)*src.Stride:][:width*4]
			for j, value := range pixels {
				// 显式转换阻止 Go 融合乘加，保留旧实现每步 float32 舍入。
				row[j] += float32(float32(value) * weight)
			}
		}
		for x, weights := range xWeights {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			var pixel [4]float32
			for i, weight := range weights.values {
				p := row[(weights.start+i)*4:][:4]
				for c := range pixel {
					pixel[c] += float32(p[c] * weight)
				}
			}
			p := dst.Pix[y*dst.Stride+x*4:][:4]
			for c, value := range pixel {
				p[c] = resizeByte(value)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return dst, nil
}

type resizeWeights struct {
	start  int
	values []float32
}

func lanczosWeights(ctx context.Context, source, target, out int, values []float32) (resizeWeights, error) {
	ratio := float32(source) / float32(target)
	scale := max(ratio, 1)
	support := 3 * scale
	// 先按 float32 舍入中心；随后减 0.5 跨 binade 时，禁止 Go 融合这两步。
	center := float32((float32(out) + 0.5) * ratio)
	left := max(0, min(int(math.Floor(float64(center-support))), source-1))
	right := max(left+1, min(int(math.Ceil(float64(center+support))), source))
	center -= 0.5
	var sum float32
	for i := left; i < right; i++ {
		if err := ctx.Err(); err != nil {
			return resizeWeights{}, err
		}
		value := lanczos3((float32(i) - center) / scale)
		values = append(values, value)
		sum += value
	}
	for i := range values {
		values[i] /= sum
	}
	return resizeWeights{start: left, values: values}, nil
}

func lanczos3(x float32) float32 {
	if x <= -3 || x >= 3 {
		return 0
	}
	sinc := func(t float32) float32 {
		if t == 0 {
			return 1
		}
		a := t * float32(math.Pi)
		return lanczosSine(a) / a
	}
	return sinc(x) * sinc(x/3)
}

// lanczos3 将角度限于 (-3π, 3π)；固定旧基准的多项式与 FMA 求值，避免平台 sinf 舍入漂移。
func lanczosSine(angle float32) float32 {
	if angle == 0 {
		return angle
	}
	x := float64(angle)
	n := int64(math.RoundToEven(x * (2 / math.Pi)))
	x = math.FMA(-float64(n), math.Pi/2, x)
	z := x * x
	var value float64
	if n&1 == 0 {
		x3 := x * z
		p := math.FMA(z, -0.00019515283191485737, 0.008332160761855575)
		base := math.FMA(x3, -0.16666654609548576, x)
		value = math.FMA(z*x3, p, base)
	} else {
		a := math.FMA(z, 2.43835673118386e-05, -0.0013886681647984319)
		b := math.FMA(z, 0.041666620357131375, -0.4999999969447599)
		value = math.FMA(z, math.FMA(z*z, a, b), 1)
	}
	if (n>>1)&1 != 0 {
		value = -value
	}
	return float32(value)
}

func resizeByte(value float32) uint8 {
	return uint8(math.Round(float64(max(0, min(value, 255)))))
}
