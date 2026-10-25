package downloader_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadSingleArtworkNormalizesFilenameAfterMIMEDetection(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "encoded unsafe extension", rawURL: "https://i.example/42.jp%2Ag%3A%7C"},
		{name: "nul", rawURL: "https://i.example/42.jp%00"},
		{name: "newline", rawURL: "https://i.example/42.jp%0A"},
		{name: "trailing space", rawURL: "https://i.example/42.jpg%20"},
		{name: "trailing dot", rawURL: "https://i.example/42."},
		{name: "without extension", rawURL: "https://i.example/42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			client := &fakePixivClient{
				details: map[int64]pixiv.Artwork{42: {
					ID: 42, Title: "single", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
					User:  pixiv.User{Name: "author"},
					Pages: []pixiv.ArtworkPage{artworkPage(test.rawURL, 0)},
				}},
			}
			client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
				body := []byte("image")
				if err := os.WriteFile(options.Path, body, 0o600); err != nil {
					return sdk.SavedResource{}, err
				}
				return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/jpeg"}, nil
			}
			m := downloader.NewArtworkDownloader(client, dir, "{id}")

			got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
			if err != nil {
				t.Fatalf("Download returned error: %v", err)
			}
			if len(got.Items) != 1 || len(got.Items[0].Files) != 1 {
				t.Fatalf("Download returned unexpected artworks: %+v", got)
			}
			path := got.Items[0].Files[0].Path
			base := filepath.Base(path)
			if base != "42.jpg" {
				t.Fatalf("download filename = %q, want %q", base, "42.jpg")
			}
			if strings.ContainsAny(base, `/\:*?"<>|`) {
				t.Fatalf("download filename contains an unsafe character: %q", base)
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

func TestDownloadUsesSDKResourceReferenceAndDestination(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42.jpg"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{42: {
			ID: 42, Title: "single", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
			User:  pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
		}},
		downloads: map[string][]byte{rawURL: []byte("jpg")},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	if _, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}}); err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if got := client.savedURLs; !slices.Equal(got, []string{rawURL}) {
		t.Fatalf("saved resource URLs = %v", got)
	}
	if got := client.destinations; len(got) != 1 || filepath.Base(got[0]) != "42.jpg" {
		t.Fatalf("SDK destinations = %v", got)
	}
}

func TestDownloadKeepsArtworkInsideDownloadRoot(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42.jpg"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			42: {
				ID: 42, Title: "single", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
			},
		},
		downloads: map[string][]byte{rawURL: []byte("jpg")},
	}
	m := downloader.NewArtworkDownloader(client, dir, "../escape/{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Files) != 1 {
		t.Fatalf("Download returned unexpected artworks: %+v", got)
	}
	rel, err := filepath.Rel(dir, got.Items[0].Files[0].Path)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("download escaped root: path=%q root=%q rel=%q", got.Items[0].Files[0].Path, dir, rel)
	}
	assertFileBody(t, got.Items[0].Files[0].Path, "jpg")
}

func TestDownloadFailureDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL := "https://i.example/42.jpg"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			42: {
				ID: 42, Title: "single", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
			},
		},
		downloadErr: errors.New("network broke"),
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
	require.NoError(t, err)
	require.Empty(t, batch.Items)
	require.Len(t, batch.Failures, 1)
	require.Equal(t, int64(42), batch.Failures[0].IllustID)
	assertFileBody(t, target, "old")
}

func TestDownloadMultiPageArtworkReturnsAllPaths(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			7: {
				ID: 7, Title: "multi", PageCount: 2, Kind: pixiv.ArtworkKindIllustration,
				User: pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{
					artworkPage("https://i.example/7_p0.png", 0),
					artworkPage("https://i.example/7_p1.png", 1),
				},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/7_p0.png": []byte("p0"),
			"https://i.example/7_p1.png": []byte("p1"),
		},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Files) != 2 {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if filepath.Base(filepath.Dir(got.Items[0].Files[0].Path)) != "7 - multi" {
		t.Fatalf("multi-page directory = %q", filepath.Dir(got.Items[0].Files[0].Path))
	}
	if filepath.Base(got.Items[0].Files[0].Path) != "7_p0.png" || filepath.Base(got.Items[0].Files[1].Path) != "7_p1.png" {
		t.Fatalf("download paths = %+v", got.Items[0].Files)
	}
	assertFileBody(t, got.Items[0].Files[0].Path, "p0")
	assertFileBody(t, got.Items[0].Files[1].Path, "p1")
}

// TestDownloadMultiPageArtworkKeepsPublishedPrefixOnPageFailure 验证同一作品的
// 前页已经发布后，后页失败仍保留 partial item，并同时报告该作品失败。
func TestDownloadMultiPageArtworkKeepsPublishedPrefixOnPageFailure(t *testing.T) {
	dir := t.TempDir()
	pageFailure := errors.New("page one failed")
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{7: {
			ID: 7, Title: "multi", PageCount: 3, Kind: pixiv.ArtworkKindIllustration,
			User: pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{
				artworkPage("https://i.example/7_p0.png", 0),
				artworkPage("https://i.example/7_p1.png", 1),
				artworkPage("https://i.example/7_p2.png", 2),
			},
		}},
	}
	calls := 0
	client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		calls++
		if calls > 1 {
			return sdk.SavedResource{}, pageFailure
		}
		body := []byte("page-zero")
		if err := os.WriteFile(options.Path, body, 0o644); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/png"}, nil
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
	require.NoError(t, err)
	require.Len(t, batch.Items, 1)
	require.Len(t, batch.Items[0].Files, 1)
	require.Len(t, batch.Failures, 1)
	require.Equal(t, int64(7), batch.Failures[0].IllustID)
	require.ErrorIs(t, batch.Failures[0].Cause, pageFailure)
	assertFileBody(t, batch.Items[0].Files[0].Path, "page-zero")
}

func TestDownloadMultiPageArtworkPublishesDetectedExtensions(t *testing.T) {
	dir := t.TempDir()
	urls := []string{
		"https://i.example/7_p0.jp%2Ag%3A%7C",
		"https://i.example/7_p1.pn%2Ag%3A%7C",
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{7: {
			ID: 7, Title: "multi", PageCount: 2, Kind: pixiv.ArtworkKindIllustration,
			User: pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{
				artworkPage(urls[0], 0),
				artworkPage(urls[1], 1),
			},
		}},
		downloads: map[string][]byte{
			urls[0]: []byte("p0"),
			urls[1]: []byte("p1"),
		},
	}
	client.saveResourceOverride = func(_ context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		payload, err := sdk.ResourceRefPayload(ref)
		if err != nil {
			return sdk.SavedResource{}, err
		}
		body, ok := client.downloads[string(payload)]
		if !ok {
			return sdk.SavedResource{}, os.ErrNotExist
		}
		if err := os.WriteFile(options.Path, body, 0o600); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/png"}, nil
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Files) != 2 {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	wantBases := []string{"7_p0.png", "7_p1.png"}
	for index, file := range got.Items[0].Files {
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
	assertFileBody(t, got.Items[0].Files[0].Path, "p0")
	assertFileBody(t, got.Items[0].Files[1].Path, "p1")
}

func TestDownloadPagesSelectionIsOneBasedAndErrorsOnMissing(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			7: {
				ID: 7, Title: "multi", PageCount: 3, Kind: pixiv.ArtworkKindIllustration, User: pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{
					artworkPage("https://i.example/7_p0.png", 0),
					artworkPage("https://i.example/7_p1.png", 1),
					artworkPage("https://i.example/7_p2.png", 2),
				},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/7_p0.png": []byte("p0"),
			"https://i.example/7_p1.png": []byte("p1"),
			"https://i.example/7_p2.png": []byte("p2"),
		},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")
	got, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{1, 3},
		Quality:   downloader.DownloadQualityOriginal,
	})
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Files) != 2 {
		t.Fatalf("files=%+v", got)
	}
	if got.Items[0].Files[0].Page != 1 || got.Items[0].Files[1].Page != 3 {
		t.Fatalf("pages=%+v", got.Items[0].Files)
	}
	if filepath.Base(got.Items[0].Files[0].Path) != "7_p0.png" || filepath.Base(got.Items[0].Files[1].Path) != "7_p2.png" {
		t.Fatalf("paths=%q %q", got.Items[0].Files[0].Path, got.Items[0].Files[1].Path)
	}

	missing, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{4},
		Quality:   downloader.DownloadQualityOriginal,
	})
	require.NoError(t, err)
	require.Empty(t, missing.Items)
	require.Len(t, missing.Failures, 1)
	require.Contains(t, missing.Failures[0].Message, "page 4 does not exist")
}

// TestDownloadRejectsInvalidFilenameTemplate 验证无效或缺少必要字段的模板在
// 下载前就被拒绝，而不是写出空文件名互相覆盖。
func TestDownloadRejectsInvalidFilenameTemplate(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42.jpg"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{42: {
			ID: 42, Title: "single", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
			User:  pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
		}},
		downloads: map[string][]byte{rawURL: []byte("jpg")},
	}
	for _, tmpl := range []string{
		"{id",             // 未闭合花括号
		"{unknown_field}", // 未知占位符
		"{date}",          // CreateDate 缺失时 GenerateChecked 报错
	} {
		t.Run(tmpl, func(t *testing.T) {
			m := downloader.NewArtworkDownloader(client, dir, tmpl)
			batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
			require.NoError(t, err)
			require.Empty(t, batch.Items)
			require.Len(t, batch.Failures, 1)
			matches, err := filepath.Glob(filepath.Join(dir, "*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("template %q wrote files before rejection: %v", tmpl, matches)
			}
		})
	}
}

// TestDownloadPopulatesDocumentedFilenamePlaceholders 验证文档承诺的占位符
// （id、title、author、author_id、date、tags、num）都被填充。
func TestDownloadPopulatesDocumentedFilenamePlaceholders(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/7.jpg"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{7: {
			ID: 7, Title: "Title", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
			User:        pixiv.User{Name: "Author", ID: 77},
			PublishedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Tags:        []pixiv.Tag{{Name: "tag1"}, {Name: "tag2"}},
			Pages:       []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
		}},
		downloads: map[string][]byte{rawURL: []byte("jpg")},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}-{title}-{author}-{author_id}-{date}-{tags}-{num}")
	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Files) != 1 {
		t.Fatalf("files=%+v", got)
	}
	base := filepath.Base(got.Items[0].Files[0].Path)
	for _, want := range []string{"7-", "Title-", "Author-", "77-", "tag1", "tag2"} {
		if !strings.Contains(base, want) {
			t.Fatalf("filename %q missing %q", base, want)
		}
	}
}

func TestDownloadPublishesDetectedImageExtension(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        []byte
		contentType string
		wantExt     string
	}{
		{name: "jpeg-mime", body: []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), contentType: "image/jpeg; charset=binary", wantExt: ".jpg"},
		{name: "png-mime", body: []byte("\x89PNG\r\n\x1a\nPNG fixture"), contentType: "image/png", wantExt: ".png"},
		{name: "gif-signature", body: []byte("GIF89aGIF fixture"), wantExt: ".gif"},
		{name: "webp-signature", body: []byte("RIFF\x00\x00\x00\x00WEBPVP8 fixture"), wantExt: ".webp"},
		{name: "mime-overrides-signature", body: []byte("\x89PNG\r\n\x1a\nPNG fixture"), contentType: "image/jpeg; charset=binary", wantExt: ".jpg"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			rawURL := "https://i.example/42.png"
			client := &fakePixivClient{
				details: map[int64]pixiv.Artwork{42: {
					ID: 42, Title: "quality", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
					User:  pixiv.User{Name: "author"},
					Pages: []pixiv.ArtworkPage{artworkPageWithArtworkRef(rawURL)},
				}},
			}
			body := append([]byte(nil), test.body...)
			client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
				if err := os.WriteFile(options.Path, body, 0o600); err != nil {
					return sdk.SavedResource{}, err
				}
				return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: test.contentType}, nil
			}

			batch, err := downloader.NewArtworkDownloader(client, dir, "{id}").Download(context.Background(), downloader.DownloadRequest{
				IllustIDs: []int64{42}, Quality: downloader.DownloadQualityOriginal,
			})
			require.NoError(t, err)
			require.Len(t, batch.Items, 1)
			require.Len(t, batch.Items[0].Files, 1)
			path := batch.Items[0].Files[0].Path
			require.Equal(t, test.wantExt, filepath.Ext(path))
			assertFileBody(t, path, string(body))
		})
	}

	t.Run("unsupported replaces nothing", func(t *testing.T) {
		dir := t.TempDir()
		client := &fakePixivClient{
			details: map[int64]pixiv.Artwork{42: {
				ID: 42, Title: "unsupported", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPageWithArtworkRef("https://i.example/42.png")},
			}},
		}
		body := []byte("not an image")
		client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
			if err := os.WriteFile(options.Path, body, 0o600); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "application/octet-stream"}, nil
		}
		batch, err := downloader.NewArtworkDownloader(client, dir, "{id}").Download(context.Background(), downloader.DownloadRequest{
			IllustIDs: []int64{42}, Quality: downloader.DownloadQualityOriginal,
		})
		require.NoError(t, err)
		require.Empty(t, batch.Items)
		require.Len(t, batch.Failures, 1)
		_, statErr := os.Stat(filepath.Join(dir, "42.png"))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func TestDownloadRejectsUndetectableStaticImageWithoutMetadata(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{42: {
			ID: 42, Title: "undetectable", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
			User:  pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{artworkPageWithArtworkRef(rawURL)},
		}},
	}
	body := []byte("not an image")
	client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		if err := os.WriteFile(options.Path, body, 0o600); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, nil
	}

	batch, err := downloader.NewArtworkDownloader(client, dir, "{id}").Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{42}, Quality: downloader.DownloadQualityOriginal,
	})
	require.NoError(t, err)
	require.Empty(t, batch.Items)
	require.Len(t, batch.Failures, 1)
	_, statErr := os.Stat(filepath.Join(dir, "42"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

// TestDownloadRequestsQualityVariant 固定 CLI/MCP 的 --quality 契约：每个派生质量
// 都请求对应 variant，original 复用页面既有 ref（variant 为空）。
func TestDownloadRequestsQualityVariant(t *testing.T) {
	for _, test := range []struct {
		quality downloader.DownloadQuality
		variant string
	}{
		{downloader.DownloadQualityOriginal, ""},
		{downloader.DownloadQualityRegular, "regular"},
		{downloader.DownloadQualitySmall, "small"},
		{downloader.DownloadQualityThumb, "thumb"},
		{downloader.DownloadQualityMini, "mini"},
	} {
		t.Run(string(test.quality), func(t *testing.T) {
			dir := t.TempDir()
			client := &fakePixivClient{
				details: map[int64]pixiv.Artwork{42: {
					ID: 42, Title: "quality", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
					User:  pixiv.User{Name: "author"},
					Pages: []pixiv.ArtworkPage{artworkPageWithArtworkRef("https://i.example/42.png")},
				}},
			}
			var payload []byte
			client.saveResourceOverride = func(_ context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
				var err error
				payload, err = sdk.ResourceRefPayload(ref)
				if err != nil {
					return sdk.SavedResource{}, err
				}
				body := []byte("\x89PNG\r\n\x1a\nPNG fixture")
				if err := os.WriteFile(options.Path, body, 0o600); err != nil {
					return sdk.SavedResource{}, err
				}
				return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, nil
			}

			if _, err := downloader.NewArtworkDownloader(client, dir, "{id}").Download(context.Background(), downloader.DownloadRequest{
				IllustIDs: []int64{42}, Quality: test.quality,
			}); err != nil {
				t.Fatalf("Download() error = %v", err)
			}
			var decoded struct {
				Kind    string `json:"k"`
				ID      int64  `json:"id"`
				Page    int    `json:"p"`
				Variant string `json:"v"`
			}
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("decode ref %q: %v", payload, err)
			}
			if decoded.Kind != "artwork" || decoded.ID != 42 || decoded.Page != 0 || decoded.Variant != test.variant {
				t.Fatalf("ref payload = %+v, want artwork/42/0/%s", decoded, test.variant)
			}
		})
	}
}
