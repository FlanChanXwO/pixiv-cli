package ugoira

import (
	"bytes"
	"image"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func TestEncoderJPEGArchive(t *testing.T) {
	frames := []Frame{{"jpeg-444.jpg", 40}, {"jpeg-progressive.jpg", 70}, {"jpeg-gray.jpg", 110}, {"jpeg-restart.jpg", 130}}
	var entries []frameZipEntry
	for i := len(frames) - 1; i >= 0; i-- {
		body, err := os.ReadFile(filepath.Join("testdata", frames[i].File))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, frameZipEntry{frames[i].File, body})
	}
	input := Input{ZipPath: writeFrameArchive(t, entries), Frames: frames, WorkDir: t.TempDir()}
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			for _, edge := range []uint32{0, 9} {
				input.Format, input.MaxEdge = format, edge
				input.OutputPath = filepath.Join(input.WorkDir, "out."+string(format))
				if err := NewEncoder().Encode(t.Context(), input); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(input.OutputPath)
				if err != nil {
					t.Fatal(err)
				}
				size := image.Pt(17, 9)
				if edge == 9 {
					size = image.Pt(9, 4)
				}
				checkJPEGAnimation(t, body, input, size)
			}
		})
	}
}

func checkJPEGAnimation(t *testing.T, body []byte, input Input, size image.Point) {
	t.Helper()
	if input.Format == FormatGIF {
		animation, err := gif.DecodeAll(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if len(animation.Image) != len(input.Frames) || animation.LoopCount != 0 || animation.Config.Width != size.X || animation.Config.Height != size.Y {
			t.Fatal("GIF frame count, loop or canvas differs")
		}
		for i, frame := range animation.Image {
			if frame.Bounds().Size() != size || animation.Delay[i]*10 != input.Frames[i].Delay {
				t.Fatalf("GIF frame %d dimensions or delay differs", i)
			}
			// 低色数 JPEG fixture 每通道允许 8 级量化误差。
			const maxChannelError = 8
			checkJPEGCanvas(t, input.Frames[i].File, frame, maxChannelError)
		}
		return
	}
	animation := decodeAPNGTest(t, body)
	if len(animation) != len(input.Frames) {
		t.Fatal("APNG frame count differs")
	}
	for i, frame := range animation {
		if frame.image.Bounds().Size() != size || int(frame.numerator)*1000 != input.Frames[i].Delay*int(frame.denominator) {
			t.Fatalf("APNG frame %d dimensions or delay differs", i)
		}
		checkJPEGCanvas(t, input.Frames[i].File, frame.image, 0)
	}
}

func checkJPEGCanvas(t *testing.T, file string, canvas image.Image, maxChannelError int) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	want, err := decodeFrameSource(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	want, err = resizeFrame(t.Context(), want, canvas.Bounds().Size())
	if err != nil {
		t.Fatal(err)
	}
	for y := range want.Rect.Dy() {
		for x := range want.Rect.Dx() {
			r, g, b, a := canvas.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()
			for _, delta := range []int{int(r) - int(wr), int(g) - int(wg), int(b) - int(wb), int(a) - int(wa)} {
				if delta < -maxChannelError*0x101 || delta > maxChannelError*0x101 {
					t.Fatalf("%s pixel (%d,%d) differs by %d/257", file, x, y, delta)
				}
			}
		}
	}
}
