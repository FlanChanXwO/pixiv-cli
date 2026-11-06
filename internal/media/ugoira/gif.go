package ugoira

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
)

func encodeGIF(ctx context.Context, output io.Writer, input Input) (err error) {
	archive, err := openFrameArchive(ctx, input)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	statistics := newColorStatistics()
	var size image.Point
	var current *image.NRGBA
	// 两遍读取同一 ZIP：先统计全局颜色，再逐帧量化，复用彩色解码缓冲。
	for i, frame := range input.Frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := gifDelay(frame.Delay, frame.File); err != nil {
			return err
		}
		current, err = archive.decodeInto(ctx, frame, current)
		if err != nil {
			return err
		}
		if i == 0 {
			width, height := scaledDimensions(uint32(current.Rect.Dx()), uint32(current.Rect.Dy()), input.MaxEdge)
			size = image.Pt(int(width), int(height))
			if width > math.MaxUint16 || height > math.MaxUint16 {
				return fmt.Errorf("frame %q size %dx%d exceeds GIF uint16 dimensions", frame.File, width, height)
			}
		}
		current, err = resizeFrame(ctx, current, size)
		if err != nil {
			return err
		}
		if err := statistics.addFrame(ctx, current); err != nil {
			return err
		}
	}
	palette, err := globalPalette(ctx, statistics)
	if err != nil {
		return err
	}
	hasTransparency := statistics.hasTransparency
	quantizer, err := newFrameQuantizer(palette, hasTransparency)
	if err != nil {
		return err
	}
	colors := make(color.Palette, 0, 256)
	if hasTransparency {
		colors = append(colors, color.NRGBA{})
	}
	for _, c := range palette {
		colors = append(colors, color.NRGBA{R: c[0], G: c[1], B: c[2], A: 255})
	}
	animation := gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: colors, Width: size.X, Height: size.Y}}
	// 标准库 EncodeAll 接收全部索引帧；彩色解码缓冲仍逐帧复用。
	for _, frame := range input.Frames {
		current, err = archive.decodeInto(ctx, frame, current)
		if err != nil {
			return err
		}
		current, err = resizeFrame(ctx, current, size)
		if err != nil {
			return err
		}
		indices, err := quantizer.quantize(ctx, current)
		if err != nil {
			return err
		}
		delay, err := gifDelay(frame.Delay, frame.File)
		if err != nil {
			return err
		}
		animation.Image = append(animation.Image, &image.Paletted{Pix: indices, Stride: size.X, Rect: image.Rectangle{Max: size}, Palette: colors})
		animation.Delay = append(animation.Delay, int(delay))
		disposal := byte(gif.DisposalNone)
		if hasTransparency {
			disposal = gif.DisposalBackground
		}
		animation.Disposal = append(animation.Disposal, disposal)
	}
	return errors.Join(gif.EncodeAll(animationWriter{ctx: ctx, output: output}, &animation), ctx.Err())
}

func gifDelay(delay int, file string) (uint16, error) {
	if delay <= 0 {
		return 0, fmt.Errorf("frame %q delay must be positive, got %d ms", file, delay)
	}
	// GIF 以百分之一秒存储 uint16 延迟，向上取整且不截断有效输入。
	ticks := (delay-1)/10 + 1
	if ticks > math.MaxUint16 {
		return 0, fmt.Errorf("frame %q delay %d ms exceeds GIF uint16 range", file, delay)
	}
	return uint16(ticks), nil
}
