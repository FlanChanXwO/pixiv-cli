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

func TestGIFTransparentFullPalette(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 256, 1))
	for x := 1; x < 256; x++ {
		src.SetNRGBA(x, 0, color.NRGBA{R: byte(x%8) * 32, G: byte(x/8%8) * 32, B: byte(x/64) * 64, A: 255})
	}
	input := Input{ZipPath: writeFrameArchive(t, []frameZipEntry{{"palette.png", framePNG(t, src)}}), Frames: []Frame{{"palette.png", 40}}}
	var out bytes.Buffer
	if err := encodeGIF(t.Context(), &out, input); err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image[0].Palette) != 256 {
		t.Fatal("expected 255 opaque colors plus transparent index")
	}
	for x := 0; x < 256; x++ {
		if got := color.NRGBAModel.Convert(decoded.Image[0].At(x, 0)); got != src.NRGBAAt(x, 0) {
			t.Fatalf("pixel %d: %v, want %v", x, got, src.NRGBAAt(x, 0))
		}
	}
}
