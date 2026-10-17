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
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	legacyjpeg "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/jpeg"
)

func requireUgoiraFFmpeg(t *testing.T) string {
	t.Helper()
	if os.Getenv("PIXIV_UGOIRA_QUALITY_FFMPEG") != "1" {
		t.Skip("set PIXIV_UGOIRA_QUALITY_FFMPEG=1 to run the optional FFmpeg comparison")
	}
	binary := os.Getenv("PIXIV_UGOIRA_QUALITY_FFMPEG_BIN")
	if binary == "" {
		binary = "ffmpeg"
	}
	command := exec.CommandContext(t.Context(), binary, "-version")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("FFmpeg quality reference -version: %v\n%s", err, output)
	}
	path, err := filepath.Abs(command.Path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUgoiraFFmpegSynthetic(t *testing.T) {
	binary := requireUgoiraFFmpeg(t)
	scenes := ugoiraSyntheticScenes()
	for _, name := range []string{"gradient-motion", "color-grid", "textured-pan"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "sample")
			captureSyntheticUgoira(t, dir, scenes[name], []int{80, 60, 40, 20})
			sample, _, err := loadUgoiraSample(dir)
			if err != nil {
				t.Fatal(err)
			}
			runUgoiraFFmpegQuality(t, binary, name, filepath.Join(dir, "frames.zip"), sample.Frames, true)
		})
	}
}

func TestUgoiraFFmpegPublic(t *testing.T) {
	binary := requireUgoiraFFmpeg(t)
	zipPath, framesPath := os.Getenv("PIXIV_UGOIRA_QUALITY_ZIP"), os.Getenv("PIXIV_UGOIRA_QUALITY_FRAMES")
	if zipPath == "" || framesPath == "" {
		t.Fatal("PIXIV_UGOIRA_QUALITY_ZIP and PIXIV_UGOIRA_QUALITY_FRAMES are required")
	}
	body, err := os.ReadFile(framesPath)
	if err != nil {
		t.Fatal(err)
	}
	var frames []ugoira.Frame
	if err := json.Unmarshal(body, &frames); err != nil {
		t.Fatalf("parse quality frames JSON: %v", err)
	}
	name := os.Getenv("PIXIV_UGOIRA_QUALITY_CASE")
	if name == "" {
		name = "public-sample"
	}
	runUgoiraFFmpegQuality(t, binary, name, zipPath, frames, false)
}

// 与旧 quality gate 一样重放 delta/透明帧和 disposal；不能只比较 GIF 的局部矩形。
func replayUgoiraQualityGIF(t *testing.T, path string, frames []ugoira.Frame, checkDelays bool, visit func(int, *image.NRGBA)) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	animation, err := gif.DecodeAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if animation.LoopCount != 0 || len(animation.Image) != len(frames) {
		t.Fatalf("%s: GIF loop or frame count differs", filepath.Base(path))
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	for i, frame := range animation.Image {
		if checkDelays && animation.Delay[i] != (frames[i].Delay+9)/10 {
			t.Fatalf("frame %d: Go GIF delay differs", i)
		}
		var before *image.NRGBA
		if animation.Disposal[i] == gif.DisposalPrevious {
			before = image.NewNRGBA(canvas.Rect)
			copy(before.Pix, canvas.Pix)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		visit(i, canvas)
		switch animation.Disposal[i] {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = before
		}
	}
}

// 该 reference 保留旧 image/zune 的解码舍入；原 parity inspect 的标准库 JPEG 口径不变。
func ugoiraFFmpegSource(t *testing.T, body []byte) *image.NRGBA {
	t.Helper()
	var source image.Image
	var err error
	if bytes.HasPrefix(body, []byte{0xff, 0xd8}) {
		source, err = legacyjpeg.Decode(bytes.NewReader(body))
	} else {
		source, _, err = image.Decode(bytes.NewReader(body))
	}
	if err != nil {
		t.Fatal(err)
	}
	result := image.NewNRGBA(source.Bounds())
	for y := result.Rect.Min.Y; y < result.Rect.Max.Y; y++ {
		for x := result.Rect.Min.X; x < result.Rect.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			switch value := source.At(x, y).(type) {
			case color.YCbCr:
				yy := int32(value.Y)*16384 + 8191
				cb, cr := int32(value.Cb)-128, int32(value.Cr)-128
				pixel = color.NRGBA{uint8(max(0, min(255, int((yy+22970*cr)>>14)))), uint8(max(0, min(255, int((yy-11700*cr-5638*cb)>>14)))), uint8(max(0, min(255, int((yy+29032*cb)>>14)))), 255}
			case color.NRGBA64:
				pixel = color.NRGBA{uint8((uint32(value.R) + 128) / 257), uint8((uint32(value.G) + 128) / 257), uint8((uint32(value.B) + 128) / 257), uint8((uint32(value.A) + 128) / 257)}
			case color.RGBA64:
				pixel = color.NRGBA{uint8((uint32(value.R) + 128) / 257), uint8((uint32(value.G) + 128) / 257), uint8((uint32(value.B) + 128) / 257), 255}
			case color.Gray16:
				gray := uint8((uint32(value.Y) + 128) / 257)
				pixel = color.NRGBA{gray, gray, gray, 255}
			}
			// 旧 into_rgb8 丢弃 alpha，不把半透明颜色与背景混合。
			pixel.A = 255
			result.SetNRGBA(x, y, pixel)
		}
	}
	return result
}

func runUgoiraFFmpegQuality(t *testing.T, binary, name, zipPath string, frames []ugoira.Frame, eachFrame bool) {
	t.Helper()
	if os.Getenv("PIXIV_UGOIRA_QUALITY_OUTPUT_DIR") != "" && (name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00")) {
		t.Fatal("quality case must be a flat artifact name")
	}
	dir := t.TempDir()
	goPath, referencePath := filepath.Join(dir, "go.gif"), filepath.Join(dir, "ffmpeg.gif")
	if err := ugoira.NewEncoder().Encode(t.Context(), ugoira.Input{ZipPath: zipPath, Frames: frames, OutputPath: goPath, Format: ugoira.FormatGIF}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := make(map[string]*zip.File)
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	extracted := filepath.Join(dir, "ffmpeg-frames")
	if err := os.Mkdir(extracted, 0o700); err != nil {
		t.Fatal(err)
	}
	readFrame := func(frame ugoira.Frame) []byte {
		entry := entries[frame.File]
		if entry == nil {
			t.Fatalf("quality archive missing frame %q", frame.File)
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read quality source: %v / %v", readErr, closeErr)
		}
		return body
	}
	var concat strings.Builder
	for i, frame := range frames {
		if filepath.Base(frame.File) != frame.File || strings.ContainsAny(frame.File, "/\\") {
			t.Fatal("quality frame names must be flat")
		}
		body := readFrame(frame)
		extension := ".png"
		if bytes.HasPrefix(body, []byte{0xff, 0xd8}) {
			extension = ".jpg"
		}
		// 只把生成的数字文件名写入 concat，避免元数据中的引号或换行被当作指令。
		local := fmt.Sprintf("%06d%s", i, extension)
		if err := os.WriteFile(filepath.Join(extracted, local), body, 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&concat, "file '%s'\nduration %.3f\n", local, float64(frame.Delay)/1000)
	}
	if err := os.WriteFile(filepath.Join(extracted, "frame_list.txt"), []byte(concat.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), binary, "-v", "error", "-f", "concat", "-safe", "0", "-i", "frame_list.txt", "-vf", "split[s0][s1];[s0]palettegen=stats_mode=full[p];[s1][p]paletteuse", "-y", referencePath)
	command.Dir = extracted
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("FFmpeg quality reference: %v\n%s", err, output)
	}
	scores := make([][2]ugoiraFrameQuality, len(frames))
	for backend, path := range []string{goPath, referencePath} {
		replayUgoiraQualityGIF(t, path, frames, backend == 0, func(i int, actual *image.NRGBA) {
			source := ugoiraFFmpegSource(t, readFrame(frames[i]))
			if source.Rect != actual.Rect {
				t.Fatalf("frame %d: quality dimensions differ", i)
			}
			scores[i][backend] = compareUgoiraFrame(source, actual)
		})
	}
	rows := [][]string{{"frame", "file", "delay_ms", "go_luma_ssim", "ffmpeg_luma_ssim", "go_rgb_ssim", "ffmpeg_rgb_ssim"}}
	var goLuma, refLuma, goRGB, refRGB float64
	for i, score := range scores {
		goLuma += score[0].Luma / float64(len(scores))
		refLuma += score[1].Luma / float64(len(scores))
		goRGB += score[0].RGB / float64(len(scores))
		refRGB += score[1].RGB / float64(len(scores))
		rows = append(rows, []string{strconv.Itoa(i), frames[i].File, strconv.Itoa(frames[i].Delay), fmt.Sprintf("%.9f", score[0].Luma), fmt.Sprintf("%.9f", score[1].Luma), fmt.Sprintf("%.9f", score[0].RGB), fmt.Sprintf("%.9f", score[1].RGB)})
		if eachFrame && (score[0].Luma+1e-12 < score[1].Luma || score[0].RGB+1e-12 < score[1].RGB) {
			t.Errorf("%s frame %d: Go RGB/luma %.12f/%.12f below FFmpeg %.12f/%.12f", name, i, score[0].RGB, score[0].Luma, score[1].RGB, score[1].Luma)
		}
	}
	t.Logf("QUALITY_RESULT case=%s frames=%d go_luma_ssim=%.9f ffmpeg_luma_ssim=%.9f go_rgb_ssim=%.9f ffmpeg_rgb_ssim=%.9f", name, len(frames), goLuma, refLuma, goRGB, refRGB)
	if output := os.Getenv("PIXIV_UGOIRA_QUALITY_OUTPUT_DIR"); output != "" {
		if err := os.MkdirAll(output, 0o700); err != nil {
			t.Fatal(err)
		}
		for backend, path := range map[string]string{"go": goPath, "go-ffmpeg": referencePath} {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, name+"-"+backend+".gif"), body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var csvBody bytes.Buffer
		writer := csv.NewWriter(&csvBody)
		if err := writer.WriteAll(rows); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, name+"-go-ssim.csv"), csvBody.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if goLuma+1e-12 < refLuma || goRGB+1e-12 < refRGB {
		t.Errorf("%s: average Go RGB/luma %.12f/%.12f below FFmpeg %.12f/%.12f", name, goRGB, goLuma, refRGB, refLuma)
	}
}

func TestUgoiraFFmpegOptInContract(t *testing.T) {
	for _, test := range []struct{ name, flag, binary, want string }{
		{"disabled", "", filepath.Join(t.TempDir(), "missing"), "SKIP"},
		{"missing", "1", filepath.Join(t.TempDir(), "missing"), "FFmpeg quality reference -version"},
		{"failed", "1", os.Args[0], "FFmpeg quality reference -version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestUgoiraFFmpegSynthetic$", "-test.v")
			command.Env = append(os.Environ(), "PIXIV_UGOIRA_QUALITY_FFMPEG="+test.flag, "PIXIV_UGOIRA_QUALITY_FFMPEG_BIN="+test.binary)
			body, err := command.CombinedOutput()
			if !strings.Contains(string(body), test.want) || (err == nil) != (test.flag == "") {
				t.Fatalf("opt-in contract: %v\n%s", err, body)
			}
		})
	}
}

func TestUgoiraFFmpegSourceReference(t *testing.T) {
	for _, kind := range []string{"444", "progressive", "gray", "restart"} {
		t.Run(kind, func(t *testing.T) {
			base := filepath.Join("..", "internal", "media", "ugoira", "testdata", "jpeg-"+kind)
			body, err := os.ReadFile(base + ".jpg")
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(base + ".rgba")
			if err != nil {
				t.Fatal(err)
			}
			if got := ugoiraFFmpegSource(t, body); !bytes.Equal(got.Pix, want) {
				t.Fatal("quality source differs from the frozen legacy JPEG reference")
			}
		})
	}
	for _, test := range []struct {
		name  string
		pixel color.Color
		want  color.NRGBA
	}{
		{"rgba16", color.RGBA64{129, 0x8080, 65535, 65535}, color.NRGBA{1, 128, 255, 255}},
		{"hidden-rgb16", color.NRGBA64{129, 258, 0xfeed, 0}, color.NRGBA{1, 1, 254, 255}},
		{"gray16", color.Gray16{Y: 129}, color.NRGBA{1, 1, 1, 255}},
	} {
		t.Run(test.name, func(t *testing.T) {
			img := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
			img.Set(0, 0, test.pixel)
			var body bytes.Buffer
			if err := png.Encode(&body, img); err != nil {
				t.Fatal(err)
			}
			if got := ugoiraFFmpegSource(t, body.Bytes()).NRGBAAt(0, 0); got != test.want {
				t.Fatalf("quality source = %v, want %v", got, test.want)
			}
		})
	}
}

func TestUgoiraQualityGIFReplay(t *testing.T) {
	palette := color.Palette{color.NRGBA{}, color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}}
	animation := &gif.GIF{Config: image.Config{ColorModel: palette, Width: 2, Height: 1}, LoopCount: 0, Delay: []int{1, 2, 3, 4}, Disposal: []byte{gif.DisposalNone, gif.DisposalPrevious, gif.DisposalBackground, gif.DisposalNone}}
	for _, frame := range []struct {
		rect   image.Rectangle
		pixels []byte
	}{
		{image.Rect(0, 0, 2, 1), []byte{1, 1}},
		{image.Rect(0, 0, 1, 1), []byte{2}},
		{image.Rect(1, 0, 2, 1), []byte{2}},
		{image.Rect(0, 0, 1, 1), []byte{0}},
	} {
		img := image.NewPaletted(frame.rect, palette)
		copy(img.Pix, frame.pixels)
		animation.Image = append(animation.Image, img)
	}
	path := filepath.Join(t.TempDir(), "delta.gif")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(file, animation); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	want := [][2]int{{1, 1}, {2, 1}, {1, 2}, {1, 0}}
	frames := []ugoira.Frame{{Delay: 10}, {Delay: 20}, {Delay: 30}, {Delay: 40}}
	replayUgoiraQualityGIF(t, path, frames, true, func(i int, img *image.NRGBA) {
		for x, index := range want[i] {
			if got := img.NRGBAAt(x, 0); got != palette[index].(color.NRGBA) {
				t.Fatalf("frame %d pixel %d: %v, want %v", i, x, got, palette[index])
			}
		}
	})
}
