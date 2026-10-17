package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// 只保存可离线复验的内容身份，不序列化 SDK Resource 中的 URL 或 headers。
type ugoiraSample struct {
	Schema       int                    `json:"schema"`
	ArtworkID    int64                  `json:"artwork_id"`
	Quality      pixivsdk.UgoiraQuality `json:"quality"`
	Frames       []ugoira.Frame         `json:"frames"`
	FramesSHA256 string                 `json:"frames_sha256"`
	ZIPBytes     int64                  `json:"zip_bytes"`
	ZIPSHA256    string                 `json:"zip_sha256"`
	Width        int                    `json:"width"`
	Height       int                    `json:"height"`
	DurationMS   int64                  `json:"duration_ms"`
}

type ugoiraSampleClient interface {
	UgoiraMetadata(context.Context, pixivsdk.UgoiraMetadataRequest) (pixivsdk.UgoiraMetadata, error)
	SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func TestRealPixivUgoiraCapture(t *testing.T) {
	if os.Getenv("PIXIV_UGOIRA_CAPTURE") != "1" {
		t.Skip("set PIXIV_UGOIRA_CAPTURE=1 to collect private ugoira samples")
	}
	ids, err := parseUgoiraSampleIDs(os.Getenv("PIXIV_UGOIRA_IDS"))
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("PIXIV_UGOIRA_DIR")
	quality := pixivsdk.UgoiraQuality(os.Getenv("PIXIV_UGOIRA_QUALITY"))
	if !filepath.IsAbs(dir) || !validUgoiraSampleQuality(quality) {
		t.Fatal("PIXIV_UGOIRA_DIR must be absolute and PIXIV_UGOIRA_QUALITY must be original or medium")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	client, _ := openPixivE2EClient(t)
	for _, id := range ids {
		sampleDir := filepath.Join(dir, strconv.FormatInt(id, 10))
		sample, err := captureUgoiraSample(t.Context(), client, sampleDir, id, quality)
		if err != nil {
			t.Fatalf("capture artwork %d: %v", id, err)
		}
		manifestHash, _, err := ugoiraFileHash(filepath.Join(sampleDir, "sample.json"))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("artwork_id=%d quality=%s frames=%d size=%dx%d duration_ms=%d zip_bytes=%d zip_sha256=%s sample_sha256=%s",
			id, sample.Quality, len(sample.Frames), sample.Width, sample.Height, sample.DurationMS, sample.ZIPBytes, sample.ZIPSHA256, manifestHash)
	}
}

func parseUgoiraSampleIDs(raw string) ([]int64, error) {
	var ids []int64
	seen := make(map[int64]bool)
	for _, value := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value || seen[id] {
			return nil, errors.New("PIXIV_UGOIRA_IDS must contain unique positive decimal IDs separated by commas")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func validUgoiraSampleQuality(quality pixivsdk.UgoiraQuality) bool {
	return quality == pixivsdk.UgoiraQualityOriginal || quality == pixivsdk.UgoiraQualityMedium
}

func captureUgoiraSample(ctx context.Context, client ugoiraSampleClient, dir string, id int64, quality pixivsdk.UgoiraQuality) (sample ugoiraSample, err error) {
	if id <= 0 || !validUgoiraSampleQuality(quality) {
		return sample, errors.New("invalid sample ID or quality")
	}
	// 既有样本禁止覆盖；失败只清理本次新建的目录。
	if err = os.Mkdir(dir, 0o700); err != nil {
		return sample, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	meta, err := client.UgoiraMetadata(ctx, pixivsdk.UgoiraMetadataRequest{ArtworkID: id})
	if err != nil {
		return sample, fmt.Errorf("ugoira metadata: %w", err)
	}
	if meta.ArtworkID != id {
		return sample, errors.New("ugoira metadata identity mismatch")
	}
	var ref sdk.ResourceRef
	for _, archive := range meta.Archives {
		if archive.Quality == quality {
			ref = archive.Resource.Ref
			break
		}
	}
	if ref.IsZero() {
		return sample, errors.New("requested archive quality is unavailable")
	}
	sample = ugoiraSample{Schema: 1, ArtworkID: id, Quality: quality}
	for _, frame := range meta.Frames {
		sample.Frames = append(sample.Frames, ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds})
	}
	if err = validateUgoiraSampleFrames(sample.Frames); err != nil {
		return sample, err
	}
	zipPath := filepath.Join(dir, "frames.zip")
	saved, err := client.SaveResource(ctx, ref, sdk.SaveOptions{Path: zipPath})
	if err != nil {
		return sample, fmt.Errorf("save ugoira archive: %w", err)
	}
	sample.ZIPSHA256, sample.ZIPBytes, err = ugoiraFileHash(zipPath)
	if err != nil {
		return sample, err
	}
	if saved.Size != sample.ZIPBytes || sample.ZIPBytes == 0 {
		return sample, errors.New("saved archive size mismatch")
	}
	sample.Width, sample.Height, err = inspectUgoiraSampleZIP(ctx, zipPath, sample.Frames)
	if err != nil {
		return sample, err
	}
	for _, frame := range sample.Frames {
		if int64(frame.Delay) > math.MaxInt64-sample.DurationMS {
			return sample, errors.New("sample duration overflow")
		}
		sample.DurationMS += int64(frame.Delay)
	}
	framesJSON, err := json.Marshal(sample.Frames)
	if err != nil {
		return sample, err
	}
	sample.FramesSHA256 = ugoiraBytesHash(framesJSON)
	body, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return sample, err
	}
	if err = ctx.Err(); err != nil {
		return sample, err
	}
	err = os.WriteFile(filepath.Join(dir, "sample.json"), append(body, '\n'), 0o600)
	return sample, err
}

func validateUgoiraSampleFrames(frames []ugoira.Frame) error {
	if len(frames) == 0 {
		return errors.New("sample has no frames")
	}
	seen := make(map[string]bool)
	for i, frame := range frames {
		if !fs.ValidPath(frame.File) || frame.File == "." || strings.ContainsAny(frame.File, "\\:") || seen[frame.File] || frame.Delay <= 0 {
			return fmt.Errorf("invalid sample frame at index %d", i)
		}
		seen[frame.File] = true
	}
	return nil
}

func inspectUgoiraSampleZIP(ctx context.Context, name string, frames []ugoira.Frame) (width, height int, err error) {
	archive, err := zip.OpenReader(name)
	if err != nil {
		return 0, 0, err
	}
	defer archive.Close()
	entries := make(map[string]*zip.File)
	for _, entry := range archive.File {
		if entries[entry.Name] != nil {
			return 0, 0, errors.New("archive contains duplicate entries")
		}
		entries[entry.Name] = entry
	}
	for i, frame := range frames {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		entry := entries[frame.File]
		if entry == nil {
			return 0, 0, fmt.Errorf("archive missing frame at index %d", i)
		}
		reader, err := entry.Open()
		if err != nil {
			return 0, 0, err
		}
		config, format, decodeErr := image.DecodeConfig(reader)
		// 读到 EOF 才能校验 ZIP CRC，不能把完整下载误判为完整有效的帧源。
		_, readErr := io.Copy(io.Discard, reader)
		if err := errors.Join(decodeErr, readErr, reader.Close()); err != nil {
			return 0, 0, fmt.Errorf("inspect frame %d: %w", i, err)
		}
		if format != "png" && format != "jpeg" {
			return 0, 0, fmt.Errorf("unsupported image format at frame %d", i)
		}
		if i == 0 {
			width, height = config.Width, config.Height
		}
		if width <= 0 || height <= 0 || config.Width != width || config.Height != height {
			return 0, 0, fmt.Errorf("inconsistent dimensions at frame %d", i)
		}
	}
	return width, height, nil
}

func ugoiraBytesHash(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func ugoiraFileHash(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, file)
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

type sampleFixtureClient struct {
	metadata  pixivsdk.UgoiraMetadata
	archive   []byte
	metaErr   error
	saveErr   error
	sizeDelta int64
}

func (c sampleFixtureClient) UgoiraMetadata(context.Context, pixivsdk.UgoiraMetadataRequest) (pixivsdk.UgoiraMetadata, error) {
	return c.metadata, c.metaErr
}

func (c sampleFixtureClient) SaveResource(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if err := os.WriteFile(options.Path, c.archive, 0o600); err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Size: int64(len(c.archive)) + c.sizeDelta}, c.saveErr
}

func ugoiraSampleFixture(t *testing.T) sampleFixtureClient {
	t.Helper()
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"kind":"ugoira_archive","id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	c := sampleFixtureClient{metadata: pixivsdk.UgoiraMetadata{
		ArtworkID: 1,
		Archives: []pixivsdk.UgoiraArchive{{Quality: pixivsdk.UgoiraQualityOriginal, Resource: sdk.Resource{
			Ref: ref, URL: "https://i.pximg.net/private?signature=do-not-persist", RequestHeaders: map[string]string{"X-Fixture": "do-not-persist"},
		}}},
		Frames: []pixivsdk.UgoiraFrame{{Filename: "second.png", DelayMilliseconds: 31}, {Filename: "first.png", DelayMilliseconds: 80}},
	}}
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	// ZIP 顺序故意与 metadata 不同，防止采集或 driver 擅自排序。
	for i, name := range []string{"first.png", "second.png"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
		for y := 0; y < 6; y++ {
			for x := 0; x < 8; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(i*100 + x*10), G: uint8(y * 30), B: 42, A: 255})
			}
		}
		if err := png.Encode(entry, img); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	c.archive = body.Bytes()
	return c
}

func TestUgoiraSampleIDs(t *testing.T) {
	ids, err := parseUgoiraSampleIDs("1,42,9223372036854775807")
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 42, math.MaxInt64}) {
		t.Fatalf("IDs=%v error=%v", ids, err)
	}
	for _, raw := range []string{"", "0", "-1", "1,1", "1,", " 1", "+1", "01", "9223372036854775808", "private-token"} {
		if _, err := parseUgoiraSampleIDs(raw); err == nil || strings.Contains(err.Error(), "private-token") {
			t.Fatal("invalid IDs were accepted or echoed")
		}
	}
}

func TestUgoiraSampleCapture(t *testing.T) {
	c := ugoiraSampleFixture(t)
	dir := filepath.Join(t.TempDir(), "sample")
	sample, err := captureUgoiraSample(t.Context(), c, dir, 1, pixivsdk.UgoiraQualityOriginal)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Width != 8 || sample.Height != 6 || sample.DurationMS != 111 || sample.Frames[0].File != "second.png" || sample.Frames[0].Delay != 31 || sample.Frames[1].Delay != 80 {
		t.Fatalf("sample playback changed: %+v", sample)
	}
	body, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"do-not-persist", "pximg", "headers", "resource", "signature"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("manifest leaked SDK field %s", forbidden)
		}
	}
	if sample.ZIPSHA256 != ugoiraBytesHash(c.archive) || sample.ZIPBytes != int64(len(c.archive)) {
		t.Fatal("archive hash or length mismatch")
	}
	if _, err := captureUgoiraSample(t.Context(), c, dir, 1, pixivsdk.UgoiraQualityOriginal); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("recapture must reject existing sample: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("recapture changed the original sample")
	}
}

func TestUgoiraSampleCaptureFailure(t *testing.T) {
	for _, name := range []string{"metadata", "save", "quality", "empty", "filename", "delay", "missing", "duplicate", "zip", "checksum", "size", "identity", "cancel"} {
		t.Run(name, func(t *testing.T) {
			c := ugoiraSampleFixture(t)
			ctx := t.Context()
			switch name {
			case "metadata":
				c.metaErr = sdk.NewError("pixiv", "UgoiraMetadata", sdk.RateLimited)
			case "save":
				c.saveErr = context.Canceled
			case "quality":
				c.metadata.Archives[0].Quality = pixivsdk.UgoiraQualityMedium
			case "empty":
				c.metadata.Frames = nil
			case "filename":
				c.metadata.Frames[0].Filename = "../private-token"
			case "delay":
				c.metadata.Frames[0].DelayMilliseconds = 0
			case "missing":
				c.metadata.Frames[0].Filename = "absent.png"
			case "duplicate":
				c.metadata.Frames[1].Filename = c.metadata.Frames[0].Filename
			case "zip":
				c.archive = []byte("broken archive")
			case "checksum":
				offset := bytes.Index(c.archive, []byte("PK\x01\x02"))
				if offset < 0 {
					t.Fatal("fixture has no central directory")
				}
				c.archive[offset+16] ^= 1
			case "size":
				c.sizeDelta = 1
			case "identity":
				c.metadata.ArtworkID = 2
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			dir := filepath.Join(t.TempDir(), "sample")
			_, err := captureUgoiraSample(ctx, c, dir, 1, pixivsdk.UgoiraQualityOriginal)
			if err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatal("capture must fail without echoing upstream input")
			}
			if name == "metadata" && !sdk.IsReason(err, sdk.RateLimited) {
				t.Fatal("lost SDK error classification")
			}
			if (name == "save" || name == "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation cause")
			}
			if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("failed capture left a partial sample")
			}
		})
	}
}
