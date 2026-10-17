package jpeg_test

import (
	"bytes"
	"errors"
	"image"
	stdjpeg "image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"

	framejpeg "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/jpeg"
)

func TestDecode(t *testing.T) {
	for _, kind := range []string{"444", "progressive", "gray", "restart"} {
		t.Run(kind, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "testdata", "jpeg-"+kind+".jpg"))
			if err != nil {
				t.Fatal(err)
			}
			whole, err := framejpeg.Decode(bytes.NewReader(body))
			if err != nil || whole.Bounds() != image.Rect(0, 0, 17, 9) {
				t.Fatalf("decode: %v", err)
			}
			chunked, err := framejpeg.Decode(oneByteReader{bytes.NewReader(body)})
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 9; y++ {
				for x := 0; x < 17; x++ {
					if whole.At(x, y) != chunked.At(x, y) {
						t.Fatalf("short reads changed pixel (%d,%d)", x, y)
					}
				}
			}
			for n := 0; n < len(body); n++ {
				if _, err := framejpeg.Decode(bytes.NewReader(body[:n])); err == nil {
					t.Fatalf("truncated input at %d accepted", n)
				}
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	sentinel := errors.New("reader failed")
	if _, err := framejpeg.Decode(failingReader{sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("reader cause lost: %v", err)
	}
	for _, body := range [][]byte{[]byte("not JPEG"), {0xff, 0xd8, 0xff, 0xdb, 0, 1}, {0xff, 0xd8, 0xff, 0xd9}} {
		if _, err := framejpeg.Decode(bytes.NewReader(body)); err == nil {
			t.Fatalf("invalid JPEG accepted: %x", body)
		}
	}
}

func TestDecodeDoesNotRegisterGlobalFormat(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "testdata", "jpeg-444.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	global, format, err := image.Decode(bytes.NewReader(body))
	if err != nil || format != "jpeg" {
		t.Fatalf("global decoder: %s %v", format, err)
	}
	standard, err := stdjpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 9; y++ {
		for x := 0; x < 17; x++ {
			if global.At(x, y) != standard.At(x, y) {
				t.Fatal("frame decoder replaced the standard global decoder")
			}
		}
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:min(len(p), 1)]) }

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestDecoderReusesPlanesAndResetsState(t *testing.T) {
	var decoder framejpeg.Decoder
	var inputs [][]byte
	for _, kind := range []string{"444", "progressive", "restart", "gray"} {
		body, err := os.ReadFile(filepath.Join("..", "testdata", "jpeg-"+kind+".jpg"))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, body)
	}
	for _, size := range []image.Point{{17, 9}, {18, 10}, {33, 17}, {1, 1}} {
		src := image.NewRGBA(image.Rectangle{Max: size})
		for i := range src.Pix {
			src.Pix[i] = byte(i*37 + 19)
		}
		var body bytes.Buffer
		if err := stdjpeg.Encode(&body, src, nil); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, body.Bytes())
	}
	for i, body := range inputs {
		want, err := framejpeg.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for repeat := range 2 {
			got, err := decoder.Decode(oneByteReader{bytes.NewReader(body)})
			if err != nil || got.Bounds() != want.Bounds() {
				t.Fatalf("input %d repeat %d: %v", i, repeat, err)
			}
			for y := want.Bounds().Min.Y; y < want.Bounds().Max.Y; y++ {
				for x := want.Bounds().Min.X; x < want.Bounds().Max.X; x++ {
					if got.At(x, y) != want.At(x, y) {
						t.Fatalf("input %d pixel (%d,%d) differs", i, x, y)
					}
				}
			}
			if first, ok := got.(*image.YCbCr); ok {
				y, cb, cr := &first.Y[0], &first.Cb[0], &first.Cr[0]
				// 故意污染复用平面，下一次解码必须像新缓冲一样清零并重建。
				for _, plane := range [][]byte{first.Y, first.Cb, first.Cr} {
					for j := range plane {
						plane[j] = 0xff
					}
				}
				again, err := decoder.Decode(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				second := again.(*image.YCbCr)
				if &second.Y[0] != y || &second.Cb[0] != cb || &second.Cr[0] != cr {
					t.Fatal("same layout allocated fresh planes")
				}
				full := want.(*image.YCbCr)
				if !bytes.Equal(second.Y, full.Y) || !bytes.Equal(second.Cb, full.Cb) || !bytes.Equal(second.Cr, full.Cr) {
					t.Fatal("reused padded planes differ from a fresh decode")
				}
			}
		}
		// 出错不能把旧 Huffman/scan/尺寸状态带进下一帧。
		for n := range len(body) {
			if _, err := decoder.Decode(bytes.NewReader(body[:n])); err == nil {
				t.Fatalf("truncated input %d at %d accepted", i, n)
			}
		}
		if _, err := decoder.Decode(bytes.NewReader(body)); err != nil {
			t.Fatalf("decode after error: %v", err)
		}
	}
	sentinel := errors.New("reused reader failed")
	if _, err := decoder.Decode(failingReader{sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("reader error lost: %v", err)
	}
}

func BenchmarkReusableDecoder(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("..", "testdata", "jpeg-444.jpg"))
	if err != nil {
		b.Fatal(err)
	}
	var decoder framejpeg.Decoder
	if _, err := decoder.Decode(bytes.NewReader(body)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := decoder.Decode(bytes.NewReader(body)); err != nil {
			b.Fatal(err)
		}
	}
}
