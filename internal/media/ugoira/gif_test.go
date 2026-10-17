package ugoira

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"testing"
)

func TestGIFReplay(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	copy(first.Pix, []byte{255, 0, 0, 255, 255, 255, 255, 128, 10, 20, 30, 0})
	second := image.NewNRGBA(first.Rect)
	copy(second.Pix, []byte{0, 0, 0, 0, 255, 0, 0, 128, 255, 255, 255, 255})
	clear := image.NewNRGBA(first.Rect)
	images := []*image.NRGBA{first, second, second, clear, first}
	delays := []int{1, 9, 10, 11, 655350}
	var entries []frameZipEntry
	var frames []Frame
	for i, img := range images {
		name := fmt.Sprintf("%d.png", i)
		entries = append(entries, frameZipEntry{name, framePNG(t, img)})
		frames = append(frames, Frame{File: name, Delay: delays[i]})
	}
	var out bytes.Buffer
	if err := encodeGIF(t.Context(), &out, Input{ZipPath: writeFrameArchive(t, entries), Frames: frames}); err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image) != len(images) || decoded.LoopCount != 0 || decoded.BackgroundIndex != 0 || decoded.Config.Width != 3 || decoded.Config.Height != 1 {
		t.Fatal("frame count, loop or canvas differs")
	}
	for i, frame := range decoded.Image {
		if frame.Rect != first.Rect || decoded.Disposal[i] != gif.DisposalBackground || decoded.Delay[i] != (delays[i]+9)/10 {
			t.Fatalf("frame %d: rectangle, disposal or delay differs", i)
		}
		for x := 0; x < 3; x++ {
			want := images[i].NRGBAAt(x, 0)
			if want.A == 0 {
				want = color.NRGBA{}
			} else {
				want.A = 255
			}
			if got := color.NRGBAModel.Convert(frame.At(x, 0)).(color.NRGBA); got != want {
				t.Errorf("frame %d pixel %d: got %v want %v", i, x, got, want)
			}
		}
	}
}

func TestGIFPaletteAndLZW(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 127, 128, 255, 256} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			palette := make([]rgb, count)
			for i := range palette {
				palette[i] = rgb{byte(i), byte(255 - i), byte(i / 2)}
			}
			var out bytes.Buffer
			w := gifWriter{ctx: t.Context(), output: &out}
			size := image.Pt(512, 64)
			if err := w.header(size, palette, false); err != nil {
				t.Fatal(err)
			}
			indices := make([]byte, size.X*size.Y)
			state := uint32(1)
			for i := range indices {
				state = state*1664525 + 1013904223
				indices[i] = byte((state >> 16) % uint32(count))
			}
			for range 2 {
				if err := w.frame(size, indices, 8, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.write([]byte{0x3b}); err != nil {
				t.Fatal(err)
			}
			decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range decoded.Image {
				if !bytes.Equal(indices, frame.Pix) {
					t.Fatal("LZW dictionary reset or indices differ")
				}
				for i, c := range palette {
					if got := color.NRGBAModel.Convert(frame.Palette[i]); got != (color.NRGBA{c[0], c[1], c[2], 255}) {
						t.Fatalf("palette %d: %v", i, got)
					}
				}
			}
			// 跳过全局表与循环扩展，独立检查每帧没有局部表和交错标志。
			body := out.Bytes()[13+3*(2<<int(out.Bytes()[10]&7))+19:]
			for range 2 {
				if body[0] != 0x21 || body[8] != 0x2c || body[17] != 0 {
					t.Fatal("unexpected GIF frame header")
				}
				body = body[19:]
				for body[0] != 0 {
					body = body[1+int(body[0]):]
				}
				body = body[1:]
			}
			if !bytes.Equal(body, []byte{0x3b}) {
				t.Fatal("GIF trailer differs")
			}
		})
	}
}

func TestGIFDelayAndDimensions(t *testing.T) {
	for _, tc := range []struct{ ms, ticks int }{{1, 1}, {9, 1}, {10, 1}, {11, 2}, {655341, 65535}, {655350, 65535}} {
		got, err := gifDelay(tc.ms, "a.png")
		if err != nil || int(got) != tc.ticks {
			t.Errorf("%d ms: %d %v", tc.ms, got, err)
		}
	}
	for _, delay := range []int{-1, 0, 655351, math.MaxInt} {
		if _, err := gifDelay(delay, "a.png"); err == nil {
			t.Fatalf("invalid delay %d accepted", delay)
		}
	}
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, image.NewNRGBA(image.Rect(0, 0, 65536, 1)))}}), Frames: []Frame{{File: "a.png", Delay: 80}}}
	if err := encodeGIF(t.Context(), io.Discard, input); err == nil {
		t.Fatal("oversized GIF accepted")
	}
	for _, edge := range []uint32{65535, 4, 1} {
		input.MaxEdge = edge
		var out bytes.Buffer
		if err := encodeGIF(t.Context(), &out, input); err != nil {
			t.Fatal(err)
		}
		decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
		if err != nil || decoded.Config.Width != int(edge) || decoded.Config.Height != 1 {
			t.Fatalf("scaled width %d: %v", edge, err)
		}
		for _, index := range decoded.Image[0].Pix {
			if index != 0 {
				t.Fatal("all-transparent frame index differs")
			}
		}
		if _, _, _, alpha := decoded.Image[0].Palette[0].RGBA(); alpha != 0 {
			t.Fatal("all-transparent palette is opaque")
		}
	}
}

func TestGIFTransparentFullPalette(t *testing.T) {
	palette := make([]rgb, 255)
	for i := range palette {
		palette[i] = rgb{byte(i), byte(255 - i), 70}
	}
	indices := make([]byte, 256)
	for i := range indices {
		indices[i] = byte(i)
	}
	var out bytes.Buffer
	w := gifWriter{ctx: t.Context(), output: &out}
	if err := w.header(image.Pt(256, 1), palette, true); err != nil {
		t.Fatal(err)
	}
	if err := w.frame(image.Pt(256, 1), indices, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := w.write([]byte{0x3b}); err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	frame := decoded.Image[0]
	if len(frame.Palette) != 256 || !bytes.Equal(frame.Pix, indices) {
		t.Fatal("255 opaque colors plus transparent index were not preserved")
	}
	for i := range indices {
		want := color.NRGBA{}
		if i > 0 {
			c := palette[i-1]
			want = color.NRGBA{c[0], c[1], c[2], 255}
		}
		if got := color.NRGBAModel.Convert(frame.At(i, 0)); got != want {
			t.Fatalf("color %d: got %v want %v", i, got, want)
		}
	}
}

func TestGIFWriteFailuresAndCancellation(t *testing.T) {
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, image.NewNRGBA(image.Rect(0, 0, 8, 4)))}}), Frames: []Frame{{File: "a.png", Delay: 80}, {File: "a.png", Delay: 80}}}
	writes := 0
	if err := encodeGIF(t.Context(), apngWriteFunc(func(p []byte) (int, error) { writes++; return len(p), nil }), input); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("output failed")
	for at := 1; at <= writes; at++ {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		output := apngWriteFunc(func(p []byte) (int, error) {
			calls++
			if calls == at {
				cancel()
				return 0, failure
			}
			return len(p), nil
		})
		err := encodeGIF(ctx, output, input)
		cancel()
		if !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
			t.Fatalf("write %d lost cause: %v", at, err)
		}
	}
	if err := encodeGIF(t.Context(), apngWriteFunc(func(p []byte) (int, error) { return len(p) - 1, nil }), input); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
}

func TestGIFSubBlocksStream(t *testing.T) {
	var out bytes.Buffer
	blocks := gifBlocks{writer: &gifWriter{ctx: t.Context(), output: &out}}
	for i := range 510 {
		if err := blocks.WriteByte(byte(i)); err != nil {
			t.Fatal(err)
		}
		if out.Len() != ((i+1)/255)*256 {
			t.Fatal("full sub-block not emitted immediately")
		}
	}
	if _, err := blocks.Write([]byte{7, 8}); err != nil {
		t.Fatal(err)
	}
	if err := blocks.Flush(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes()[512:], []byte{2, 7, 8}) {
		t.Fatal("partial sub-block differs")
	}
}

func TestGIFCompressorResetsBetweenFrames(t *testing.T) {
	for _, afterFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(afterFailure), func(t *testing.T) {
			var out bytes.Buffer
			w := gifWriter{ctx: t.Context(), output: &out}
			size := image.Pt(512, 64)
			if afterFailure {
				failure := errors.New("sub-block failed")
				writes := 0
				w.output = apngWriteFunc(func(p []byte) (int, error) {
					writes++
					if writes == 4 {
						return 0, failure
					}
					return len(p), nil
				})
				if err := w.frame(size, make([]byte, size.X*size.Y), 8, false); !errors.Is(err, failure) {
					t.Fatalf("sub-block failure lost: %v", err)
				}
				w.output = &out
			}
			palette := make([]rgb, 256)
			for i := range palette {
				palette[i] = rgb{byte(i), byte(255 - i), byte(i / 2)}
			}
			if err := w.header(size, palette, false); err != nil {
				t.Fatal(err)
			}
			var frames [][]byte
			for _, count := range []int{2, 256, 4, 128, 1, 32, 256} {
				indices := make([]byte, size.X*size.Y)
				state := uint32(1)
				for i := range indices {
					state = state*1664525 + 1013904223
					indices[i] = byte((state >> 16) % uint32(count))
				}
				if err := w.frame(size, indices, 8, false); err != nil {
					t.Fatal(err)
				}
				frames = append(frames, indices)
			}
			if err := w.write([]byte{0x3b}); err != nil {
				t.Fatal(err)
			}
			decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if len(decoded.Image) != len(frames) {
				t.Fatal("frame count differs")
			}
			for i, frame := range decoded.Image {
				if !bytes.Equal(frame.Pix, frames[i]) {
					t.Fatalf("frame %d: previous LZW state leaked", i)
				}
			}
		})
	}
}
