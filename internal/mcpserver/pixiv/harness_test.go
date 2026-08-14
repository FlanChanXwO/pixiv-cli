package pixiv_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 本文件是 Pixiv MCP 产品级测试 harness：fake SDK client、fake 下载器与
// session/调用 helper。它被本包内多个 owner 契约测试文件共享，因此集中在此，
// 而不是散落在某一个 owner 测试文件的末尾。
func assertDownloadRandomCountError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
}

func assertNoDownloadRandomDownstream(t *testing.T, probe *downloadRandomProbe) {
	t.Helper()
	if probe.openCalls != 0 || probe.recommendationCalls != 0 || probe.managerFactoryCalls != 0 || probe.downloads.downloadCalls != 0 || len(probe.downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

type downloadRandomProbe struct {
	openCalls           int
	recommendationCalls int
	managerFactoryCalls int
	downloads           *fakeDownloads
}

func newDownloadRandomProbeSession(t *testing.T, recommendationIDs []int64) (*mcp.ClientSession, func(), *downloadRandomProbe) {
	t.Helper()
	probe := &downloadRandomProbe{downloads: &fakeDownloads{}}
	sdkClient := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		probe.recommendationCalls++
		illusts := make([]pixiv.Artwork, len(recommendationIDs))
		for i, id := range recommendationIDs {
			illusts[i] = testSDKIllust(id, "recommended", 1)
		}
		return sdk.Page[pixiv.Artwork]{Items: illusts}, nil
	}}
	wireClient := openWireClient(t, sdkClient)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			probe.openCalls++
			return wireClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, wireClient)
			return err
		},
	}
	server := pixivmcpserver.NewWithSDKDownloadFactory(probe.downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		probe.managerFactoryCalls++
		return probe.downloads
	}, ports, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return session, func() {
		_ = session.Close()
		cancel()
	}, probe
}

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

// fakeAPI 只占据 New/NewWithSDK 保留的已废弃兼容参数；MCP 不得调用它。
type fakeAPI struct{}

func newTestSession(t *testing.T, downloads *fakeDownloads) (*mcp.ClientSession, func()) {
	t.Helper()
	server := pixivmcpserver.New(&fakeAPI{}, downloads)
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

func newSDKTestSession(t *testing.T, sdkClient *fakeSDKClient) (*mcp.ClientSession, func()) {
	return newSDKTestSessionWithAPI(t, &fakeAPI{}, sdkClient)
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

func newSDKTestSessionWithAPI(t *testing.T, api any, sdkClient *fakeSDKClient) (*mcp.ClientSession, func()) {
	t.Helper()
	ports, _ := newTestSDKPorts(t, sdkClient)
	return newSDKTestSessionWithPorts(t, api, ports, pixivmcpserver.Account{})
}

func newSDKTestSessionWithPorts(t *testing.T, api any, ports pixivmcpserver.SDKPorts, account pixivmcpserver.Account) (*mcp.ClientSession, func()) {
	t.Helper()
	server := pixivmcpserver.NewWithSDKDownloadFactory(&fakeDownloads{}, func(*pixiv.Client) pixivmcpserver.DownloadManager { return &fakeDownloads{} }, ports, account)
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

	userID                int64
	searchIllust          func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	searchNovel           func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	searchUser            func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	illustDetail          func(context.Context, int64) (pixiv.Artwork, error)
	novelDetail           func(context.Context, int64) (pixiv.Novel, error)
	artworks              []pixiv.Artwork
	bookmarks             []pixiv.Artwork
	following             []pixiv.UserPreview
	followingIllusts      func(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	followingNovels       func(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	latestIllusts         func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	latestNovels          func(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	myPixivUsers          func(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	myPixivIllusts        func(context.Context, pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	myPixivNovels         func(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userNovels            func(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userNovelBookmarks    func(context.Context, pixiv.UserNovelBookmarksRequest) (sdk.Page[pixiv.Novel], error)
	userFollowing         func(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	userFollowers         func(context.Context, pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedUsers          func(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	blockedUsers          func(context.Context, pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	recommendedArtworks   func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	illustRanking         func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	novelRecommended      func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userRecommended       func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedArtworks       func(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	artworkSeries         func(context.Context, pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error)
	novelSeries           func(context.Context, pixiv.NovelSeriesRequest) (sdk.Page[pixiv.Novel], error)
	userDetailResult      pixiv.UserDetail
	userDetailErr         error
	artworkBookmarkDetail pixiv.ArtworkBookmarkDetail
	artworkBookmarkErr    error
	bookmarkTags          []pixiv.BookmarkTag
	illustComments        []pixiv.Comment
	novelComments         []pixiv.Comment
	trendingTags          []pixiv.TrendingTag
	ugoiraMetadata        pixiv.UgoiraMetadata
	addBookmarkErr        error
	removeBookmarkErr     error
	followUserErr         error
	unfollowUserErr       error

	// capture
	searchIllustRequest        pixiv.SearchArtworksRequest
	searchNovelRequest         pixiv.SearchNovelsRequest
	searchUserRequest          pixiv.SearchUsersRequest
	illustDetailRequest        int64
	novelDetailRequest         int64
	relatedArtworksRequest     pixiv.RelatedArtworksRequest
	artworkSeriesRequest       pixiv.ArtworkSeriesRequest
	illustRankingRequest       pixiv.ArtworkRankingRequest
	recommendedArtworksRequest pixiv.RecommendedArtworksRequest
	followingIllustsRequest    pixiv.FollowingArtworksRequest
	followingNovelsRequest     pixiv.FollowingNovelsRequest
	latestIllustsRequest       pixiv.LatestArtworksRequest
	latestNovelsRequest        pixiv.LatestNovelsRequest
	myPixivUsersRequest        pixiv.MyPixivUsersRequest
	myPixivIllustsRequest      pixiv.MyPixivArtworksRequest
	myPixivNovelsRequest       pixiv.MyPixivNovelsRequest
	novelRecommendedRequest    pixiv.RecommendedNovelsRequest
	userRecommendedRequest     pixiv.RecommendedUsersRequest
	userDetailRequest          pixiv.UserRequest
	artworksRequest            pixiv.UserArtworksRequest
	artworksRequests           []pixiv.UserArtworksRequest
	userArtworksFunc           func(pixiv.UserArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	userArtworksCalls          int
	recommendedArtworksCalls   int
	bookmarksRequest           pixiv.UserArtworkBookmarksRequest
	userBookmarksErr           error
	userNovelBookmarksRequest  pixiv.UserNovelBookmarksRequest
	userNovelsRequest          pixiv.UserNovelsRequest
	followingRequest           pixiv.UserFollowingRequest
	userFollowersRequest       pixiv.UserFollowersRequest
	relatedUsersRequest        pixiv.RelatedUsersRequest
	blockedUsersRequest        pixiv.UserBlockedUsersRequest
	bookmarkTagsRequest        pixiv.UserArtworkBookmarkTagsRequest
	artworkBookmarkRequest     pixiv.ArtworkBookmarkRequest
	novelSeriesRequest         pixiv.NovelSeriesRequest
	illustCommentsRequest      pixiv.ArtworkCommentsRequest
	novelCommentsRequest       pixiv.NovelCommentsRequest
	ugoiraMetadataRequest      pixiv.UgoiraMetadataRequest
	addBookmarkRequest         pixiv.AddBookmarkRequest
	removeBookmarkRequest      pixiv.RemoveBookmarkRequest
	followUserRequest          pixiv.FollowUserRequest
	unfollowUserRequest        pixiv.UnfollowUserRequest

	// typed read tools canned results (task11)
	artworkSeriesPage      sdk.Page[pixiv.Artwork]
	novelDetailResult      pixiv.Novel
	novelRequest           pixiv.NovelRequest
	novelContentHTML       string
	novelContentRequest    pixiv.NovelContentRequest
	novelSeriesResult      pixiv.NovelSeriesResult
	artworkCommentsResult  pixiv.CommentPage
	artworkCommentsRequest pixiv.ArtworkCommentsRequest
	novelCommentsResult    pixiv.CommentPage
	bookmarkTagsPage       sdk.Page[pixiv.BookmarkTag]
	bookmarkDetailResult   pixiv.ArtworkBookmarkDetail
	bookmarkDetailRequest  pixiv.ArtworkBookmarkRequest
	novelBookmarksPage     sdk.Page[pixiv.Novel]
	novelBookmarksRequest  pixiv.UserNovelBookmarksRequest
	followersPage          sdk.Page[pixiv.UserPreview]
	followersRequest       pixiv.UserFollowersRequest
	relatedPage            sdk.Page[pixiv.UserPreview]
	relatedRequest         pixiv.RelatedUsersRequest
	blockedPage            sdk.Page[pixiv.UserPreview]
	blockedRequest         pixiv.UserBlockedUsersRequest
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

func assertEmptyDownloadResult(t *testing.T, result *mcp.CallToolResult, wantDelivery, wantText string) {
	t.Helper()
	out := decodeDownloadOut(t, result)
	if !result.IsError || out.Delivery != wantDelivery || out.Text != wantText || out.Items == nil || len(out.Items) != 0 || out.Files == nil || len(out.Files) != 0 {
		t.Fatalf("result=%+v output=%+v", result, out)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content=%+v want one text item without image", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text != wantText {
		t.Fatalf("content=%+v want text %q", result.Content, wantText)
	}
}

type fakeDownloads struct {
	artworks      []downloader.DownloadedArtwork
	downloadCalls int
	downloadIDs   []int64
	lastRequest   downloader.DownloadRequest
	err           error
}

func (fakeDownloads) SetDownloadPath(string) error { return nil }
func (d *fakeDownloads) Download(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	ids := request.IllustIDs

	d.downloadCalls++
	d.downloadIDs = append([]int64(nil), ids...)
	d.lastRequest = request
	return d.artworks, d.err
}
