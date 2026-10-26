package ugoira

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"math"
)

func encodeAPNG(ctx context.Context, output io.Writer, input Input) (err error) {
	if uint64(len(input.Frames)) > math.MaxUint32 {
		return errors.New("APNG frame count exceeds uint32")
	}
	archive, err := openFrameArchive(ctx, input)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	w := apngWriter{ctx: ctx, output: output}
	var previous, spare *image.NRGBA
	var size image.Point
	for i, frame := range input.Frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		numerator, denominator, err := apngDelay(frame.Delay, frame.File)
		if err != nil {
			return err
		}
		current, err := archive.decodeInto(ctx, frame, spare)
		if err != nil {
			return err
		}
		if i == 0 {
			width, height := scaledDimensions(uint32(current.Rect.Dx()), uint32(current.Rect.Dy()), input.MaxEdge)
			size = image.Pt(int(width), int(height))
		}
		current, err = resizeFrame(ctx, current, size)
		if err != nil {
			return err
		}
		region := current.Rect
		if i == 0 {
			if err := w.header(size, uint32(len(input.Frames))); err != nil {
				return err
			}
		} else {
			region, err = changedRegion(ctx, previous, current)
			if err != nil {
				return err
			}
		}
		if err := w.frame(current, previous, region, numerator, denominator); err != nil {
			return fmt.Errorf("encode APNG frame %q: %w", frame.File, err)
		}
		previous, spare = current, previous
	}
	return w.chunk("IEND")
}

func apngDelay(delay int, file string) (uint16, uint16, error) {
	if delay <= 0 {
		return 0, 0, fmt.Errorf("frame %q delay must be positive, got %d ms", file, delay)
	}
	divisor, remainder := delay, 1000
	for remainder != 0 {
		divisor, remainder = remainder, divisor%remainder
	}
	numerator := delay / divisor
	if numerator > math.MaxUint16 {
		return 0, 0, fmt.Errorf("frame %q delay %d ms exceeds APNG fraction range", file, delay)
	}
	return uint16(numerator), uint16(1000 / divisor), nil
}

func changedRegion(ctx context.Context, previous, current *image.NRGBA) (image.Rectangle, error) {
	region := image.Rectangle{Min: current.Rect.Max, Max: current.Rect.Min}
	for y := current.Rect.Min.Y; y < current.Rect.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return image.Rectangle{}, err
		}
		old := previous.Pix[previous.PixOffset(previous.Rect.Min.X, y):][:current.Rect.Dx()*4]
		row := current.Pix[current.PixOffset(current.Rect.Min.X, y):][:current.Rect.Dx()*4]
		if bytes.Equal(old, row) {
			continue
		}
		for x := current.Rect.Min.X; x < current.Rect.Max.X; x++ {
			i := (x - current.Rect.Min.X) * 4
			if binary.LittleEndian.Uint32(old[i:]) == binary.LittleEndian.Uint32(row[i:]) {
				continue
			}
			region.Min.X = min(region.Min.X, x)
			region.Min.Y = min(region.Min.Y, y)
			region.Max.X = max(region.Max.X, x+1)
			region.Max.Y = max(region.Max.Y, y+1)
		}
	}
	if region.Empty() {
		return image.Rectangle{Min: current.Rect.Min, Max: current.Rect.Min.Add(image.Pt(1, 1))}, nil
	}
	return region, nil
}

type apngWriter struct {
	ctx         context.Context
	output      io.Writer
	sequence    uint64
	compressor  *zlib.Writer
	chunkBuffer *bufio.Writer
}

func (w *apngWriter) header(size image.Point, count uint32) error {
	if err := writeAnimation(w.ctx, w.output, []byte("\x89PNG\r\n\x1a\n")); err != nil {
		return err
	}
	var header [13]byte
	binary.BigEndian.PutUint32(header[0:4], uint32(size.X))
	binary.BigEndian.PutUint32(header[4:8], uint32(size.Y))
	header[8], header[9] = 8, 6 // 固定 RGBA8，避免全不透明帧自动变为 RGB。
	if err := w.chunk("IHDR", header[:]); err != nil {
		return err
	}
	var animation [8]byte
	binary.BigEndian.PutUint32(animation[:4], count) // play count 0：无限循环。
	return w.chunk("acTL", animation[:])
}

func (w *apngWriter) frame(img, previous *image.NRGBA, region image.Rectangle, numerator, denominator uint16) error {
	over, err := apngOverRegion(w.ctx, previous, img, region)
	if err != nil {
		return err
	}
	sequence, err := w.nextSequence()
	if err != nil {
		return err
	}
	var control [26]byte
	for i, value := range []uint32{sequence, uint32(region.Dx()), uint32(region.Dy()), uint32(region.Min.X - img.Rect.Min.X), uint32(region.Min.Y - img.Rect.Min.Y)} {
		binary.BigEndian.PutUint32(control[i*4:], value)
	}
	binary.BigEndian.PutUint16(control[20:22], numerator)
	binary.BigEndian.PutUint16(control[22:24], denominator)
	// 只有不透明变化允许 Over；透明变化仍以 Source 精确替换。
	if over {
		control[25] = 1
	}
	if err := w.chunk("fcTL", control[:]); err != nil {
		return err
	}
	data := apngFrameData{writer: w, first: previous == nil}
	if w.chunkBuffer == nil {
		// 与标准库 PNG 一样合并为32KiB写入，避免每次 flate 小刷新都分配一个 chunk。
		w.chunkBuffer = bufio.NewWriterSize(data, 1<<15)
	} else {
		w.chunkBuffer.Reset(data)
	}
	if w.compressor == nil {
		// Go 1.27 快速档的高纹理样本超出原10%体积门槛，改用level 7的完整匹配。
		w.compressor, err = zlib.NewWriterLevel(w.chunkBuffer, 7)
		if err != nil {
			return err
		}
	} else {
		// 每帧独立的 zlib 流；Reset 只复用压缩器内存，不带入上一帧字典。
		w.compressor.Reset(w.chunkBuffer)
	}
	rowBytes := region.Dx() * 4
	filtered := make([]byte, rowBytes+1)
	var delta []byte
	if over {
		delta = make([]byte, rowBytes)
	}
	for y := region.Min.Y; y < region.Max.Y; y++ {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		row := img.Pix[img.PixOffset(region.Min.X, y):][:rowBytes]
		if over {
			prior := previous.Pix[previous.PixOffset(region.Min.X, y):][:rowBytes]
			for i := 0; i < rowBytes; i += 4 {
				if bytes.Equal(row[i:i+4], prior[i:i+4]) {
					clear(delta[i : i+4])
				} else {
					copy(delta[i:i+4], row[i:i+4])
				}
			}
			row = delta
		}
		filterAPNGRow(row, filtered)
		if _, err := w.compressor.Write(filtered); err != nil {
			return errors.Join(err, w.ctx.Err())
		}
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if err := w.compressor.Close(); err != nil {
		return errors.Join(err, w.ctx.Err())
	}
	return errors.Join(w.chunkBuffer.Flush(), w.ctx.Err())
}

func (w *apngWriter) nextSequence() (uint32, error) {
	if w.sequence > math.MaxUint32 {
		return 0, errors.New("APNG sequence overflow")
	}
	sequence := uint32(w.sequence)
	w.sequence++
	return sequence, nil
}

func (w *apngWriter) chunk(kind string, parts ...[]byte) error {
	var length uint64
	for _, part := range parts {
		length += uint64(len(part))
		if length > math.MaxInt32 { // PNG 规范限制 chunk length 为 2^31-1。
			return errors.New("PNG chunk length exceeds 2^31-1")
		}
	}
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(length))
	copy(header[4:], kind)
	if err := writeAnimation(w.ctx, w.output, header[:]); err != nil {
		return err
	}
	checksum := crc32.ChecksumIEEE(header[4:])
	for _, part := range parts {
		if err := writeAnimation(w.ctx, w.output, part); err != nil {
			return err
		}
		checksum = crc32.Update(checksum, crc32.IEEETable, part)
	}
	var trailer [4]byte
	binary.BigEndian.PutUint32(trailer[:], checksum)
	return writeAnimation(w.ctx, w.output, trailer[:])
}

type apngFrameData struct {
	writer *apngWriter
	first  bool
}

func (d apngFrameData) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var err error
	if d.first {
		err = d.writer.chunk("IDAT", data)
	} else {
		sequence, sequenceErr := d.writer.nextSequence()
		if sequenceErr != nil {
			return 0, sequenceErr
		}
		var prefix [4]byte
		binary.BigEndian.PutUint32(prefix[:], sequence)
		err = d.writer.chunk("fdAT", prefix[:], data)
	}
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func apngOverRegion(ctx context.Context, previous, current *image.NRGBA, region image.Rectangle) (bool, error) {
	if previous == nil {
		return false, nil
	}
	// Sub 的非零残差位置越少越易压缩；避免透明差分打碎原本连续的色块。
	var sourceRuns, deltaRuns uint64
	for y := region.Min.Y; y < region.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		old := previous.Pix[previous.PixOffset(region.Min.X, y):][:region.Dx()*4]
		row := current.Pix[current.PixOffset(region.Min.X, y):][:region.Dx()*4]
		var sourceLeft, deltaLeft uint32
		for i := 0; i < len(row); i += 4 {
			pixel := binary.LittleEndian.Uint32(row[i:])
			delta := pixel
			if pixel == binary.LittleEndian.Uint32(old[i:]) {
				delta = 0
			} else if row[i+3] != 255 {
				return false, nil
			}
			if pixel != sourceLeft {
				sourceRuns++
			}
			if delta != deltaLeft {
				deltaRuns++
			}
			sourceLeft, deltaLeft = pixel, delta
		}
	}
	return deltaRuns < sourceRuns, nil
}
