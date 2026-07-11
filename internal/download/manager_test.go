package download

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
)

func TestSanitizeAndGenerateFilename(t *testing.T) {
	illust := utils.FilenameData{ID: 123, Title: `a/b:c`, PageCount: 2, Author: `x*y`}
	got := utils.GenerateFilename(illust, 1, "{author}_{id}_{title}")
	if got != "x_y_123_a_b_c_p1" {
		t.Fatalf("filename = %q", got)
	}
}

func TestGenerateFilenameSanitizesTemplatePathSeparators(t *testing.T) {
	illust := utils.FilenameData{ID: 123, Title: "safe", PageCount: 1, Author: "artist"}
	got := utils.GenerateFilename(illust, 0, "../nested/{author}/{id}")
	if got != ".._nested_artist_123" {
		t.Fatalf("filename = %q", got)
	}
	if strings.ContainsAny(got, `/\`) {
		t.Fatalf("filename still contains path separator: %q", got)
	}
}

func TestDeduplicate(t *testing.T) {
	got := utils.Deduplicate([]int64{3, 1, 3, 0, -1, 2})
	want := []int64{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("Deduplicate = %v, want %v", got, want)
	}
}

func TestSetDownloadPathCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	m := NewManager(nil, nil, t.TempDir(), "{id}")
	if err := m.SetDownloadPath(dir); err != nil {
		t.Fatalf("SetDownloadPath returned error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("download dir was not created: %v", err)
	}
}

func TestDownloadSingleArtworkReturnsPath(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			42: {
				ID:        42,
				Title:     "single",
				PageCount: 1,
				Type:      "illust",
				User:      pixiv.User{Name: "author"},
				MetaSinglePage: pixiv.SinglePage{
					OriginalImageURL: "https://i.example/42.jpg",
				},
			},
		},
		downloads: map[string][]byte{"https://i.example/42.jpg": []byte("jpg")},
	}
	m := NewManager(client, nil, dir, "{id}")

	got, err := m.Download(context.Background(), []int64{42})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("Download returned unexpected artworks: %+v", got)
	}
	if got[0].IllustID != 42 || got[0].Title != "single" || got[0].Author != "author" {
		t.Fatalf("Download returned wrong metadata: %+v", got[0])
	}
	if filepath.Base(got[0].Files[0].Path) != "42.jpg" {
		t.Fatalf("download path = %q", got[0].Files[0].Path)
	}
	assertFileBody(t, got[0].Files[0].Path, "jpg")
}

func TestDownloadKeepsArtworkInsideDownloadRoot(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			42: {
				ID:        42,
				Title:     "single",
				PageCount: 1,
				Type:      "illust",
				User:      pixiv.User{Name: "author"},
				MetaSinglePage: pixiv.SinglePage{
					OriginalImageURL: "https://i.example/42.jpg",
				},
			},
		},
		downloads: map[string][]byte{"https://i.example/42.jpg": []byte("jpg")},
	}
	m := NewManager(client, nil, dir, "../escape/{id}")

	got, err := m.Download(context.Background(), []int64{42})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("Download returned unexpected artworks: %+v", got)
	}
	rel, err := filepath.Rel(dir, got[0].Files[0].Path)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("download escaped root: path=%q root=%q rel=%q", got[0].Files[0].Path, dir, rel)
	}
	assertFileBody(t, got[0].Files[0].Path, "jpg")
}

func TestDownloadFailureDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			42: {
				ID:        42,
				Title:     "single",
				PageCount: 1,
				Type:      "illust",
				User:      pixiv.User{Name: "author"},
				MetaSinglePage: pixiv.SinglePage{
					OriginalImageURL: "https://i.example/42.jpg",
				},
			},
		},
		downloadErr: errors.New("network broke"),
	}
	m := NewManager(client, nil, dir, "{id}")

	_, err := m.Download(context.Background(), []int64{42})
	if err == nil {
		t.Fatal("Download returned nil error")
	}
	assertFileBody(t, target, "old")
}

func TestDownloadSuccessReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			42: {
				ID:        42,
				Title:     "single",
				PageCount: 1,
				Type:      "illust",
				User:      pixiv.User{Name: "author"},
				MetaSinglePage: pixiv.SinglePage{
					OriginalImageURL: "https://i.example/42.jpg",
				},
			},
		},
		downloads: map[string][]byte{"https://i.example/42.jpg": []byte("new")},
	}
	m := NewManager(client, nil, dir, "{id}")

	if _, err := m.Download(context.Background(), []int64{42}); err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	assertFileBody(t, target, "new")
}

func TestDownloadMultiPageArtworkReturnsAllPaths(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			7: {
				ID:        7,
				Title:     "multi",
				PageCount: 2,
				Type:      "illust",
				User:      pixiv.User{Name: "author"},
				MetaPages: []pixiv.MetaPage{
					{ImageURLs: pixiv.ImageURLs{Original: "https://i.example/7_p0.png"}},
					{ImageURLs: pixiv.ImageURLs{Original: "https://i.example/7_p1.png"}},
				},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/7_p0.png": []byte("p0"),
			"https://i.example/7_p1.png": []byte("p1"),
		},
	}
	m := NewManager(client, nil, dir, "{id}")

	got, err := m.Download(context.Background(), []int64{7})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 2 {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if filepath.Base(filepath.Dir(got[0].Files[0].Path)) != "7 - multi" {
		t.Fatalf("multi-page directory = %q", filepath.Dir(got[0].Files[0].Path))
	}
	if filepath.Base(got[0].Files[0].Path) != "7_p0.png" || filepath.Base(got[0].Files[1].Path) != "7_p1.png" {
		t.Fatalf("download paths = %+v", got[0].Files)
	}
	assertFileBody(t, got[0].Files[0].Path, "p0")
	assertFileBody(t, got[0].Files[1].Path, "p1")
}

func TestConvertUgoiraUsesInjectedEncoder(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", []byte("frame"))

	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m := NewManager(nil, nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)
	outPath := filepath.Join(dir, "out.gif")
	err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}, dir, outPath)
	if err != nil {
		t.Fatalf("ConvertUgoira returned error: %v", err)
	}
	if encoder.input.ZipPath != zipPath || encoder.input.WorkDir != dir || encoder.input.Format != AnimationFormatGIF || encoder.input.MaxEdge != 0 {
		t.Fatalf("encoder input = %+v", encoder.input)
	}
	assertFileBody(t, outPath, "gif")
}

func TestConvertUgoiraFailureDoesNotReplaceExistingGIF(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", []byte("frame"))
	outPath := filepath.Join(dir, "out.gif")
	if err := os.WriteFile(outPath, []byte("old-gif"), 0o644); err != nil {
		t.Fatal(err)
	}

	encoder := &recordingUgoiraEncoder{err: errors.New("encoder failed"), output: []byte("partial")}
	m := NewManager(nil, nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}, dir, outPath)
	if err == nil {
		t.Fatal("ConvertUgoira returned nil error")
	}
	assertFileBody(t, outPath, "old-gif")
}

func TestConvertUgoiraSuccessReplacesExistingGIF(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", []byte("frame"))
	outPath := filepath.Join(dir, "out.gif")
	if err := os.WriteFile(outPath, []byte("old-gif"), 0o644); err != nil {
		t.Fatal(err)
	}

	encoder := &recordingUgoiraEncoder{output: []byte("new-gif")}
	m := NewManager(nil, nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	if err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}, dir, outPath); err != nil {
		t.Fatalf("ConvertUgoira returned error: %v", err)
	}
	assertFileBody(t, outPath, "new-gif")
}

func TestDownloadUgoiraZipFailureCleansTemporaryZip(t *testing.T) {
	dir := t.TempDir()
	meta := pixiv.UgoiraMetadata{
		Frames: []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}},
	}
	meta.ZipURLs.Medium = "https://i.example/ugoira.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			9: {
				ID:        9,
				Title:     "ugo",
				PageCount: 1,
				Type:      "ugoira",
				User:      pixiv.User{Name: "author"},
			},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: meta,
		},
		downloadErr: errors.New("zip download failed"),
	}
	m := NewManager(client, nil, dir, "{id}")
	m.SetUgoiraEncoder(&recordingUgoiraEncoder{output: []byte("gif")})

	_, err := m.Download(context.Background(), []int64{9})
	if err == nil {
		t.Fatal("Download returned nil error")
	}
	matches, err := filepath.Glob(filepath.Join(dir, "9 - ugo", "ugoira-*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary ugoira zip files remain: %v", matches)
	}
}

func TestDownloadUgoiraReturnsFinalGIFOnly(t *testing.T) {
	dir := t.TempDir()
	meta := pixiv.UgoiraMetadata{
		Frames: []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}},
	}
	meta.ZipURLs.Medium = "https://i.example/ugoira.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			9: {
				ID:        9,
				Title:     "ugo",
				PageCount: 1,
				Type:      "ugoira",
				User:      pixiv.User{Name: "author"},
			},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: meta,
		},
		downloads: map[string][]byte{"https://i.example/ugoira.zip": makeZip(t, "000000.jpg", []byte("frame"))},
	}
	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m := NewManager(client, nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), []int64{9})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if filepath.Ext(got[0].Files[0].Path) != ".gif" {
		t.Fatalf("ugoira output path = %q", got[0].Files[0].Path)
	}
	if strings.HasSuffix(got[0].Files[0].Path, ".zip") {
		t.Fatalf("ugoira returned temporary zip path: %q", got[0].Files[0].Path)
	}
	assertFileBody(t, got[0].Files[0].Path, "gif")
}

func TestDownloadMissingImageURLReturnsError(t *testing.T) {
	m := NewManager(&fakePixivClient{
		details: map[int64]pixiv.Illust{
			1: {ID: 1, Title: "empty", PageCount: 1, Type: "illust"},
		},
	}, nil, t.TempDir(), "{id}")

	_, err := m.Download(context.Background(), []int64{1})
	if err == nil || !strings.Contains(err.Error(), "no downloadable image url") {
		t.Fatalf("Download error = %v", err)
	}
}

func TestDownloadUgoiraWithoutFFmpegUsesRustEncoder(t *testing.T) {
	dir := t.TempDir()
	meta := pixiv.UgoiraMetadata{Frames: []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}}
	meta.ZipURLs.Medium = "https://i.example/ugoira.zip"
	m := NewManager(&fakePixivClient{
		details: map[int64]pixiv.Illust{
			1: {ID: 1, Title: "ugo", PageCount: 1, Type: "ugoira"},
		},
		ugoira:    map[int64]pixiv.UgoiraMetadata{1: meta},
		downloads: map[string][]byte{"https://i.example/ugoira.zip": makeZip(t, "000000.jpg", []byte("frame"))},
	}, nil, dir, "{id}")
	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 || filepath.Ext(got[0].Files[0].Path) != ".gif" {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if encoder.input.ZipPath == "" || encoder.input.Format != AnimationFormatGIF {
		t.Fatalf("encoder input = %+v", encoder.input)
	}
}

type recordingUgoiraEncoder struct {
	input  UgoiraEncodeInput
	output []byte
	err    error
}

func (e *recordingUgoiraEncoder) Encode(ctx context.Context, input UgoiraEncodeInput) error {
	e.input = input
	return writeTempAnimation(ctx, input.OutputPath, func(tmpOutput string) error {
		if len(e.output) > 0 {
			if err := os.WriteFile(tmpOutput, e.output, 0o644); err != nil {
				return err
			}
		}
		return e.err
	})
}

func createZip(t *testing.T, path, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, makeZip(t, name, body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fakePixivClient struct {
	details     map[int64]pixiv.Illust
	ugoira      map[int64]pixiv.UgoiraMetadata
	downloads   map[string][]byte
	downloadErr error
}

func (c *fakePixivClient) IllustDetail(_ context.Context, id int64) (*pixiv.IllustDetail, error) {
	illust, ok := c.details[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &pixiv.IllustDetail{Illust: illust}, nil
}

func (c *fakePixivClient) UgoiraMetadata(_ context.Context, id int64) (*pixiv.UgoiraMetadataResult, error) {
	meta, ok := c.ugoira[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &pixiv.UgoiraMetadataResult{UgoiraMetadata: meta}, nil
}

func (c *fakePixivClient) Download(_ context.Context, rawURL string, dst io.Writer) error {
	if c.downloadErr != nil {
		return c.downloadErr
	}
	body, ok := c.downloads[rawURL]
	if !ok {
		return os.ErrNotExist
	}
	_, err := dst.Write(body)
	return err
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("%s body = %q, want %q", path, string(body), want)
	}
}
