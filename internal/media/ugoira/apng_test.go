package ugoira

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"math/rand/v2"
	"testing"
)

func TestAPNGReplay(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	copy(first.Pix, []byte{10, 20, 30, 40, 60, 70, 80, 90, 110, 120, 130, 140, 1, 2, 3, 0, 4, 5, 6, 128, 7, 8, 9, 255})
	second := cloneAPNGFrame(first)
	second.SetNRGBA(1, 0, color.NRGBA{200, 10, 20, 0})
	second.SetNRGBA(2, 1, color.NRGBA{5, 220, 30, 255})
	hidden := cloneAPNGFrame(second)
	hidden.SetNRGBA(1, 0, color.NRGBA{90, 20, 30, 0})
	clear := image.NewNRGBA(first.Rect)
	images := []*image.NRGBA{first, second, second, hidden, clear, first}
	delays := []int{1, 9, 10, 65535000, 655350, 80}
	var entries []frameZipEntry
	var frames []Frame
	for i, img := range images {
		name := fmt.Sprintf("%d.png", i)
		entries = append(entries, frameZipEntry{name, framePNG(t, img)})
		frames = append(frames, Frame{File: name, Delay: delays[i]})
	}
	input := Input{ZipPath: writeFrameArchive(t, entries), Frames: frames}
	var out bytes.Buffer
	if err := encodeAPNG(t.Context(), &out, input); err != nil {
		t.Fatal(err)
	}
	decoded := decodeAPNGTest(t, out.Bytes())
	wantRegions := []image.Rectangle{first.Rect, image.Rect(1, 0, 3, 2), image.Rect(0, 0, 1, 1), image.Rect(1, 0, 2, 1), first.Rect, first.Rect}
	if len(decoded) != len(images) {
		t.Fatalf("frames=%d, want %d", len(decoded), len(images))
	}
	for i, frame := range decoded {
		if frame.region != wantRegions[i] || !bytes.Equal(frame.image.Pix, images[i].Pix) {
			t.Errorf("frame %d: region=%v (want %v), pixels=%v (want %v)", i, frame.region, wantRegions[i], frame.image.Pix, images[i].Pix)
		}
		if uint64(frame.numerator)*1000 != uint64(delays[i])*uint64(frame.denominator) {
			t.Errorf("frame %d: delay %d/%d differs from %d ms", i, frame.numerator, frame.denominator, delays[i])
		}
	}
}

func TestAPNGResizeAndSourceValidation(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	draw.Draw(img, img.Rect, image.NewUniform(color.NRGBA{10, 30, 50, 70}), image.Point{}, draw.Src)
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, img)}, {"different.png", framePNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))}}), Frames: []Frame{{File: "a.png", Delay: 80}}}
	for _, maxEdge := range []uint32{0, 1, 4, 540} {
		input.MaxEdge = maxEdge
		var out bytes.Buffer
		if err := encodeAPNG(t.Context(), &out, input); err != nil {
			t.Fatal(err)
		}
		decoded := decodeAPNGTest(t, out.Bytes())
		w, h := scaledDimensions(8, 4, maxEdge)
		if len(decoded) != 1 || decoded[0].image.Rect.Size() != image.Pt(int(w), int(h)) {
			t.Fatalf("maxEdge=%d: dimensions or frame count changed", maxEdge)
		}
	}
	input.Frames = append(input.Frames, Frame{File: "different.png", Delay: 80})
	if err := encodeAPNG(t.Context(), io.Discard, input); err == nil {
		t.Fatal("different source dimensions accepted")
	}
	input.Frames = nil
	if err := encodeAPNG(t.Context(), io.Discard, input); err == nil {
		t.Fatal("empty frames accepted")
	}
	input.Frames = []Frame{{File: "a.png", Delay: 0}}
	if err := encodeAPNG(t.Context(), io.Discard, input); err == nil {
		t.Fatal("invalid delay accepted")
	}
}

func TestAPNGDelay(t *testing.T) {
	for _, tc := range []struct{ delay, n, d int }{{1, 1, 1000}, {9, 9, 1000}, {10, 1, 100}, {80, 2, 25}, {65535, 13107, 200}, {65536, 8192, 125}, {65535000, 65535, 1}} {
		n, d, err := apngDelay(tc.delay, "a.png")
		if err != nil || int(n) != tc.n || int(d) != tc.d {
			t.Errorf("%d ms: got %d/%d, %v; want %d/%d", tc.delay, n, d, err, tc.n, tc.d)
		}
	}
	for _, delay := range []int{-1, 0, 65537, 65536000, math.MaxInt} {
		if _, _, err := apngDelay(delay, "a.png"); err == nil {
			t.Errorf("invalid delay %d accepted", delay)
		}
	}
}

func TestAPNGSequenceAndStreaming(t *testing.T) {
	var out bytes.Buffer
	w := apngWriter{ctx: t.Context(), output: &out, sequence: math.MaxUint32 - 1}
	d := apngFrameData{writer: &w}
	for _, payload := range [][]byte{{1, 2}, {3, 4, 5}} {
		n, err := d.Write(payload)
		if err != nil || n != len(payload) || out.Len() == 0 {
			t.Fatalf("stream write: %d, %v", n, err)
		}
	}
	if n, err := d.Write([]byte{6}); n != 0 || err == nil {
		t.Fatalf("sequence overflow: %d, %v", n, err)
	}
	if _, err := w.nextSequence(); err == nil {
		t.Fatal("frame control sequence overflow accepted")
	}
	body := out.Bytes()
	for i := 0; i < 2; i++ {
		n := int(binary.BigEndian.Uint32(body[:4]))
		if string(body[4:8]) != "fdAT" || binary.BigEndian.Uint32(body[8:12]) != math.MaxUint32-1+uint32(i) || crc32.ChecksumIEEE(body[4:8+n]) != binary.BigEndian.Uint32(body[8+n:12+n]) {
			t.Fatal("fdAT length, sequence or CRC differs")
		}
		body = body[n+12:]
	}
}

func TestAPNGWriteFailuresAndCancellation(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, img)}}), Frames: []Frame{{File: "a.png", Delay: 80}, {File: "a.png", Delay: 80}}}
	failure := errors.New("output failed")
	for _, budget := range []int{0, 8, 29, 41, 55, 87, 101, 140, 175} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			remaining := budget
			output := apngWriteFunc(func(p []byte) (int, error) {
				if len(p) > remaining {
					return 0, failure
				}
				remaining -= len(p)
				return len(p), nil
			})
			if err := encodeAPNG(t.Context(), output, input); !errors.Is(err, failure) {
				t.Fatalf("lost output error: %v", err)
			}
		})
	}
	short := apngWriteFunc(func(p []byte) (int, error) { return len(p) - 1, nil })
	if err := encodeAPNG(t.Context(), short, input); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
	for _, kind := range []string{"IDAT", "fdAT", "IEND"} {
		ctx, cancel := context.WithCancel(t.Context())
		output := apngWriteFunc(func(p []byte) (int, error) {
			if len(p) == 8 && string(p[4:]) == kind {
				cancel()
				return 0, failure
			}
			return len(p), nil
		})
		err := encodeAPNG(ctx, output, input)
		cancel()
		if !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
			t.Errorf("%s canceled write lost cause: %v", kind, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := encodeAPNG(ctx, io.Discard, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled: %v", err)
	}
	if _, err := changedRegion(ctx, img, img); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delta scan: %v", err)
	}
}

type apngWriteFunc func([]byte) (int, error)

func (f apngWriteFunc) Write(p []byte) (int, error) { return f(p) }

type apngTestFrame struct {
	image                  *image.NRGBA
	region                 image.Rectangle
	numerator, denominator uint16
}

func cloneAPNGFrame(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Rect)
	draw.Draw(dst, dst.Rect, src, src.Rect.Min, draw.Src)
	return dst
}

// 独立校验 chunk/CRC/序列号，用标准库重建PNG并按 Source/None 重放。
func decodeAPNGTest(t *testing.T, body []byte) []apngTestFrame {
	t.Helper()
	if len(body) < 8 || string(body[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("invalid PNG signature")
	}
	body = body[8:]
	var header, control []byte
	var compressed bytes.Buffer
	var canvas *image.NRGBA
	var frames []apngTestFrame
	var count, sequence uint32
	emit := func() {
		t.Helper()
		if len(control) != 26 || compressed.Len() == 0 {
			t.Fatal("missing frame control/data")
		}
		w, h := binary.BigEndian.Uint32(control[4:8]), binary.BigEndian.Uint32(control[8:12])
		x, y := binary.BigEndian.Uint32(control[12:16]), binary.BigEndian.Uint32(control[16:20])
		region := image.Rect(int(x), int(y), int(x+w), int(y+h))
		if region.Empty() || !region.In(canvas.Rect) || len(frames) == 0 && region != canvas.Rect || control[24] != 0 || control[25] != 0 {
			t.Fatal("frame bounds or disposal/blend differ")
		}
		ihdr := bytes.Clone(header)
		binary.BigEndian.PutUint32(ihdr[:4], w)
		binary.BigEndian.PutUint32(ihdr[4:8], h)
		var single bytes.Buffer
		single.WriteString("\x89PNG\r\n\x1a\n")
		for _, chunk := range []struct {
			kind string
			data []byte
		}{{"IHDR", ihdr}, {"IDAT", compressed.Bytes()}, {"IEND", nil}} {
			_ = binary.Write(&single, binary.BigEndian, uint32(len(chunk.data)))
			payload := append([]byte(chunk.kind), chunk.data...)
			single.Write(payload)
			_ = binary.Write(&single, binary.BigEndian, crc32.ChecksumIEEE(payload))
		}
		img, err := png.Decode(&single)
		if err != nil {
			t.Fatal(err)
		}
		draw.Draw(canvas, region, img, image.Point{}, draw.Src)
		frames = append(frames, apngTestFrame{cloneAPNGFrame(canvas), region, binary.BigEndian.Uint16(control[20:22]), binary.BigEndian.Uint16(control[22:24])})
		compressed.Reset()
	}
	for len(body) >= 12 {
		length := uint64(binary.BigEndian.Uint32(body[:4]))
		if length > uint64(len(body)-12) {
			t.Fatal("truncated PNG chunk")
		}
		kind, data := string(body[4:8]), body[8:8+length]
		if crc32.ChecksumIEEE(body[4:8+length]) != binary.BigEndian.Uint32(body[8+length:12+length]) {
			t.Fatal("chunk CRC mismatch")
		}
		body = body[12+length:]
		switch kind {
		case "IHDR":
			if header != nil || len(data) != 13 || !bytes.Equal(data[8:], []byte{8, 6, 0, 0, 0}) {
				t.Fatal("expected RGBA8 noninterlaced IHDR")
			}
			header = bytes.Clone(data)
			canvas = image.NewNRGBA(image.Rect(0, 0, int(binary.BigEndian.Uint32(data[:4])), int(binary.BigEndian.Uint32(data[4:8]))))
		case "acTL":
			if header == nil || count != 0 || len(data) != 8 || binary.BigEndian.Uint32(data[4:]) != 0 {
				t.Fatal("animation control/loop differs")
			}
			count = binary.BigEndian.Uint32(data[:4])
		case "fcTL":
			if count == 0 || len(data) != 26 || binary.BigEndian.Uint32(data[:4]) != sequence {
				t.Fatal("frame control sequence differs")
			}
			sequence++
			if control != nil {
				emit()
			}
			control = bytes.Clone(data)
		case "IDAT":
			if control == nil || len(frames) != 0 {
				t.Fatal("unexpected IDAT")
			}
			compressed.Write(data)
		case "fdAT":
			if control == nil || len(frames) == 0 || len(data) < 4 || binary.BigEndian.Uint32(data[:4]) != sequence {
				t.Fatal("frame data sequence differs")
			}
			sequence++
			compressed.Write(data[4:])
		case "IEND":
			if len(data) != 0 || len(body) != 0 {
				t.Fatal("invalid PNG end")
			}
			emit()
			if uint32(len(frames)) != count {
				t.Fatal("frame count differs")
			}
			return frames
		default:
			t.Fatalf("unexpected chunk %q", kind)
		}
	}
	t.Fatal("missing IEND")
	return nil
}

func TestAPNGOpaqueTextureCompression(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 128))
	state := uint32(1)
	for i := 0; i < len(img.Pix); i += 4 {
		for c := range 3 {
			state = 1664525*state + 1013904223
			img.Pix[i+c] = byte(state >> 24)
		}
		img.Pix[i+3] = 255
	}
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, img)}}), Frames: []Frame{{File: "a.png", Delay: 80}}}
	var out bytes.Buffer
	if err := encodeAPNG(t.Context(), &out, input); err != nil {
		t.Fatal(err)
	}
	// 同一像素 fixture 的封存 Rust 输出为113171字节；保留原10%体积门槛。
	const maximumBytes = 113171 * 110 / 100
	if out.Len() > maximumBytes {
		t.Fatalf("opaque texture APNG is %d bytes, exceeds legacy +10%% (%d)", out.Len(), maximumBytes)
	}
	decoded := decodeAPNGTest(t, out.Bytes())
	if len(decoded) != 1 || !bytes.Equal(decoded[0].image.Pix, img.Pix) {
		t.Fatal("opaque texture pixels changed")
	}
}

func TestAPNGCoalescesCompressedWrites(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 128))
	rng := rand.New(rand.NewPCG(3, 7))
	for i := range img.Pix {
		img.Pix[i] = byte(rng.Uint32()&15) * 17
	}
	var out bytes.Buffer
	w := apngWriter{ctx: t.Context(), output: &out}
	if err := w.header(img.Rect.Size(), 2); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := w.frame(img, img.Rect, 1, 100, i == 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.chunk("IEND"); err != nil {
		t.Fatal(err)
	}
	decoded := decodeAPNGTest(t, out.Bytes())
	if len(decoded) != 2 {
		t.Fatal("buffering lost frame")
	}
	for i, frame := range decoded {
		if !bytes.Equal(frame.image.Pix, img.Pix) {
			t.Fatalf("frame %d pixel mismatch", i)
		}
	}
	chunks, payload := 0, 0
	for body := out.Bytes()[8:]; len(body) > 0; {
		n := int(binary.BigEndian.Uint32(body[:4]))
		switch string(body[4:8]) {
		case "IDAT":
			chunks++
			payload += n
		case "fdAT":
			chunks++
			payload += n - 4
		}
		body = body[n+12:]
	}
	// 不能把 flate 的约240字节刷新直接变成 PNG chunk；保留至少4KiB的合并效果。
	if chunks > payload/4096+4 {
		t.Fatalf("fragmented compressed data: %d chunks for %d bytes", chunks, payload)
	}
}
