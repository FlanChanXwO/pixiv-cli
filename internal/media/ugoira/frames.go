package ugoira

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
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

	framejpeg "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/jpeg"
)

// 沿用旧 image 0.25 的默认 max_alloc：分别约束单帧源字节和原色型解码缓冲。
const frameAllocationLimit = 512 * 1024 * 1024

type frameArchive struct {
	zip    *zip.ReadCloser
	files  map[string]*zip.File
	size   image.Point
	jpeg   framejpeg.Decoder
	source frameSource
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
		// 与旧 zip::by_name 一致：精确匹配，重名时最后一个 entry 生效；不提取到磁盘。
		a.files[entry.Name] = entry
	}
	return a, nil
}

func (a *frameArchive) Close() error { return a.zip.Close() }

func (a *frameArchive) decode(ctx context.Context, frame Frame) (*image.NRGBA, error) {
	return a.decodeInto(ctx, frame, nil)
}

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
	source, readErr := a.source.read(ctx, r, frame.File, entry.UncompressedSize64, frameAllocationLimit)
	if err := errors.Join(readErr, r.Close()); err != nil {
		return nil, err
	}
	decoded, err := decodeFrameSourceInto(ctx, source, reuse, &a.jpeg)
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

func readFrameSource(ctx context.Context, r io.Reader, file string, declaredSize, limit uint64) ([]byte, error) {
	var source frameSource
	return source.read(ctx, r, file, declaredSize, limit)
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
	// 32 KiB 只决定取消检查粒度，与旧读取器相同，不截断帧源。
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

func decodeFrameSource(ctx context.Context, source []byte) (*image.NRGBA, error) {
	var decoder framejpeg.Decoder
	return decodeFrameSourceInto(ctx, source, nil, &decoder)
}

func decodeFrameSourceInto(ctx context.Context, source []byte, reuse *image.NRGBA, decoder *framejpeg.Decoder) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source))
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	var bytesPerPixel uint64
	switch format {
	case "jpeg":
		bytesPerPixel = 3
		if config.ColorModel == color.GrayModel {
			bytesPerPixel = 1
		}
	case "png":
		bytesPerPixel, err = pngDecodedPixelBytes(source)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported frame image format %q; expected jpeg or png", format)
	}
	if err := validateDecodedSize(config.Width, config.Height, bytesPerPixel); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 标准库不提供解码循环的取消回调；只能在单帧解码返回后立即观察取消。
	var decoded image.Image
	if format == "jpeg" {
		decoded, err = decoder.Decode(bytes.NewReader(source))
	} else {
		decoded, _, err = image.Decode(bytes.NewReader(source))
	}
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	return frameNRGBAInto(ctx, decoded, reuse)
}

func validateDecodedSize(width, height int, bytesPerPixel uint64) error {
	if width <= 0 || height <= 0 {
		return errors.New("image dimensions must be non-zero")
	}
	w, h := uint64(width), uint64(height)
	if w > math.MaxUint64/h || w*h > uint64(math.MaxInt)/4 {
		return errors.New("image dimensions overflow RGBA buffer size")
	}
	if bytesPerPixel == 0 || w*h > frameAllocationLimit/bytesPerPixel {
		return fmt.Errorf("decoded image exceeds image allocation limit %d bytes", frameAllocationLimit)
	}
	return nil
}

// PNG EXPAND 后的原色型决定旧解码缓冲大小；Gray/GrayAlpha 不能一律按 RGBA 算。
func pngDecodedPixelBytes(source []byte) (uint64, error) {
	if len(source) < 33 {
		return 0, io.ErrUnexpectedEOF
	}
	var channels uint64
	switch source[25] {
	case 0:
		channels = 1
	case 2, 3:
		channels = 3
	case 4:
		channels = 2
	case 6:
		channels = 4
	default:
		return 0, errors.New("invalid PNG color type")
	}
	for offset := 8; ; {
		if len(source)-offset < 12 {
			return 0, io.ErrUnexpectedEOF
		}
		length := uint64(binary.BigEndian.Uint32(source[offset:]))
		if length > uint64(len(source)-offset-12) {
			return 0, io.ErrUnexpectedEOF
		}
		kind := string(source[offset+4 : offset+8])
		if kind == "tRNS" {
			if source[25] == 0 {
				channels = 2
			} else {
				channels = 4
			}
		}
		if kind == "IDAT" || kind == "IEND" {
			break
		}
		offset += int(length) + 12
	}
	if source[24] == 16 {
		channels *= 2
	}
	return channels, nil
}

func frameNRGBA(ctx context.Context, src image.Image) (*image.NRGBA, error) {
	return frameNRGBAInto(ctx, src, nil)
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
		case *image.YCbCr:
			// 沿用 zune-jpeg 0.5.15 的 14-bit BT.601 系数与 0.4999 舍入，避免量化中心漂移。
			// Copyright (c) 2023 zune-image developers, MIT；见 third_party/licenses/zune-jpeg-0.5.15/LICENSE-MIT。
			// 4:4:4 的三平面每像素连续，按行取片，避免逐像素重复 COffset 分派。
			var ys, cbs, crs []byte
			if img.SubsampleRatio == image.YCbCrSubsampleRatio444 {
				width := dst.Rect.Dx()
				yi, ci := img.YOffset(bounds.Min.X, sy), img.COffset(bounds.Min.X, sy)
				ys, cbs, crs = img.Y[yi:][:width], img.Cb[ci:][:width], img.Cr[ci:][:width]
			}
			for x := 0; x < dst.Rect.Dx(); x++ {
				var c color.YCbCr
				if ys != nil {
					c = color.YCbCr{Y: ys[x], Cb: cbs[x], Cr: crs[x]}
				} else {
					c = img.YCbCrAt(x+bounds.Min.X, sy)
				}
				yy := int32(c.Y)*16384 + 8191
				cb, cr := int32(c.Cb)-128, int32(c.Cr)-128
				p := dst.Pix[y*dst.Stride+x*4:][:4]
				p[0] = uint8(max(0, min(255, (yy+22970*cr)>>14)))
				p[1] = uint8(max(0, min(255, (yy-11700*cr-5638*cb)>>14)))
				p[2] = uint8(max(0, min(255, (yy+29032*cb)>>14)))
				p[3] = 255
			}
		case *image.NRGBA64, *image.RGBA64, *image.Gray16, *image.Paletted:
			for x := 0; x < dst.Rect.Dx(); x++ {
				var c color.NRGBA
				switch v := img.At(x+bounds.Min.X, sy).(type) {
				case color.NRGBA64:
					// 保留隐藏 RGB；16→8 沿用 image 的四舍五入，不走预乘 alpha 或高字节截断。
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
