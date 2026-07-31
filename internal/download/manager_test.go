package download

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/ids"
	pixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestSanitizeAndGenerateFilename(t *testing.T) {
	illust := filename.FilenameData{ID: 123, Title: `a/b:c`, PageCount: 2, Author: `x*y`}
	got := filename.Generate(illust, 1, "{author}_{id}_{title}")
	if got != "x_y_123_a_b_c_p1" {
		t.Fatalf("filename = %q", got)
	}
}

func TestGenerateFilenameSanitizesTemplatePathSeparators(t *testing.T) {
	illust := filename.FilenameData{ID: 123, Title: "safe", PageCount: 1, Author: "artist"}
	got := filename.Generate(illust, 0, "../nested/{author}/{id}")
	if got != ".._nested_artist_123" {
		t.Fatalf("filename = %q", got)
	}
	if strings.ContainsAny(got, `/\`) {
		t.Fatalf("filename still contains path separator: %q", got)
	}
}

func TestDeduplicate(t *testing.T) {
	got := ids.DeduplicatePositive([]int64{3, 1, 3, 0, -1, 2})
	want := []int64{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("Deduplicate = %v, want %v", got, want)
	}
}

func TestSetDownloadPathCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	m := NewManager(nil, t.TempDir(), "{id}")
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
	m := NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
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

func TestDownloadSingleArtworkSanitizesExtensionFromURL(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42.jp%2Ag%3A%7C"
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{42: {
			ID: 42, Title: "single", PageCount: 1, Type: "illust",
			User:           pixiv.User{Name: "author"},
			MetaSinglePage: pixiv.SinglePage{OriginalImageURL: rawURL},
		}},
		downloads: map[string][]byte{rawURL: []byte("image")},
	}
	m := NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("Download returned unexpected artworks: %+v", got)
	}
	path := got[0].Files[0].Path
	if base := filepath.Base(path); base != "42.jp_g__" {
		t.Fatalf("download filename = %q, want %q", base, "42.jp_g__")
	}
	if strings.ContainsAny(filepath.Base(path), `/\:*?"<>|`) {
		t.Fatalf("download filename contains an unsafe character: %q", path)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("download escaped root: path=%q root=%q rel=%q", path, dir, rel)
	}
	assertFileBody(t, path, "image")
}

func TestDownloadSingleArtworkNormalizesPlatformInvalidExtensionEndings(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "nul", rawURL: "https://i.example/42.jp%00", want: "42.jp_"},
		{name: "newline", rawURL: "https://i.example/42.jp%0A", want: "42.jp_"},
		{name: "trailing space", rawURL: "https://i.example/42.jpg%20", want: "42.jpg"},
		{name: "trailing dot", rawURL: "https://i.example/42.", want: "42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			client := &fakePixivClient{
				details: map[int64]pixiv.Illust{42: {
					ID: 42, Title: "single", PageCount: 1, Type: "illust",
					User:           pixiv.User{Name: "author"},
					MetaSinglePage: pixiv.SinglePage{OriginalImageURL: test.rawURL},
				}},
				downloads: map[string][]byte{test.rawURL: []byte("image")},
			}
			m := NewManager(client, dir, "{id}")

			got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
			if err != nil {
				t.Fatalf("Download returned error: %v", err)
			}
			if len(got) != 1 || len(got[0].Files) != 1 {
				t.Fatalf("Download returned unexpected artworks: %+v", got)
			}
			path := got[0].Files[0].Path
			base := filepath.Base(path)
			if base != test.want {
				t.Fatalf("download filename = %q, want %q", base, test.want)
			}
			for _, character := range base {
				if character < 0x20 || character == 0x7f {
					t.Fatalf("download filename contains ASCII control %#U: %q", character, base)
				}
			}
			if strings.TrimRight(base, ". ") != base {
				t.Fatalf("download filename has a Windows-invalid ending: %q", base)
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				t.Fatalf("Rel returned error: %v", err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				t.Fatalf("download escaped root: path=%q root=%q rel=%q", path, dir, rel)
			}
			assertFileBody(t, path, "image")
		})
	}
}

func TestDownloadSingleArtworkWithoutURLExtensionDoesNotInventOne(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42"
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{42: {
			ID: 42, Title: "single", PageCount: 1, Type: "illust",
			User:           pixiv.User{Name: "author"},
			MetaSinglePage: pixiv.SinglePage{OriginalImageURL: rawURL},
		}},
		downloads: map[string][]byte{rawURL: []byte("image")},
	}
	m := NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("Download returned unexpected artworks: %+v", got)
	}
	path := got[0].Files[0].Path
	if base := filepath.Base(path); base != "42" {
		t.Fatalf("download filename = %q, want %q", base, "42")
	}
	assertFileBody(t, path, "image")
}

// 下载管理器只能把作品中取得的资源地址交给公开 SDK 解析和下载；它不再保留
// legacy Source 的原始 URL 写入接口，也不自行实现 SDK 已提供的原子替换。
func TestDownloadUsesSDKResourceReferenceAndDestination(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{42: {
			ID: 42, Title: "single", PageCount: 1, Type: "illust",
			User:           pixiv.User{Name: "author"},
			MetaSinglePage: pixiv.SinglePage{OriginalImageURL: "https://i.example/42.jpg"},
		}},
		downloads: map[string][]byte{"https://i.example/42.jpg": []byte("jpg")},
	}
	m := NewManager(client, dir, "{id}")

	if _, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}}); err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if got := client.parsedURLs; !slices.Equal(got, []string{"https://i.example/42.jpg"}) {
		t.Fatalf("ParseResourceRef URLs = %v", got)
	}
	if got := client.destinations; len(got) != 1 || filepath.Base(got[0]) != "42.jpg" {
		t.Fatalf("SDK destinations = %v", got)
	}
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
	m := NewManager(client, dir, "../escape/{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
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
	m := NewManager(client, dir, "{id}")

	_, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}})
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
	m := NewManager(client, dir, "{id}")

	if _, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{42}}); err != nil {
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
	m := NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{7}})
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

func TestDownloadMultiPageArtworkSanitizesExtensionsFromURLs(t *testing.T) {
	dir := t.TempDir()
	urls := []string{
		"https://i.example/7_p0.jp%2Ag%3A%7C",
		"https://i.example/7_p1.pn%2Ag%3A%7C",
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{7: {
			ID: 7, Title: "multi", PageCount: 2, Type: "illust",
			User: pixiv.User{Name: "author"},
			MetaPages: []pixiv.MetaPage{
				{ImageURLs: pixiv.ImageURLs{Original: urls[0]}},
				{ImageURLs: pixiv.ImageURLs{Original: urls[1]}},
			},
		}},
		downloads: map[string][]byte{
			urls[0]: []byte("p0"),
			urls[1]: []byte("p1"),
		},
	}
	m := NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{7}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 2 {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	wantBases := []string{"7_p0.jp_g__", "7_p1.pn_g__"}
	for index, file := range got[0].Files {
		if base := filepath.Base(file.Path); base != wantBases[index] {
			t.Fatalf("file %d basename = %q, want %q", index, base, wantBases[index])
		}
		if strings.ContainsAny(filepath.Base(file.Path), `/\:*?"<>|`) {
			t.Fatalf("file %d contains an unsafe character: %q", index, file.Path)
		}
		rel, err := filepath.Rel(dir, file.Path)
		if err != nil {
			t.Fatalf("Rel(%d) returned error: %v", index, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			t.Fatalf("file %d escaped root: path=%q root=%q rel=%q", index, file.Path, dir, rel)
		}
	}
	assertFileBody(t, got[0].Files[0].Path, "p0")
	assertFileBody(t, got[0].Files[1].Path, "p1")
}

func TestConvertUgoiraUsesInjectedEncoder(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", []byte("frame"))

	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m := NewManager(nil, dir, "{id}")
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
	m := NewManager(nil, dir, "{id}")
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
	m := NewManager(nil, dir, "{id}")
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
	meta.DownloadURL = "https://i.example/ugoira.zip"
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
	m := NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(&recordingUgoiraEncoder{output: []byte("gif")})

	_, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{9}})
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
	meta.DownloadURL = "https://i.example/ugoira.zip"
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
	m := NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{9}})
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

func TestDownloadUgoiraUsesSDKDownloadURL(t *testing.T) {
	dir := t.TempDir()
	meta := pixiv.UgoiraMetadata{
		ZipURLs:         pixiv.UgoiraZipURLs{Medium: "https://i.example/medium.zip", Original: "https://i.example/original.zip"},
		Frames:          []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}},
		DownloadURL:     "https://i.example/selected.zip",
		DownloadQuality: pixiv.UgoiraZipQualityOriginal,
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			9: {ID: 9, Title: "ugo", PageCount: 1, Type: "ugoira", User: pixiv.User{Name: "author"}},
		},
		ugoira:    map[int64]pixiv.UgoiraMetadata{9: meta},
		downloads: map[string][]byte{"https://i.example/selected.zip": makeZip(t, "000000.jpg", []byte("frame"))},
	}
	m := NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(&recordingUgoiraEncoder{output: []byte("gif")})

	if _, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{9}}); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if len(client.parsedURLs) != 1 || client.parsedURLs[0] != "https://i.example/selected.zip" {
		t.Fatalf("parsed URLs = %v", client.parsedURLs)
	}
}

func TestDownloadMissingImageURLReturnsError(t *testing.T) {
	m := NewManager(&fakePixivClient{
		details: map[int64]pixiv.Illust{
			1: {ID: 1, Title: "empty", PageCount: 1, Type: "illust"},
		},
	}, t.TempDir(), "{id}")

	_, err := m.Download(context.Background(), application.DownloadRequest{IllustIDs: []int64{1}})
	if err == nil || !strings.Contains(err.Error(), "no original image url") {
		t.Fatalf("Download error = %v", err)
	}
}

func TestDownloadUgoiraExplicitAPNGUsesRustEncoder(t *testing.T) {
	dir := t.TempDir()
	meta := pixiv.UgoiraMetadata{Frames: []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}}
	meta.ZipURLs.Medium = "https://i.example/ugoira.zip"
	meta.DownloadURL = "https://i.example/ugoira.zip"
	m := NewManager(&fakePixivClient{
		details: map[int64]pixiv.Illust{
			1: {ID: 1, Title: "ugo", PageCount: 1, Type: "ugoira"},
		},
		ugoira:    map[int64]pixiv.UgoiraMetadata{1: meta},
		downloads: map[string][]byte{"https://i.example/ugoira.zip": makeZip(t, "000000.jpg", []byte("frame"))},
	}, dir, "{id}")
	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), application.DownloadRequest{
		IllustIDs:    []int64{1},
		UgoiraFormat: application.UgoiraFormatAPNG,
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 || filepath.Ext(got[0].Files[0].Path) != ".apng" {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if encoder.input.ZipPath == "" || encoder.input.Format != AnimationFormatAPNG {
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
	details      map[int64]pixiv.Illust
	ugoira       map[int64]pixiv.UgoiraMetadata
	downloads    map[string][]byte
	downloadErr  error
	parsedURLs   []string
	destinations []string
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

func (c *fakePixivClient) ParseResourceRef(rawURL string) (pixiv.ResourceRef, error) {
	c.parsedURLs = append(c.parsedURLs, rawURL)
	return pixiv.ResourceRef{URL: rawURL}, nil
}

func (c *fakePixivClient) DownloadResource(_ context.Context, ref pixiv.ResourceRef, destination string) (pixiv.ResourceDownloadResult, error) {
	if c.downloadErr != nil {
		return pixiv.ResourceDownloadResult{}, c.downloadErr
	}
	body, ok := c.downloads[ref.URL]
	if !ok {
		return pixiv.ResourceDownloadResult{}, os.ErrNotExist
	}
	c.destinations = append(c.destinations, destination)
	if err := os.WriteFile(destination, body, 0o644); err != nil {
		return pixiv.ResourceDownloadResult{}, err
	}
	return pixiv.ResourceDownloadResult{DestinationPath: destination}, nil
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
