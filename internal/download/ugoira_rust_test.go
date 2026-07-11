//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package download

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
)

func TestRustUgoiraEncoderNativeGIFAndAPNG(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", rustUgoiraJPEG(t))
	frames := []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}

	gifPath := filepath.Join(dir, "out.gif")
	if err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: gifPath, Format: AnimationFormatGIF,
	}); err != nil {
		t.Fatalf("encode GIF: %v", err)
	}
	gifFile, err := os.Open(gifPath)
	if err != nil {
		t.Fatal(err)
	}
	defer gifFile.Close()
	if _, err := gif.Decode(gifFile); err != nil {
		t.Fatalf("decode GIF: %v", err)
	}

	apngPath := filepath.Join(dir, "out.apng")
	if err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: apngPath, Format: AnimationFormatAPNG,
	}); err != nil {
		t.Fatalf("encode APNG: %v", err)
	}
	body, err := os.ReadFile(apngPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) || !bytes.Contains(body, []byte("acTL")) {
		t.Fatalf("APNG output does not contain a PNG signature and animation control chunk")
	}
	apngFile, err := os.Open(apngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer apngFile.Close()
	if _, err := png.Decode(apngFile); err != nil {
		t.Fatalf("decode APNG base PNG frame: %v", err)
	}
}

func TestCGODisabledBuildRejectsMissingRustStaticlib(t *testing.T) {
	command := exec.Command("go", "build", "./cmd/pixiv")
	command.Dir = filepath.Clean(filepath.Join("..", ".."))
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	body, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("CGO_ENABLED=0 go build unexpectedly succeeded")
	}
	message := strings.ToLower(string(body))
	for _, want := range []string{"go 1.26.3", "cgo", "staticlib", "c linker"} {
		if !strings.Contains(message, want) {
			t.Fatalf("CGO_ENABLED=0 build error does not contain %q:\n%s", want, body)
		}
	}
}

func rustUgoiraJPEG(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	imageBody := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			imageBody.SetRGBA(x, y, color.RGBA{R: 30, G: 160, B: 220, A: 255})
		}
	}
	body.Reset()
	if err := jpeg.Encode(&body, imageBody, nil); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
