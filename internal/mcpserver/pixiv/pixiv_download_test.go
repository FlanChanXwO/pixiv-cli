package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/download"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestPixivMCPDiagnosticsUseStableLocalRequestIDs(t *testing.T) {
	var (
		mu     sync.Mutex
		events []diagnostics.Event
	)
	sink := diagnostics.SinkFunc(func(event diagnostics.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	rootCtx, cancel := context.WithCancel(diagnostics.WithScope(context.Background(), sink, diagnostics.ModulePixivCLI, 0))
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "debug-test", Version: "1"}, nil)
	runtime.AddTool(runtime.NewApp(nil, nil, runtime.SDKPorts{}, runtime.Account{}), server, &mcp.Tool{Name: "diagnostic_test"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{}, struct{}{}, nil
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(rootCtx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "debug-client", Version: "1"}, nil)
	session, err := client.Connect(rootCtx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	callTool(t, session, "diagnostic_test", map[string]any{})
	callTool(t, session, "diagnostic_test", map[string]any{})

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 {
		t.Fatalf("events=%+v want two start/complete pairs", events)
	}
	for index, event := range events {
		wantID := uint64(index/2 + 1)
		if event.Module != diagnostics.ModulePixivMCPServer || event.RequestID != wantID {
			t.Fatalf("event[%d]=%+v", index, event)
		}
	}
}

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

func TestDownloadRejectsUnsupportedDelivery(t *testing.T) {
	for _, delivery := range []string{"invalid-delivery", "image_content"} {
		t.Run(delivery, func(t *testing.T) {
			downloads := &fakeDownloads{}
			session, closeSession := newTestSession(t, downloads)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "download",
				Arguments: map[string]any{
					"src":      "42",
					"delivery": delivery,
				},
			})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			assertEmptyDownloadResult(t, result, "local_path", `Error: delivery supports only "local_path".`)
			if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
				t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
			}
		})
	}
}

func TestDownloadBatchFailuresPreserveBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{result: downloader.DownloadBatchResult{
		Failures: []downloader.DownloadFailure{{
			IllustID: 42,
			Message:  "download sentinel",
			Cause:    errors.New("download sentinel"),
		}},
	}}
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

func TestDownloadManagerOperationErrorDoesNotBecomeBusinessFailure(t *testing.T) {
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
	if !result.IsError || len(out.Failures) != 0 || !strings.Contains(out.Text, "Download failed: download sentinel") {
		t.Fatalf("result=%+v output=%+v", result, out)
	}
}

func TestDownloadManagerOperationErrorPreservesStructuredReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "42.png")
	if err := os.WriteFile(path, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	operationErr := errors.New("download sentinel")
	businessErr := errors.New("page one failed")
	downloads := &fakeDownloads{
		result: downloader.DownloadBatchResult{
			Items: []downloader.DownloadedArtwork{{
				IllustID: 42,
				Title:    "partial",
				Author:   "artist",
				Type:     "illust",
				Files:    []downloader.DownloadedFile{{Path: path, Page: 1}},
			}},
			Failures: []downloader.DownloadFailure{{
				IllustID: 42,
				Message:  businessErr.Error(),
				Cause:    businessErr,
			}},
		},
		err: operationErr,
	}
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
	if !result.IsError || len(out.Items) != 1 || len(out.Items[0].Files) != 1 || len(out.Files) != 1 || len(out.Failures) != 1 {
		t.Fatalf("result=%+v output=%+v", result, out)
	}
	if out.Failures[0].Message != businessErr.Error() || !strings.Contains(out.Text, businessErr.Error()) || !strings.Contains(out.Text, "Download failed: "+operationErr.Error()) {
		t.Fatalf("output=%+v", out)
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

// download_random_from_recommendation tool 的 owner 契约：count 语义、默认值、上限与错误形状。
func TestDownloadRandomFromRecommendationUsesSDKAndPreservesCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommended.jpg")
	if err := os.WriteFile(path, []byte("recommended"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{IllustID: 77, Files: []downloader.DownloadedFile{{Path: path}}}}}
	var requests []pixiv.RecommendedArtworksRequest
	sdkClient := &fakeSDKClient{recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(77, "recommended", 1)}}, nil
	}}
	server := pixivmcpserver.NewWithSDK(&fakeAPI{}, downloads, testSDKPorts(t, sdkClient), pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 1})
	if result.IsError || len(result.Content) != 1 || len(requests) != 1 || requests[0] != (pixiv.RecommendedArtworksRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestDownloadParsers(t *testing.T) {
	tests := []struct {
		name        string
		parse       func(string, string, string) ([]int, downloader.DownloadQuality, downloader.UgoiraFormat, error)
		pages       string
		quality     string
		ugoiraMode  string
		wantPages   []int
		wantQuality downloader.DownloadQuality
		wantFormat  downloader.UgoiraFormat
		wantError   bool
	}{
		{
			name:        "selection maps explicit mode",
			parse:       download.ParseDownloadSelection,
			pages:       "1,3",
			quality:     "regular",
			ugoiraMode:  "apng",
			wantPages:   []int{1, 3},
			wantQuality: downloader.DownloadQualityRegular,
			wantFormat:  downloader.UgoiraFormatAPNG,
		},
		{
			name:        "selection defaults to gif",
			parse:       download.ParseDownloadSelection,
			wantQuality: downloader.DownloadQualityOriginal,
			wantFormat:  downloader.UgoiraFormatGIF,
		},
		{
			name:        "options expands closed range",
			parse:       download.ParseDownloadOptions,
			pages:       "1,3-5",
			quality:     "regular",
			ugoiraMode:  "apng",
			wantPages:   []int{1, 3, 4, 5},
			wantQuality: downloader.DownloadQualityRegular,
			wantFormat:  downloader.UgoiraFormatAPNG,
		},
		{
			name:       "options rejects open range",
			parse:      download.ParseDownloadOptions,
			pages:      "3-",
			quality:    "regular",
			ugoiraMode: "gif",
			wantError:  true,
		},
		{
			name:       "options rejects unknown mode",
			parse:      download.ParseDownloadOptions,
			pages:      "1",
			quality:    "regular",
			ugoiraMode: "frames",
			wantError:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pages, quality, format, err := test.parse(test.pages, test.quality, test.ugoiraMode)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantPages, pages)
			require.Equal(t, test.wantQuality, quality)
			require.Equal(t, test.wantFormat, format)
		})
	}
}
