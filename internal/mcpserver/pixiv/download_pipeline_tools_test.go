package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
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

func TestDownloadSchemaPublishesOnlyMappedOptions(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	var rawSchema any
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "download" {
			rawSchema = tool.InputSchema
			break
		}
	}
	if rawSchema == nil {
		t.Fatal("download tool not found")
	}
	raw, err := json.Marshal(rawSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"src", "srcs", "pages", "quality", "ugoira_mode", "delivery"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("download schema missing mapped field %q: %s", field, raw)
		}
	}
	for _, field := range []string{"concurrency", "filter", "archive", "directory_template", "write_metadata", "retries", "retry_delay"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("download schema publishes unmapped field %q: %s", field, raw)
		}
	}
}

func newSDKDownloadTestSession(t *testing.T, sdkClient *fakeSDKClient, downloads pixivmcpserver.DownloadManager) (*mcp.ClientSession, func()) {
	t.Helper()
	ports, _ := newTestSDKPorts(t, sdkClient)
	server := pixivmcpserver.NewWithSDKDownloadFactory(downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager { return downloads }, ports, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect: %v", err)
	}
	return session, func() {
		_ = session.Close()
		cancel()
	}
}

// downloadOut 是 download tool 的本地测试镜像，与生产 download 包的输出契约保持
// 相同 JSON 字段；外部测试包不能直接使用未导出的 download.downloadOut。
type downloadOut struct {
	Delivery string               `json:"delivery"`
	Items    []downloadItemOut    `json:"items"`
	Failures []downloadFailureOut `json:"failures"`
	Files    []downloadFileOut    `json:"files"`
	Text     string               `json:"text"`
}

type downloadItemOut struct {
	URL      string            `json:"url"`
	IllustID int64             `json:"illust_id"`
	Title    string            `json:"title"`
	Author   string            `json:"author"`
	Type     string            `json:"type"`
	Files    []downloadFileOut `json:"files"`
}

type downloadFailureOut struct {
	URL      string `json:"url"`
	IllustID int64  `json:"illust_id"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

type downloadFileOut struct {
	IllustID  int64  `json:"illust_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Path      string `json:"path"`
	FileURI   string `json:"file_uri"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Page      int    `json:"page,omitempty"`
}

func decodeDownloadOut(t *testing.T, result *mcp.CallToolResult) downloadOut {
	t.Helper()
	var out downloadOut
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal StructuredContent: %v", err)
	}
	return out
}
