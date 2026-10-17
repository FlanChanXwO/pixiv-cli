package e2e

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// 前三场景逐像素保留旧 quality_gate.rs 的输入与 wrapping 运算。
func ugoiraSyntheticScenes() map[string][]*image.NRGBA {
	scenes := make(map[string][]*image.NRGBA)
	for _, name := range []string{"gradient-motion", "color-grid", "textured-pan", "alpha-repeat"} {
		for f := 0; f < 4; f++ {
			img := image.NewNRGBA(image.Rect(0, 0, 96, 64))
			for y := 0; y < 64; y++ {
				for x := 0; x < 96; x++ {
					var c color.NRGBA
					switch name {
					case "gradient-motion":
						c = color.NRGBA{uint8(x * 255 / 95), uint8(y * 255 / 63), uint8((x + y) * 255 / 158), 255}
						if x >= 8+f*16 && x < 28+f*16 && y >= 20 && y < 44 {
							c = color.NRGBA{245, 48, 180, 255}
						}
					case "color-grid":
						cell := uint8(x/8 + y/8*12 + f*3)
						c = color.NRGBA{cell*37 + uint8(x*3), cell*73 + uint8(y*5), cell*109 + uint8((x+y)*2), 255}
					case "textured-pan":
						shifted := uint32(x + f*7)
						seed := shifted*1103515245 + uint32(y)*12345
						noise := uint8(seed ^ (seed >> 11) ^ (seed >> 19))
						c = color.NRGBA{noise, noise + uint8(y*3), noise + uint8(shifted*2), 255}
					case "alpha-repeat":
						// 帧1重复帧0；帧2清除旧区域；帧3全透明，覆盖隐藏RGB和半透明。
						if f < 2 && x < 32 {
							c = color.NRGBA{240, 30, 80, 255}
						} else if f < 3 && x >= 48 && x < 64 {
							c = color.NRGBA{20, 180, 100, 128}
						} else {
							c = color.NRGBA{90, 70, 150, 0}
						}
					}
					img.SetNRGBA(x, y, c)
				}
			}
			scenes[name] = append(scenes[name], img)
		}
	}
	for _, name := range []string{"one-pixel", "odd-edge"} {
		w, h := 1, 1
		if name == "odd-edge" {
			w, h = 9, 7
		}
		for f := 0; f < 4; f++ {
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					img.SetNRGBA(x, y, color.NRGBA{uint8(f * 70), uint8(x * 20), uint8(y * 30), 255})
				}
			}
			scenes[name] = append(scenes[name], img)
		}
	}
	return scenes
}

func captureSyntheticUgoira(t *testing.T, dir string, images []*image.NRGBA, delays []int) {
	t.Helper()
	c := ugoiraSampleFixture(t)
	c.metadata.Frames = nil
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	for i, img := range images {
		name := fmt.Sprintf("%06d.png", i)
		c.metadata.Frames = append(c.metadata.Frames, pixivsdk.UgoiraFrame{Filename: name, DelayMilliseconds: delays[i%len(delays)]})
		w, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(w, img); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	c.archive = body.Bytes()
	if _, err := captureUgoiraSample(t.Context(), c, dir, 1, pixivsdk.UgoiraQualityOriginal); err != nil {
		t.Fatal(err)
	}
}

func TestUgoiraSyntheticCorpus(t *testing.T) {
	dir := os.Getenv("UGOIRA_PARITY_CORPUS")
	if dir == "" {
		dir = t.TempDir()
	} else if !filepath.IsAbs(dir) {
		t.Fatal("UGOIRA_PARITY_CORPUS must be absolute")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, images := range ugoiraSyntheticScenes() {
		t.Run(name, func(t *testing.T) {
			delays := []int{80, 60, 40, 20}
			if name == "one-pixel" {
				delays = []int{1, 9, 10, 655350}
			}
			target := filepath.Join(dir, name)
			captureSyntheticUgoira(t, target, images, delays)
			sample, _, err := loadUgoiraSample(target)
			if err != nil {
				t.Fatal(err)
			}
			if len(sample.Frames) != len(images) || sample.Width != images[0].Rect.Dx() || sample.Height != images[0].Rect.Dy() {
				t.Fatal("corpus shape changed")
			}
			readUgoiraSources(t, target, sample, func(i int, actual *image.NRGBA) {
				if !bytes.Equal(actual.Pix, images[i].Pix) {
					t.Fatalf("frame %d differs after ZIP/PNG roundtrip", i)
				}
			})
		})
	}
}
