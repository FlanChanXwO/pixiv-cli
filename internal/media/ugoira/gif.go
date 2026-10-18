package ugoira

import (
	"compress/lzw"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/bits"
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
	// 两遍读取同一 ZIP：先采样全局颜色，再逐帧量化，不累计解码帧。
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
	w := gifWriter{ctx: ctx, output: output}
	if err := w.header(size, palette, hasTransparency); err != nil {
		return err
	}
	var indices, previous []byte
	for _, frame := range input.Frames {
		current, err = archive.decodeInto(ctx, frame, current)
		if err != nil {
			return err
		}
		current, err = resizeFrame(ctx, current, size)
		if err != nil {
			return err
		}
		indices, err = quantizer.quantizeInto(ctx, current, indices)
		if err != nil {
			return err
		}
		delay, err := gifDelay(frame.Delay, frame.File)
		if err != nil {
			return err
		}
		region := image.Rectangle{Max: size}
		if previous != nil {
			region, err = changedIndexRegion(ctx, previous, indices, size)
			if err != nil {
				return err
			}
		}
		if err := w.frame(region, size.X, indices, delay, hasTransparency); err != nil {
			return fmt.Errorf("encode GIF frame %q: %w", frame.File, err)
		}
		if !hasTransparency {
			previous = append(previous[:0], indices...)
		}
	}
	return w.write([]byte{0x3b})
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

type gifWriter struct {
	ctx        context.Context
	output     io.Writer
	compressor lzw.Writer
}

func (w *gifWriter) header(size image.Point, palette []rgb, transparent bool) error {
	var table [256 * 3]byte
	count := 0
	if transparent {
		count = 1 // 透明索引 0 的 RGB 固定为黑色。
	}
	for _, c := range palette {
		copy(table[count*3:], c[:])
		count++
	}
	tableBits := max(1, bits.Len(uint(count-1)))
	var header [13]byte
	copy(header[:], "GIF89a")
	binary.LittleEndian.PutUint16(header[6:8], uint16(size.X))
	binary.LittleEndian.PutUint16(header[8:10], uint16(size.Y))
	header[10] = 0x80 | byte(tableBits-1)*0x11
	if err := w.write(header[:]); err != nil {
		return err
	}
	if err := w.write(table[:3*(1<<tableBits)]); err != nil {
		return err
	}
	return w.write([]byte("\x21\xff\x0bNETSCAPE2.0\x03\x01\x00\x00\x00"))
}

func (w *gifWriter) frame(region image.Rectangle, stride int, indices []byte, delay uint16, transparent bool) error {
	control := [8]byte{0x21, 0xf9, 4, 2 << 2, 0, 0, 0, 0} // Background disposal。
	if transparent {
		control[3] |= 1
	} else {
		control[3] = 1 << 2
	}
	binary.LittleEndian.PutUint16(control[4:6], delay)
	if err := w.write(control[:]); err != nil {
		return err
	}
	descriptor := [10]byte{0x2c}
	binary.LittleEndian.PutUint16(descriptor[1:3], uint16(region.Min.X))
	binary.LittleEndian.PutUint16(descriptor[3:5], uint16(region.Min.Y))
	binary.LittleEndian.PutUint16(descriptor[5:7], uint16(region.Dx()))
	binary.LittleEndian.PutUint16(descriptor[7:9], uint16(region.Dy()))
	if err := w.write(descriptor[:]); err != nil {
		return err
	}
	var highest byte
	for y := region.Min.Y; y < region.Max.Y; y++ {
		row := y*stride + region.Min.X
		if err := w.ctx.Err(); err != nil {
			return err
		}
		for _, index := range indices[row : row+region.Dx()] {
			highest = max(highest, index)
		}
		if highest > 127 {
			break
		}
	}
	codeSize := max(2, bits.Len8(highest))
	if err := w.write([]byte{byte(codeSize)}); err != nil {
		return err
	}
	blocks := gifBlocks{writer: w}
	compressor := &w.compressor
	compressor.Reset(&blocks, lzw.LSB, codeSize)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		row := y*stride + region.Min.X
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if _, err := compressor.Write(indices[row : row+region.Dx()]); err != nil {
			return errors.Join(err, w.ctx.Err())
		}
	}
	if err := compressor.Close(); err != nil {
		return errors.Join(err, w.ctx.Err())
	}
	return w.write([]byte{0})
}

func (w *gifWriter) write(data []byte) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	n, err := w.output.Write(data)
	if n != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	return errors.Join(err, w.ctx.Err())
}

// GIF 子块最多 255 字节；直接接收 LZW 字节，避免缓存整帧压缩结果。
type gifBlocks struct {
	writer *gifWriter
	buffer [256]byte
	used   int
}

func (b *gifBlocks) WriteByte(value byte) error {
	b.used++
	b.buffer[b.used] = value
	if b.used == 255 {
		return b.Flush()
	}
	return nil
}

func (b *gifBlocks) Write(data []byte) (int, error) {
	for i, value := range data {
		if err := b.WriteByte(value); err != nil {
			return i, err
		}
	}
	return len(data), nil
}

func (b *gifBlocks) Flush() error {
	if b.used == 0 {
		return nil
	}
	b.buffer[0] = byte(b.used)
	if err := b.writer.write(b.buffer[:b.used+1]); err != nil {
		return err
	}
	b.used = 0
	return nil
}

func changedIndexRegion(ctx context.Context, previous, current []byte, size image.Point) (image.Rectangle, error) {
	region := image.Rectangle{Min: size}
	for y := 0; y < size.Y; y++ {
		if err := ctx.Err(); err != nil {
			return image.Rectangle{}, err
		}
		for x := 0; x < size.X; x++ {
			i := y*size.X + x
			if previous[i] == current[i] {
				continue
			}
			region.Min.X = min(region.Min.X, x)
			region.Min.Y = min(region.Min.Y, y)
			region.Max.X = max(region.Max.X, x+1)
			region.Max.Y = max(region.Max.Y, y+1)
		}
	}
	if region.Empty() {
		return image.Rect(0, 0, 1, 1), nil
	}
	return region, nil
}
