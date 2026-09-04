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
	userFollowing         func(context.Context, pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedUsers          func(context.Context, pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	recommendedArtworks   func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	illustRanking         func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	novelRecommended      func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userRecommended       func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	relatedArtworks       func(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	artworkSeries         func(context.Context, pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error)
	userDetailResult      pixiv.UserDetail
	userDetailErr         error
	artworkBookmarkDetail pixiv.ArtworkBookmarkDetail
	bookmarkTags          []pixiv.BookmarkTag
	illustComments        []pixiv.Comment
	trendingTags          []pixiv.TrendingTag
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
	userNovelsRequest          pixiv.UserNovelsRequest
	followingRequest           pixiv.UserFollowingRequest
	relatedUsersRequest        pixiv.RelatedUsersRequest
	bookmarkTagsRequest        pixiv.UserArtworkBookmarkTagsRequest
	artworkBookmarkRequest     pixiv.ArtworkBookmarkRequest
	illustCommentsRequest      pixiv.ArtworkCommentsRequest
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
	artworkCommentsResult  pixiv.CommentPage
	artworkCommentsRequest pixiv.ArtworkCommentsRequest
	bookmarkTagsPage       sdk.Page[pixiv.BookmarkTag]
	bookmarkDetailResult   pixiv.ArtworkBookmarkDetail
	bookmarkDetailRequest  pixiv.ArtworkBookmarkRequest
	relatedPage            sdk.Page[pixiv.UserPreview]
	relatedRequest         pixiv.RelatedUsersRequest
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
	result        downloader.DownloadBatchResult
	downloadCalls int
	downloadIDs   []int64
	lastRequest   downloader.DownloadRequest
	err           error
}

func (fakeDownloads) SetDownloadPath(string) error { return nil }
func (d *fakeDownloads) Download(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadBatchResult, error) {
	ids := request.IllustIDs

	d.downloadCalls++
	d.downloadIDs = append([]int64(nil), ids...)
	d.lastRequest = request
	result := d.result
	if result.Items == nil && d.artworks != nil {
		result.Items = d.artworks
	}
	return result, d.err
}

// 收藏/关注 mutation tool 的 owner 契约：结构化成功结果。
