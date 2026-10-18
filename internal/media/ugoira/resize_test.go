package ugoira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"math"
	"os"
	"testing"
)

func TestScaledDimensions(t *testing.T) {
	for _, tc := range []struct {
		width, height, maxEdge uint32
		wantWidth, wantHeight  uint32
	}{
		{0, 7, 3, 0, 0}, {7, 0, 3, 0, 0},
		{1920, 1080, 0, 1920, 1080}, {13, 9, 13, 13, 9}, {9, 13, 99, 9, 13},
		{1920, 1080, 600, 600, 337}, {1080, 1920, 600, 337, 600},
		{13, 9, 7, 7, 4}, {9, 13, 7, 4, 7}, {13, 13, 7, 7, 7},
		{1920, 1080, 1, 1, 1}, {1, 61, 7, 1, 7}, {61, 1, 7, 7, 1},
		{math.MaxUint32, 1, 1, 1, 1}, {1, math.MaxUint32, 1, 1, 1},
		{math.MaxUint32, math.MaxUint32 - 1, math.MaxUint32 - 1, math.MaxUint32 - 1, math.MaxUint32 - 2},
		{math.MaxUint32 - 1, math.MaxUint32, math.MaxUint32 - 1, math.MaxUint32 - 2, math.MaxUint32 - 1},
	} {
		w, h := scaledDimensions(tc.width, tc.height, tc.maxEdge)
		if w != tc.wantWidth || h != tc.wantHeight {
			t.Errorf("%dx%d maxEdge=%d: got %dx%d, want %dx%d", tc.width, tc.height, tc.maxEdge, w, h, tc.wantWidth, tc.wantHeight)
		}
	}
}

func TestResizeFrameReference(t *testing.T) {
	data, err := os.ReadFile("testdata/resize-lanczos3.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name         string
			Width        int
			Height       int
			OutputWidth  int `json:"output_width"`
			OutputHeight int `json:"output_height"`
			Input        []byte
			Output       []byte
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 10 {
		t.Fatalf("reference cases: %d, want 10", len(fixture.Cases))
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if len(tc.Input) != tc.Width*tc.Height*4 || len(tc.Output) != tc.OutputWidth*tc.OutputHeight*4 {
				t.Fatal("reference byte length does not match dimensions")
			}
			// 子图同时覆盖非零坐标和行尾 padding，避免把 Pix 当成紧密画布。
			backing := image.NewNRGBA(image.Rect(-2, -3, tc.Width+4, tc.Height+5))
			src := backing.SubImage(image.Rect(1, 2, 1+tc.Width, 2+tc.Height)).(*image.NRGBA)
			for y := 0; y < tc.Height; y++ {
				copy(src.Pix[y*src.Stride:][:tc.Width*4], tc.Input[y*tc.Width*4:][:tc.Width*4])
			}
			before := bytes.Clone(backing.Pix)
			got, err := resizeFrame(t.Context(), src, image.Pt(tc.OutputWidth, tc.OutputHeight))
			if err != nil {
				t.Fatal(err)
			}
			if got.Rect.Size() != image.Pt(tc.OutputWidth, tc.OutputHeight) {
				t.Fatalf("output bounds: %v", got.Rect)
			}
			for y := 0; y < tc.OutputHeight; y++ {
				if !bytes.Equal(got.Pix[y*got.Stride:][:tc.OutputWidth*4], tc.Output[y*tc.OutputWidth*4:][:tc.OutputWidth*4]) {
					t.Fatalf("row %d differs from old image Lanczos3 reference: got %v; want %v", y, got.Pix[y*got.Stride:][:tc.OutputWidth*4], tc.Output[y*tc.OutputWidth*4:][:tc.OutputWidth*4])
				}
			}
			if !bytes.Equal(backing.Pix, before) {
				t.Fatal("resize modified the source")
			}
			if tc.Width == tc.OutputWidth && tc.Height == tc.OutputHeight && got != src {
				t.Fatal("unchanged dimensions allocated a new frame")
			}
		})
	}
}

func TestLanczos3NativeReference(t *testing.T) {
	data, err := os.ReadFile("testdata/lanczos-kernel.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct{ Cases [][2]uint32 }
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Cases) != 1031 {
		t.Fatalf("reference cases: %d, want 1031", len(reference.Cases))
	}
	for _, pair := range reference.Cases {
		if got := math.Float32bits(lanczos3(math.Float32frombits(pair[0]))); got != pair[1] {
			t.Errorf("Lanczos3(%08x): got %08x, want legacy %08x", pair[0], got, pair[1])
		}
	}
}

func TestLanczosWeightsFloat32Center(t *testing.T) {
	// 旧 image 参考：中心减去 0.5 时跨越 float32 binade，不能与前一次乘法融合。
	want := []uint32{0x35cc1915, 0x3c4c5eab, 0xba81c60e, 0xbd8d58e5, 0x3bbc2f03, 0x3ea207ff, 0x3f00d06b, 0x3e95e31f, 0xbc109f53, 0xbd86871c, 0x3b2a1c9b, 0x3c3955e6, 0}
	got, err := lanczosWeights(t.Context(), 480, 241, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.start != 122 || len(got.values) != len(want) {
		t.Fatalf("start=%d weights=%d, want start=122 weights=%d", got.start, len(got.values), len(want))
	}
	for i, value := range got.values {
		if bits := math.Float32bits(value); bits != want[i] {
			t.Errorf("weight %d: got %08x, want %08x", i, bits, want[i])
		}
	}
}

func TestResizeByte(t *testing.T) {
	for _, tc := range []struct {
		value float32
		want  byte
	}{{-20, 0}, {0.49, 0}, {0.5, 1}, {127.5, 128}, {254.5, 255}, {270, 255}} {
		if got := resizeByte(tc.value); got != tc.want {
			t.Errorf("round/clamp %v: got %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestResizeFrameInvalidSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for _, size := range []image.Point{{0, 1}, {1, 0}, {-1, 1}, {4, 2}, {3, 3}} {
		if _, err := resizeFrame(t.Context(), src, size); err == nil {
			t.Errorf("invalid size %v accepted", size)
		}
	}
	wide := &image.NRGBA{Rect: image.Rect(0, 0, math.MaxInt/16+1, 1)}
	if _, err := resizeFrame(t.Context(), wide, image.Pt(1, 1)); err == nil {
		t.Fatal("float32 row size overflow accepted")
	}
}

func TestResizeFrameCancellation(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 512, 256))
	// 在权重生成、重采样及下一行开始时取消，避免依赖机器速度或 sleep。
	for _, checks := range []int{1, 8, 3060, 3070, 3200, 3330} {
		ctx := &resizeCancelContext{Context: t.Context(), remaining: checks}
		if got, err := resizeFrame(ctx, src, image.Pt(256, 128)); got != nil || !errors.Is(err, context.Canceled) {
			t.Errorf("cancel after %d checks: frame=%v, error=%v", checks, got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resizeFrame(ctx, src, src.Rect.Size()); !errors.Is(err, context.Canceled) {
		t.Fatalf("unchanged dimensions ignored cancellation: %v", err)
	}
}

type resizeCancelContext struct {
	context.Context
	remaining int
}

func (c *resizeCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return c.Context.Err()
}
