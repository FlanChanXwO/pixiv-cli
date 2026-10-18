package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// 保留 image-compare 0.5 的 MSSIMSimple：非重叠8×8窗口，协方差不除以N。
func ugoiraSSIM(a, b *image.NRGBA, channel int) float64 {
	value := func(img *image.NRGBA, x, y int) float64 {
		i := img.PixOffset(x, y)
		if channel < 3 {
			return float64(img.Pix[i+channel])
		}
		// DynamicImage::into_luma8走CICP：BT.709/D65矩阵、float32和四舍五入。
		var luma float32
		for c, bits := range [...]uint32{0x3e59c05b, 0x3f3715fc, 0x3d93cf75} {
			component := float32(float32(img.Pix[i+c]) * float32(1.0/255))
			luma = float32(math.FMA(float64(component), float64(math.Float32frombits(bits)), float64(luma)))
		}
		return math.Round(float64(float32(luma * 255)))
	}
	total := 0.0
	for x := a.Rect.Min.X; x < a.Rect.Max.X; x += 8 {
		for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y += 8 {
			r := image.Rect(x, y, x+8, y+8).Intersect(a.Rect)
			n := float64(r.Dx() * r.Dy())
			mx, my := 0.0, 0.0
			for yy := r.Min.Y; yy < r.Max.Y; yy++ {
				for xx := r.Min.X; xx < r.Max.X; xx++ {
					mx += value(a, xx, yy)
					my += value(b, xx, yy)
				}
			}
			mx /= n
			my /= n
			vx, vy, cov := 0.0, 0.0, 0.0
			for yy := r.Min.Y; yy < r.Max.Y; yy++ {
				for xx := r.Min.X; xx < r.Max.X; xx++ {
					dx, dy := value(a, xx, yy)-mx, value(b, xx, yy)-my
					vx += dx * dx
					vy += dy * dy
					cov += dx * dy
				}
			}
			const c1 = 6.5025
			const c2 = 58.5225
			total += n * ((2*mx*my + c1) * (2*cov + c2)) / ((mx*mx + my*my + c1) * (vx + vy + c2))
		}
	}
	return total / float64(a.Rect.Dx()*a.Rect.Dy())
}

func ugoiraNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	n := image.NewNRGBA(img.Bounds())
	draw.Draw(n, n.Rect, img, img.Bounds().Min, draw.Src)
	return n
}

func openUgoiraSources(t *testing.T, dir string) *zip.ReadCloser {
	t.Helper()
	z, err := zip.OpenReader(filepath.Join(dir, "frames.zip"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := z.Close(); err != nil {
			t.Error(err)
		}
	})
	return z
}

func decodeUgoiraSource(t *testing.T, z *zip.ReadCloser, name string) *image.NRGBA {
	t.Helper()
	r, err := z.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	img, _, decodeErr := image.Decode(r)
	_, readErr := io.Copy(io.Discard, r)
	closeErr := r.Close()
	for _, err := range []error{decodeErr, readErr, closeErr} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return ugoiraNRGBA(img)
}

func readUgoiraSources(t *testing.T, dir string, sample ugoiraSample, visit func(int, *image.NRGBA)) {
	t.Helper()
	z := openUgoiraSources(t, dir)
	for i, frame := range sample.Frames {
		img := decodeUgoiraSource(t, z, frame.File)
		if img.Rect.Dx() != sample.Width || img.Rect.Dy() != sample.Height {
			t.Fatalf("source frame %d dimensions differ", i)
		}
		visit(i, img)
	}
}

type ugoiraFrameQuality struct {
	RGB                  float64 `json:"rgb_ssim"`
	Luma                 float64 `json:"luma_ssim"`
	DifferentPixels      int     `json:"different_pixels"`
	MaxChannelError      int     `json:"max_channel_error"`
	AlphaDifferentPixels int     `json:"alpha_different_pixels"`
}

func compareUgoiraFrame(a, b *image.NRGBA) ugoiraFrameQuality {
	q := ugoiraFrameQuality{Luma: ugoiraSSIM(a, b, 3)}
	for c := 0; c < 3; c++ {
		q.RGB += ugoiraSSIM(a, b, c) / 3
	}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			ca, cb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			if ca != cb {
				q.DifferentPixels++
			}
			if ca.A != cb.A {
				q.AlphaDifferentPixels++
			}
			for _, d := range []int{int(ca.R) - int(cb.R), int(ca.G) - int(cb.G), int(ca.B) - int(cb.B), int(ca.A) - int(cb.A)} {
				q.MaxChannelError = max(q.MaxChannelError, int(math.Abs(float64(d))))
			}
		}
	}
	return q
}

// 质量重放在计时进程之外运行，PNG精确比较，JPEG解码差异单列。
func TestUgoiraParityInspect(t *testing.T) {
	dir, output := os.Getenv("UGOIRA_PARITY_SAMPLE"), os.Getenv("UGOIRA_PARITY_OUTPUT")
	if dir == "" {
		t.Skip("set UGOIRA_PARITY_SAMPLE and UGOIRA_PARITY_OUTPUT to inspect offline")
	}
	if !filepath.IsAbs(dir) || !filepath.IsAbs(output) {
		t.Fatal("inspection paths must be absolute")
	}
	sample, hash, err := loadUgoiraSample(dir)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	z := openUgoiraSources(t, dir)
	var scores []ugoiraFrameQuality
	visit := func(index int, img *image.NRGBA, delayMS float64) error {
		if index >= len(sample.Frames) || img.Rect != image.Rect(0, 0, sample.Width, sample.Height) {
			return fmt.Errorf("frame %d count or dimensions differ", index)
		}
		frame := sample.Frames[index]
		expected := float64(frame.Delay)
		if filepath.Ext(output) == ".gif" {
			expected = math.Ceil(expected/10) * 10
		}
		if math.Abs(delayMS-expected) > 1e-9 {
			return fmt.Errorf("frame %d delay %g != %g", index, delayMS, expected)
		}
		source := decodeUgoiraSource(t, z, frame.File)
		if source.Rect != img.Rect {
			return fmt.Errorf("source frame %d dimensions differ", index)
		}
		q := compareUgoiraFrame(source, img)
		scores = append(scores, q)
		if filepath.Ext(output) == ".apng" && filepath.Ext(frame.File) == ".png" && q.DifferentPixels != 0 {
			return fmt.Errorf("lossy PNG source frame %d", index)
		}
		// GIF只给alpha=0分配透明索引；半透明像素保持可见。
		if filepath.Ext(output) == ".gif" {
			for y := 0; y < sample.Height; y++ {
				for x := 0; x < sample.Width; x++ {
					if (source.NRGBAAt(x, y).A == 0) != (img.NRGBAAt(x, y).A == 0) {
						return fmt.Errorf("GIF transparency differs in frame %d", index)
					}
				}
			}
		}
		return nil
	}
	if filepath.Ext(output) == ".gif" {
		animation, err := gif.DecodeAll(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if animation.LoopCount != 0 || animation.Config.Width != sample.Width || animation.Config.Height != sample.Height {
			t.Fatal("GIF loop or canvas changed")
		}
		for i, img := range animation.Image {
			if animation.Disposal[i] != gif.DisposalBackground && animation.Disposal[i] != gif.DisposalNone {
				t.Fatalf("frame %d disposal changed", i)
			}
			if err := visit(i, ugoiraNRGBA(img), float64(animation.Delay[i]*10)); err != nil {
				t.Fatal(err)
			}
		}
	} else if filepath.Ext(output) == ".apng" {
		if err := replayUgoiraAPNG(body, visit); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("inspection supports gif/apng")
	}
	if len(scores) != len(sample.Frames) {
		t.Fatal("decoded frame count differs")
	}
	result := struct {
		SampleSHA256 string               `json:"sample_sha256"`
		OutputSHA256 string               `json:"output_sha256"`
		Reference    string               `json:"reference"`
		Frames       []ugoiraFrameQuality `json:"frames"`
	}{hash, ugoiraBytesHash(body), "Go image/jpeg or lossless PNG; JPEG decoder differences included explicitly", scores}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("UGOIRA_QUALITY_RESULT %s", encoded)
}

func TestUgoiraSSIM(t *testing.T) {
	a, b := image.NewNRGBA(image.Rect(0, 0, 9, 7)), image.NewNRGBA(image.Rect(0, 0, 9, 7))
	for i := range b.Pix {
		b.Pix[i] = 5
	}
	if got := ugoiraSSIM(a, a, 0); got != 1 {
		t.Fatal(got)
	}
	if got := ugoiraSSIM(a, b, 0); math.Abs(got-0.20641218950876916) > 1e-15 {
		t.Fatal(got)
	}
	// 末列是一个独立边缘窗口，按面积加权，不能补零或平均窗口。
	for y := 0; y < 7; y++ {
		b.SetNRGBA(8, y, color.NRGBA{})
	}
	expected := (8*0.20641218950876916 + 1) / 9
	if got := ugoiraSSIM(a, b, 0); math.Abs(got-expected) > 1e-15 {
		t.Fatal(got)
	}
}

func TestUgoiraSSIMCovarianceAndLuma(t *testing.T) {
	a, b := image.NewNRGBA(image.Rect(0, 0, 3, 3)), image.NewNRGBA(image.Rect(0, 0, 3, 3))
	a.SetNRGBA(1, 1, color.NRGBA{R: 9})
	// 旧Rust单测中的均值1、偏差平方和72，不能除以窗口面积9。
	expected := (6.5025 * 58.5225) / ((1 + 6.5025) * (72 + 58.5225))
	if got := ugoiraSSIM(a, b, 0); math.Abs(got-expected) > 1e-15 {
		t.Fatal(got)
	}
	a, b = image.NewNRGBA(image.Rect(0, 0, 1, 1)), image.NewNRGBA(image.Rect(0, 0, 1, 1))
	a.SetNRGBA(0, 0, color.NRGBA{G: 1})
	b.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 1, B: 1})
	if got := ugoiraSSIM(a, b, 3); got != 1 {
		t.Fatalf("CICP luma rounding changed: %g", got)
	}
}

func TestUgoiraSSIMRustCalibration(t *testing.T) {
	dir := os.Getenv("UGOIRA_PARITY_RUST_QUALITY")
	if dir == "" {
		t.Skip("set UGOIRA_PARITY_RUST_QUALITY to saved old quality_gate outputs")
	}
	for name, images := range ugoiraSyntheticScenes() {
		if name != "gradient-motion" && name != "color-grid" && name != "textured-pan" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name+"-ssim.csv"))
			if err != nil {
				t.Fatal(err)
			}
			rows, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, name+"-rust.gif"))
			if err != nil {
				t.Fatal(err)
			}
			animation, err := gif.DecodeAll(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(images)+1 || len(animation.Image) != len(images) {
				t.Fatal("calibration frame count differs")
			}
			for i, img := range animation.Image {
				q := compareUgoiraFrame(images[i], ugoiraNRGBA(img))
				for column, actual := range map[int]float64{3: q.Luma, 5: q.RGB} {
					if len(rows[i+1]) != 7 {
						t.Fatal("invalid calibration CSV row")
					}
					expected, err := strconv.ParseFloat(rows[i+1][column], 64)
					if err != nil {
						t.Fatal(err)
					}
					if math.IsNaN(expected) || math.IsInf(expected, 0) || math.Abs(actual-expected) > 1e-9 {
						t.Fatalf("frame %d metric %d: Go %.12f Rust %.12f", i, column, actual, expected)
					}
				}
			}
		})
	}
}
