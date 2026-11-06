package ugoira

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
)

// 分别限制帧源数据与解码工作区，避免上游压缩帧导致无界分配。
const (
	maxFrameSourceBytes  = 512 * 1024 * 1024
	maxFrameDecodedBytes = 512 * 1024 * 1024
	// 覆盖四分量渐进 JPEG 的系数、像素平面与转换输出；PNG 最大为 RGBA64。
	decodedPixelBytes = 24
	// JPEG 的 MCU 最多按 32 像素对齐分配平面。
	decodedBlockEdge = 32
)

type frameArchive struct {
	zip    *zip.ReadCloser
	files  map[string]*zip.File
	size   image.Point
	source frameSource
	header bufio.Reader
}

func openFrameArchive(ctx context.Context, input Input) (*frameArchive, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(input.Frames) == 0 {
		return nil, errors.New("ugoira frames list is empty")
	}
	z, err := zip.OpenReader(input.ZipPath)
	if err != nil {
		if z != nil {
			err = errors.Join(err, z.Close())
		}
		return nil, fmt.Errorf("open ugoira zip: %w", err)
	}
	a := &frameArchive{zip: z, files: make(map[string]*zip.File, len(z.File))}
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, z.Close())
		}
		// 精确匹配成员名，重名时最后一个成员生效；不提取到磁盘。
		a.files[entry.Name] = entry
	}
	return a, nil
}

func (a *frameArchive) Close() error { return a.zip.Close() }

// reuse 必须是调用者已释放的帧；APNG 比较仍在使用的前帧不能传入。
func (a *frameArchive) decodeInto(ctx context.Context, frame Frame, reuse *image.NRGBA) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entry, ok := a.files[frame.File]
	if !ok {
		return nil, fmt.Errorf("frame %q: %w", frame.File, fs.ErrNotExist)
	}
	r, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open frame %q: %w", frame.File, err)
	}
	source, readErr := a.source.read(ctx, r, frame.File, entry.UncompressedSize64, maxFrameSourceBytes)
	if err := errors.Join(readErr, r.Close()); err != nil {
		return nil, err
	}
	decoded, err := decodeFrameSourceInto(ctx, source, reuse, &a.header)
	if err != nil {
		return nil, fmt.Errorf("decode frame %q: %w", frame.File, err)
	}
	size := decoded.Bounds().Size()
	if a.size != (image.Point{}) && size != a.size {
		return nil, fmt.Errorf("frame %q is %dx%d, expected %dx%d", frame.File, size.X, size.Y, a.size.X, a.size.Y)
	}
	a.size = size
	return decoded, nil
}

// frameSource 独占源数据与读取暂存区；read 结果仅在下一次同工作区读取前有效。
type frameSource struct {
	data  []byte
	chunk [32 * 1024]byte
}

func (s *frameSource) read(ctx context.Context, r io.Reader, file string, declaredSize, limit uint64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tooLarge := func(size uint64) error {
		return fmt.Errorf("frame %q source is %d bytes, exceeding image allocation limit %d bytes", file, size, limit)
	}
	if declaredSize > limit {
		return nil, tooLarge(declaredSize)
	}
	if declaredSize > uint64(math.MaxInt) {
		return nil, fmt.Errorf("cannot allocate %d bytes for frame %q source", declaredSize, file)
	}
	// 输入已消费完才复用；保留本次 limit 下的容量语义，不让旧缓冲绕过增长检查。
	source := s.data[:0:min(cap(s.data), int(min(limit, uint64(math.MaxInt))))]
	if cap(source) < int(declaredSize) {
		capacity := min(limit, uint64(math.MaxInt), max(declaredSize, uint64(cap(source))*2))
		source = make([]byte, 0, int(capacity))
	}
	// 分块读取使取消能在整帧读取完成前生效。
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := r.Read(s.chunk[:])
		if err := ctx.Err(); err != nil {
			if readErr == io.EOF {
				readErr = nil
			}
			return nil, errors.Join(err, readErr)
		}
		actualSize := uint64(len(source)) + uint64(n)
		if actualSize > limit {
			return nil, tooLarge(actualSize)
		}
		if actualSize > uint64(math.MaxInt) {
			return nil, fmt.Errorf("cannot allocate %d bytes for frame %q source", actualSize, file)
		}
		if actualSize > uint64(cap(source)) {
			capacity := min(limit, uint64(math.MaxInt), max(actualSize, uint64(cap(source))*2))
			grown := make([]byte, len(source), int(capacity))
			copy(grown, source)
			source = grown
		}
		source = append(source, s.chunk[:n]...)
		if readErr == io.EOF {
			s.data = source
			return source, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("read frame %q: %w", file, readErr)
		}
	}
}

func decodeFrameSourceInto(ctx context.Context, source []byte, reuse *image.NRGBA, header *bufio.Reader) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 复用格式探测的4KiB缓冲，Reset 丢弃上一帧头部或失败读取的残留。
	header.Reset(bytes.NewReader(source))
	config, format, err := image.DecodeConfig(header)
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("unsupported frame image format %q; expected jpeg or png", format)
	}
	if err := validateDecodedSize(config.Width, config.Height); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 标准库不提供解码循环的取消回调；只能在单帧解码返回后立即观察取消。
	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	return frameNRGBAInto(ctx, decoded, reuse)
}

func validateDecodedSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return errors.New("image dimensions must be non-zero")
	}
	w, h := uint64(width), uint64(height)
	// 在解码前计入块对齐，覆盖 JPEG 平面填充；NRGBA 转换缓冲也在此上限内。
	w = (w + decodedBlockEdge - 1) / decodedBlockEdge * decodedBlockEdge
	h = (h + decodedBlockEdge - 1) / decodedBlockEdge * decodedBlockEdge
	if w > uint64(math.MaxInt)/decodedPixelBytes/h {
		return errors.New("image dimensions overflow decoded buffer size")
	}
	if w*h > maxFrameDecodedBytes/decodedPixelBytes {
		return fmt.Errorf("decoded image exceeds frame allocation limit %d bytes", maxFrameDecodedBytes)
	}
	return nil
}

func frameNRGBAInto(ctx context.Context, src image.Image, dst *image.NRGBA) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if img, ok := src.(*image.NRGBA); ok {
		return img, nil
	}
	bounds := src.Bounds()
	if dst == nil || dst.Rect != (image.Rectangle{Max: bounds.Size()}) {
		dst = image.NewNRGBA(image.Rectangle{Max: bounds.Size()})
	}
	opaque := &image.RGBA{Pix: dst.Pix, Stride: dst.Stride, Rect: dst.Rect}
	round16 := func(v uint16) uint8 { return uint8((uint32(v) + 128) / 257) }
	for y := 0; y < dst.Rect.Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sy := y + bounds.Min.Y
		switch img := src.(type) {
		case *image.NRGBA64, *image.RGBA64, *image.Gray16, *image.Paletted:
			for x := 0; x < dst.Rect.Dx(); x++ {
				var c color.NRGBA
				switch v := img.At(x+bounds.Min.X, sy).(type) {
				case color.NRGBA64:
					// 保留透明像素隐藏的 RGB，并将 16 位通道舍入到 8 位。
					c = color.NRGBA{R: round16(v.R), G: round16(v.G), B: round16(v.B), A: round16(v.A)}
				case color.RGBA64:
					c = color.NRGBA{R: round16(v.R), G: round16(v.G), B: round16(v.B), A: 255}
				case color.Gray16:
					gray := round16(v.Y)
					c = color.NRGBA{R: gray, G: gray, B: gray, A: 255}
				default:
					c = color.NRGBAModel.Convert(v).(color.NRGBA)
				}
				dst.SetNRGBA(x, y, c)
			}
		default:
			// 其余返回值均不透明，可复用 draw 的 Gray/RGBA 快路径。
			draw.Draw(opaque, image.Rect(0, y, dst.Rect.Dx(), y+1), src, image.Pt(bounds.Min.X, sy), draw.Src)
		}
	}
	return dst, ctx.Err()
}
