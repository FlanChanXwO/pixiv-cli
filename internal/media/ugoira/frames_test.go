package ugoira

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameArchiveOrderAndPixels(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	copy(first.Pix, []byte{9, 31, 77, 0, 83, 157, 203, 129, 1, 2, 3, 255})
	second := image.NewNRGBA(first.Rect)
	copy(second.Pix, []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255})
	archive := writeFrameArchive(t, []frameZipEntry{
		{"second.bin", framePNG(t, second)},
		{"first.bin", framePNG(t, second)},
		{"first.bin", framePNG(t, first)},
	})
	frames := []Frame{{File: "first.bin", Delay: 10}, {File: "second.bin", Delay: 20}, {File: "first.bin", Delay: 30}}
	a, err := openFrameArchive(t.Context(), Input{ZipPath: archive, Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i, want := range []*image.NRGBA{first, second, first} {
		got, err := a.decode(t.Context(), frames[i])
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("frame %d: got %v %v, want %v %v", i, got.Bounds(), got.Pix, want.Bounds(), want.Pix)
		}
	}
}

func TestFrameArchiveFailures(t *testing.T) {
	pngBody := framePNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 1)))
	path := writeFrameArchive(t, []frameZipEntry{{"a.png", pngBody}, {"different.png", framePNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))}, {"broken.png", []byte("not an image")}})
	input := Input{ZipPath: path, Frames: []Frame{{File: "a.png", Delay: 10}}}
	if _, err := openFrameArchive(t.Context(), Input{ZipPath: path}); err == nil || !strings.Contains(err.Error(), "frames list is empty") {
		t.Fatalf("empty frames: %v", err)
	}
	if _, err := openFrameArchive(t.Context(), Input{ZipPath: path + ".missing", Frames: input.Frames}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing archive: %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(bad, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openFrameArchive(t.Context(), Input{ZipPath: bad, Frames: input.Frames}); !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("broken archive: %v", err)
	}
	empty := writeFrameArchive(t, nil)
	a, err := openFrameArchive(t.Context(), Input{ZipPath: empty, Frames: input.Frames})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.decode(t.Context(), input.Frames[0]); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty archive: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = openFrameArchive(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.decode(t.Context(), input.Frames[0]); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, want string }{{"absent.png", "file does not exist"}, {"different.png", "expected 2x1"}, {"broken.png", "image: unknown format"}} {
		t.Run(tc.file, func(t *testing.T) {
			if _, err := a.decode(t.Context(), Frame{File: tc.file}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decode: %v; want %q", err, tc.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := openFrameArchive(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("open canceled: %v", err)
	}
	if _, err := a.decode(ctx, input.Frames[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode canceled: %v", err)
	}
}

func TestFrameArchiveRejectsCRCAndLengthCorruption(t *testing.T) {
	body := framePNG(t, image.NewGray(image.Rect(0, 0, 2, 2)))
	for _, mode := range []string{"crc", "short", "long", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			path := writeFrameArchive(t, []frameZipEntry{{"frame.png", body}})
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			central := bytes.Index(data, []byte{'P', 'K', 1, 2})
			switch mode {
			case "crc":
				data[central+16] ^= 1
			case "short":
				binary.LittleEndian.PutUint32(data[central+24:], uint32(len(body)-1))
			case "long":
				binary.LittleEndian.PutUint32(data[central+24:], uint32(len(body)+1))
			case "oversized":
				binary.LittleEndian.PutUint32(data[central+24:], frameAllocationLimit+1)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			a, err := openFrameArchive(t.Context(), Input{ZipPath: path, Frames: []Frame{{File: "frame.png"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			_, err = a.decode(t.Context(), Frame{File: "frame.png"})
			if err == nil {
				t.Fatal("corrupt entry accepted")
			}
			if mode == "crc" && !errors.Is(err, zip.ErrChecksum) {
				t.Fatalf("CRC error lost: %v", err)
			}
			if mode == "oversized" && !strings.Contains(err.Error(), "exceeding image allocation limit") {
				t.Fatalf("source limit: %v", err)
			}
		})
	}
}

func TestFrameArchiveHonorsZIPPathPolicy(t *testing.T) {
	t.Setenv("GODEBUG", "zipinsecurepath=0")
	path := writeFrameArchive(t, []frameZipEntry{{"../frame.png", framePNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))}})
	if _, err := openFrameArchive(t.Context(), Input{ZipPath: path, Frames: []Frame{{File: "../frame.png"}}}); !errors.Is(err, zip.ErrInsecurePath) {
		t.Fatalf("ZIP path policy error lost: %v", err)
	}
}

func TestReadFrameSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared uint64
		limit    uint64
		body     string
		wantErr  string
	}{
		{"exact", 8, 8, "12345678", ""},
		{"short reads grow within limit", 5, 8, "12345678", ""},
		{"actual overflow", 4, 8, "123456789", "exceeding image allocation limit 8"},
		{"zero", 0, 0, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &frameShortReader{Reader: strings.NewReader(tc.body)}
			got, err := readFrameSource(t.Context(), r, "sample", tc.declared, tc.limit)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || string(got) != tc.body || uint64(cap(got)) > tc.limit {
				t.Fatalf("got %q cap=%d err=%v", got, cap(got), err)
			}
		})
	}
	mustNotRead := frameReadFunc(func([]byte) (int, error) { t.Fatal("read oversized source"); return 0, nil })
	for _, sizes := range [][2]uint64{{9, 8}, {math.MaxUint64, math.MaxUint64}} {
		if _, err := readFrameSource(t.Context(), mustNotRead, "large", sizes[0], sizes[1]); err == nil {
			t.Fatal("oversized declaration accepted")
		}
	}
	readErr := errors.New("source read failed")
	ctx, cancel := context.WithCancel(t.Context())
	r := frameReadFunc(func(p []byte) (int, error) { cancel(); p[0] = 1; return 1, readErr })
	if _, err := readFrameSource(ctx, r, "cancel", 1, 8); !errors.Is(err, context.Canceled) || !errors.Is(err, readErr) {
		t.Fatalf("read/cancellation cause lost: %v", err)
	}
	if _, err := readFrameSource(ctx, mustNotRead, "cancel", 1, 8); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled read: %v", err)
	}
	r = frameReadFunc(func([]byte) (int, error) { return 0, readErr })
	if _, err := readFrameSource(t.Context(), r, "error", 0, 8); !errors.Is(err, readErr) {
		t.Fatalf("read cause lost: %v", err)
	}
}

func TestDecodedFrameLimits(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		bpp           uint64
		valid         bool
	}{
		{1, 1, 4, true}, {1 << 17, 1 << 12, 1, true}, {1 << 17, (1 << 12) + 1, 1, false},
		{1 << 13, 1 << 13, 8, true}, {(1 << 13) + 1, 1 << 13, 8, false},
		{0, 1, 4, false}, {1, -1, 4, false}, {math.MaxInt, math.MaxInt, 4, false},
		{math.MaxInt / 4, 2, 1, false}, {1, 1, 0, false},
	} {
		err := validateDecodedSize(tc.width, tc.height, tc.bpp)
		if (err == nil) != tc.valid {
			t.Errorf("%dx%d/%d: err=%v, valid=%v", tc.width, tc.height, tc.bpp, err, tc.valid)
		}
	}
	body := framePNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(body[16:], math.MaxInt32)
	binary.BigEndian.PutUint32(body[20:], math.MaxInt32)
	binary.BigEndian.PutUint32(body[29:], crc32.ChecksumIEEE(body[12:29]))
	if _, err := decodeFrameSource(t.Context(), body); err == nil {
		t.Fatal("huge header accepted")
	}
	if _, err := decodeFrameSource(t.Context(), body[:len(body)-8]); err == nil {
		t.Fatal("truncated PNG accepted")
	}
}

func TestDecodeFrameFormats(t *testing.T) {
	wide := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	wide.SetNRGBA64(0, 0, color.NRGBA64{R: 129, G: 32768, B: 65406, A: 0})
	wide.SetNRGBA64(1, 0, color.NRGBA64{R: 128, G: 32767, B: 65535, A: 32768})
	gray := image.NewGray16(image.Rect(0, 0, 2, 1))
	gray.SetGray16(0, 0, color.Gray16{Y: 129})
	gray.SetGray16(1, 0, color.Gray16{Y: 65406})
	palette := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.NRGBA{R: 17, G: 31, B: 71, A: 0}, color.NRGBA{R: 33, G: 159, B: 199, A: 129}})
	palette.Pix[1] = 1
	for _, tc := range []struct {
		name string
		src  image.Image
		want []byte
	}{
		{"RGBA16", wide, []byte{1, 128, 254, 0, 0, 127, 255, 128}},
		{"gray16", gray, []byte{1, 1, 1, 255, 254, 254, 254, 255}},
		{"palette alpha", palette, []byte{17, 31, 71, 0, 33, 159, 199, 129}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFrameSource(t.Context(), framePNG(t, tc.src))
			if err != nil || !bytes.Equal(got.Pix, tc.want) {
				t.Fatalf("got %v, err=%v; want %v", got, err, tc.want)
			}
		})
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, gray, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	got, err := decodeFrameSource(t.Context(), encoded.Bytes())
	if err != nil || got.Bounds() != gray.Bounds() || !got.Opaque() {
		t.Fatalf("JPEG: got %v, err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := decodeFrameSource(ctx, encoded.Bytes()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled decode: %v", err)
	}
	encoded.Reset()
	if err := gif.Encode(&encoded, palette, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFrameSource(t.Context(), encoded.Bytes()); err == nil || !strings.Contains(err.Error(), "unsupported frame image format") {
		t.Fatalf("GIF frame was not rejected: %v", err)
	}
}

func TestDecodePNGColorTypes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		depth, kind byte
		row, trns   []byte
		bpp         uint64
		want        []byte
	}{
		{"gray1", 1, 0, []byte{0x80}, nil, 1, []byte{255, 255, 255, 255}},
		{"gray8 transparent", 8, 0, []byte{31}, []byte{0, 31}, 2, []byte{31, 31, 31, 0}},
		{"gray16 transparent", 16, 0, []byte{0, 129}, []byte{0, 129}, 4, []byte{1, 1, 1, 0}},
		{"gray alpha8", 8, 4, []byte{71, 129}, nil, 2, []byte{71, 71, 71, 129}},
		{"gray alpha16", 16, 4, []byte{0, 129, 0x80, 0}, nil, 4, []byte{1, 1, 1, 128}},
		{"rgb8", 8, 2, []byte{17, 31, 71}, nil, 3, []byte{17, 31, 71, 255}},
		{"rgb8 transparent", 8, 2, []byte{17, 31, 71}, []byte{0, 17, 0, 31, 0, 71}, 4, []byte{17, 31, 71, 0}},
		{"rgb16", 16, 2, []byte{0, 129, 0x80, 0, 0xff, 0x7e}, nil, 6, []byte{1, 128, 254, 255}},
		{"rgb16 transparent", 16, 2, []byte{0, 129, 0x80, 0, 0xff, 0x7e}, []byte{0, 129, 0x80, 0, 0xff, 0x7e}, 8, []byte{1, 128, 254, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := frameRawPNG(t, tc.depth, tc.kind, tc.row, tc.trns)
			if bpp, err := pngDecodedPixelBytes(body); err != nil || bpp != tc.bpp {
				t.Fatalf("decoded pixel size %d, err=%v; want %d", bpp, err, tc.bpp)
			}
			got, err := decodeFrameSource(t.Context(), body)
			if err != nil || !bytes.Equal(got.Pix, tc.want) {
				t.Fatalf("got %v, err=%v; want %v", got, err, tc.want)
			}
		})
	}
	body := frameRawPNG(t, 8, 0, []byte{1}, nil)
	for _, pos := range []int{33, 41} {
		bad := bytes.Clone(body)
		bad[pos] ^= 0xff
		if _, err := decodeFrameSource(t.Context(), bad); err == nil {
			t.Fatalf("bad PNG length/payload at %d accepted", pos)
		}
	}
}

func TestDecodeJPEGMatchesOldPixels(t *testing.T) {
	// 17×9 合成图；参考像素由旧 zune-jpeg 0.5.15 解码，包含 IDCT 舍入边界。
	for _, kind := range []string{"444", "progressive", "gray", "restart"} {
		t.Run(kind, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "jpeg-"+kind+".jpg"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "jpeg-"+kind+".rgba"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeFrameSource(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			if got.Rect != image.Rect(0, 0, 17, 9) || !bytes.Equal(got.Pix, want) {
				t.Fatal("JPEG IDCT/color conversion changed the reference pixels")
			}
		})
	}
}

func TestFrameNRGBACancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	src := &frameCancelImage{Image: image.NewGray(image.Rect(0, 0, 2, 2)), cancel: cancel}
	if _, err := frameNRGBA(ctx, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("normalization canceled: %v", err)
	}
	if _, err := frameNRGBA(ctx, image.NewNRGBA(image.Rect(0, 0, 1, 1))); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled normalization: %v", err)
	}
}

type frameCancelImage struct {
	image.Image
	cancel context.CancelFunc
}

func (img *frameCancelImage) At(x, y int) color.Color {
	img.cancel()
	return img.Image.At(x, y)
}

func frameRawPNG(t *testing.T, depth, kind byte, row, transparency []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		head := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
		out.Write(head)
		payload := append([]byte(kind), data...)
		out.Write(payload)
		out.Write(binary.BigEndian.AppendUint32(nil, crc32.ChecksumIEEE(payload)))
	}
	chunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, depth, kind, 0, 0, 0})
	if transparency != nil {
		chunk("tRNS", transparency)
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(append([]byte{0}, row...)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	chunk("IDAT", compressed.Bytes())
	chunk("IEND", nil)
	return out.Bytes()
}

type frameZipEntry struct {
	name string
	body []byte
}

func writeFrameArchive(t *testing.T, entries []frameZipEntry) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range entries {
		f, err := w.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "frames.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func framePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type frameReadFunc func([]byte) (int, error)

func (f frameReadFunc) Read(p []byte) (int, error) { return f(p) }

type frameShortReader struct{ io.Reader }

func (r *frameShortReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:min(len(p), 3)]) }

func TestFrameArchiveReusesOnlyReleasedFrame(t *testing.T) {
	red := image.NewRGBA(image.Rect(0, 0, 17, 9))
	for i := 0; i < len(red.Pix); i += 4 {
		copy(red.Pix[i:i+4], []byte{210, 30, 60, 255})
	}
	blue := image.NewRGBA(red.Rect)
	for i := 0; i < len(blue.Pix); i += 4 {
		copy(blue.Pix[i:i+4], []byte{10, 40, 230, 255})
	}
	var entries []frameZipEntry
	for i, img := range []image.Image{red, blue} {
		var b bytes.Buffer
		if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, frameZipEntry{fmt.Sprintf("%d.jpg", i), b.Bytes()})
	}
	entries = append(entries, frameZipEntry{"alpha.png", framePNG(t, image.NewNRGBA(red.Rect))})
	frames := []Frame{{File: "0.jpg"}, {File: "1.jpg"}, {File: "alpha.png"}, {File: "0.jpg"}, {File: "1.jpg"}, {File: "0.jpg"}}
	a, err := openFrameArchive(t.Context(), Input{ZipPath: writeFrameArchive(t, entries), Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	first, err := a.decode(t.Context(), frames[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := bytes.Clone(first.Pix)
	if _, err := a.decode(t.Context(), frames[1]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Pix, snapshot) {
		t.Fatal("independent decode invalidated prior image")
	}
	var previous, spare *image.NRGBA
	for _, frame := range frames {
		var held []byte
		if previous != nil {
			held = bytes.Clone(previous.Pix)
		}
		current, err := a.decodeInto(t.Context(), frame, spare)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(previous.Pix, held) {
			t.Fatal("decode overwrote previous comparison frame")
		}
		var source []byte
		for _, entry := range entries {
			if entry.name == frame.File {
				source = entry.body
			}
		}
		want, err := decodeFrameSource(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(current.Pix, want.Pix) {
			t.Fatalf("reused %s changed pixels", frame.File)
		}
		if spare != nil && strings.HasSuffix(frame.File, ".jpg") && current != spare {
			t.Fatal("released same-size RGBA buffer not reused")
		}
		previous, spare = current, previous
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := bytes.Clone(previous.Pix)
	if _, err := a.decodeInto(ctx, frames[0], previous); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancel: %v", err)
	}
	if !bytes.Equal(previous.Pix, before) {
		t.Fatal("pre-cancel modified destination")
	}
	// 不同尺寸的已释放缓冲不能决定输出尺寸或覆盖范围。
	small := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if got, err := a.decodeInto(t.Context(), frames[0], small); err != nil || got.Rect != red.Rect || got == small {
		t.Fatalf("different-size reuse: %v", err)
	}
}

func TestReadFrameSourceReusesStorage(t *testing.T) {
	storage := make([]byte, 16)
	source := frameSource{data: storage}
	for _, body := range []string{"12345678", "abc", "", "ABCDEFGH"} {
		got, err := source.read(t.Context(), &frameShortReader{Reader: strings.NewReader(body)}, "reuse", uint64(len(body)), 8)
		if err != nil || string(got) != body || cap(got) > 8 {
			t.Fatalf("body %q: got %q cap=%d error=%v", body, got, cap(got), err)
		}
		if len(got) > 0 && &got[0] != &storage[0] {
			t.Fatal("available source storage not reused")
		}
	}
	growSource := frameSource{data: storage[:8:8]}
	grown, err := growSource.read(t.Context(), strings.NewReader("123456789"), "grow", 9, 16)
	if err != nil || string(grown) != "123456789" || cap(grown) != 16 {
		t.Fatalf("bounded source growth: cap=%d error=%v", cap(grown), err)
	}
	for _, declared := range []uint64{4, 9} {
		if _, err := source.read(t.Context(), strings.NewReader("123456789"), "limit", declared, 8); err == nil {
			t.Fatal("reuse bypassed source limit")
		}
	}
	before := bytes.Clone(storage)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := source.read(ctx, strings.NewReader("new"), "cancel", 3, 8); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancel: %v", err)
	}
	if !bytes.Equal(storage, before) {
		t.Fatal("pre-cancel changed reused source")
	}
}

func TestFrameNRGBA444SubImage(t *testing.T) {
	img := image.NewYCbCr(image.Rect(3, 5, 10, 10), image.YCbCrSubsampleRatio444)
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = 128, 128
	}
	for y := 5; y < 10; y++ {
		for x := 3; x < 10; x++ {
			img.Y[img.YOffset(x, y)] = byte(x*7 + y*3)
		}
	}
	crop := image.Rect(4, 6, 9, 9)
	got, err := frameNRGBA(t.Context(), img.SubImage(crop))
	if err != nil {
		t.Fatal(err)
	}
	if got.Rect != (image.Rectangle{Max: crop.Size()}) {
		t.Fatal("output was not normalized")
	}
	for y := 0; y < crop.Dy(); y++ {
		for x := 0; x < crop.Dx(); x++ {
			v := byte((x+crop.Min.X)*7 + (y+crop.Min.Y)*3)
			if c := got.NRGBAAt(x, y); c != (color.NRGBA{v, v, v, 255}) {
				t.Fatalf("(%d,%d): got %v want gray %d", x, y, c, v)
			}
		}
	}
}

func BenchmarkReadFrameSourceReuse(b *testing.B) {
	payload := bytes.Repeat([]byte("abcd"), 256)
	reader := bytes.NewReader(payload)
	source := frameSource{data: make([]byte, 0, len(payload))}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(payload)
		_, err := source.read(ctx, reader, "sample", uint64(len(payload)), uint64(len(payload)))
		if err != nil {
			b.Fatal(err)
		}
	}
}
