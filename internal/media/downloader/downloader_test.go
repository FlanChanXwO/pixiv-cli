package downloader_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"

	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"

	"slices"
	"strings"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"

	sharedugoira "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
)

func TestDownloadSingleArtworkNormalizesFilename(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "encoded unsafe extension", rawURL: "https://i.example/42.jp%2Ag%3A%7C", want: "42.jp_g__"},
		{name: "nul", rawURL: "https://i.example/42.jp%00", want: "42.jp_"},
		{name: "newline", rawURL: "https://i.example/42.jp%0A", want: "42.jp_"},
		{name: "trailing space", rawURL: "https://i.example/42.jpg%20", want: "42.jpg"},
		{name: "trailing dot", rawURL: "https://i.example/42.", want: "42"},
		{name: "without extension", rawURL: "https://i.example/42", want: "42"},
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
				downloads: map[string][]byte{test.rawURL: []byte("image")},
			}
			m := downloader.NewManager(client, dir, "{id}")

			got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
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
	m := downloader.NewManager(client, dir, "{id}")

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
	m := downloader.NewManager(client, dir, "../escape/{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
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
	m := downloader.NewManager(client, dir, "{id}")

	_, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}})
	if err == nil {
		t.Fatal("Download returned nil error")
	}
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
	m := downloader.NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
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
	m := downloader.NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
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
	m := downloader.NewManager(nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)
	outPath := filepath.Join(dir, "out.gif")
	err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}}, dir, outPath)
	if err != nil {
		t.Fatalf("ConvertUgoira returned error: %v", err)
	}
	if encoder.input.ZipPath != zipPath || encoder.input.WorkDir != dir || encoder.input.Format != sharedugoira.FormatGIF || encoder.input.MaxEdge != 0 {
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
	m := downloader.NewManager(nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}}, dir, outPath)
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
	m := downloader.NewManager(nil, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	if err := m.ConvertUgoira(context.Background(), zipPath, []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}}, dir, outPath); err != nil {
		t.Fatalf("ConvertUgoira returned error: %v", err)
	}
	assertFileBody(t, outPath, "new-gif")
}

func TestDownloadUgoiraZipFailureCleansTemporaryZip(t *testing.T) {
	dir := t.TempDir()
	zipURL := "https://i.example/ugoira.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {
				ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira,
				User: pixiv.User{Name: "author"},
			},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: ugoiraMetadata(zipURL),
		},
		downloadErr: errors.New("zip download failed"),
	}
	m := downloader.NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(&recordingUgoiraEncoder{output: []byte("gif")})

	_, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}})
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
	zipURL := "https://i.example/ugoira.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {
				ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira,
				User: pixiv.User{Name: "author"},
			},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: ugoiraMetadata(zipURL),
		},
		downloads: map[string][]byte{zipURL: makeZip(t, "000000.jpg", []byte("frame"))},
	}
	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m := downloader.NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}})
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

func TestDownloadUgoiraUsesOriginalArchive(t *testing.T) {
	dir := t.TempDir()
	originalURL := "https://i.example/original.zip"
	mediumURL := "https://i.example/medium.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira, User: pixiv.User{Name: "author"}},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: {
				Frames: []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}},
				Archives: []pixiv.UgoiraArchive{
					{Quality: pixiv.UgoiraQualityMedium, Resource: testResource(mediumURL)},
					{Quality: pixiv.UgoiraQualityOriginal, Resource: testResource(originalURL)},
				},
			},
		},
		downloads: map[string][]byte{originalURL: makeZip(t, "000000.jpg", []byte("frame"))},
	}
	m := downloader.NewManager(client, dir, "{id}")
	m.SetUgoiraEncoder(&recordingUgoiraEncoder{output: []byte("gif")})

	if _, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}}); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if len(client.savedURLs) != 1 || client.savedURLs[0] != originalURL {
		t.Fatalf("saved URLs = %v", client.savedURLs)
	}
}

func TestDownloadUgoiraExplicitAPNGUsesRustEncoder(t *testing.T) {
	dir := t.TempDir()
	zipURL := "https://i.example/ugoira.zip"
	m := downloader.NewManager(&fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {ID: 1, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira},
		},
		ugoira:    map[int64]pixiv.UgoiraMetadata{1: ugoiraMetadata(zipURL)},
		downloads: map[string][]byte{zipURL: makeZip(t, "000000.jpg", []byte("frame"))},
	}, dir, "{id}")
	encoder := &recordingUgoiraEncoder{output: []byte("gif")}
	m.SetUgoiraEncoder(encoder)

	got, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs:    []int64{1},
		UgoiraFormat: downloader.UgoiraFormatAPNG,
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 || filepath.Ext(got[0].Files[0].Path) != ".apng" {
		t.Fatalf("Download returned unexpected files: %+v", got)
	}
	if encoder.input.ZipPath == "" || encoder.input.Format != sharedugoira.FormatAPNG {
		t.Fatalf("encoder input = %+v", encoder.input)
	}
}

func ugoiraMetadata(zipURL string) pixiv.UgoiraMetadata {
	return pixiv.UgoiraMetadata{
		Archives: []pixiv.UgoiraArchive{{Quality: pixiv.UgoiraQualityOriginal, Resource: testResource(zipURL)}},
		Frames:   []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}},
	}
}

type recordingUgoiraEncoder struct {
	input  sharedugoira.Input
	output []byte
	err    error
}

func (e *recordingUgoiraEncoder) Encode(_ context.Context, input sharedugoira.Input) error {
	e.input = input
	if e.err != nil {
		return e.err
	}
	return os.WriteFile(input.OutputPath, e.output, 0o644)
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

// testResource 构造一个 Ref 编码了 URL 的资源，使 fake SaveResource 能还原 URL。
func testResource(rawURL string) sdk.Resource {
	ref, err := sdk.NewResourceRef("test", []byte(rawURL))
	if err != nil {
		panic(err)
	}
	return sdk.Resource{URL: rawURL, Ref: ref}
}

func artworkPage(rawURL string, index int) pixiv.ArtworkPage {
	return pixiv.ArtworkPage{PageIndex: index, Image: pixiv.ImageResource{Resource: testResource(rawURL)}}
}

type fakePixivClient struct {
	details              map[int64]pixiv.Artwork
	ugoira               map[int64]pixiv.UgoiraMetadata
	downloads            map[string][]byte
	downloadErr          error
	savedURLs            []string
	destinations         []string
	saveResourceOverride func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *fakePixivClient) Artwork(_ context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	artwork, ok := c.details[request.ArtworkID]
	if !ok {
		return pixiv.Artwork{}, os.ErrNotExist
	}
	return artwork, nil
}

func (c *fakePixivClient) UgoiraMetadata(_ context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	meta, ok := c.ugoira[request.ArtworkID]
	if !ok {
		return pixiv.UgoiraMetadata{}, os.ErrNotExist
	}
	return meta, nil
}

func (c *fakePixivClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (c *fakePixivClient) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResourceOverride != nil {
		return c.saveResourceOverride(ctx, ref, options)
	}
	if c.downloadErr != nil {
		return sdk.SavedResource{}, c.downloadErr
	}
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		return sdk.SavedResource{}, err
	}
	rawURL := string(payload)
	body, ok := c.downloads[rawURL]
	if !ok {
		return sdk.SavedResource{}, os.ErrNotExist
	}
	c.savedURLs = append(c.savedURLs, rawURL)
	c.destinations = append(c.destinations, options.Path)
	if err := os.WriteFile(options.Path, body, 0o644); err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, nil
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
	m := downloader.NewManager(client, dir, "{id}")
	got, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{1, 3},
		Quality:   downloader.DownloadQualityOriginal,
	})
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 2 {
		t.Fatalf("files=%+v", got)
	}
	if got[0].Files[0].Page != 1 || got[0].Files[1].Page != 3 {
		t.Fatalf("pages=%+v", got[0].Files)
	}
	if filepath.Base(got[0].Files[0].Path) != "7_p0.png" || filepath.Base(got[0].Files[1].Path) != "7_p2.png" {
		t.Fatalf("paths=%q %q", got[0].Files[0].Path, got[0].Files[1].Path)
	}

	_, err = m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{4},
		Quality:   downloader.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page 4 does not exist") {
		t.Fatalf("missing page error=%v", err)
	}
}

func TestDownloadUgoiraRejectsQualityAndPages(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {ID: 9, Title: "u", Kind: pixiv.ArtworkKindUgoira, PageCount: 1, User: pixiv.User{Name: "a"}},
		},
	}
	m := downloader.NewManager(client, dir, "{id}")
	_, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{9},
		Quality:   downloader.DownloadQualityRegular,
	})
	if err == nil || !strings.Contains(err.Error(), "ugoira quality") {
		t.Fatalf("quality error=%v", err)
	}
	_, err = m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{9},
		Pages:     []int{1},
		Quality:   downloader.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page selection is unsupported") {
		t.Fatalf("pages error=%v", err)
	}
}

func TestParsePageSpec(t *testing.T) {
	for _, test := range []struct {
		name      string
		spec      string
		wantPages []int
		wantNil   bool
		wantError bool
	}{
		{name: "ranges dedup and sort", spec: "3,1,2-4,1", wantPages: []int{1, 2, 3, 4}},
		{name: "empty means all", spec: "  ", wantNil: true},
		{name: "zero", spec: "0", wantError: true},
		{name: "open range", spec: "1-", wantError: true},
		{name: "negative range", spec: "-2", wantError: true},
		{name: "non integer", spec: "a", wantError: true},
		{name: "reversed range", spec: "2-1", wantError: true},
		{name: "empty item", spec: "1,,2", wantError: true},
		{name: "multiple ranges", spec: "1-2-3", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pages, err := downloader.ParsePageSpec(test.spec)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if test.wantNil {
				require.Nil(t, pages)
				return
			}
			require.Equal(t, test.wantPages, pages)
		})
	}
}

func TestValidateDownloadQuality(t *testing.T) {
	for _, q := range []downloader.DownloadQuality{
		downloader.DownloadQualityOriginal,
		downloader.DownloadQualityRegular,
		downloader.DownloadQualitySmall,
		downloader.DownloadQualityThumb,
		downloader.DownloadQualityMini,
	} {
		require.NoError(t, downloader.ValidateDownloadQuality(q))
	}
	require.Error(t, downloader.ValidateDownloadQuality("huge"))
}

func TestDownloadServiceDelegatesOperationClientAndRequest(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "same-context")
	client := &downloadClientStub{}
	want := []downloader.DownloadedArtwork{{
		IllustID: 42,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: "/tmp/downloads/42.jpg", Page: 3}},
	}}
	manager := &downloadManagerStub{download: func(gotContext context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
		require.Same(t, ctx, gotContext)
		require.Equal(t, []int64{42, 84}, request.IllustIDs)
		return want, nil
	}}
	service := downloader.DownloadService{NewManager: func(gotClient downloader.DownloadClient, gotPath, gotTemplate string) (downloader.DownloadManager, error) {
		require.Same(t, client, gotClient)
		require.Equal(t, "/tmp/downloads", gotPath)
		require.Equal(t, "{id}-{title}", gotTemplate)
		return manager, nil
	}}

	got, err := service.Download(ctx, client, downloader.DownloadRequest{
		IllustIDs:        []int64{42, 84},
		DownloadPath:     "/tmp/downloads",
		FilenameTemplate: "{id}-{title}",
	})

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDownloadSourcesDeduplicatesCanonicalArtwork(t *testing.T) {
	client := &downloadSourcesStub{}
	var downloaded []int64
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return []downloader.DownloadedArtwork{{IllustID: 1}}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/artworks/1", "2"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, downloaded)
	require.Len(t, report.Failures, 0)
}

func TestDownloadSourcesExpandsUserArtworksAndDeduplicates(t *testing.T) {
	client := &downloadSourcesStub{}
	client.userArtworks = func(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
			{ID: 1, Kind: pixiv.ArtworkKindIllustration},
			{ID: 2, Kind: pixiv.ArtworkKindIllustration},
		}}, nil
	}
	var downloaded []int64
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return []downloader.DownloadedArtwork{{IllustID: 1}}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/users/7/artworks"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, downloaded)
	require.Len(t, report.Failures, 0)
}

func TestDownloadSourcesRedactsRejectedSource(t *testing.T) {
	source := "https://signed.example/private?signature=secret"
	client := &downloadSourcesStub{}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{source}, downloader.DownloadRequest{})

	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
	require.Equal(t, "[redacted source]", report.Failures[0].URL)
	require.NotContains(t, report.Failures[0].URL, source)
}

func TestDownloadServiceStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(ctx context.Context, _ downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			calls++
			return nil, ctx.Err()
		}}, nil
	}}

	report, err := service.DownloadSources(ctx, &downloadSourcesStub{}, []string{"1"}, downloader.DownloadRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
	require.Empty(t, report.Items)
	require.Empty(t, report.Failures)
}

func TestDownloadServiceRejectsMissingDependencies(t *testing.T) {
	var typedNilClient *typedNilDownloadClient
	var typedNilManager *typedNilDownloadManager
	for _, test := range []struct {
		name             string
		client           downloader.DownloadClient
		factoryMode      string
		wantError        string
		wantFactoryCalls int
	}{
		{
			name:        "missing factory",
			client:      &downloadClientStub{},
			factoryMode: "missing",
			wantError:   "download manager factory is not configured",
		},
		{
			name:             "missing operation client",
			factoryMode:      "valid",
			wantError:        "download operation client is not configured",
			wantFactoryCalls: 0,
		},
		{
			name:             "typed nil operation client",
			client:           typedNilClient,
			factoryMode:      "valid",
			wantError:        "download operation client is not configured",
			wantFactoryCalls: 0,
		},
		{
			name:             "missing manager",
			client:           &downloadClientStub{},
			factoryMode:      "missing-manager",
			wantError:        "download manager factory returned nil",
			wantFactoryCalls: 1,
		},
		{
			name:             "typed nil manager",
			client:           &downloadClientStub{},
			factoryMode:      "typed nil-manager",
			wantError:        "download manager factory returned nil",
			wantFactoryCalls: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			factoryCalls := 0
			service := downloader.DownloadService{}
			if test.factoryMode != "missing" {
				service.NewManager = func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
					factoryCalls++
					switch test.factoryMode {
					case "missing-manager":
						return nil, nil
					case "typed nil-manager":
						return typedNilManager, nil
					default:
						return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
							return nil, nil
						}}, nil
					}
				}
			}

			_, err := service.Download(context.Background(), test.client, downloader.DownloadRequest{})
			require.EqualError(t, err, test.wantError)
			require.Equal(t, test.wantFactoryCalls, factoryCalls)
		})
	}
}

func TestDownloadServicePropagatesManagerFailure(t *testing.T) {
	want := errors.New("download failed")
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	}}

	_, err := service.Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{IllustIDs: []int64{42}})
	require.ErrorIs(t, err, want)
}

type downloadManagerStub struct {
	download func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error)
}

func (m *downloadManagerStub) Download(ctx context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	return m.download(ctx, request)
}

type typedNilDownloadManager struct{}

func (*typedNilDownloadManager) Download(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	panic("typed-nil download manager must not be called")
}

type downloadClientStub struct {
	artwork          func(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	ugoiraMetadata   func(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)
	parseResourceRef func(string) (sdk.ResourceRef, error)
	saveResource     func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *downloadClientStub) Artwork(ctx context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	if c.artwork != nil {
		return c.artwork(ctx, request)
	}
	return pixiv.Artwork{}, nil
}

func (c *downloadClientStub) UgoiraMetadata(ctx context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	if c.ugoiraMetadata != nil {
		return c.ugoiraMetadata(ctx, request)
	}
	return pixiv.UgoiraMetadata{}, nil
}

func (c *downloadClientStub) ParseResourceRef(value string) (sdk.ResourceRef, error) {
	if c.parseResourceRef != nil {
		return c.parseResourceRef(value)
	}
	return sdk.ResourceRef{}, nil
}

func (c *downloadClientStub) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResource != nil {
		return c.saveResource(ctx, ref, options)
	}
	return sdk.SavedResource{}, nil
}

type typedNilDownloadClient struct{}

func (*typedNilDownloadClient) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error) {
	panic("typed-nil download client must not be called")
}

// downloadSourcesStub 实现下载用例需要的窄 port，只覆写 DownloadSources
// 触达的方法。
type downloadSourcesStub struct {
	userArtworks         func(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	userArtworkBookmarks func(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	saveResource         func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *downloadSourcesStub) UserArtworks(ctx context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworks != nil {
		return c.userArtworks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworkBookmarks != nil {
		return c.userArtworkBookmarks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResource != nil {
		return c.saveResource(ctx, ref, options)
	}
	return sdk.SavedResource{}, nil
}

func (c *downloadSourcesStub) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	panic("downloadSourcesStub.Artwork must not be called by source expansion")
}

func (c *downloadSourcesStub) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	panic("downloadSourcesStub.UgoiraMetadata must not be called by source expansion")
}

var _ downloader.DownloadTargetClient = (*downloadSourcesStub)(nil)

// TestDownloadPreservesPublishedArtworkOnPartialFailure 验证 finding #7：当
// 批量下载中部分作品失败、其余成功时，已原子发布的成功作品必须随返回值返回，
// 错误不丢弃它们；只有当全部失败时才回退为单一错误。
func TestDownloadPreservesPublishedArtworkOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {
				ID: 1, Title: "ok1", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/1.jpg", 0)},
			},
			2: {
				ID: 2, Title: "ok2", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/2.jpg", 0)},
			},
			3: {
				ID: 3, Title: "fails", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/3.jpg", 0)},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/1.jpg": []byte("p1"),
			"https://i.example/2.jpg": []byte("p2"),
			"https://i.example/3.jpg": []byte("p3"),
		},
	}
	// 只让第 3 个作品在 SaveResource 时失败，其余按 ref payload 查表写盘。
	// 模板是 {id}，所以失败作品的文件名以 3.jpg 结尾。
	var saveMu sync.Mutex
	client.saveResourceOverride = func(_ context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		saveMu.Lock()
		defer saveMu.Unlock()
		if filepath.Base(options.Path) == "3.jpg" {
			return sdk.SavedResource{}, errors.New("network broke for 3")
		}
		payload, err := sdk.ResourceRefPayload(ref)
		if err != nil {
			return sdk.SavedResource{}, err
		}
		body, ok := client.downloads[string(payload)]
		if !ok {
			return sdk.SavedResource{}, os.ErrNotExist
		}
		client.savedURLs = append(client.savedURLs, string(payload))
		client.destinations = append(client.destinations, options.Path)
		if err := os.WriteFile(options.Path, body, 0o644); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/jpeg"}, nil
	}
	m := downloader.NewManager(client, dir, "{id}")

	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{1, 2, 3}})
	if err != nil {
		t.Fatalf("Download returned error on partial success: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Download returned %d artworks, want 2 published", len(got))
	}
	// 两个成功作品都已落盘。
	published := map[string]bool{}
	for _, artwork := range got {
		for _, file := range artwork.Files {
			body, rerr := os.ReadFile(file.Path)
			if rerr != nil {
				t.Fatalf("ReadFile(%q): %v", file.Path, rerr)
			}
			published[string(body)] = true
		}
	}
	if !published["p1"] || !published["p2"] {
		t.Fatalf("published bodies = %v, want p1 and p2", published)
	}
}

// TestDownloadReturnsErrorWhenAllArtworksFail 验证全部失败时仍返回单一错误，
// 便于账号池与 shell 判断。
func TestDownloadReturnsErrorWhenAllArtworksFail(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {
				ID: 1, Title: "fails", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/1.jpg", 0)},
			},
		},
		downloadErr: errors.New("network broke"),
	}
	m := downloader.NewManager(client, dir, "{id}")

	if _, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{1}}); err == nil {
		t.Fatal("Download returned nil error when all artworks failed")
	}
}

// TestDownloadRejectsUnboundedPageRange 验证 finding #14：ParsePageSpec 拒绝
// 会展开为无界页数的范围。
func TestDownloadRejectsUnboundedPageRange(t *testing.T) {
	big := "1-100002"
	if _, err := downloader.ParsePageSpec(big); err == nil {
		t.Fatalf("ParsePageSpec(%q) should reject unbounded range", big)
	}
	// 合理范围仍被接受。
	pages, err := downloader.ParsePageSpec("1-3")
	if err != nil {
		t.Fatalf("ParsePageSpec(1-3): %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("ParsePageSpec(1-3) = %v, want 3 pages", pages)
	}
}

// TestDownloadRejectsInvalidFilenameTemplate 验证 finding #9：无效或缺少必要
// 字段的模板在下载前就被拒绝，而不是写出空文件名互相覆盖。
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
			m := downloader.NewManager(client, dir, tmpl)
			if _, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{42}}); err == nil {
				t.Fatalf("template %q should be rejected before download", tmpl)
			}
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

// TestDownloadPopulatesDocumentedFilenamePlaceholders 验证 finding #11：文档
// 承诺的占位符（id、title、author、author_id、date、tags、num）都被填充。
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
	m := downloader.NewManager(client, dir, "{id}-{title}-{author}-{author_id}-{date}-{tags}-{num}")
	got, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{7}})
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 {
		t.Fatalf("files=%+v", got)
	}
	base := filepath.Base(got[0].Files[0].Path)
	for _, want := range []string{"7-", "Title-", "Author-", "77-", "tag1", "tag2"} {
		if !strings.Contains(base, want) {
			t.Fatalf("filename %q missing %q", base, want)
		}
	}
}

// TestDirectResourceUsesCollisionResistantNames 验证 finding #18：两个共享
// 前缀的 resource ref 必须落盘到不同文件，而不是互相覆盖。directResourcePath
// 现在对完整 ref 做 sha256 摘要。
func TestDirectResourceUsesCollisionResistantNames(t *testing.T) {
	dir := t.TempDir()
	// 两个 ref 仅末尾页号不同，共享长前缀；截断式文件名会让它们冲突。
	refA, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		t.Fatal(err)
	}
	refB, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if refA.String() == refB.String() {
		t.Fatal("test fixtures must produce distinct refs")
	}
	var seen []string
	client := &downloadSourcesStub{
		saveResource: func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
			seen = append(seen, options.Path)
			if err := os.WriteFile(options.Path, []byte("data"), 0o644); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: 4}, nil
		},
	}
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{refA.String(), refB.String()}, downloader.DownloadRequest{DownloadPath: dir})
	require.NoError(t, err)
	require.Len(t, report.Failures, 0)
	require.Len(t, report.Items, 2)
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("expected 2 distinct resource paths, got %v", seen)
	}
	// 两个 ref 必须都落盘（第二个不会覆盖第一个）。
	for _, path := range seen {
		assertFileBody(t, path, "data")
	}
}

func TestThumbnailDownloadPublishesDetectedImageExtension(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/42.png"
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{42: {
			ID: 42, Title: "thumb", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
			User:  pixiv.User{Name: "author"},
			Pages: []pixiv.ArtworkPage{{PageIndex: 0, Image: pixiv.ImageResource{Resource: sdk.Resource{URL: rawURL, Ref: ref}}}},
		}},
	}
	client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		body := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
		if err := os.WriteFile(options.Path, body, 0o600); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, nil
	}
	got, err := downloader.NewManager(client, dir, "{id}").Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{42}, Quality: downloader.DownloadQualityThumb,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 1 || filepath.Ext(got[0].Files[0].Path) != ".jpg" {
		t.Fatalf("downloaded = %#v", got)
	}
}
