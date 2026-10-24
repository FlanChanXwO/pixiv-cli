package pixiv_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testSDKIllust(id int64, title string, userID int64) pixiv.Artwork {
	return pixiv.Artwork{
		ID:          id,
		Title:       title,
		Kind:        pixiv.ArtworkKindIllustration,
		PublishedAt: time.Date(2024, 5, 1, 1, 0, 0, 0, time.UTC),
		User:        pixiv.User{ID: userID, Name: "artist"},
		Tags:        []pixiv.Tag{},
	}
}

// testPageCursor 构造一个可复现的 opaque cursor，用于模拟上游下一页。
func testPageCursor(seed byte) sdk.Cursor {
	cursor, err := sdk.NewCursor("pixiv", "test", 1, "q", []byte{seed})
	if err != nil {
		panic(err)
	}
	return cursor
}

func newTestServer(ports pixivmcpserver.SDKPorts, account pixivmcpserver.Account) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	pixivmcpserver.Register(server, ports, account)
	return server
}

func newTestSession(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()
	return newSDKTestSessionWithPorts(t, httpFixturePorts(t), pixivmcpserver.Account{})
}

func newSDKTestSession(t *testing.T, sdkClient *fakeSDKClient) (*mcp.ClientSession, func()) {
	t.Helper()
	ports, _ := newTestSDKPorts(t, sdkClient)
	return newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
}

func newSDKTestSessionWithPorts(t *testing.T, ports pixivmcpserver.SDKPorts, account pixivmcpserver.Account) (*mcp.ClientSession, func()) {
	t.Helper()
	server := newTestServer(ports, account)
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

func callTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return result
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}

func resultHasText(result *mcp.CallToolResult, wanted string) bool {
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if ok && strings.Contains(text.Text, wanted) {
			return true
		}
	}
	return false
}

// fakeSDKClient 是 MCP 测试的 typed SDK fixture。testSDKTransport 按 App API
// path 分发请求，调用这里的 typed func 字段并把结果编码为 wire JSON；同时把
// 解析后的请求写回 capture 字段。
type fakeSDKClient struct {
	mu sync.Mutex

	userID                  int64
	searchIllust            func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	searchNovel             func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	searchUser              func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	illustDetail            func(context.Context, int64) (pixiv.Artwork, error)
	novelDetail             func(context.Context, int64) (pixiv.Novel, error)
	artworks                []pixiv.Artwork
	bookmarks               []pixiv.Artwork
	novelBookmarks          []pixiv.Novel
	following               []pixiv.UserPreview
	followingIllusts        func(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	followingNovels         func(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	latestIllusts           func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	latestNovels            func(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	myPixivUsers            func(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	myPixivIllusts          func(context.Context, pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	myPixivNovels           func(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userNovels              func(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userFollowing           func(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	userFollowers           func(context.Context, pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error)
	userBlockedUsers        func(context.Context, pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedUsers            func(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	recommendedArtworks     func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	illustRanking           func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	novelRecommended        func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userRecommended         func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedArtworks         func(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	artworkSeries           func(context.Context, pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error)
	userBookmarksFunc       func(pixiv.UserArtworkBookmarksRequest, int) (sdk.Page[pixiv.Artwork], error)
	novelSeries             func(context.Context, pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error)
	userDetailResult        pixiv.UserDetail
	userDetailErr           error
	artworkBookmarkDetail   pixiv.ArtworkBookmarkDetail
	bookmarkTags            []pixiv.BookmarkTag
	illustComments          []pixiv.Comment
	novelBookmarksErr       error
	novelBookmarkTagsErr    error
	novelBookmarkDetailErr  error
	trendingTags            []pixiv.TrendingTag
	addBookmarkErr          error
	removeBookmarkErr       error
	addNovelBookmarkErr     error
	removeNovelBookmarkErr  error
	followUserErr           error
	unfollowUserErr         error
	createArtworkCommentErr error
	replyArtworkCommentErr  error
	stampArtworkCommentErr  error
	deleteArtworkCommentErr error
	createNovelCommentErr   error
	replyNovelCommentErr    error
	stampNovelCommentErr    error
	deleteNovelCommentErr   error

	// capture
	searchIllustRequest         pixiv.SearchArtworksRequest
	searchNovelRequest          pixiv.SearchNovelsRequest
	searchUserRequest           pixiv.SearchUsersRequest
	illustDetailRequest         int64
	novelDetailRequest          int64
	relatedArtworksRequest      pixiv.RelatedArtworksRequest
	artworkSeriesRequest        pixiv.ArtworkSeriesRequest
	illustRankingRequest        pixiv.ArtworkRankingRequest
	recommendedArtworksRequest  pixiv.RecommendedArtworksRequest
	followingIllustsRequest     pixiv.FollowingArtworksRequest
	followingNovelsRequest      pixiv.FollowingNovelsRequest
	latestIllustsRequest        pixiv.LatestArtworksRequest
	latestNovelsRequest         pixiv.LatestNovelsRequest
	myPixivUsersRequest         pixiv.MyPixivUsersRequest
	myPixivIllustsRequest       pixiv.MyPixivArtworksRequest
	myPixivNovelsRequest        pixiv.MyPixivNovelsRequest
	novelRecommendedRequest     pixiv.RecommendedNovelsRequest
	userRecommendedRequest      pixiv.RecommendedUsersRequest
	userDetailRequest           pixiv.UserRequest
	artworksRequest             pixiv.UserArtworksRequest
	artworksRequests            []pixiv.UserArtworksRequest
	userArtworksFunc            func(pixiv.UserArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	userArtworksCalls           int
	recommendedArtworksCalls    int
	bookmarksRequest            pixiv.UserArtworkBookmarksRequest
	bookmarksRequests           []pixiv.UserArtworkBookmarksRequest
	bookmarksCalls              int
	userNovelsRequest           pixiv.UserNovelsRequest
	novelBookmarksRequest       pixiv.UserNovelBookmarksRequest
	novelBookmarksRequests      []pixiv.UserNovelBookmarksRequest
	followingRequest            pixiv.UserFollowingRequest
	followersRequest            pixiv.UserFollowersRequest
	blockedUsersRequest         pixiv.UserBlockedUsersRequest
	relatedUsersRequest         pixiv.RelatedUsersRequest
	bookmarkTagsRequest         pixiv.UserArtworkBookmarkTagsRequest
	novelBookmarkTagsRequest    pixiv.UserNovelBookmarkTagsRequest
	artworkBookmarkRequest      pixiv.ArtworkBookmarkRequest
	novelBookmarkRequest        pixiv.NovelBookmarkRequest
	illustCommentsRequest       pixiv.ArtworkCommentsRequest
	addBookmarkRequest          pixiv.AddArtworkBookmarkRequest
	removeBookmarkRequest       pixiv.RemoveArtworkBookmarkRequest
	addNovelBookmarkRequest     pixiv.AddNovelBookmarkRequest
	removeNovelBookmarkRequest  pixiv.RemoveNovelBookmarkRequest
	followUserRequest           pixiv.FollowUserRequest
	unfollowUserRequest         pixiv.UnfollowUserRequest
	createArtworkCommentRequest pixiv.PostArtworkCommentRequest
	replyArtworkCommentRequest  pixiv.ReplyArtworkCommentRequest
	stampArtworkCommentRequest  pixiv.StampArtworkCommentRequest
	deleteArtworkCommentRequest pixiv.DeleteArtworkCommentRequest
	artworkCommentWireCalls     int
	createNovelCommentRequest   pixiv.PostNovelCommentRequest
	replyNovelCommentRequest    pixiv.ReplyNovelCommentRequest
	stampNovelCommentRequest    pixiv.StampNovelCommentRequest
	deleteNovelCommentRequest   pixiv.DeleteNovelCommentRequest
	novelCommentWireCalls       int

	// typed read tools canned results
	artworkSeriesPage         sdk.Page[pixiv.Artwork]
	novelDetailResult         pixiv.Novel
	novelRequest              pixiv.NovelRequest
	novelSeriesResult         pixiv.NovelSeriesResult
	novelSeriesRequest        pixiv.NovelSeriesRequest
	novelContentHTML          string
	novelContentRequest       pixiv.NovelContentRequest
	artworkCommentsResult     pixiv.CommentPage
	artworkCommentsRequest    pixiv.ArtworkCommentsRequest
	novelCommentsResult       pixiv.CommentPage
	novelCommentsRequest      pixiv.NovelCommentsRequest
	bookmarkTagsPage          sdk.Page[pixiv.BookmarkTag]
	novelBookmarkTagsPage     sdk.Page[pixiv.BookmarkTag]
	bookmarkDetailResult      pixiv.ArtworkBookmarkDetail
	bookmarkDetailRequest     pixiv.ArtworkBookmarkRequest
	novelBookmarkDetailResult pixiv.NovelBookmarkDetail
	relatedPage               sdk.Page[pixiv.UserPreview]
	relatedRequest            pixiv.RelatedUsersRequest
}

// 收藏/关注 mutation tool 的 owner 契约：结构化成功结果。

// 同一产品 registry 必须由官方 stateless transport 支持新发现协议和旧初始化协议。
func TestStatelessHTTPProtocolNegotiation(t *testing.T) {
	server := newTestServer(pixivmcpserver.SDKPorts{}, pixivmcpserver.Account{})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	for _, version := range []string{"2025-06-18", "2026-07-28"} {
		t.Run(version, func(t *testing.T) {
			method := "initialize"
			params := map[string]any{"protocolVersion": version, "capabilities": map[string]any{},
				"clientInfo": map[string]any{"name": "protocol-test", "version": "1"}}
			if version == "2026-07-28" {
				method = "server/discover"
				params = map[string]any{"_meta": map[string]any{
					"io.modelcontextprotocol/protocolVersion":    version,
					"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "protocol-test", "version": "1"},
					"io.modelcontextprotocol/clientCapabilities": map[string]any{},
				}}
			}
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", bytes.NewReader(body)).WithContext(t.Context())
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", version)
			request.Header.Set("Mcp-Method", method)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var wire struct {
				Result struct {
					ProtocolVersion   string   `json:"protocolVersion"`
					SupportedVersions []string `json:"supportedVersions"`
				} `json:"result"`
				Error json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
				t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
			}
			if len(wire.Error) != 0 {
				t.Fatalf("protocol negotiation failed: %s", wire.Error)
			}
			if version == "2026-07-28" {
				if !slices.Contains(wire.Result.SupportedVersions, version) {
					t.Errorf("supported versions = %v, missing %s", wire.Result.SupportedVersions, version)
				}
			} else if wire.Result.ProtocolVersion != version {
				t.Errorf("negotiated version = %q, want %q", wire.Result.ProtocolVersion, version)
			}
			if id := response.Header().Get("Mcp-Session-Id"); id != "" {
				t.Errorf("stateless response created session %q", id)
			}
		})
	}
}

func TestPixivMCPDiagnosticsUseStableLocalRequestIDs(t *testing.T) {
	var (
		mu     sync.Mutex
		events []logging.Event
	)
	sink := logging.SinkFunc(func(event logging.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	rootCtx, cancel := context.WithCancel(logging.WithScope(context.Background(), sink, logging.ModulePixivCLI, 0))
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "debug-test", Version: "1"}, nil)
	toolhost.AddTool(toolhost.NewApp(toolhost.SDKPorts{}, toolhost.Account{}), server, &mcp.Tool{Name: "diagnostic_test"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
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
		if event.Module != logging.ModulePixivMCPServer || event.RequestID != wantID {
			t.Fatalf("event[%d]=%+v", index, event)
		}
	}
}

func httpFixturePorts(t *testing.T) pixivmcpserver.SDKPorts {
	t.Helper()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if _, err := store.Init(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	media, err := mediaproxy.New("https://media.test", store, nil)
	if err != nil {
		t.Fatal(err)
	}
	login, err := accounts.NewLoginManager(t.Context(), "https://media.test/pixiv-login", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{}, errors.New("fixture login unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(login.Close)
	return pixivmcpserver.SDKPorts{Media: media, Accounts: accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) { return accounts.LocalSnapshot{}, nil }}, Login: login}
}

type mcpDownloadFunc func(context.Context, downloader.DownloadRequest) (downloader.DownloadReport, error)

func (f mcpDownloadFunc) Download(ctx context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
	return f(ctx, request)
}

func TestLocalDownloadsUseSharedServiceAndReportPartialFiles(t *testing.T) {
	fake := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(42, "sample", 7)}}, nil
	}}
	ports, account := newTestSDKPorts(t, fake)
	root := t.TempDir()
	file := filepath.Join(root, "42.gif")
	require.NoError(t, os.WriteFile(file, []byte("fixture"), 0600))
	var requests []downloader.DownloadRequest
	fail := false
	ports.Downloads = &pixivmcpserver.LocalDownloads{Path: root, FilenameTemplate: "{id}", DirectoryTemplate: "{user_id}", Service: downloader.DownloadService{
		NewBatchDownloader: func(_ downloader.DownloadClient, path, template string) (downloader.BatchDownloader, error) {
			require.Equal(t, root, path)
			require.Equal(t, "{id}", template)
			return mcpDownloadFunc(func(_ context.Context, req downloader.DownloadRequest) (downloader.DownloadReport, error) {
				requests = append(requests, req)
				result := downloader.DownloadReport{Items: []downloader.DownloadedArtwork{{IllustID: 42, Files: []downloader.DownloadedFile{{Path: file, Page: 1}}}}}
				if fail {
					return result, errors.New("fixture interrupted after write")
				}
				return result, nil
			}), nil
		},
	}}
	session, closeSession := newSDKTestSessionWithPorts(t, ports, account)
	defer closeSession()
	result := callTool(t, session, "pixiv_download", map[string]any{"srcs": []string{"42", "https://www.pixiv.net/artworks/43"}, "pages": "1,3-4", "quality": "regular", "ugoira_mode": "apng"})
	require.False(t, result.IsError)
	require.Len(t, requests, 1)
	require.Equal(t, []int64{42, 43}, requests[0].IllustIDs)
	require.Equal(t, []int{1, 3, 4}, requests[0].Pages)
	require.Equal(t, downloader.DownloadQuality("regular"), requests[0].Quality)
	require.Equal(t, downloader.UgoiraFormat("apng"), requests[0].UgoiraFormat)
	require.Equal(t, "{user_id}", requests[0].DirectoryTemplate)
	var out struct {
		Files []struct {
			Path     string `json:"path"`
			FileURI  string `json:"file_uri"`
			MIMEType string `json:"mime_type"`
			Size     int64  `json:"size_bytes"`
		}
	}
	decodeStructured(t, result, &out)
	require.Len(t, out.Files, 1)
	require.Equal(t, file, out.Files[0].Path)
	require.Contains(t, out.Files[0].FileURI, "file://")
	require.Contains(t, out.Files[0].FileURI, "42.gif")
	require.Equal(t, "image/gif", out.Files[0].MIMEType)
	require.EqualValues(t, 7, out.Files[0].Size)
	fail = true
	result = callTool(t, session, "pixiv_download_random_from_recommendation", map[string]any{"count": 1})
	require.True(t, result.IsError)
	decodeStructured(t, result, &out)
	require.Len(t, out.Files, 1, "interruption must retain committed paths")
	require.Equal(t, []int64{42}, requests[1].IllustIDs)
	before := len(requests)
	for _, args := range []map[string]any{{}, {"src": "42", "srcs": []string{"43"}}, {"src": "42", "pages": "0"}, {"src": "42", "quality": "invalid"}} {
		require.True(t, callTool(t, session, "pixiv_download", args).IsError)
	}
	require.Equal(t, before, len(requests), "invalid inputs must not write files")
}
