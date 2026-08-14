package pixiv_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// download tool 的 owner 契约：输入校验、投递方式、manager 错误形状与 URL 展开。
func TestDownloadWithoutIDsPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "download",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Error: provide src (one source) or srcs (a source list)"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadInvalidDeliveryPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "invalid-delivery",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = `Error: delivery supports only "local_path".`
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadManagerErrorPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{err: errors.New("download sentinel")}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	out := decodeDownloadOut(t, result)
	if !result.IsError || len(out.Failures) != 1 || out.Failures[0].Message != "download sentinel" {
		t.Fatalf("result=%+v output=%+v", result, out)
	}
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadBuildErrorPreservesBusinessErrorShape(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	_, statErr := os.Stat(missing)
	if statErr == nil {
		t.Fatal("missing test file unexpectedly exists")
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42,
		Files:    []downloader.DownloadedFile{{Path: missing}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadRejectsImageContentDelivery(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "image_content",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertEmptyDownloadResult(t, result, "local_path", `Error: delivery supports only "local_path".`)
	if downloads.downloadCalls != 0 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
}

func TestDownloadPassesPagesAndQualityToManager(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "9.png")
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 9,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: path, Page: 1}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":     "9",
			"pages":   "1,3-4",
			"quality": "small",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("download failed: %+v", result)
	}
	if downloads.downloadCalls != 1 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
	got := downloads.lastRequest
	if len(got.IllustIDs) != 1 || got.IllustIDs[0] != 9 {
		t.Fatalf("ids=%v", got.IllustIDs)
	}
	if got.Quality != downloader.DownloadQualitySmall {
		t.Fatalf("quality=%q", got.Quality)
	}
	if len(got.Pages) != 3 || got.Pages[0] != 1 || got.Pages[1] != 3 || got.Pages[2] != 4 {
		t.Fatalf("pages=%v", got.Pages)
	}
	out := decodeDownloadOut(t, result)
	if out.Delivery != "local_path" || len(out.Files) != 1 || out.Files[0].Path == "" || out.Files[0].FileURI == "" || out.Files[0].MIMEType == "" {
		t.Fatalf("local delivery output=%+v", out)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks=%d want text only", len(result.Content))
	}
}

func TestDownloadRejectsInvalidPagesAndQualityBeforeManager(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()
	for _, args := range []map[string]any{
		{"src": "1", "pages": "0"},
		{"src": "1", "quality": "huge"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "download", Arguments: args})
		if err != nil {
			t.Fatalf("call tool: %v", err)
		}
		out := decodeDownloadOut(t, result)
		if out.Delivery != "local_path" || len(out.Files) != 0 || !strings.HasPrefix(out.Text, "Error: ") {
			t.Fatalf("args=%v output=%+v", args, out)
		}
	}
	if downloads.downloadCalls != 0 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
}

func TestDownloadDefaultsToLocalPathResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42,
		Title:    "title",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: path}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "download",
		Arguments: map[string]any{"srcs": []string{"42", "42"}},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len = %d, want text only", len(result.Content))
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || !strings.Contains(text.Text, path) {
		t.Fatalf("unexpected text content: %#v", result.Content[0])
	}
	out := decodeDownloadOut(t, result)
	if out.Delivery != "local_path" || len(out.Files) != 1 {
		t.Fatalf("unexpected structured output: %+v", out)
	}
	if out.Files[0].MIMEType != "image/jpeg" || out.Files[0].SizeBytes != 4 || !strings.HasPrefix(out.Files[0].FileURI, "file://") {
		t.Fatalf("unexpected file output: %+v", out.Files[0])
	}
	if !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download IDs = %v", downloads.downloadIDs)
	}
}

func TestDownloadAcceptsArtworkURLsAndIncludesCanonicalURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloader.DownloadedFile{{Path: path, Page: 1}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result := callTool(t, session, "download", map[string]any{"src": "https://www.pixiv.net/en/artworks/42?from=share"})
	if result.IsError {
		t.Fatalf("download result=%+v", result)
	}
	out := decodeDownloadOut(t, result)
	if len(out.Items) != 1 || out.Items[0].URL != "https://www.pixiv.net/artworks/42" || downloads.downloadIDs[0] != 42 {
		t.Fatalf("download output=%+v ids=%v", out, downloads.downloadIDs)
	}
}

func TestDownloadUserURLExpandsEveryVisualArtworkType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloader.DownloadedFile{{Path: path}},
	}}}
	client := &fakeSDKClient{artworks: []pixiv.Artwork{testSDKIllust(42, "work", 7)}}
	session, closeSession := newSDKDownloadTestSession(t, client, downloads)
	defer closeSession()

	result := callTool(t, session, "download", map[string]any{"src": "https://www.pixiv.net/users/7/artworks"})
	if result.IsError {
		t.Fatalf("download result=%+v", result)
	}
	out := decodeDownloadOut(t, result)
	if len(out.Items) != 1 || downloads.downloadCalls != 1 {
		t.Fatalf("download output=%+v calls=%d", out, downloads.downloadCalls)
	}
	gotTypes := make([]pixiv.ArtworkKind, 0, len(client.artworksRequests))
	for _, request := range client.artworksRequests {
		gotTypes = append(gotTypes, request.Kind)
	}
	if !slices.Equal(gotTypes, []pixiv.ArtworkKind{pixiv.ArtworkKindIllustration, pixiv.ArtworkKindManga, pixiv.ArtworkKindUgoira}) {
		t.Fatalf("UserArtworks types = %v", gotTypes)
	}
}
