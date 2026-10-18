package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/gif"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type ugoiraEncodeResult struct {
	SampleSHA256 string        `json:"sample_sha256"`
	ZIPSHA256    string        `json:"zip_sha256"`
	FramesSHA256 string        `json:"frames_sha256"`
	Format       ugoira.Format `json:"format"`
	MaxEdge      uint32        `json:"max_edge"`
	Seconds      float64       `json:"encode_seconds"`
	Bytes        int64         `json:"output_bytes"`
	GoVersion    string        `json:"go_version"`
	GOMAXPROCS   int           `json:"gomaxprocs"`
}

// 独立 test binary 固定所编译的编码实现；入口本身不加载账号或访问网络。
func TestUgoiraParityEncode(t *testing.T) {
	dir := os.Getenv("UGOIRA_PARITY_SAMPLE")
	if dir == "" {
		t.Skip("set UGOIRA_PARITY_SAMPLE to encode a captured sample offline")
	}
	output := os.Getenv("UGOIRA_PARITY_OUTPUT")
	format := ugoira.Format(os.Getenv("UGOIRA_PARITY_FORMAT"))
	edge, err := strconv.ParseUint(os.Getenv("UGOIRA_PARITY_MAX_EDGE"), 10, 32)
	if err != nil || !filepath.IsAbs(dir) || !filepath.IsAbs(output) {
		t.Fatal("sample/output paths must be absolute; UGOIRA_PARITY_MAX_EDGE must be a uint32 (0 disables resizing)")
	}
	result, err := encodeUgoiraSample(t.Context(), ugoira.NewEncoder(), dir, output, format, uint32(edge))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("UGOIRA_PARITY_RESULT %s", body)
}

func loadUgoiraSample(dir string) (ugoiraSample, string, error) {
	var sample ugoiraSample
	body, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		return sample, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sample); err != nil {
		return sample, "", errors.New("invalid sample manifest JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return sample, "", errors.New("sample manifest has trailing JSON")
	}
	if sample.Schema != 1 || sample.ArtworkID <= 0 || !validUgoiraSampleQuality(sample.Quality) || sample.Width <= 0 || sample.Height <= 0 || sample.ZIPBytes <= 0 {
		return sample, "", errors.New("invalid sample manifest fields")
	}
	if err := validateUgoiraSampleFrames(sample.Frames); err != nil {
		return sample, "", err
	}
	framesJSON, err := json.Marshal(sample.Frames)
	if err != nil {
		return sample, "", err
	}
	if ugoiraBytesHash(framesJSON) != sample.FramesSHA256 {
		return sample, "", errors.New("sample frame metadata hash mismatch")
	}
	var duration int64
	for _, frame := range sample.Frames {
		if int64(frame.Delay) > math.MaxInt64-duration {
			return sample, "", errors.New("sample duration overflow")
		}
		duration += int64(frame.Delay)
	}
	if duration != sample.DurationMS {
		return sample, "", errors.New("sample duration mismatch")
	}
	hash, size, err := ugoiraFileHash(filepath.Join(dir, "frames.zip"))
	if err != nil {
		return sample, "", err
	}
	if hash != sample.ZIPSHA256 || size != sample.ZIPBytes {
		return sample, "", errors.New("sample archive hash or length mismatch")
	}
	return sample, ugoiraBytesHash(body), nil
}

func encodeUgoiraSample(ctx context.Context, encoder ugoira.Encoder, dir, output string, format ugoira.Format, maxEdge uint32) (ugoiraEncodeResult, error) {
	var result ugoiraEncodeResult
	if format != ugoira.FormatGIF && format != ugoira.FormatAPNG {
		return result, errors.New("UGOIRA_PARITY_FORMAT must be gif or apng")
	}
	if filepath.Ext(output) != "."+string(format) {
		return result, errors.New("output extension must match the selected animation format")
	}
	if _, err := os.Lstat(output); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			err = fs.ErrExist
		}
		return result, fmt.Errorf("output must be new: %w", err)
	}
	sample, hash, err := loadUgoiraSample(dir)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// 只计 Encode；读取/校验样本、进程启动和输出检查不计入编码耗时。
	start := time.Now()
	err = encoder.Encode(ctx, ugoira.Input{ZipPath: filepath.Join(dir, "frames.zip"), Frames: sample.Frames,
		WorkDir: filepath.Dir(output), OutputPath: output, Format: format, MaxEdge: maxEdge})
	seconds := time.Since(start).Seconds()
	if err != nil {
		return result, err
	}
	info, err := os.Stat(output)
	if err != nil {
		return result, err
	}
	if info.Size() == 0 {
		return result, errors.New("encoder produced an empty output")
	}
	return ugoiraEncodeResult{SampleSHA256: hash, ZIPSHA256: sample.ZIPSHA256, FramesSHA256: sample.FramesSHA256,
		Format: format, MaxEdge: maxEdge, Seconds: seconds, Bytes: info.Size(), GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0)}, nil
}

func TestUgoiraParityEncodeFixture(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample")
	if _, err := captureUgoiraSample(t.Context(), ugoiraSampleFixture(t), dir, 1, pixivsdk.UgoiraQualityOriginal); err != nil {
		t.Fatal(err)
	}
	for _, format := range []ugoira.Format{ugoira.FormatGIF, ugoira.FormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "out."+string(format))
			result, err := encodeUgoiraSample(t.Context(), ugoira.NewEncoder(), dir, output, format, 4)
			if err != nil {
				t.Fatal(err)
			}
			if result.Format != format || result.MaxEdge != 4 || result.Seconds <= 0 || result.Bytes <= 0 || len(result.SampleSHA256) != 64 {
				t.Fatalf("incomplete result: %+v", result)
			}
			body, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if format == ugoira.FormatGIF {
				animation, err := gif.DecodeAll(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if len(animation.Image) != 2 || animation.Config.Width != 4 || animation.Config.Height != 3 || animation.Delay[0] != 4 || animation.Delay[1] != 8 {
					t.Fatal("driver changed GIF frame count, dimensions or delays")
				}
			} else {
				config, err := png.DecodeConfig(bytes.NewReader(body))
				if err != nil || config.Width != 4 || config.Height != 3 || !bytes.Contains(body, []byte("acTL")) {
					t.Fatalf("driver did not produce the expected APNG: %v", err)
				}
			}
			if _, err := encodeUgoiraSample(t.Context(), ugoira.NewEncoder(), dir, output, format, 4); !errors.Is(err, fs.ErrExist) {
				t.Fatalf("driver overwrote a previous output: %v", err)
			}
		})
	}
}

func TestUgoiraParityEncodeEntrypoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample")
	if _, err := captureUgoiraSample(t.Context(), ugoiraSampleFixture(t), dir, 1, pixivsdk.UgoiraQualityOriginal); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "driver.gif")
	t.Setenv("UGOIRA_PARITY_SAMPLE", dir)
	t.Setenv("UGOIRA_PARITY_OUTPUT", output)
	t.Setenv("UGOIRA_PARITY_FORMAT", "gif")
	t.Setenv("UGOIRA_PARITY_MAX_EDGE", "0")
	if !t.Run("offline", TestUgoiraParityEncode) {
		t.Fatal("driver entrypoint failed")
	}
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		t.Fatal("driver entrypoint produced no output")
	}
}

func TestUgoiraSampleLoadRejectsModifiedInput(t *testing.T) {
	for _, mutation := range []string{"schema", "frames", "duration", "archive", "unknown", "trailing"} {
		t.Run(mutation, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "sample")
			sample, err := captureUgoiraSample(t.Context(), ugoiraSampleFixture(t), dir, 1, pixivsdk.UgoiraQualityOriginal)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadUgoiraSample(dir); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "schema":
				sample.Schema++
			case "frames":
				sample.Frames[0].Delay++
			case "duration":
				sample.DurationMS++
			case "archive":
				if err := os.WriteFile(filepath.Join(dir, "frames.zip"), []byte("corrupt"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			body, err := json.Marshal(sample)
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "unknown" {
				body = append([]byte(`{"private-token":1,`), body[1:]...)
			}
			if mutation == "trailing" {
				body = append(body, []byte(` {"extra":true}`)...)
			}
			if err := os.WriteFile(filepath.Join(dir, "sample.json"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadUgoiraSample(dir); err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatal("modified input accepted or unsafe JSON field echoed")
			}
		})
	}
}

type failingUgoiraEncoder struct{ err error }

func (e failingUgoiraEncoder) Encode(context.Context, ugoira.Input) error { return e.err }

func TestUgoiraParityEncodeFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample")
	if _, err := captureUgoiraSample(t.Context(), ugoiraSampleFixture(t), dir, 1, pixivsdk.UgoiraQualityOriginal); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.gif")
	_, err := encodeUgoiraSample(t.Context(), failingUgoiraEncoder{context.Canceled}, dir, output, ugoira.FormatGIF, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost encoder cause: %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("failure created output")
	}
	if _, err := encodeUgoiraSample(t.Context(), nil, dir, output, "webp", 0); err == nil {
		t.Fatal("invalid format accepted")
	}
}
