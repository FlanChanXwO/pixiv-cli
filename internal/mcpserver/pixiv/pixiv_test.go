package pixiv_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
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
func findTool(tools []*mcp.Tool, name string) *mcp.Tool {
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func TestPixivMCPToolLogsUseStableLocalCallIDs(t *testing.T) {
	output := &syncBuffer{}
	logger, err := logging.New(output, "debug", "json")
	if err != nil {
		t.Fatal(err)
	}
	rootCtx, cancel := context.WithCancel(logging.WithCall(context.Background(), logger, "", ""))
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "debug-test", Version: "1"}, nil)
	toolhost.AddTool(toolhost.NewApp(toolhost.SDKPorts{Logger: logger}, toolhost.Account{}), server, &mcp.Tool{Name: "diagnostic_test"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, errors.New("fixture failure")
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(rootCtx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "debug-client", Version: "1"}, nil)
	session, err := client.Connect(rootCtx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "diagnostic_test", Arguments: map[string]any{}})
	_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "diagnostic_test", Arguments: map[string]any{}})

	text := output.String()
	for _, want := range []string{`"call_id":"1"`, `"call_id":"2"`, `"operation":"diagnostic_test"`, `"msg":"Tool failed"`, `"error":"fixture failure"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("tool logs missing %s:\n%s", want, text)
		}
	}
}

// syncBuffer 让并发写入的日志在测试中可安全读取。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
	login, err := accounts.NewLogin(t.Context(), "https://media.test/pixiv-login", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{}, errors.New("fixture login unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(login.Close)
	return pixivmcpserver.SDKPorts{Media: media, Accounts: accounts.Selection{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) { return accounts.LocalSnapshot{}, nil }}, Login: login}
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

// TestAccountToolsShareSelectionWithoutOpeningSDK 也固定账号 tool 的注册与本地 effect
// 注解：只访问本地账号选择，不打开 SDK。
func TestAccountToolsShareSelectionWithoutOpeningSDK(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	selection := accounts.Selection{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42, Username: "first", HasCredentials: true}, {UserID: 73, Username: "second", HasCredentials: true}}}, nil
	}}
	ports := pixivmcpserver.SDKPorts{Accounts: selection}
	first, closeFirst := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
	defer closeFirst()
	second, closeSecond := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
	defer closeSecond()
	result := callTool(t, first, "pixiv_account_list", map[string]any{})
	require.False(t, result.IsError)
	var out outputs.AccountStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "selection_required", out.SelectionState)
	require.Len(t, out.Accounts, 2)
	result = callTool(t, first, "pixiv_account_use", map[string]any{"user_id": 73})
	require.False(t, result.IsError)
	decodeStructured(t, result, &out)
	require.Equal(t, int64(73), out.SelectedUserID)
	result = callTool(t, second, "pixiv_account_status", map[string]any{})
	require.False(t, result.IsError)
	decodeStructured(t, result, &out)
	require.Equal(t, int64(73), out.SelectedUserID)
	require.Equal(t, "present_unverified", out.CredentialState)
	for _, args := range []map[string]any{{}, {"user_id": 0}, {"user_id": -1}, {"user_id": 999}, {"user_id": 73, "refresh_token": "not-accepted"}} {
		result = callTool(t, first, "pixiv_account_use", args)
		require.True(t, result.IsError)
	}
	saved, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), saved.SelectedPixivUserID)
	tools, err := first.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, name := range []string{"pixiv_account_list", "pixiv_account_status", "pixiv_account_use"} {
		tool := findTool(tools.Tools, name)
		require.NotNil(t, tool, "missing %s", name)
		require.NotNil(t, tool.Annotations)
		require.False(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
		require.False(t, *tool.Annotations.OpenWorldHint)
		require.True(t, tool.Annotations.IdempotentHint)
		require.NotNil(t, tool.OutputSchema)
	}
}

func TestAccountToolsRedactLocalErrorsAndPreserveStructuredFailure(t *testing.T) {
	selection := accounts.Selection{Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{}, errors.New("fixture-secret /private/database-path")
	}}
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{Accounts: selection}, pixivmcpserver.Account{})
	defer closeSession()
	for _, name := range []string{"pixiv_account_list", "pixiv_account_status", "pixiv_account_use"} {
		args := map[string]any{}
		if name == "pixiv_account_use" {
			args["user_id"] = 73
		}

		result := callTool(t, session, name, args)
		require.True(t, result.IsError)
		body, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(body), "fixture-secret")
		require.NotContains(t, string(body), "database-path")

		var out outputs.AccountStatus
		decodeStructured(t, result, &out)
		require.Equal(t, "local_state_error", out.Error)
		require.Equal(t, "unknown", out.SelectionState)
		require.Empty(t, out.Accounts)
	}
	// schema 拒绝的错误同样不回显原始输入值，并保留 structured content。
	invalid := callTool(t, session, "pixiv_account_use", map[string]any{"user_id": "fixture-secret-value"})
	require.True(t, invalid.IsError)
	body, err := json.Marshal(invalid)
	require.NoError(t, err)
	require.NotContains(t, string(body), "fixture-secret-value")
	require.NotNil(t, invalid.StructuredContent)
}

func TestAccountUseDoesNotReportSuccessWhenStateIsUnavailable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("unchanged"), 0600))
	selection := accounts.Selection{Store: auth.Store{Path: filepath.Join(file, "state.json")}, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 73, HasCredentials: true}}}, nil
	}}
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{Accounts: selection}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_account_use", map[string]any{"user_id": 73})
	require.True(t, result.IsError)
	var out outputs.AccountStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "local_state_error", out.Error)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(body))
}
func TestLoginToolsSharePendingSessionAndQueryWithoutLocalState(t *testing.T) {
	loginRelay, err := accounts.NewLogin(t.Context(), "https://relay.example/pixiv-login", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true }, Complete: func(context.Context, string) (accounts.LoginResult, error) { return accounts.LoginResult{}, nil }}, nil
	})
	require.NoError(t, err)
	defer loginRelay.Close()
	ports := pixivmcpserver.SDKPorts{Login: loginRelay}
	first, closeFirst := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
	defer closeFirst()
	second, closeSecond := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
	defer closeSecond()
	tools, err := first.ListTools(t.Context(), nil)
	require.NoError(t, err)
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "pixiv_account_login_start" {
			found = true
			require.False(t, tool.Annotations.ReadOnlyHint)
			require.True(t, *tool.Annotations.DestructiveHint)
			require.False(t, tool.Annotations.IdempotentHint)
			require.True(t, *tool.Annotations.OpenWorldHint)
		}
	}
	require.True(t, found, "missing real login-start tool")
	result := callTool(t, first, "pixiv_account_login_start", map[string]any{})
	require.False(t, result.IsError)
	var started struct {
		LoginID      string `json:"login_id"`
		URL          string `json:"authorization_url"`
		Status       string `json:"status"`
		Helper       bool   `json:"requires_local_helper"`
		Instructions string `json:"instructions"`
	}
	decodeStructured(t, result, &started)
	require.NotEmpty(t, started.LoginID)
	require.Contains(t, started.URL, "https://relay.example/pixiv-login/session/")
	require.Equal(t, "waiting_for_user", started.Status)
	require.True(t, started.Helper)
	require.NotEmpty(t, started.Instructions)
	again := callTool(t, second, "pixiv_account_login_start", map[string]any{})
	require.False(t, again.IsError)
	var reused struct {
		LoginID string `json:"login_id"`
	}
	decodeStructured(t, again, &reused)
	require.Equal(t, started.LoginID, reused.LoginID)
	status := callTool(t, second, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	require.False(t, status.IsError)
	var queried struct {
		Login accounts.LoginStatus `json:"login"`
	}
	decodeStructured(t, status, &queried)
	require.Equal(t, "waiting_for_user", queried.Login.Status)
	require.Equal(t, started.LoginID, queried.Login.LoginID)
	restart := callTool(t, second, "pixiv_account_login_start", map[string]any{"restart": true})
	require.False(t, restart.IsError)
	decodeStructured(t, restart, &reused)
	require.NotEqual(t, started.LoginID, reused.LoginID)
	status = callTool(t, first, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	decodeStructured(t, status, &queried)
	require.Equal(t, "not_found", queried.Login.Status)
}

func TestLoginStartErrorsKeepLoginEnvelopeAndHideArguments(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, httpFixturePorts(t), pixivmcpserver.Account{})
	defer closeSession()
	for _, args := range []map[string]any{{"restart": "fixture-secret"}, {"callback_url": "fixture-secret"}} {
		result := callTool(t, session, "pixiv_account_login_start", args)
		require.True(t, result.IsError)
		var out outputs.LoginStatus
		decodeStructured(t, result, &out)
		require.Equal(t, "failed", out.Status)
		require.Equal(t, "invalid_request", out.Error)
		body, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(body), "fixture-secret")
	}
	result := callTool(t, session, "pixiv_account_login_start", map[string]any{})
	require.True(t, result.IsError)
	var out outputs.LoginStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "login_start_failed", out.Error)
}

func TestLoginStatusToolRetainsSavedAccountOnSelectionFailure(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	loginRelay, err := accounts.NewLogin(t.Context(), "https://relay.example", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{
			AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true },
			Complete: func(ctx context.Context, _ string) (accounts.LoginResult, error) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return accounts.LoginResult{}, ctx.Err()
				}
				return accounts.LoginResult{Account: accounts.Account{UserID: 73, HasCredentials: true}, AccountSaved: true}, errors.New("fixture-private-store-path")
			},
		}, nil
	})
	require.NoError(t, err)
	defer loginRelay.Close()
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{Login: loginRelay}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_account_login_start", map[string]any{})
	var started outputs.LoginStatus
	decodeStructured(t, result, &started)
	page := httptest.NewRecorder()
	loginRelay.ServeHTTP(page, httptest.NewRequest("GET", started.AuthorizationURL, nil))
	link, err := url.Parse(page.Header().Get("Location"))
	require.NoError(t, err)
	id, proof := link.Query().Get("session"), link.Query().Get("access")
	start := httptest.NewRecorder()
	loginRelay.ServeHTTP(start, httptest.NewRequest("POST", "/start/"+id, strings.NewReader(`{"proof":"`+proof+`"}`)))
	require.Equal(t, 200, start.Code)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		loginRelay.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/callback/"+id, strings.NewReader(`{"proof":"`+proof+`","callback_url":"pixiv://account/login?code=fixture"}`)).WithContext(ctx))
		close(done)
	}()
	<-entered
	result = callTool(t, session, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	var status outputs.AccountStatus
	decodeStructured(t, result, &status)
	require.Equal(t, "exchanging", status.Login.Status)
	cancel()
	<-done
	close(release)
	require.Eventually(t, func() bool { return loginRelay.Status(started.LoginID).Status == "failed" }, 5*time.Second, time.Millisecond)
	result = callTool(t, session, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	require.True(t, result.IsError)
	decodeStructured(t, result, &status)
	require.Equal(t, "failed", status.Login.Status)
	require.True(t, status.Login.AccountSaved)
	require.False(t, status.Login.SelectionUpdated)
	require.Equal(t, int64(73), status.Login.Account.UserID)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(body), "fixture-private-store-path")
	require.NotContains(t, string(body), proof)
}

type mediaTransport func(*http.Request) (*http.Response, error)

func (f mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type mediaOutput struct {
	TotalPages     int    `json:"total_pages"`
	RequestedPages []int  `json:"requested_pages"`
	DeliveredPages []int  `json:"delivered_pages"`
	Complete       bool   `json:"complete"`
	Error          string `json:"error"`
	Pages          []struct {
		Page         int    `json:"page"`
		ContentIndex int    `json:"content_index"`
		MIME         string `json:"mime_type"`
		Size         int    `json:"size"`
	} `json:"pages"`
	Failures []struct {
		Page  int    `json:"page"`
		Error string `json:"error"`
	} `json:"failures"`
}

func TestArtworkMediaDeliversAllPagesAndExplicitSelection(t *testing.T) {
	for _, tc := range []struct {
		name, quality, path string
		pages               []int
		want                []int
		failPage            int
		badMIME             bool
		invalid             bool
		animation           string
	}{
		{name: "default-all", path: "c/1200x1200/", want: []int{1, 2, 3}},
		{name: "original-ordered-deduplicated", quality: "original", path: "img-original/", pages: []int{3, 1, 3}, want: []int{3, 1}},
		{name: "thumbnail", quality: "thumbnail", path: "c/250x250_80_a2/", pages: []int{2}, want: []int{2}},
		{name: "partial", path: "c/1200x1200/", want: []int{1, 2, 3}, failPage: 2},
		{name: "invalid-image", path: "c/1200x1200/", want: []int{1, 2, 3}, failPage: 2, badMIME: true},
		{name: "out-of-range", pages: []int{1, 4}, want: []int{1, 4}, invalid: true},
		{name: "static-animation-parameter", pages: []int{1}, want: []int{1}, animation: "gif", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))
			data := buf.Bytes()
			var opens, closes, reads atomic.Int32
			transport := mediaTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "app-api.pixiv.net" {
					pages := make([]any, 3)
					for i := range pages {
						pages[i] = map[string]any{"image_urls": map[string]string{"original": fmt.Sprintf("https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p%d.png", i)}}
					}
					body, _ := json.Marshal(map[string]any{"illust": map[string]any{"id": 42, "title": "fixture", "type": "manga", "page_count": 3, "create_date": "2026-01-01T00:00:00Z", "user": map[string]any{"id": 7, "name": "artist"}, "meta_pages": pages}})
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
				}
				require.Equal(t, "i.pximg.net", r.URL.Host)
				require.NotEmpty(t, r.Header.Get("Referer"))
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				require.Contains(t, r.URL.Path, tc.path)
				reads.Add(1)
				code := 200
				payload := data
				if tc.failPage > 0 && strings.Contains(r.URL.Path, fmt.Sprintf("42_p%d", tc.failPage-1)) {
					code = 403
					if tc.badMIME {
						code = 200
						payload = []byte("<html>fixture-secret-upstream-error</html>")
					}
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(bytes.NewReader(payload))}, nil
			})
			ports := pixivmcpserver.SDKPorts{OpenLease: func(context.Context, pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				opens.Add(1)
				client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
				return lifecycle.NewLease(client, func() error { closes.Add(1); return nil }), err
			}}
			session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
			defer closeSession()
			tools, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)
			found := false
			for _, tool := range tools.Tools {
				if tool.Name == "pixiv_artwork_media" {
					found = true
					require.True(t, tool.Annotations.ReadOnlyHint)
				}
			}
			require.True(t, found, "missing artwork media tool")
			args := map[string]any{"illust_id": 42}
			if tc.quality != "" {
				args["quality"] = tc.quality
			}
			if tc.pages != nil {
				args["pages"] = tc.pages
			}
			if tc.animation != "" {
				args["animation_format"] = tc.animation
			}
			result := callTool(t, session, "pixiv_artwork_media", args)
			var out mediaOutput
			decodeStructured(t, result, &out)
			require.Equal(t, 3, out.TotalPages)
			require.Equal(t, tc.want, out.RequestedPages)
			require.Equal(t, tc.failPage == 0 && !tc.invalid, out.Complete)
			require.Equal(t, tc.failPage != 0 || tc.invalid, result.IsError)
			wantDelivered := []int{}
			for _, p := range tc.want {
				if p != tc.failPage && !tc.invalid {
					wantDelivered = append(wantDelivered, p)
				}
			}
			require.Equal(t, wantDelivered, out.DeliveredPages)
			require.Len(t, out.Pages, len(wantDelivered))
			for i, p := range out.Pages {
				require.Equal(t, wantDelivered[i], p.Page)
				require.Equal(t, "image/png", p.MIME)
				require.Equal(t, len(data), p.Size)
				require.Less(t, p.ContentIndex, len(result.Content))
				img, ok := result.Content[p.ContentIndex].(*mcp.ImageContent)
				require.True(t, ok)
				require.Equal(t, data, img.Data)
				require.Equal(t, p.MIME, img.MIMEType)
			}
			if tc.failPage > 0 {
				require.Len(t, out.Failures, 1)
				require.Equal(t, tc.failPage, out.Failures[0].Page)
			} else {
				require.Empty(t, out.Failures)
			}
			expectedReads := len(tc.want)
			if tc.invalid {
				expectedReads = 0
			}
			require.Equal(t, int32(expectedReads), reads.Load())
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "fixture-secret-upstream-error")
			require.Equal(t, int32(1), opens.Load())
			require.Equal(t, opens.Load(), closes.Load())
		})
	}
}

func TestArtworkMediaRejectsInvalidInputsBeforeOpening(t *testing.T) {
	var opens atomic.Int32
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
		opens.Add(1)
		return nil, fmt.Errorf("fixture-private-token")
	}}, pixivmcpserver.Account{})
	defer closeSession()
	for _, args := range []map[string]any{{"illust_id": "fixture-secret-argument"}, {"illust_id": 42, "unknown": "fixture-secret-argument"}, {"illust_id": 0}, {"illust_id": 42, "pages": []int{}}, {"illust_id": 42, "pages": []int{0}}, {"illust_id": 42, "quality": "bad"}, {"illust_id": 42, "animation_format": "bad"}} {
		result := callTool(t, session, "pixiv_artwork_media", args)
		require.True(t, result.IsError)
		require.NotNil(t, result.StructuredContent)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "fixture-secret-argument")
	}
	require.Zero(t, opens.Load())
}

// mediaWaitingBody models an in-flight HTTP body, not a detached download job.
type mediaWaitingBody struct {
	ctx     context.Context
	entered chan struct{}
	closed  *atomic.Bool
}

func (b *mediaWaitingBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *mediaWaitingBody) Close() error { b.closed.Store(true); return nil }

func TestArtworkMediaCancellationClosesBodyAndAccountLease(t *testing.T) {
	entered := make(chan struct{})
	var bodyClosed, leaseClosed atomic.Bool
	var mediaReads atomic.Int32
	client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			body := `{"illust":{"id":42,"title":"fixture","type":"illust","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png"}}}`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		require.Equal(t, "i.pximg.net", r.URL.Host)
		mediaReads.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: &mediaWaitingBody{ctx: r.Context(), entered: entered, closed: &bodyClosed}}, nil
	})}})
	require.NoError(t, err)
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{OpenLease: func(context.Context, pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
		return lifecycle.NewLease(client, func() error { leaseClosed.Store(true); return nil }), nil
	}}, pixivmcpserver.Account{})
	defer closeSession()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_artwork_media", Arguments: map[string]any{"illust_id": 42}})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("media read never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tool cancellation did not return")
	}
	require.Eventually(t, func() bool { return bodyClosed.Load() && leaseClosed.Load() }, 5*time.Second, time.Millisecond)
	require.Equal(t, int32(1), mediaReads.Load())
}

func TestArtworkMediaUgoiraDeliversAnimationAndPreview(t *testing.T) {
	for _, tc := range []struct{ format, mime, quality, ui string }{{"", "image/gif", "original", ""}, {"apng", "image/apng", "original", ""}, {"gif", "image/gif", "medium", ""}, {"gif", "image/gif", "original", "text/html;profile=mcp-app"}, {"apng", "image/apng", "original", "text/html;profile=mcp-app"}, {"gif", "image/gif", "original", "text/plain"}} {
		t.Run(tc.format+tc.quality+"/"+tc.ui, func(t *testing.T) {
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			for i, c := range []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
				frame, err := zw.Create(fmt.Sprintf("%d.png", i))
				require.NoError(t, err)
				img := image.NewRGBA(image.Rect(0, 0, 2, 2))
				for y := 0; y < 2; y++ {
					for x := 0; x < 2; x++ {
						img.SetRGBA(x, y, c)
					}
				}
				require.NoError(t, png.Encode(frame, img))
			}
			require.NoError(t, zw.Close())
			var closed, archiveReads atomic.Int32
			client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch {
				case r.URL.Host == "app-api.pixiv.net" && strings.Contains(r.URL.Path, "ugoira"):
					urls := map[string]string{"medium": "https://i.pximg.net/medium.zip"}
					if tc.quality == "original" {
						urls["original"] = "https://i.pximg.net/original.zip"
					}
					raw, _ := json.Marshal(map[string]any{"ugoira_metadata": map[string]any{"zip_urls": urls, "frames": []any{map[string]any{"file": "0.png", "delay": 80}, map[string]any{"file": "1.png", "delay": 120}}}})
					body = string(raw)
				case r.URL.Host == "app-api.pixiv.net":
					body = `{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`
				case r.URL.Host == "i.pximg.net":
					archiveReads.Add(1)
					require.Equal(t, "/"+tc.quality+".zip", r.URL.Path)
					require.NotEmpty(t, r.Header.Get("Referer"))
					require.Empty(t, r.Header.Get("Authorization"))
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/zip"}}, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
				default:
					return nil, fmt.Errorf("unexpected fixture host")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}})
			require.NoError(t, err)
			output := &syncBuffer{}
			logger, err := logging.New(output, "debug", "json")
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(logging.WithCall(t.Context(), logger, "", ""))
			defer cancel()
			protocol := newTestServer(pixivmcpserver.SDKPorts{Logger: logger, OpenLease: func(context.Context, pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				return lifecycle.NewLease(client, func() error { closed.Add(1); return nil }), nil
			}}, pixivmcpserver.Account{})
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			go func() { _ = protocol.Run(ctx, serverTransport) }()
			var options *mcp.ClientOptions
			if tc.ui != "" {
				options = &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{Extensions: map[string]any{"io.modelcontextprotocol/ui": map[string]any{"mimeTypes": []string{tc.ui}}}}}
			}
			session, err := mcp.NewClient(&mcp.Implementation{Name: "media-observation-test", Version: "1"}, options).Connect(ctx, clientTransport, nil)
			require.NoError(t, err)
			defer session.Close()
			args := map[string]any{"illust_id": 42}
			if tc.format != "" {
				args["animation_format"] = tc.format
			}
			result := callTool(t, session, "pixiv_artwork_media", args)
			require.False(t, result.IsError, "missing full Ugoira delivery: %+v", result)
			var out struct {
				Complete bool `json:"complete"`
				Pages    []struct {
					Filename       string `json:"filename"`
					MIME           string `json:"mime_type"`
					Size           int    `json:"size"`
					ContentIndex   int    `json:"content_index"`
					PreviewIndex   *int   `json:"preview_content_index"`
					ArchiveQuality string `json:"archive_quality"`
				} `json:"pages"`
			}
			decodeStructured(t, result, &out)
			require.True(t, out.Complete)
			require.Len(t, out.Pages, 1)
			p := out.Pages[0]
			require.Equal(t, tc.mime, p.MIME)
			require.Equal(t, tc.quality, p.ArchiveQuality)
			format := tc.format
			if format == "" {
				format = "gif"
			}
			require.Equal(t, "42."+format, p.Filename)
			contents := result.Content
			require.Len(t, result.Content, 2, "stdio retains the complete animation and preview even when the host supports Apps")
			require.NotContains(t, result.Meta, "pixiv/mediaContents")
			animation, ok := contents[p.ContentIndex].(*mcp.ImageContent)
			require.True(t, ok, "animation must be complete image content")
			contentMIME := tc.mime
			if format == "apng" {
				contentMIME = "image/png"
			}
			require.Equal(t, contentMIME, animation.MIMEType)
			animationBytes := animation.Data
			require.Equal(t, p.Size, len(animationBytes))
			stages := mediaStages(t, output.String())
			for _, stage := range []string{"artwork_query", "animation_metadata", "animation_zip_download", "animation_encode_wait", "animation_encode", "animation_result_read", "animation_generation"} {
				event, ok := stages[stage]
				require.True(t, ok, "missing actual stage %s", stage)
				require.Equal(t, "DEBUG", event["level"])
				require.GreaterOrEqual(t, event["duration"], float64(0))
			}
			require.Equal(t, float64(len(archive.Bytes())), stages["animation_zip_download"]["count"])
			require.Equal(t, float64(p.Size), stages["animation_result_read"]["count"])
			require.Equal(t, float64(p.Size), stages["animation_generation"]["count"])
			require.NotContains(t, output.String(), "fixture-token")
			require.NotContains(t, output.String(), "pximg.net")
			require.NotContains(t, output.String(), temp)

			if format == "gif" {
				decoded, err := gif.DecodeAll(bytes.NewReader(animationBytes))
				require.NoError(t, err)
				require.Len(t, decoded.Image, 2)
				require.Equal(t, []int{8, 12}, decoded.Delay)
			} else {
				require.True(t, bytes.HasPrefix(animationBytes, []byte("\x89PNG\r\n\x1a\n")))
				require.Contains(t, string(animationBytes), "acTL")
			}
			require.NotNil(t, p.PreviewIndex)
			require.NotEqual(t, p.ContentIndex, *p.PreviewIndex)
			preview, ok := contents[*p.PreviewIndex].(*mcp.ImageContent)
			require.True(t, ok)
			require.Equal(t, "image/png", preview.MIMEType)

			first, err := png.Decode(bytes.NewReader(preview.Data))
			require.NoError(t, err)
			require.Equal(t, image.Rect(0, 0, 2, 2), first.Bounds())
			require.Equal(t, int32(1), closed.Load())
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries, "temporary encoder files remain")
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(raw), temp)
			require.NotContains(t, string(raw), "fixture-token")
			for _, args := range []map[string]any{{"illust_id": 42, "pages": []int{1}}, {"illust_id": 42, "quality": "regular"}} {
				invalid := callTool(t, session, "pixiv_artwork_media", args)
				require.True(t, invalid.IsError)
				var rejected mediaOutput
				decodeStructured(t, invalid, &rejected)
				require.Equal(t, "static_parameters_not_applicable", rejected.Error)
			}
			require.Equal(t, int32(1), archiveReads.Load(), "invalid static parameters fetched animation bytes")
		})
	}
}

func TestArtworkMediaUgoiraFailureAndCancellationCleanTemporaryFiles(t *testing.T) {
	for _, mode := range []string{"corrupt", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			entered := make(chan struct{})
			var bodyClosed, leaseClosed atomic.Bool
			client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "app-api.pixiv.net" {
					body := `{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`
					if strings.Contains(r.URL.Path, "ugoira") {
						body = `{"ugoira_metadata":{"zip_urls":{"original":"https://i.pximg.net/a.zip"},"frames":[{"file":"0.png","delay":80}]}}`
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				}
				require.Equal(t, "i.pximg.net", r.URL.Host)
				var body io.ReadCloser = io.NopCloser(strings.NewReader("fixture-private-corrupt-zip"))
				if mode == "cancel" {
					body = &mediaWaitingBody{ctx: r.Context(), entered: entered, closed: &bodyClosed}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/zip"}}, Body: body}, nil
			})}})
			require.NoError(t, err)
			output := &syncBuffer{}
			logger, err := logging.New(output, "debug", "json")
			require.NoError(t, err)
			lifetime, stop := context.WithCancel(logging.WithCall(t.Context(), logger, "", ""))
			defer stop()
			protocol := newTestServer(pixivmcpserver.SDKPorts{Logger: logger, OpenLease: func(context.Context, pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				return lifecycle.NewLease(client, func() error { leaseClosed.Store(true); return nil }), nil
			}}, pixivmcpserver.Account{})
			ct, st := mcp.NewInMemoryTransports()
			go func() { _ = protocol.Run(lifetime, st) }()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "media-failure-observation-test", Version: "1"}, nil).Connect(lifetime, ct, nil)
			require.NoError(t, err)
			defer session.Close()
			if mode == "cancel" {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_artwork_media", Arguments: map[string]any{"illust_id": 42}})
				}()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("archive read did not start")
				}
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("canceled tool did not return")
				}
				require.Eventually(t, func() bool { return bodyClosed.Load() && leaseClosed.Load() }, 5*time.Second, time.Millisecond)
			} else {
				result := callTool(t, session, "pixiv_artwork_media", map[string]any{"illust_id": 42})
				require.True(t, result.IsError)
				var out mediaOutput
				decodeStructured(t, result, &out)
				require.False(t, out.Complete)
				require.Empty(t, out.DeliveredPages)
				raw, err := json.Marshal(result)
				require.NoError(t, err)
				require.NotContains(t, string(raw), temp)
				require.NotContains(t, string(raw), "fixture-private-corrupt-zip")
				require.True(t, leaseClosed.Load())
			}
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries, "failed/canceled encode left temporary files")
			var terminal map[string]any
			require.Eventually(t, func() bool {
				stages := mediaStages(t, output.String())
				terminal = stages["animation_generation"]
				return terminal != nil
			}, 5*time.Second, time.Millisecond)
			require.Equal(t, "WARN", terminal["level"])
			reason := "tool failed"
			if mode == "cancel" {
				reason = "request cancelled"
			}
			require.Equal(t, reason, terminal["reason"])
			require.Zero(t, terminal["count"], "failed work must not report a delivered animation")
			require.NotContains(t, output.String(), "fixture-private-corrupt-zip")
			require.NotContains(t, output.String(), "fixture-token")
			require.NotContains(t, output.String(), temp)

		})
	}
}

// mediaStages 按消息里的阶段名聚合本测试输出的 JSON 日志行。
func mediaStages(t *testing.T, output string) map[string]map[string]any {
	t.Helper()
	stages := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), line)
		if stage, ok := record[logging.AttrStage].(string); ok && stage != "mcp_http_write" && stage != "mcp_http_flush" {
			stages[stage] = record
		}
	}
	return stages
}

type bookmarkAggregateOutput struct {
	Records []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"records"`
	Pagination struct {
		Page     int  `json:"page"`
		Returned int  `json:"returned"`
		HasMore  bool `json:"has_more"`
		NextPage *int `json:"next_page"`
	} `json:"pagination"`
}

type bookmarkTagsAggregateOutput struct {
	Tags []struct {
		Name        string `json:"name"`
		Count       int    `json:"count"`
		ContentType string `json:"content_type"`
	} `json:"bookmark_tags"`
	Pagination struct {
		Returned int `json:"returned"`
	} `json:"pagination"`
}

func TestBookmarkListAllUsesArtworkThenNovelWithOneLogicalPageBudget(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks:      []pixiv.Artwork{testSDKIllust(11, "saved artwork", 90)},
		novelBookmarks: []pixiv.Novel{{ID: 22, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	first := callTool(t, session, "pixiv_bookmark_list_all", map[string]any{"user_id": 90, "limit": 1})
	if first.IsError {
		t.Fatalf("bookmark_list_all first page returned MCP error: %+v", first)
	}
	var firstOut bookmarkAggregateOutput
	decodeStructured(t, first, &firstOut)
	if len(firstOut.Records) != 1 || firstOut.Records[0].ID != "11" || firstOut.Records[0].Type != "illust" || !firstOut.Pagination.HasMore || firstOut.Pagination.NextPage == nil || *firstOut.Pagination.NextPage != 2 {
		t.Fatalf("first aggregate page=%+v, want artwork page with novel continuation", firstOut)
	}

	second := callTool(t, session, "pixiv_bookmark_list_all", map[string]any{"user_id": 90, "page": 2, "limit": 1})
	if second.IsError {
		t.Fatalf("bookmark_list_all second page returned MCP error: %+v", second)
	}
	var secondOut bookmarkAggregateOutput
	decodeStructured(t, second, &secondOut)
	if len(secondOut.Records) != 1 || secondOut.Records[0].ID != "22" || secondOut.Records[0].Type != "novel" || secondOut.Pagination.Returned != 1 || secondOut.Pagination.HasMore {
		t.Fatalf("second aggregate page=%+v, want one novel without duplicate artwork", secondOut)
	}

	if len(client.bookmarksRequests) == 0 || len(client.novelBookmarksRequests) == 0 {
		t.Fatalf("aggregate requests missing artwork=%v novel=%v", client.bookmarksRequests, client.novelBookmarksRequests)
	}
}
func TestBookmarkListAllFailureDoesNotExposePartialRecords(t *testing.T) {
	streamErr := errors.New("novel bookmark stream failed")
	client := &fakeSDKClient{
		bookmarks:         []pixiv.Artwork{testSDKIllust(41, "saved artwork", 90)},
		novelBookmarks:    []pixiv.Novel{{ID: 42, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
		novelBookmarksErr: streamErr,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_bookmark_list_all", map[string]any{"user_id": 90, "limit": 2})
	if !result.IsError || !resultHasText(result, "Error: pixiv:UserNovelBookmarks: upstream_error") {
		t.Fatalf("bookmark_list_all failure=%+v, want structured stream error", result)
	}
	var out bookmarkAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("failed aggregate output=%+v, want no partial records", out)
	}
}

func TestBookmarkTagsAllPreservesSameNameCountsAndContentType(t *testing.T) {
	client := &fakeSDKClient{
		bookmarkTagsPage:      sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "same", Count: 3}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "same", Count: 7}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_bookmark_tags_all", map[string]any{"user_id": 90, "limit": 0})
	if result.IsError {
		t.Fatalf("bookmark_tags_all returned MCP error: %+v", result)
	}
	var out bookmarkTagsAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Tags) != 2 || out.Pagination.Returned != 2 {
		t.Fatalf("aggregate tags output=%+v, want two typed tags", out)
	}
	if out.Tags[0].Name != "same" || out.Tags[0].Count != 3 || out.Tags[0].ContentType != "artwork" ||
		out.Tags[1].Name != "same" || out.Tags[1].Count != 7 || out.Tags[1].ContentType != "novel" {
		t.Fatalf("aggregate tags=%+v, want artwork then novel with independent counts", out.Tags)
	}
}

func TestBookmarkTagsAllFailureDoesNotExposePartialTags(t *testing.T) {
	streamErr := errors.New("novel bookmark tags failed")
	client := &fakeSDKClient{
		bookmarkTagsPage:      sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "art", Count: 3}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "novel", Count: 7}}},
		novelBookmarkTagsErr:  streamErr,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_bookmark_tags_all", map[string]any{"user_id": 90, "limit": 2})
	if !result.IsError || !resultHasText(result, "Error: pixiv:UserNovelBookmarkTags: upstream_error") {
		t.Fatalf("bookmark_tags_all failure=%+v, want structured stream error", result)
	}
	var out bookmarkTagsAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Tags) != 0 {
		t.Fatalf("failed aggregate tags output=%+v, want no partial tags", out)
	}
}
func TestMutationTypedErrorsAreMCPErrors(t *testing.T) {
	upstream := func(operation string) *sdk.Error {
		return &sdk.Error{Product: "pixiv", Operation: operation, Reason: sdk.UpstreamError, HTTPStatus: http.StatusBadGateway}
	}
	for _, test := range []struct {
		name  string
		tool  string
		args  map[string]any
		err   *sdk.Error
		check func(*testing.T, outputs.Mutation)
	}{
		{name: "add artwork bookmark", tool: "pixiv_bookmark_add", args: map[string]any{"kind": "artwork", "id": 41}, err: upstream("AddBookmark")},
		{name: "add novel bookmark", tool: "pixiv_bookmark_add", args: map[string]any{"kind": "novel", "id": 41}, err: upstream("AddNovelBookmark"),
			check: func(t *testing.T, out outputs.Mutation) {
				if out.NovelID != 41 {
					t.Fatalf("novel mutation output = %+v", out)
				}
			}},
		{name: "follow", tool: "pixiv_follow_user", args: map[string]any{"user_id": 8}, err: upstream("FollowUser")},
		{name: "unfollow", tool: "pixiv_unfollow_user", args: map[string]any{"user_id": 8}, err: upstream("UnfollowUser")},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeSDKClient{addBookmarkErr: test.err, addNovelBookmarkErr: test.err, followUserErr: test.err, unfollowUserErr: test.err}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			result := callTool(t, session, test.tool, test.args)
			if !result.IsError {
				t.Fatalf("typed SDK %s failure must be an MCP error: %+v", test.tool, result)
			}
			var out outputs.Mutation
			decodeStructured(t, result, &out)
			if out.Success || !strings.Contains(out.Text, "upstream_error") {
				t.Fatalf("structured %s mutation error = %+v", test.tool, out)
			}
			if test.check != nil {
				test.check(t, out)
			}
		})
	}
}

func TestSDKMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name     string
		args     map[string]any
		want     string
		wantText string
	}{
		{"pixiv_bookmark_add", map[string]any{"kind": "artwork", "id": 9, "restrict": "private", "tags": []string{"one"}}, "add_bookmark", "Bookmarked artwork 9."},
		{"pixiv_bookmark_remove", map[string]any{"kind": "artwork", "id": 9}, "remove_bookmark", "Removed bookmark from artwork 9."},
		{"pixiv_bookmark_add", map[string]any{"kind": "novel", "id": 9, "restrict": "private", "tags": []string{"one"}}, "add_novel_bookmark", "Bookmarked novel 9."},
		{"pixiv_bookmark_remove", map[string]any{"kind": "novel", "id": 9}, "remove_novel_bookmark", "Removed bookmark from novel 9."},
		{"pixiv_follow_user", map[string]any{"user_id": 8, "restrict": "private"}, "follow_user", "Followed user 8."},
		{"pixiv_unfollow_user", map[string]any{"user_id": 8}, "unfollow_user", "Unfollowed user 8."},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			var out outputs.Mutation
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.want || out.Text != test.wantText {
				t.Fatalf("mutation output = %+v", out)
			}
			if strings.Contains(test.name, "novel") && out.NovelID != 9 {
				t.Fatalf("novel mutation output = %+v", out)
			}
		})
	}
	if client.addBookmarkRequest.ArtworkID != 9 || client.addBookmarkRequest.Restrict != pixiv.RestrictPrivate || !slices.Equal(client.addBookmarkRequest.Tags, []string{"one"}) {
		t.Fatalf("add bookmark request = %+v", client.addBookmarkRequest)
	}
	if client.removeBookmarkRequest.ArtworkID != 9 || client.followUserRequest.UserID != 8 || client.followUserRequest.Restrict != pixiv.RestrictPrivate || client.unfollowUserRequest.UserID != 8 {
		t.Fatalf("mutation requests = remove=%+v follow=%+v unfollow=%+v", client.removeBookmarkRequest, client.followUserRequest, client.unfollowUserRequest)
	}
	if client.addNovelBookmarkRequest.NovelID != 9 || client.addNovelBookmarkRequest.Restrict != pixiv.RestrictPrivate || !slices.Equal(client.addNovelBookmarkRequest.Tags, []string{"one"}) || client.removeNovelBookmarkRequest.NovelID != 9 {
		t.Fatalf("novel mutation requests = add=%+v remove=%+v", client.addNovelBookmarkRequest, client.removeNovelBookmarkRequest)
	}
}

// unsafeOutputPropertyNames are JSON property names that would expose request
// transport or credential material to an MCP client. A tool output may carry
// an opaque resource reference, never the headers, cookies, signed locator or
// expiry needed to replay a request outside the SDK.
var unsafeOutputPropertyNames = map[string]struct{}{
	"cookie":          {},
	"cookies":         {},
	"expiry":          {},
	"expires_at":      {},
	"headers":         {},
	"locator":         {},
	"request_headers": {},
	"refresh_token":   {},
	"access_token":    {},
	"token":           {},
}

// TestEveryToolOutputSchemaOmitsTransportAndCredentialFields walks the output
// schema of every registered Pixiv tool as an MCP client sees it and rejects
// any property that would hand a client replayable transport state.
//
// This asserts against the live registry rather than source text, so adding a
// tool, renaming its output type, or moving it to another package cannot slip
// past the check. A tool without an output schema is reported too: an
// unschematized output is exactly the case where an unsafe field would go
// unnoticed.
func TestEveryToolOutputSchemaOmitsTransportAndCredentialFields(t *testing.T) {
	tools := connectAndListTools(t)
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	violations := unsafeSchemaProperties(t, tools)
	if len(violations) > 0 {
		t.Fatalf("tool output schemas expose transport or credential fields:\n  %s", strings.Join(violations, "\n  "))
	}
}
func unsafeSchemaProperties(t *testing.T, tools []*mcp.Tool) []string {
	t.Helper()
	violations := make([]string, 0)
	for _, tool := range tools {
		if tool.OutputSchema == nil {
			violations = append(violations, tool.Name+": no output schema")
			continue
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", tool.Name, err)
		}
		var schema any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s output schema: %v", tool.Name, err)
		}
		for _, path := range unsafeSchemaKeys(schema, "") {
			violations = append(violations, tool.Name+": "+path)
		}
	}
	sort.Strings(violations)
	return violations
}

// unsafeSchemaKeys reports every object key in the decoded schema whose name is
// forbidden, at any depth. Walking raw keys rather than only `properties` maps
// keeps the check independent of which schema keywords the generator emits, so
// a nested `$defs`, `items`, `anyOf` or vendor extension cannot hide a field.
// JSON Schema keywords themselves are never in the forbidden set, so this
// cannot produce a false positive from schema structure alone.
func unsafeSchemaKeys(node any, path string) []string {
	found := make([]string, 0)
	switch value := node.(type) {
	case map[string]any:
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child := path + "/" + name
			if _, unsafe := unsafeOutputPropertyNames[strings.ToLower(name)]; unsafe {
				found = append(found, child)
			}
			found = append(found, unsafeSchemaKeys(value[name], child)...)
		}
	case []any:
		for index, child := range value {
			found = append(found, unsafeSchemaKeys(child, path+"/"+strconv.Itoa(index))...)
		}
	}
	return found
}

// toolSchemas 是每个注册 tool 的输入/输出 schema 外形：字段集合与 required 列表。
// 同族 tool 共用 schema 构造器，所以只在这张表断言一次，不在各读取族重复。
type toolSchemaShape struct {
	inputFields, inputRequired, outputFields, outputRequired []string
}

var toolSchemas = map[string]toolSchemaShape{
	"pixiv_artwork_media": {
		inputFields: []string{"animation_format", "illust_id", "pages", "quality"}, inputRequired: []string{"illust_id"},
		outputFields: []string{"complete", "delivered_pages", "error", "failures", "illust_id", "pages", "requested_pages", "title", "total_pages"}, outputRequired: []string{"complete", "delivered_pages", "failures", "illust_id", "pages", "requested_pages", "title", "total_pages"},
	},
	"pixiv_blocked_users": {
		inputFields: []string{"limit", "page", "restrict", "user_id"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_bookmark_add": {
		inputFields: []string{"id", "kind", "restrict", "tags"}, inputRequired: []string{"id", "kind"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_bookmark_detail": {
		inputFields: []string{"illust_id"}, inputRequired: []string{"illust_id"},
		outputFields: []string{"bookmarked", "restrict", "tags"}, outputRequired: []string{"bookmarked", "tags"},
	},
	"pixiv_bookmark_list_all": {
		inputFields: []string{"limit", "page", "restrict", "tag", "user_id"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_bookmark_remove": {
		inputFields: []string{"id", "kind"}, inputRequired: []string{"id", "kind"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_bookmark_tags": {
		inputFields: []string{"kind", "limit", "page", "restrict", "user_id"}, inputRequired: []string{},
		outputFields: []string{"bookmark_tags", "pagination"}, outputRequired: []string{"bookmark_tags", "pagination"},
	},
	"pixiv_bookmark_tags_all": {
		inputFields: []string{"limit", "page", "restrict", "user_id"}, inputRequired: []string{},
		outputFields: []string{"bookmark_tags", "pagination"}, outputRequired: []string{"bookmark_tags", "pagination"},
	},
	"pixiv_comment_create": {
		inputFields: []string{"comment", "id", "kind"}, inputRequired: []string{"comment", "id", "kind"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_comment_delete": {
		inputFields: []string{"comment_id", "kind"}, inputRequired: []string{"comment_id", "kind"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_comment_reply": {
		inputFields: []string{"comment", "id", "kind", "parent_comment_id"}, inputRequired: []string{"comment", "id", "kind", "parent_comment_id"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_comment_stamp": {
		inputFields: []string{"comment", "id", "kind", "stamp_id"}, inputRequired: []string{"id", "kind", "stamp_id"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_comments": {
		inputFields: []string{"id", "kind", "limit", "page"}, inputRequired: []string{"id", "kind"},
		outputFields: []string{"access_control", "comments", "pagination", "total"}, outputRequired: []string{"comments", "pagination"},
	},
	"pixiv_follow_user": {
		inputFields: []string{"restrict", "user_id"}, inputRequired: []string{"user_id"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_illust_detail": {
		inputFields: []string{"illust_id", "url"}, inputRequired: []string{},
		outputFields: []string{"records"}, outputRequired: []string{"records"},
	},
	"pixiv_illust_ranking": {
		inputFields: []string{"cursor", "date", "illust_filter", "limit", "mode", "page"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_illust_recommended": {
		inputFields: []string{"cursor", "illust_filter", "limit", "page"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_illust_related": {
		inputFields: []string{"cursor", "illust_filter", "illust_id", "limit", "page"}, inputRequired: []string{"illust_id"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_illust_series": {
		inputFields: []string{"limit", "page", "series_id"}, inputRequired: []string{"series_id"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_mypixiv": {
		inputFields: []string{"illust_filter", "kind", "limit", "novel_filter", "page", "user_filter"}, inputRequired: []string{"kind"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_novel_bookmark_detail": {
		inputFields: []string{"novel_id"}, inputRequired: []string{"novel_id"},
		outputFields: []string{"bookmarked", "restrict", "tags"}, outputRequired: []string{"bookmarked", "tags"},
	},
	"pixiv_novel_content": {
		inputFields: []string{"novel_id"}, inputRequired: []string{"novel_id"},
		outputFields: []string{"content"}, outputRequired: []string{"content"},
	},
	"pixiv_novel_detail": {
		inputFields: []string{"novel_id"}, inputRequired: []string{"novel_id"},
		outputFields: []string{"records"}, outputRequired: []string{"records"},
	},
	"pixiv_novel_series": {
		inputFields: []string{"limit", "page", "series_id"}, inputRequired: []string{"series_id"},
		outputFields: []string{"pagination", "records", "series"}, outputRequired: []string{"pagination", "records", "series"},
	},
	"pixiv_recommended": {
		inputFields: []string{"cursor", "illust_filter", "kind", "limit", "novel_filter", "page", "user_filter"}, inputRequired: []string{"kind"},
		outputFields: []string{"pagination", "records"}, outputRequired: []string{"pagination", "records"},
	},
	"pixiv_reverse_search": {
		inputFields: []string{"image", "provider", "source"}, inputRequired: []string{},
		outputFields: []string{"input", "partial", "provider_errors", "providers", "records", "results"}, outputRequired: []string{"input", "partial", "provider_errors", "providers", "records", "results"},
	},
	"pixiv_search_illust": {
		inputFields: []string{"ai_mode", "aspect_ratio", "bookmark_max", "bookmark_min", "bookmark_strategy", "content_type", "cursor", "duration", "end_date", "illust_filter", "limit", "page", "resolution", "search_target", "sort", "start_date", "tool", "word"}, inputRequired: []string{"word"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_search_novel": {
		inputFields: []string{"duration", "limit", "novel_filter", "page", "search_target", "sort", "word"}, inputRequired: []string{"word"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_search_user": {
		inputFields: []string{"limit", "page", "user_filter", "word"}, inputRequired: []string{"word"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_timeline_following": {
		inputFields: []string{"illust_filter", "kind", "limit", "novel_filter", "page", "restrict"}, inputRequired: []string{"kind"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_timeline_latest": {
		inputFields: []string{"content_type", "illust_filter", "limit", "novel_filter", "page"}, inputRequired: []string{"content_type"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_trending_tags_illust": {
		inputFields: []string{}, inputRequired: []string{},
		outputFields: []string{"tags", "text"}, outputRequired: []string{"tags", "text"},
	},
	"pixiv_unfollow_user": {
		inputFields: []string{"user_id"}, inputRequired: []string{"user_id"},
		outputFields: []string{"action", "comment_id", "illust_id", "novel_id", "success", "text", "user_id"}, outputRequired: []string{"action", "success", "text"},
	},
	"pixiv_user_artworks": {
		inputFields: []string{"illust_filter", "limit", "page", "type", "user_id"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_user_bookmarks": {
		inputFields: []string{"illust_filter", "kind", "limit", "page", "restrict", "tag", "user_id"}, inputRequired: []string{"kind"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_user_detail": {
		inputFields: []string{"user_id"}, inputRequired: []string{"user_id"},
		outputFields: []string{"records"}, outputRequired: []string{"records"},
	},
	"pixiv_user_novels": {
		inputFields: []string{"limit", "novel_filter", "page", "user_id"}, inputRequired: []string{},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
	"pixiv_user_relations": {
		inputFields: []string{"limit", "page", "relation", "restrict", "user_filter", "user_id"}, inputRequired: []string{"relation"},
		outputFields: []string{"access_control", "bookmark_tags", "bookmarked", "comments", "content", "filter", "pagination", "records", "restrict", "series", "tags", "total"}, outputRequired: []string{"records"},
	},
}

// TestRegisteredToolSchemasMatchContract 用一张表覆盖全部注册 tool 的 schema 外形；
// 表行数与注册数必须一致，新增或删除 tool 时无法悄悄跳过。
func TestRegisteredToolSchemasMatchContract(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if len(byName) != len(toolSchemas) {
		t.Fatalf("registered tools=%d, table rows=%d", len(byName), len(toolSchemas))
	}
	for name, want := range toolSchemas {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("tool %q is not registered", name)
		}
		input := feedSchemaObject(t, name+" input", tool.InputSchema)
		assertSchemaFields(t, input, want.inputFields)
		assertSchemaRequired(t, input, want.inputRequired)
		output := feedSchemaObject(t, name+" output", tool.OutputSchema)
		assertSchemaFields(t, output, want.outputFields)
		assertSchemaRequired(t, output, want.outputRequired)
	}
}

func connectAndListTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	server := newTestServer(pixivmcpserver.SDKPorts{}, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools := make([]*mcp.Tool, 0)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

const (
	// canaryURL 必须通过 SDK 的 resource URL policy（https + i.pximg.net host），
	// 同时在 query 里携带 signature canary；handler 若把原始 URL 复制进输出，
	// 就一定会带上 query。
	canaryURL    = "https://i.pximg.net/artworks/101/1.png?signature=topsecret"
	canaryCookie = "canary-session=topsecret"
	canaryAuth   = "Bearer canary-token"
)

func canaryStrings() []string {
	return []string{canaryURL, canaryCookie, canaryAuth, "topsecret"}
}

func resourceWithCanary() sdk.Resource {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"kind":"artwork-cover","id":101}`))
	if err != nil {
		panic(err)
	}
	return sdk.Resource{
		Ref:            ref,
		URL:            canaryURL,
		RequestHeaders: map[string]string{"Cookie": canaryCookie, "Authorization": canaryAuth},
	}
}

func assertNoCanaryLeak(t *testing.T, tool string, result *mcp.CallToolResult) {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	joined := strings.ToLower(string(raw))
	for _, c := range canaryStrings() {
		if strings.Contains(joined, strings.ToLower(c)) {
			t.Fatalf("tool %s structured output leaked canary %q: %s", tool, c, raw)
		}
	}
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		lower := strings.ToLower(text.Text)
		for _, c := range canaryStrings() {
			if strings.Contains(lower, strings.ToLower(c)) {
				t.Fatalf("tool %s text output leaked canary %q: %s", tool, c, text.Text)
			}
		}
	}
}

// TestToolOutputValuesDoNotLeakResourceTransportCanary 覆盖资源/实体读取路径：
// fake SDK 返回的 artwork/bookmark cover 携带签名 URL 与 Cookie，handler 必须只
// 输出 opaque resource ref，不能把 URL/Cookie 复制进结构化输出或文本摘要。
func TestToolOutputValuesDoNotLeakResourceTransportCanary(t *testing.T) {
	client := &fakeSDKClient{
		userID: 7,
		artworks: []pixiv.Artwork{{
			ID:    101,
			Title: "canary-art",
			Kind:  pixiv.ArtworkKindIllustration,
			User:  pixiv.User{ID: 7, Name: "artist"},
			Cover: pixiv.ImageResource{Resource: resourceWithCanary()},
		}},
		bookmarks: []pixiv.Artwork{{
			ID:    102,
			Title: "canary-bookmark",
			Kind:  pixiv.ArtworkKindIllustration,
			User:  pixiv.User{ID: 7, Name: "artist"},
			Cover: pixiv.ImageResource{Resource: resourceWithCanary()},
		}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"pixiv_user_artworks", map[string]any{"user_id": 7, "limit": 1}},
		{"pixiv_user_bookmarks", map[string]any{"kind": "artwork", "user_id": 7, "limit": 1}},
	} {
		result := callTool(t, session, tc.tool, tc.args)
		if result.IsError {
			t.Fatalf("tool %s failed: %+v", tc.tool, result)
		}
		assertNoCanaryLeak(t, tc.tool, result)
	}
}

// TestToolErrorOutputDoesNotLeakCanary 覆盖错误路径：SDK 返回带 canary 的上游
// 错误文本时，MCP error result 的 text 与 structured 输出都不能原样带上它。
func TestToolErrorOutputDoesNotLeakCanary(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{}, fmt.Errorf("upstream refused %s %s", canaryURL, canaryAuth)
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_search_illust", map[string]any{"word": "canary"})
	if !result.IsError {
		t.Fatalf("search must fail: %+v", result)
	}
	assertNoCanaryLeak(t, "search_illust(error)", result)
}

// illust_ranking 的 owner 契约：请求映射与全部 mode。

func TestConsolidatedToolsRejectInvalidInputsBeforeAccountAccess(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{"pixiv_comments", map[string]any{"id": 1}},
		{"pixiv_comments", map[string]any{"kind": "all", "id": 1}},
		{"pixiv_comment_create", map[string]any{"kind": "novel", "id": 1, "comment": ""}},
		{"pixiv_bookmark_add", map[string]any{"kind": "artwork", "id": 0}},
		{"pixiv_bookmark_remove", map[string]any{"kind": "artwork", "id": 1, "restrict": "private"}},
		{"pixiv_user_bookmarks", map[string]any{}},
		{"pixiv_user_bookmarks", map[string]any{"kind": "all"}},
		{"pixiv_user_bookmarks", map[string]any{"kind": "novel", "illust_filter": map[string]any{}}},
		{"pixiv_user_bookmarks", map[string]any{"kind": "artwork", "page": 2}},
		{"pixiv_user_relations", map[string]any{}},
		{"pixiv_user_relations", map[string]any{"relation": "blocked"}},
		{"pixiv_user_relations", map[string]any{"relation": "followers", "user_filter": map[string]any{}}},
		{"pixiv_timeline_following", map[string]any{}},
		{"pixiv_timeline_following", map[string]any{"kind": "artwork", "novel_filter": map[string]any{}}},
		{"pixiv_timeline_following", map[string]any{"kind": "novel", "illust_filter": map[string]any{}}},
		{"pixiv_timeline_latest", map[string]any{}},
		{"pixiv_timeline_latest", map[string]any{"content_type": "artwork"}},
		{"pixiv_timeline_latest", map[string]any{"content_type": "novel", "illust_filter": map[string]any{}}},
		{"pixiv_timeline_latest", map[string]any{"content_type": "manga", "novel_filter": map[string]any{}}},
		{"pixiv_mypixiv", map[string]any{}},
		{"pixiv_mypixiv", map[string]any{"kind": "all"}},
		{"pixiv_mypixiv", map[string]any{"kind": "user", "illust_filter": map[string]any{}}},
		{"pixiv_mypixiv", map[string]any{"kind": "artwork", "novel_filter": map[string]any{}}},
		{"pixiv_mypixiv", map[string]any{"kind": "novel", "user_filter": map[string]any{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ports := testSDKPorts(t, &fakeSDKClient{})
			accesses := 0
			ports.ResolveAccount = func(context.Context, pixivmcpserver.Account) (pixivmcpserver.Account, error) {
				accesses++
				return pixivmcpserver.Account{}, errors.New("unexpected account access")
			}
			session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
			defer closeSession()
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid input accepted: %+v", result)
			}
			if accesses != 0 {
				t.Fatalf("invalid input accessed account %d times", accesses)
			}
		})
	}
}

type commentMutationOutput struct {
	Success   bool   `json:"success"`
	Action    string `json:"action"`
	IllustID  int64  `json:"illust_id,omitempty"`
	NovelID   int64  `json:"novel_id,omitempty"`
	CommentID int64  `json:"comment_id,omitempty"`
	Text      string `json:"text"`
}

func TestArtworkCommentMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name          string
		args          map[string]any
		wantAction    string
		wantText      string
		wantCommentID int64
	}{
		{
			name:          "pixiv_comment_create",
			args:          map[string]any{"kind": "novel", "id": 9, "comment": "hello"},
			wantAction:    "create_novel_comment",
			wantText:      "Created comment on novel 9.",
			wantCommentID: 901,
		},
		{
			name:          "pixiv_comment_create",
			args:          map[string]any{"kind": "artwork", "id": 9, "comment": "hello"},
			wantAction:    "create_artwork_comment",
			wantText:      "Created comment on artwork 9.",
			wantCommentID: 901,
		},
		{
			name:          "pixiv_comment_reply",
			args:          map[string]any{"kind": "novel", "id": 9, "comment": "reply", "parent_comment_id": 11},
			wantAction:    "reply_novel_comment",
			wantText:      "Replied to comment 11 on novel 9.",
			wantCommentID: 902,
		},
		{
			name:          "pixiv_comment_stamp",
			args:          map[string]any{"kind": "novel", "id": 9, "comment": "stamp", "stamp_id": 7},
			wantAction:    "stamp_novel_comment",
			wantText:      "Added stamp comment to novel 9.",
			wantCommentID: 903,
		},
		{
			name:          "pixiv_comment_delete",
			args:          map[string]any{"kind": "novel", "comment_id": 12},
			wantAction:    "delete_novel_comment",
			wantText:      "Deleted comment 12.",
			wantCommentID: 12,
		},
		{
			name:          "pixiv_comment_reply",
			args:          map[string]any{"kind": "artwork", "id": 9, "comment": "reply", "parent_comment_id": 11},
			wantAction:    "reply_artwork_comment",
			wantText:      "Replied to comment 11 on artwork 9.",
			wantCommentID: 902,
		},
		{
			name:          "pixiv_comment_stamp",
			args:          map[string]any{"kind": "artwork", "id": 9, "comment": "stamp", "stamp_id": 7},
			wantAction:    "stamp_artwork_comment",
			wantText:      "Added stamp comment to artwork 9.",
			wantCommentID: 903,
		},
		{
			name:          "pixiv_comment_delete",
			args:          map[string]any{"kind": "artwork", "comment_id": 12},
			wantAction:    "delete_artwork_comment",
			wantText:      "Deleted comment 12.",
			wantCommentID: 12,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			if result.IsError {
				t.Fatalf("mutation failed: %+v", result)
			}
			var out commentMutationOutput
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.wantAction || out.Text != test.wantText || out.CommentID != test.wantCommentID {
				t.Fatalf("mutation output = %+v, want action=%q text=%q comment_id=%d", out, test.wantAction, test.wantText, test.wantCommentID)
			}
			if test.args["kind"] == "novel" && test.name != "pixiv_comment_delete" {
				if out.NovelID != 9 {
					t.Fatalf("novel mutation output = %+v", out)
				}
			} else if test.args["kind"] != "novel" && out.IllustID != 9 && test.name != "pixiv_comment_delete" {
				t.Fatalf("artwork mutation output = %+v", out)
			}
		})
	}

	if client.createArtworkCommentRequest != (pixiv.PostArtworkCommentRequest{ArtworkID: 9, Comment: "hello"}) {
		t.Fatalf("create artwork comment request = %+v", client.createArtworkCommentRequest)
	}
	if client.replyArtworkCommentRequest != (pixiv.ReplyArtworkCommentRequest{ArtworkID: 9, Comment: "reply", ParentCommentID: 11}) {
		t.Fatalf("reply artwork comment request = %+v", client.replyArtworkCommentRequest)
	}
	if client.stampArtworkCommentRequest != (pixiv.StampArtworkCommentRequest{ArtworkID: 9, Comment: "stamp", StampID: 7}) {
		t.Fatalf("stamp artwork comment request = %+v", client.stampArtworkCommentRequest)
	}
	if client.deleteArtworkCommentRequest != (pixiv.DeleteArtworkCommentRequest{CommentID: 12}) {
		t.Fatalf("delete artwork comment request = %+v", client.deleteArtworkCommentRequest)
	}
	if client.createNovelCommentRequest != (pixiv.PostNovelCommentRequest{NovelID: 9, Comment: "hello"}) {
		t.Fatalf("create novel comment request = %+v", client.createNovelCommentRequest)
	}
	if client.replyNovelCommentRequest != (pixiv.ReplyNovelCommentRequest{NovelID: 9, Comment: "reply", ParentCommentID: 11}) {
		t.Fatalf("reply novel comment request = %+v", client.replyNovelCommentRequest)
	}
	if client.stampNovelCommentRequest != (pixiv.StampNovelCommentRequest{NovelID: 9, Comment: "stamp", StampID: 7}) {
		t.Fatalf("stamp novel comment request = %+v", client.stampNovelCommentRequest)
	}
	if client.deleteNovelCommentRequest != (pixiv.DeleteNovelCommentRequest{CommentID: 12}) {
		t.Fatalf("delete novel comment request = %+v", client.deleteNovelCommentRequest)
	}
}

func TestArtworkCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay(t *testing.T) {
	client := &fakeSDKClient{createArtworkCommentErr: &sdk.Error{
		Product:    "pixiv",
		Operation:  "PostArtworkComment",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_comment_create", map[string]any{"kind": "artwork", "id": 41, "comment": "uncertain"})
	if !result.IsError {
		t.Fatalf("typed artwork comment failure must be an MCP error: %+v", result)
	}
	var out commentMutationOutput
	decodeStructured(t, result, &out)
	if out.Success || out.IllustID != 41 || out.CommentID != 0 || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured artwork comment error = %+v", out)
	}
	if client.artworkCommentWireCalls != 1 {
		t.Fatalf("uncertain artwork comment mutation replayed %d times", client.artworkCommentWireCalls)
	}
}
func TestArtworkCommentMutationRejectsInvalidInputBeforeSDKExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{name: "pixiv_comment_create artwork", args: map[string]any{"kind": "artwork", "id": 0, "comment": "hello"}},
		{name: "pixiv_comment_create novel", args: map[string]any{"kind": "novel", "id": 0, "comment": "hello"}},
		{name: "pixiv_comment_reply artwork", args: map[string]any{"kind": "artwork", "id": 9, "comment": "reply", "parent_comment_id": 0}},
		{name: "pixiv_comment_reply novel", args: map[string]any{"kind": "novel", "id": 9, "comment": "reply", "parent_comment_id": 0}},
		{name: "pixiv_comment_stamp artwork", args: map[string]any{"kind": "artwork", "id": 9, "comment": "stamp", "stamp_id": 0}},
		{name: "pixiv_comment_stamp novel", args: map[string]any{"kind": "novel", "id": 9, "comment": "stamp", "stamp_id": 0}},
		{name: "pixiv_comment_delete artwork", args: map[string]any{"kind": "artwork", "comment_id": 0}},
		{name: "pixiv_comment_delete novel", args: map[string]any{"kind": "novel", "comment_id": 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeSDKClient{}
			ports := testSDKPorts(t, client)
			executions := 0
			baseExecute := ports.Execute
			ports.Execute = func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				executions++
				return baseExecute(ctx, account, attempt)
			}
			session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.name, Arguments: test.args})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid artwork comment input result=%+v err=%v executions=%d", result, err, executions)
			}
			if executions != 0 || client.artworkCommentWireCalls != 0 || client.novelCommentWireCalls != 0 {
				t.Fatalf("invalid comment input reached SDK: executions=%d artwork_wire=%d novel_wire=%d result=%+v err=%v", executions, client.artworkCommentWireCalls, client.novelCommentWireCalls, result, err)
			}
		})
	}
}

// TestDefaultBatchContinuation 是默认批次续读的失败模式表：一个逻辑批次只读一次上游，
// 返回 opaque cursor；带 cursor 的下一批返回剩余项，非法 cursor 组合在进入 SDK 前被拒。
func TestDefaultBatchContinuation(t *testing.T) {
	for _, test := range []struct {
		name   string
		tool   string
		args   map[string]any
		client func(page func(sdk.Cursor) (sdk.Page[pixiv.Artwork], error)) *fakeSDKClient
		shape  any
	}{
		{name: "related", tool: "pixiv_illust_related", args: map[string]any{"illust_id": 9},
			client: func(page func(sdk.Cursor) (sdk.Page[pixiv.Artwork], error)) *fakeSDKClient {
				return &fakeSDKClient{userID: 7, relatedArtworks: func(_ context.Context, req pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					return page(req.Cursor)
				}}
			}},
		{name: "ranking", tool: "pixiv_illust_ranking", args: map[string]any{},
			client: func(page func(sdk.Cursor) (sdk.Page[pixiv.Artwork], error)) *fakeSDKClient {
				return &fakeSDKClient{userID: 7, illustRanking: func(_ context.Context, req pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
					return page(req.Cursor)
				}}
			}},
		{name: "illust recommended", tool: "pixiv_illust_recommended", args: map[string]any{},
			client: func(page func(sdk.Cursor) (sdk.Page[pixiv.Artwork], error)) *fakeSDKClient {
				return &fakeSDKClient{userID: 7, recommendedArtworks: func(_ context.Context, req pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
					return page(req.Cursor)
				}}
			}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			page := func(cursor sdk.Cursor) (sdk.Page[pixiv.Artwork], error) {
				calls++
				if cursor.IsZero() {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(201, "first", 7)}, Next: testPageCursor(1)}, nil
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(202, "second", 7)}}, nil
			}
			session, closeSession := newSDKTestSession(t, test.client(page))
			defer closeSession()

			first := callTool(t, session, test.tool, test.args)
			var out outputs.Records
			decodeStructured(t, first, &out)
			if first.IsError || out.Pagination.NextCursor == "" || calls != 1 {
				t.Fatalf("missing default batch continuation: error=%v cursor=%q calls=%d", first.IsError, out.Pagination.NextCursor, calls)
			}
			cursor := out.Pagination.NextCursor
			secondArgs := map[string]any{}
			for key, value := range test.args {
				secondArgs[key] = value
			}
			secondArgs["cursor"] = cursor
			second := callTool(t, session, test.tool, secondArgs)
			out = outputs.Records{}
			decodeStructured(t, second, &out)
			if second.IsError || calls != 2 || len(out.Records) != 1 || out.Records[0].ID() != "202" || out.Pagination.NextCursor != "" || out.Pagination.HasMore {
				t.Fatalf("second batch error=%v calls=%d output=%+v", second.IsError, calls, out)
			}
			for _, args := range []map[string]any{{"cursor": cursor, "limit": 1}, {"cursor": cursor, "illust_filter": map[string]any{"min_pages": 2}}, {"cursor": "bad"}} {
				merged := map[string]any{}
				for key, value := range test.args {
					merged[key] = value
				}
				for key, value := range args {
					merged[key] = value
				}
				if r := callTool(t, session, test.tool, merged); !r.IsError {
					t.Fatalf("invalid continuation accepted for args=%v", merged)
				}
			}
			if calls != 2 {
				t.Fatalf("invalid cursor reached data API: %d", calls)
			}
		})
	}
}

func TestSearchDefaultBatchContinuation(t *testing.T) {
	for _, strategy := range []string{"", "auto", "local", "best_effort"} {
		t.Run("strategy="+strategy, func(t *testing.T) {
			calls := 0
			client := &fakeSDKClient{userID: 7, searchIllust: func(_ context.Context, r pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				calls++
				item := testSDKIllust(301, "first", 7)
				item.TotalBookmarks = 25
				item.PageCount = 2
				if r.Cursor.IsZero() {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}, Next: testPageCursor(1)}, nil
				}
				item.ID = 302
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}}, nil
			}}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()
			args := map[string]any{"word": "cats", "illust_filter": map[string]any{"min_pages": 2}}
			if strategy != "" {
				args["bookmark_min"] = 20
				args["bookmark_strategy"] = strategy
			}
			first := callTool(t, session, "pixiv_search_illust", args)
			var out outputs.Records
			decodeStructured(t, first, &out)
			if first.IsError || out.Pagination.NextCursor == "" || calls != 1 {
				t.Fatalf("missing search continuation error=%v calls=%d", first.IsError, calls)
			}
			cursor := out.Pagination.NextCursor
			args["cursor"] = cursor
			second := callTool(t, session, "pixiv_search_illust", args)
			out = outputs.Records{}
			decodeStructured(t, second, &out)
			if second.IsError || calls != 2 || len(out.Records) != 1 || out.Records[0].ID() != "302" || out.Pagination.HasMore || out.Pagination.NextCursor != "" {
				t.Fatalf("search second batch error=%v output=%+v calls=%d", second.IsError, out, calls)
			}
			wantCompleteness := "complete_for_source"
			if strategy == "best_effort" {
				wantCompleteness = "partial"
			}
			if strategy != "" && (out.Filter == nil || out.Filter.Completeness != wantCompleteness) {
				t.Fatalf("filter metadata lost: %+v", out.Filter)
			}
			args["illust_filter"] = map[string]any{"min_pages": 3}
			if r := callTool(t, session, "pixiv_search_illust", args); !r.IsError {
				t.Fatal("changed structured filter accepted")
			}
			args["illust_filter"] = map[string]any{"min_pages": 2}
			args["bookmark_strategy"] = "server"
			if r := callTool(t, session, "pixiv_search_illust", args); !r.IsError {
				t.Fatal("changed bookmark strategy accepted")
			}
			if strategy == "" {
				delete(args, "bookmark_strategy")
			} else {
				args["bookmark_strategy"] = strategy
			}
			args["word"] = "dogs"
			if r := callTool(t, session, "pixiv_search_illust", args); !r.IsError {
				t.Fatal("changed query accepted")
			}
			args["word"] = "cats"
			args["limit"] = 1
			if r := callTool(t, session, "pixiv_search_illust", args); !r.IsError {
				t.Fatal("cursor plus limit accepted")
			}
			if calls != 2 {
				t.Fatalf("invalid cursor reached data API: %d", calls)
			}
		})
	}
}

// TestInvalidInputsRejectBeforeSDKExecution 是输入校验的失败模式表：推荐过滤器
// 互相冲突、ranking 的 mode/date 非法时都必须直接返回结构化错误，不进入 SDK。
func TestInvalidInputsRejectBeforeSDKExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "recommended illust rejects novel filter", tool: "pixiv_recommended", args: map[string]any{"kind": "illust", "novel_filter": map[string]any{"id": 201}}},
		{name: "recommended manga rejects conflicting artwork type", tool: "pixiv_recommended", args: map[string]any{"kind": "manga", "illust_filter": map[string]any{"type": "illust"}}},
		{name: "recommended novel rejects user filter", tool: "pixiv_recommended", args: map[string]any{"kind": "novel", "user_filter": map[string]any{"id": 401}}},
		{name: "recommended user rejects artwork filter", tool: "pixiv_recommended", args: map[string]any{"kind": "user", "illust_filter": map[string]any{"type": "illust"}}},
		{name: "illust detail both references", tool: "pixiv_illust_detail", args: map[string]any{"illust_id": 42, "url": "https://www.pixiv.net/artworks/42"}},
		{name: "ranking unknown mode", tool: "pixiv_illust_ranking", args: map[string]any{"mode": "not-a-ranking"}},
		{name: "ranking invalid calendar date", tool: "pixiv_illust_ranking", args: map[string]any{"date": "2025-02-30"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var executions int
			ports := testSDKPorts(t, &fakeSDKClient{})
			baseExecute := ports.Execute
			ports.Execute = func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				executions++
				return baseExecute(ctx, account, attempt)
			}
			session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
			defer closeSession()

			result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.args})
			if callErr == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid input unexpectedly succeeded: result=%+v err=%v", result, callErr)
			}
			if executions != 0 {
				t.Fatalf("invalid input entered SDK execution: %d", executions)
			}
		})
	}
}

func feedSchemaObject(t *testing.T, name string, raw any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal %s schema: %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("decode %s schema: %v", name, err)
	}
	if schema["type"] != "object" {
		t.Fatalf("%s schema type=%#v", name, schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("%s schema must be closed: %#v", name, schema["additionalProperties"])
	}
	return schema
}
func assertSchemaFields(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%#v", schema["properties"])
	}
	got := make([]string, 0, len(properties))
	for field := range properties {
		got = append(got, field)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("schema fields=%v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("schema fields=%v, want %v", got, want)
		}
	}
}

func assertSchemaRequired(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	encoded, err := json.Marshal(schema["required"])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if string(encoded) != "null" {
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("schema required=%v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("schema required=%v, want %v", got, want)
		}
	}
}
func TestMixedRecommendedDefaultCursors(t *testing.T) {
	calls := map[string]int{}
	client := &fakeSDKClient{userID: 7,
		recommendedArtworks: func(_ context.Context, r pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
			calls["art"]++
			a := testSDKIllust(1, "art", 7)
			m := testSDKIllust(2, "manga", 7)
			m.Kind = pixiv.ArtworkKindManga
			var next sdk.Cursor
			if r.Cursor.IsZero() {
				next = testPageCursor(1)
			} else {
				a.ID = 11
				m.ID = 12
			}
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{a, m}, Next: next}, nil
		},
		novelRecommended: func(_ context.Context, r pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			calls["novel"]++
			id := int64(3)
			var next sdk.Cursor
			if r.Cursor.IsZero() {
				next = testPageCursor(2)
			} else {
				id = 13
			}
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: id, Title: "novel", User: pixiv.User{ID: 7}}}, Next: next}, nil
		},
		userRecommended: func(_ context.Context, r pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			calls["user"]++
			id := int64(4)
			var next sdk.Cursor
			if r.Cursor.IsZero() {
				next = testPageCursor(3)
			} else {
				id = 14
			}
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: id, Name: "user"}}}, Next: next}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	first := callTool(t, session, "pixiv_recommended", map[string]any{"kind": "all"})
	var out outputs.Recommended
	decodeStructured(t, first, &out)
	if first.IsError || len(out.Records) != 4 {
		t.Fatalf("first error=%v records=%d", first.IsError, len(out.Records))
	}
	cursors := map[string]string{"illust": out.Pagination.Illust.NextCursor, "manga": out.Pagination.Manga.NextCursor, "novel": out.Pagination.Novel.NextCursor, "user": out.Pagination.User.NextCursor}
	for kind, cursor := range cursors {
		if cursor == "" {
			t.Fatalf("missing %s continuation", kind)
		}
		result := callTool(t, session, "pixiv_recommended", map[string]any{"kind": kind, "cursor": cursor})
		var continued outputs.Recommended
		decodeStructured(t, result, &continued)
		if result.IsError || len(continued.Records) != 1 {
			t.Fatalf("kind=%s error=%v records=%d", kind, result.IsError, len(continued.Records))
		}
		want := map[string]string{"illust": "11", "manga": "12", "novel": "13", "user": "14"}[kind]
		if continued.Records[0].ID() != want {
			t.Fatalf("kind=%s id=%s", kind, continued.Records[0].ID())
		}
	}
	if calls["art"] != 3 || calls["novel"] != 2 || calls["user"] != 2 {
		t.Fatalf("other streams refetched: %v", calls)
	}
	for _, args := range []map[string]any{{"kind": "all", "cursor": cursors["illust"]}, {"kind": "manga", "cursor": cursors["illust"]}, {"kind": "novel", "cursor": cursors["novel"], "limit": 1}} {
		if r := callTool(t, session, "pixiv_recommended", args); !r.IsError {
			t.Fatal("invalid cursor accepted")
		}
	}
}
func TestMixedRecommendedEmptyVisualBatchKeepsCursor(t *testing.T) {
	calls := 0
	client := &fakeSDKClient{userID: 7, recommendedArtworks: func(_ context.Context, r pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
		calls++
		item := testSDKIllust(1, "filtered", 7)
		var next sdk.Cursor
		if r.Cursor.IsZero() {
			next = testPageCursor(1)
		} else {
			item.PageCount = 2
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}, Next: next}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	filter := map[string]any{"min_pages": 2}
	first := callTool(t, session, "pixiv_recommended", map[string]any{"kind": "all", "illust_filter": filter})
	var out outputs.Recommended
	decodeStructured(t, first, &out)
	if first.IsError || len(out.Records) != 0 || out.Pagination.Illust.NextCursor == "" || calls != 1 {
		t.Fatalf("empty visual batch lost continuation: error=%v calls=%d", first.IsError, calls)
	}
	second := callTool(t, session, "pixiv_recommended", map[string]any{"kind": "illust", "illust_filter": filter, "cursor": out.Pagination.Illust.NextCursor})
	out = outputs.Recommended{}
	decodeStructured(t, second, &out)
	if second.IsError || len(out.Records) != 1 || out.Pagination.Illust.HasMore || out.Pagination.Illust.NextCursor != "" || calls != 2 {
		t.Fatalf("filtered continuation error=%v calls=%d", second.IsError, calls)
	}
}

func TestSearchUserRejectsBlankWordBeforeSDKExecution(t *testing.T) {
	executions := 0
	client := openWireClient(t, &fakeSDKClient{})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Execute: func(ctx context.Context, _ pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			executions++
			_, err := attempt(ctx, client)
			return err
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_search_user", map[string]any{"word": " \t"})
	if !result.IsError || executions != 0 || !resultHasText(result, "search word is required") {
		t.Fatalf("blank search_user result=%+v executions=%d", result, executions)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("blank search_user records=%+v", out.Records)
	}
}

// TestReadToolSDKFailuresRemainStructured 是读取 tool 的失败模式表：SDK 返回的类型化
// 错误必须以 isError 暴露、保留结构化空 envelope，并且不静默回退到其它端点。
func TestReadToolSDKFailuresRemainStructured(t *testing.T) {
	for _, test := range []struct {
		name   string
		tool   string
		args   map[string]any
		client *fakeSDKClient
		calls  func(*fakeSDKClient) int
	}{
		{name: "search user", tool: "pixiv_search_user", args: map[string]any{"word": "miku"}, client: &fakeSDKClient{searchUser: func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{}, sdk.NewError("pixiv", "SearchUsers", sdk.Forbidden)
		}}},
		{name: "user detail", tool: "pixiv_user_detail", args: map[string]any{"user_id": 401}, client: &fakeSDKClient{userDetailErr: sdk.NewError("pixiv", "User", sdk.Forbidden)}},
		{name: "blocked users", tool: "pixiv_blocked_users", args: map[string]any{}, client: &fakeSDKClient{userID: 71, userBlockedUsers: func(context.Context, pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{}, &sdk.Error{Product: "pixiv", Operation: "UserBlockedUsers", Reason: sdk.Forbidden}
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result := callTool(t, session, test.tool, test.args)
			if !result.IsError || !resultHasText(result, "forbidden") {
				t.Fatalf("%s failure result=%+v", test.tool, result)
			}
			var out outputs.Records
			decodeStructured(t, result, &out)
			if len(out.Records) != 0 {
				t.Fatalf("%s failure records=%+v", test.tool, out.Records)
			}
		})
	}
}

func TestRelatedUsersResolvesCurrentIdentityAndFiltersBeforeLogicalPagination(t *testing.T) {
	client := &fakeSDKClient{
		userID: 71,
		relatedUsers: func(_ context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			if request.UserID != 71 {
				t.Fatalf("related user request=%+v, want current user 71", request)
			}
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{
				{User: pixiv.User{ID: 70, Name: "other"}},
				{User: pixiv.User{ID: 71, Name: "current"}},
			}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_user_relations", map[string]any{"relation": "related",
		"user_filter": map[string]any{"id": 71},
		"limit":       1,
	})
	if result.IsError {
		t.Fatalf("related_users returned error: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "71" {
		t.Fatalf("related_users output=%+v", out)
	}
	if out.Pagination.Returned != 1 || out.Pagination.HasMore {
		t.Fatalf("related_users pagination=%+v", out.Pagination)
	}

	if client.relatedUsersRequest.UserID != 71 {
		t.Fatalf("related_users request=%+v", client.relatedUsersRequest)
	}
}
func TestUserReadFiltersFillLogicalPagesAcrossSDKCursors(t *testing.T) {
	tests := []struct {
		name   string
		args   map[string]any
		client *fakeSDKClient
	}{
		{
			name: "pixiv_search_user",
			args: map[string]any{"word": "miku", "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(1)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "pixiv_mypixiv",
			args: map[string]any{"kind": "user", "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{myPixivUsers: func(_ context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(2)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "pixiv_user_novels",
			args: map[string]any{"user_id": 71, "novel_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 1, User: pixiv.User{ID: 71}}}, Next: testPageCursor(3)}, nil
				}
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 2, User: pixiv.User{ID: 71}}}}, nil
			}},
		},
		{
			name: "pixiv_user_artworks",
			args: map[string]any{"user_id": 71, "illust_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
				if call == 1 {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 71)}, Next: testPageCursor(4)}, nil
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "second", 71)}}, nil
			}},
		},
		{
			name: "pixiv_user_relations",
			args: map[string]any{"relation": "following", "user_id": 71, "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userFollowing: func(_ context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(5)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "pixiv_user_relations",
			args: map[string]any{"relation": "related", "user_id": 71, "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{relatedUsers: func(_ context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(6)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result := callTool(t, session, test.name, test.args)
			if result.IsError {
				for _, content := range result.Content {
					if text, ok := content.(*mcp.TextContent); ok {
						t.Fatalf("filtered %s returned error: %s", test.name, text.Text)
					}
				}
				t.Fatalf("filtered %s returned error without text content: %+v", test.name, result)
			}
			var out outputs.Records
			decodeStructured(t, result, &out)
			if len(out.Records) != 1 || out.Records[0].ID() != "2" {
				t.Fatalf("filtered %s output=%+v", test.name, out)
			}
			if out.Pagination.Returned != 1 || out.Pagination.HasMore {
				t.Fatalf("filtered %s pagination=%+v", test.name, out.Pagination)
			}
		})
	}
}

func TestSDKRecommendedAllReturnsEveryStreamAndPagination(t *testing.T) {
	client := &fakeSDKClient{}
	var order []string
	client.recommendedArtworks = func(_ context.Context, _ pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if call != 1 {
			return sdk.Page[pixiv.Artwork]{}, errors.New("recommended all traversed artwork stream more than once")
		}
		order = append(order, "visual")
		manga := testSDKIllust(2, "manga", 20)
		manga.Kind = pixiv.ArtworkKindManga
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "illust", 10), manga}, Next: testPageCursor(2)}, nil
	}
	client.novelRecommended = func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		order = append(order, "novel")
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 30}, Tags: []pixiv.Tag{}}}}, nil
	}
	client.userRecommended = func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		order = append(order, "user")
		return sdk.Page[pixiv.UserPreview]{
			Items: []pixiv.UserPreview{{
				User:    pixiv.User{ID: 4},
				Illusts: []pixiv.Artwork{},
				Novels:  []pixiv.Novel{{ID: 5, User: pixiv.User{ID: 40}}},
			}},
		}, nil
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "pixiv_recommended", map[string]any{"kind": "all", "limit": 2})
	if result.IsError || !slices.Equal(order, []string{"visual", "novel", "user"}) {
		t.Fatalf("recommended all result=%+v order=%v", result, order)
	}
	var structured map[string]any
	decodeStructured(t, result, &structured)
	pagination, ok := structured["pagination"].(map[string]any)
	if !ok || len(pagination) != 4 {
		t.Fatalf("missing independent pagination: %#v", structured)
	}
	records, ok := structured["records"].([]any)
	if !ok || len(records) != 4 {
		t.Fatalf("records = %#v", structured["records"])
	}
	raw, err := json.Marshal(structured)
	if err != nil || strings.Contains(string(raw), "cursor") || strings.Contains(string(raw), "next_url") {
		t.Fatalf("structured output leaks continuation: %s, err=%v", raw, err)
	}
}
func TestSDKRecommendedAllFailureDoesNotExposePartialStructuredOutput(t *testing.T) {
	client := &fakeSDKClient{
		recommendedArtworks: func(_ context.Context, _ pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 1)}}, nil
		},
		novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			return sdk.Page[pixiv.Novel]{}, errors.New("malformed upstream response")
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "pixiv_recommended", map[string]any{"kind": "all"})
	if !result.IsError {
		t.Fatalf("all failure result=%+v", result)
	}
	var out outputs.Recommended
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || out.Pagination != (outputs.RecommendedPagination{}) {
		t.Fatalf("partial structured output=%+v", out)
	}
}
func filteredTestIllust(id int64, views, pages int, tag string) pixiv.Artwork {
	illust := testSDKIllust(id, "work", 7)
	illust.TotalViews = views
	illust.PageCount = pages
	illust.Tags = []pixiv.Tag{{Name: tag}}
	return illust
}

// TestEntityToolsExposeOnlyRecordsContract 固定所有实体读取 tool 的共同输出：
// structured content 只承载 records 和分页信息，Content 只用于短摘要而不复制实体。
func TestEntityToolsExposeOnlyRecordsContract(t *testing.T) {
	client := &fakeSDKClient{
		userID:    90,
		artworks:  []pixiv.Artwork{testSDKIllust(21, "artwork", 90)},
		bookmarks: []pixiv.Artwork{testSDKIllust(22, "bookmark", 90)},
		following: []pixiv.UserPreview{{User: pixiv.User{ID: 23, Name: "following"}}},
		searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "search-illust", 1)}}, nil
		},
		searchNovel: func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 2, Title: "search-novel", User: pixiv.User{ID: 2}}}}, nil
		},
		searchUser: func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 3, Name: "search-user"}}}}, nil
		},
		illustDetail: func(context.Context, int64) (pixiv.Artwork, error) {
			return testSDKIllust(4, "detail", 4), nil
		},
		illustRanking: func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(5, "ranking", 5)}}, nil
		},
		recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(6, "pixiv_recommended", 6)}}, nil
		},
		userDetailResult: pixiv.UserDetail{User: pixiv.User{ID: 7, Name: "detail-user"}, Profile: pixiv.UserProfile{Region: "Tokyo"}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"pixiv_search_illust", map[string]any{"word": "record"}},
		{"pixiv_search_novel", map[string]any{"word": "record"}},
		{"pixiv_illust_detail", map[string]any{"illust_id": 4}},
		{"pixiv_illust_related", map[string]any{"illust_id": 4}},
		{"pixiv_illust_ranking", map[string]any{}},
		{"pixiv_illust_recommended", map[string]any{}},
		{"pixiv_timeline_following", map[string]any{"kind": "artwork"}},
		{"pixiv_search_user", map[string]any{"word": "record"}},
		{"pixiv_user_detail", map[string]any{"user_id": 7}},
		{"pixiv_user_artworks", map[string]any{"user_id": 90}},
		{"pixiv_user_bookmarks", map[string]any{"kind": "artwork", "user_id": 90}},
		{"pixiv_user_relations", map[string]any{"relation": "following", "user_id": 90}},
	} {
		t.Run(tool.name, func(t *testing.T) {
			result := callTool(t, session, tool.name, tool.args)
			if result.IsError {
				t.Fatalf("%s returned error: %+v", tool.name, result)
			}
			assertRecordsOnlyStructuredOutput(t, result)
		})
	}
}

func assertRecordsOnlyStructuredOutput(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	var structured map[string]any
	decodeStructured(t, result, &structured)
	records, ok := structured["records"].([]any)
	if !ok {
		t.Fatalf("structured records=%#v", structured["records"])
	}
	for _, forbidden := range []string{"items", "illust", "illusts", "manga", "novels", "user_previews", "user_id", "kind", "source", "text"} {
		if _, exists := structured[forbidden]; exists {
			t.Fatalf("structured output retains forbidden %q: %#v", forbidden, structured)
		}
	}
	for _, raw := range records {
		record, ok := raw.(map[string]any)
		if !ok || record["id"] == "" || record["type"] == "" || record["url"] == "" {
			t.Fatalf("invalid record=%#v", raw)
		}
		assertNoRecordVersionMetadata(t, record)
	}
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("entity Content=%T, want only short text summary", content)
		}
		raw, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text.Text, string(raw)) {
			t.Fatalf("Content duplicates record JSON: %q", text.Text)
		}
	}
	return structured
}

func assertNoRecordVersionMetadata(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
			if _, prohibited := map[string]struct{}{"version": {}, "schema": {}, "apiversion": {}, "schemaversion": {}, "protocolversion": {}, "formatversion": {}, "recordversion": {}, "sdkversion": {}, "mcpversion": {}, "cliversion": {}, "versioninfo": {}}[normalized]; prohibited {
				t.Fatalf("record includes prohibited metadata key %q", key)
			}
			assertNoRecordVersionMetadata(t, child)
		}
	case []any:
		for _, child := range value {
			assertNoRecordVersionMetadata(t, child)
		}
	}
}

// 搜索类 tool 的 owner 契约：过滤器映射、逻辑分页、schema 拒绝与结构化输出。

type reverseSearchOutputFixture struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	Results        []reversesearch.Result          `json:"results"`
	Records        []record.Record                 `json:"records"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

type reverseSearcherFunc func(context.Context, reversesearch.Request) (reversesearch.Response, error)

func (f reverseSearcherFunc) Search(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
	return f(ctx, request)
}

func TestReverseSearchPublishesClosedInputAndEnvelopeSchemas(t *testing.T) {
	tools := connectAndListTools(t)
	var reverseToolName string
	var inputSchema, outputSchema any
	for _, tool := range tools {
		if tool.Name != "pixiv_reverse_search" {
			continue
		}
		reverseToolName = tool.Name
		require.JSONEq(t, `["image"]`, string(mustJSON(t, tool.Meta["openai/fileParams"])))
		if err := json.Unmarshal(mustJSON(t, tool.InputSchema), &inputSchema); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(mustJSON(t, tool.OutputSchema), &outputSchema); err != nil {
			t.Fatal(err)
		}
	}
	if reverseToolName == "" {
		t.Fatal("reverse_search tool is not registered")
	}

	input, ok := inputSchema.(map[string]any)
	if !ok || input["type"] != "object" || input["additionalProperties"] != false {
		t.Fatalf("reverse_search input schema = %#v, want closed object", inputSchema)
	}
	require.NotContains(t, string(mustJSON(t, input["required"])), `"source"`)
	properties, ok := input["properties"].(map[string]any)
	if !ok || properties["source"] == nil || properties["provider"] == nil {
		t.Fatalf("reverse_search input schema properties = %#v", input["properties"])
	}
	require.Len(t, properties, 3)
	image, ok := properties["image"].(map[string]any)
	require.True(t, ok, "image must be an object schema")
	require.Equal(t, "object", image["type"])
	require.Equal(t, false, image["additionalProperties"])
	require.JSONEq(t, `["download_url", "file_id"]`, string(mustJSON(t, image["required"])))
	imageFields, ok := image["properties"].(map[string]any)
	require.True(t, ok)
	require.Len(t, imageFields, 4)
	for _, name := range []string{"download_url", "file_id", "mime_type", "file_name"} {
		field, ok := imageFields[name].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "string", field["type"])
	}
	if _, ok := properties["pixiv_only"]; ok {
		t.Fatalf("reverse_search input schema exposes server config: %#v", properties)
	}
	provider, ok := properties["provider"].(map[string]any)
	if !ok || !strings.Contains(string(mustJSON(t, provider["enum"])), `"all"`) {
		t.Fatalf("reverse_search provider schema = %#v", properties["provider"])
	}

	output, ok := outputSchema.(map[string]any)
	if !ok || output["type"] != "object" || output["additionalProperties"] != false {
		t.Fatalf("reverse_search output schema = %#v, want closed object", outputSchema)
	}
	for _, field := range []string{"input", "providers", "results", "records", "provider_errors", "partial"} {
		if !strings.Contains(string(mustJSON(t, output["required"])), `"`+field+`"`) {
			t.Fatalf("reverse_search output schema missing required %q: %#v", field, output)
		}
	}
}

func TestReverseSearchReturnsStructuredEnvelopeAndRecord(t *testing.T) {
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{
					Input:     reversesearch.Input{Kind: reversesearch.SourceKindFile, SHA256: "hash-42"},
					Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1}},
					Results:   []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 42}, Evidence: []reversesearch.Evidence{{Provider: reversesearch.ProviderASCII2DColor, Rank: 1}}}},
				}, nil
			}),
			Provider:  reversesearch.ProviderSauceNAO,
			PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{
		"source": "/private/source-secret.png", "provider": "ascii2d-color",
	})
	if result.IsError {
		t.Fatalf("reverse_search returned MCP error: %+v", result)
	}
	if got.Source != "/private/source-secret.png" || got.Provider != reversesearch.ProviderASCII2DColor || !got.PixivOnly {
		t.Fatalf("reverse search request = %+v", got)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if out.Input.Kind != reversesearch.SourceKindFile || out.Input.SHA256 != "hash-42" || len(out.Providers) != 1 || len(out.Results) != 1 || len(out.Records) != 1 {
		t.Fatalf("reverse search structured output = %+v", out)
	}
	if out.Records[0].Type() != "artwork" || out.Records[0].ID() != "42" || out.Records[0].URL() != "https://www.pixiv.net/artworks/42" {
		t.Fatalf("reverse search record = %+v", out.Records[0])
	}
	raw := string(mustJSON(t, result.StructuredContent))
	for _, secret := range []string{"/private/source-secret.png", "source-secret.png"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("reverse search structured output leaked %q: %s", secret, raw)
		}
	}
}

func TestReverseSearchUsesStartupProviderWhenInputOmitsOverride(t *testing.T) {
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{}, nil
			}),
			Provider:  reversesearch.ProviderASCII2DBOVW,
			PixivOnly: false,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "https://image.example.test/picture.png"})
	if result.IsError {
		t.Fatalf("reverse_search returned MCP error: %+v", result)
	}
	if got.Provider != reversesearch.ProviderASCII2DBOVW || got.PixivOnly {
		t.Fatalf("reverse search startup defaults = %+v", got)
	}
}

func TestReverseSearchPartialResultIsNotMCPError(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
				return reversesearch.Response{
					Providers: []reversesearch.ProviderSummary{
						{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusError},
						{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
					},
					Results:        []reversesearch.Result{{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 7}}},
					ProviderErrors: []reversesearch.ProviderError{{Provider: reversesearch.ProviderSauceNAO, Code: reversesearch.CodeMissingCredential, Message: "SauceNAO API key is required"}},
					Partial:        true,
				}, nil
			}),
			PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "https://image.example.test/picture.png", "provider": "all"})
	if result.IsError {
		t.Fatalf("partial reverse search must not be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if !out.Partial || len(out.ProviderErrors) != 1 || len(out.Records) != 1 {
		t.Fatalf("partial reverse search output = %+v", out)
	}
}

// TestReverseSearchErrorsKeepSafeStructuredEnvelope 是反向搜图的失败模式表：稳定
// solver 错误与全部 provider 失败都必须 isError=true、保留结构化 envelope，且不回显
// 上游 cause 里的密钥、来源 URL 或响应体。
func TestReverseSearchErrorsKeepSafeStructuredEnvelope(t *testing.T) {
	for _, test := range []struct {
		name        string
		provider    reversesearch.Provider
		code        reversesearch.ErrorCode
		secrets     []string
		providers   []reversesearch.ProviderSummary
		providerErr []reversesearch.ProviderError
	}{
		{
			name: "solver failed", provider: reversesearch.ProviderASCII2DColor, code: reversesearch.CodeSolverFailed,
			secrets:   []string{"solver-secret", "source-secret", "csrf-secret"},
			providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusError}},
			providerErr: []reversesearch.ProviderError{{
				Provider: reversesearch.ProviderASCII2DColor, Code: reversesearch.CodeSolverFailed, Message: "ascii2d challenge solver failed",
			}},
		},
		{
			name: "all providers failed", provider: reversesearch.ProviderAll, code: reversesearch.CodeAllProvidersFailed,
			secrets:   []string{"source-secret.example.test", "api-key-secret", "upstream-body-secret", "csrf-secret", "location-secret"},
			providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderAll, Status: reversesearch.ProviderStatusError}},
			providerErr: []reversesearch.ProviderError{{
				Provider: reversesearch.ProviderAll, Code: reversesearch.CodeAllProvidersFailed, Message: "all reverse-search providers failed",
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			secretCause := strings.Join(test.secrets, " ")
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
				ReverseSearch: pixivmcpserver.ReverseSearchPorts{
					Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
						return reversesearch.Response{Providers: test.providers, Results: []reversesearch.Result{}, ProviderErrors: test.providerErr},
							reversesearch.NewError(test.code, test.providerErr[0].Message, errors.New(secretCause))
					}),
				},
			}, pixivmcpserver.Account{})
			defer closeSession()

			result := callTool(t, session, "pixiv_reverse_search", map[string]any{
				"source": "https://source-secret.example.test/image?token=" + test.secrets[0], "provider": string(test.provider),
			})
			if !result.IsError {
				t.Fatalf("%s must be an MCP error: %+v", test.name, result)
			}
			var out reverseSearchOutputFixture
			decodeStructured(t, result, &out)
			if len(out.Providers) != 1 || len(out.ProviderErrors) != 1 || out.Records == nil || out.Results == nil {
				t.Fatalf("%s envelope is not preserved: %+v", test.name, out)
			}
			if out.ProviderErrors[0].Code != test.code {
				t.Fatalf("provider error code = %q, want %q", out.ProviderErrors[0].Code, test.code)
			}
			raw := string(mustJSON(t, result))
			if !strings.Contains(raw, string(test.code)) {
				t.Fatalf("%s MCP error did not expose stable code: %s", test.name, raw)
			}
			for _, secret := range test.secrets {
				if strings.Contains(raw, secret) {
					t.Fatalf("%s leaked %q: %s", test.name, secret, raw)
				}
			}
		})
	}
}

func TestReverseSearchConfigurationFailureKeepsEmptyEnvelope(t *testing.T) {
	session, closeSession := newTestSession(t)
	defer closeSession()

	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"source": "/private/source-secret.png"})
	if !result.IsError {
		t.Fatalf("unconfigured reverse search must be an MCP error: %+v", result)
	}
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	if out.Providers == nil || out.Results == nil || out.Records == nil || out.ProviderErrors == nil {
		t.Fatalf("configuration failure envelope has nil collections: %+v", out)
	}
	if strings.Contains(string(mustJSON(t, result)), "source-secret") {
		t.Fatalf("configuration failure leaked source: %s", mustJSON(t, result))
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return data
}

func TestReverseSearchImageUsesDownloadURLAndStartupSettings(t *testing.T) {
	const source = "https://image.example.test/photo?signature=private-signature"
	var got reversesearch.Request
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: reverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
				got = request
				return reversesearch.Response{Input: reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "safe-hash"}}, nil
			}),
			Provider: reversesearch.ProviderASCII2DBOVW, PixivOnly: true,
		},
	}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{
		"download_url": source, "file_id": "private-file-id", "mime_type": "image/png", "file_name": "private-name.png",
	}})
	require.False(t, result.IsError, "%s", mustJSON(t, result))
	require.Equal(t, source, got.Source)
	require.Equal(t, reversesearch.ProviderASCII2DBOVW, got.Provider)
	require.True(t, got.PixivOnly)
	var out reverseSearchOutputFixture
	decodeStructured(t, result, &out)
	require.Equal(t, "safe-hash", out.Input.SHA256)
	for _, secret := range []string{source, "private-signature", "private-file-id", "private-name.png"} {
		require.NotContains(t, string(mustJSON(t, result)), secret)
	}
}

func TestReverseSearchRejectsInvalidInputsWithoutLeakingOrSearching(t *testing.T) {
	image := func(source string) map[string]any {
		return map[string]any{"download_url": source, "file_id": "private-file-id"}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"neither", map[string]any{}},
		{"remote file authority", map[string]any{"source": "file://other-host/private/source-secret"}},
		{"file userinfo", map[string]any{"source": "file://source-secret@localhost/private/photo"}},
		{"file query", map[string]any{"source": "file:///private/source-secret?private-download"}},
		{"empty file query", map[string]any{"source": "file:///private/source-secret?"}},
		{"file fragment", map[string]any{"source": "file:///private/source-secret#private-download"}},
		{"empty file fragment", map[string]any{"source": "file:///private/source-secret#"}},
		{"file UNC path", map[string]any{"source": "file:////private/source-secret"}},
		{"empty file path", map[string]any{"source": "file://localhost"}},
		{"malformed file escape", map[string]any{"source": "file:///private/%source-secret"}},
		{"file NUL", map[string]any{"source": "file:///private/source-secret%00"}},
		{"both", map[string]any{"source": "/private/source-secret", "image": image("https://example.test/private-download")}},
		{"empty source with image", map[string]any{"source": "", "image": image("https://example.test/private-download")}},
		{"empty source", map[string]any{"source": "  "}},
		{"null source", map[string]any{"source": nil}},
		{"null image", map[string]any{"image": nil}},
		{"wrong source type", map[string]any{"source": []string{"source-secret"}}},
		{"wrong image type", map[string]any{"image": "private-download"}},
		{"missing file id", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download"}}},
		{"missing download url", map[string]any{"image": map[string]any{"file_id": "private-file-id"}}},
		{"empty file id", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download", "file_id": "  "}}},
		{"unknown image field", map[string]any{"image": map[string]any{"download_url": "https://example.test/private-download", "file_id": "private-file-id", "extra": "source-secret"}}},
		{"local image path", map[string]any{"image": image("/private/source-secret")}},
		{"file image URI", map[string]any{"image": image("file:///private/source-secret")}},
		{"ftp image URL", map[string]any{"image": image("ftp://example.test/private-download")}},
		{"empty image URL", map[string]any{"image": image("")}},
		{"missing URL host", map[string]any{"image": image("https:///private-download")}},
		{"URL userinfo", map[string]any{"image": image("https://source-secret:private-password@example.test/private-download")}},
		{"malformed URL", map[string]any{"image": image("https://example.test/%private-download")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{
				Searcher: reverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
					called = true
					return reversesearch.Response{}, nil
				}),
			}}, pixivmcpserver.Account{})
			defer closeSession()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pixiv_reverse_search", Arguments: tc.args})
			require.True(t, err != nil || (result != nil && result.IsError), "invalid input accepted: %s", mustJSON(t, result))
			require.False(t, called)
			raw := string(mustJSON(t, result))
			if err != nil {
				raw += err.Error()
			}
			for _, secret := range []string{"source-secret", "private-download", "private-file-id", "private-password"} {
				require.NotContains(t, raw, secret)
			}
		})
	}
}

type reversePayloadFunc func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error)

func (f reversePayloadFunc) Preflight(context.Context, reversesearch.PayloadQuery) error { return nil }
func (f reversePayloadFunc) SearchPayload(ctx context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
	return f(ctx, request)
}

func TestReverseSearchInputsSharePrivateSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private 空格#%.png")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0600))
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	fileURL := (&url.URL{Scheme: "file", Path: uriPath}).String()
	localURL := (&url.URL{Scheme: "file", Host: "localhost", Path: uriPath}).String()
	var fetched atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetched.Add(1); _, _ = io.WriteString(w, "hello\n") }))
	defer upstream.Close()
	for _, tc := range []struct {
		name    string
		args    map[string]any
		kind    reversesearch.SourceKind
		fetches int
	}{
		{"path", map[string]any{"source": path}, reversesearch.SourceKindFile, 0},
		{"file URI", map[string]any{"source": fileURL}, reversesearch.SourceKindFile, 0},
		{"localhost file URI", map[string]any{"source": localURL}, reversesearch.SourceKindFile, 0},
		{"HTTP source", map[string]any{"source": upstream.URL + "/private-download"}, reversesearch.SourceKindURL, 1},
		{"host image", map[string]any{"image": map[string]any{"download_url": upstream.URL + "/private-download?sig=private-signature", "file_id": "private-file-id"}}, reversesearch.SourceKindURL, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temp := t.TempDir()
			before := fetched.Load()
			var snapshot *reversesearch.Snapshot
			service := reversesearch.NewService(
				reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: upstream.Client()}),
				reversePayloadFunc(func(_ context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
					snapshot = request.Snapshot
					require.Equal(t, reversesearch.ProviderASCII2DBOVW, request.Provider)
					require.True(t, request.PixivOnly)
					reader, err := snapshot.Open()
					require.NoError(t, err)
					defer reader.Close()
					data, err := io.ReadAll(reader)
					require.NoError(t, err)
					require.Equal(t, "hello\n", string(data))
					entries, err := os.ReadDir(temp)
					require.NoError(t, err)
					require.Len(t, entries, 1)
					info, err := entries[0].Info()
					require.NoError(t, err)
					if os.PathSeparator != '\\' {
						require.Equal(t, os.FileMode(0600), info.Mode().Perm())
					}
					return reversesearch.Response{}, nil
				}),
			)
			session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: service, Provider: reversesearch.ProviderASCII2DBOVW, PixivOnly: true}}, pixivmcpserver.Account{})
			defer closeSession()
			result := callTool(t, session, "pixiv_reverse_search", tc.args)
			require.False(t, result.IsError, "%s", mustJSON(t, result))
			require.NotNil(t, snapshot)
			var out reverseSearchOutputFixture
			decodeStructured(t, result, &out)
			require.Equal(t, tc.kind, out.Input.Kind)
			require.Equal(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", out.Input.SHA256)
			require.Equal(t, int32(tc.fetches), fetched.Load()-before)
			_, err := snapshot.Open()
			require.Error(t, err)
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries)
			for _, secret := range []string{path, fileURL, "private-download", "private-signature", "private-file-id"} {
				require.NotContains(t, string(mustJSON(t, result)), secret)
			}
		})
	}
}

type reverseImageTransport func(*http.Request) (*http.Response, error)

func (f reverseImageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type reverseCancelBody struct {
	cancel context.CancelFunc
	closed bool
}

func (b *reverseCancelBody) Read(p []byte) (int, error) {
	// 首次读取发生在私有 snapshot 创建之后；取消必须删除这个未完成文件。
	n := copy(p, "partial image")
	b.cancel()
	return n, nil
}
func (b *reverseCancelBody) Close() error { b.closed = true; return nil }

func TestReverseSearchHostImageCancellationCleansPartialSnapshot(t *testing.T) {
	temp := t.TempDir()
	var body *reverseCancelBody
	searched := false
	searcher := reverseSearcherFunc(func(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		body = &reverseCancelBody{cancel: cancel}
		client := &http.Client{Transport: reverseImageTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Request: r, Body: body, Header: make(http.Header)}, nil
		})}
		service := reversesearch.NewService(
			reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: temp, HTTPClient: client}),
			reversePayloadFunc(func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error) {
				searched = true
				return reversesearch.Response{}, nil
			}),
		)
		return service.Search(ctx, request)
	})
	session, closeSession := newSDKTestSessionWithPorts(t, pixivmcpserver.SDKPorts{ReverseSearch: pixivmcpserver.ReverseSearchPorts{Searcher: searcher}}, pixivmcpserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_reverse_search", map[string]any{"image": map[string]any{"download_url": "http://127.0.0.1/private-download", "file_id": "private-file-id"}})
	require.True(t, result.IsError)
	require.False(t, searched)
	require.NotNil(t, body)
	require.True(t, body.closed)
	entries, err := os.ReadDir(temp)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NotContains(t, string(mustJSON(t, result)), "private-download")
	require.NotContains(t, string(mustJSON(t, result)), "private-file-id")
}

type testSDKTransport struct {
	t    *testing.T
	fake *fakeSDKClient
	path string
}

func (tr *testSDKTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if tr.fake == nil {
		tr.t.Fatalf("wire transport has no fake fixture")
	}
	tr.fake.mu.Lock()
	defer tr.fake.mu.Unlock()
	if request.URL.Path == "/auth/token" {
		userID := tr.fake.userID
		if userID <= 0 {
			userID = 42
		}
		body, _ := json.Marshal(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
			"user":          map[string]any{"id": userID, "name": "artist"},
		})
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	tr.path = request.URL.Path
	var status int
	var body []byte
	var err error
	switch request.URL.Path {
	case "/v1/search/illust":
		req := parseSearchArtworks(request.URL.Query())
		tr.fake.searchIllustRequest = req
		page, callErr := callIllustPage(tr.fake.searchIllust, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/search/novel":
		req := parseSearchNovels(request.URL.Query())
		tr.fake.searchNovelRequest = req
		page, callErr := callNovelPage(tr.fake.searchNovel, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/search/user":
		req := parseSearchUsers(request.URL.Query())
		tr.fake.searchUserRequest = req
		page, callErr := callUserPage(tr.fake.searchUser, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/illust/detail":
		id := queryInt64(request.URL.Query(), "illust_id")
		tr.fake.illustDetailRequest = id
		artwork, callErr := callIllustDetail(tr.fake.illustDetail, id)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = wireArtwork(artwork)
	case "/v2/illust/related":
		req := pixiv.RelatedArtworksRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.relatedArtworksRequest = req
		page, callErr := callIllustPageFunc(tr.fake.relatedArtworks, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/illust/series":
		req := pixiv.ArtworkSeriesRequest{SeriesID: queryInt64(request.URL.Query(), "illust_series_id")}
		tr.fake.artworkSeriesRequest = req
		page, callErr := callIllustPageFunc(tr.fake.artworkSeries, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.artworkSeries == nil && len(tr.fake.artworkSeriesPage.Items) > 0 {
			page = tr.fake.artworkSeriesPage
		}
		status, body, err = tr.wireArtworkSeriesPage(page)
	case "/v1/illust/ranking":
		req := parseArtworkRanking(request.URL.Query())
		tr.fake.illustRankingRequest = req
		page, callErr := callIllustPageFunc(tr.fake.illustRanking, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/illust/recommended":
		tr.fake.recommendedArtworksCalls++
		req := pixiv.RecommendedArtworksRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.recommendedArtworksRequest = req
		page, callErr := callRecommendedArtworks(tr.fake.recommendedArtworks, req, tr.fake.recommendedArtworksCalls)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v2/illust/follow":
		req := pixiv.FollowingArtworksRequest{}
		tr.fake.followingIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.followingIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/follow":
		req := pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(request.URL.Query().Get("restrict"))}
		tr.fake.followingNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.followingNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/illust/new":
		req := pixiv.LatestArtworksRequest{ContentType: pixiv.SearchContentType(request.URL.Query().Get("content_type"))}
		tr.fake.latestIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.latestIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/new":
		req := pixiv.LatestNovelsRequest{}
		tr.fake.latestNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.latestNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/mypixiv":
		req := pixiv.MyPixivUsersRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.myPixivUsersRequest = req
		page, callErr := callUserPageFunc(tr.fake.myPixivUsers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v2/illust/mypixiv":
		req := pixiv.MyPixivArtworksRequest{}
		tr.fake.myPixivIllustsRequest = req
		page, callErr := callIllustPageFunc(tr.fake.myPixivIllusts, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/novel/mypixiv":
		req := pixiv.MyPixivNovelsRequest{}
		tr.fake.myPixivNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.myPixivNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/novel/recommended":
		req := pixiv.RecommendedNovelsRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.novelRecommendedRequest = req
		page, callErr := callNovelPageFunc(tr.fake.novelRecommended, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/recommended":
		req := pixiv.RecommendedUsersRequest{Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.userRecommendedRequest = req
		page, callErr := callUserPageFunc(tr.fake.userRecommended, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/detail":
		req := pixiv.UserRequest{UserID: queryInt64(request.URL.Query(), "user_id")}
		tr.fake.userDetailRequest = req
		if tr.fake.userDetailErr != nil {
			return wireErrorResponse(tr.fake.userDetailErr)
		}
		status, body, err = wireUserDetail(tr.fake.userDetailResult)
	case "/v1/user/illusts":
		req := pixiv.UserArtworksRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Kind: kindFromType(request.URL.Query().Get("type")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.artworksRequest = req
		tr.fake.artworksRequests = append(tr.fake.artworksRequests, req)
		tr.fake.userArtworksCalls++
		page, callErr := callUserArtworks(tr.fake.userArtworksFunc, tr.fake.artworks, req, tr.fake.userArtworksCalls)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/user/novels":
		req := pixiv.UserNovelsRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.userNovelsRequest = req
		page, callErr := callNovelPageFunc(tr.fake.userNovels, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireNovelPage(page)
	case "/v1/user/bookmarks/illust":
		req := pixiv.UserArtworkBookmarksRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixiv.Restrict(request.URL.Query().Get("restrict")), Tag: request.URL.Query().Get("tag"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "max_bookmark_id"))}
		tr.fake.bookmarksRequest = req
		tr.fake.bookmarksRequests = append(tr.fake.bookmarksRequests, req)
		tr.fake.bookmarksCalls++
		page, callErr := callUserBookmarks(tr.fake.userBookmarksFunc, req, tr.fake.bookmarksCalls, tr.fake.bookmarks)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireArtworkPage(page)
	case "/v1/user/bookmarks/novel":
		query := request.URL.Query()
		req := pixiv.UserNovelBookmarksRequest{UserID: queryInt64(query, "user_id"), Restrict: pixiv.Restrict(query.Get("restrict")), Tag: query.Get("tag"), Cursor: cursorFromOffset(queryInt(query, "max_bookmark_id"))}
		tr.fake.novelBookmarksRequest = req
		tr.fake.novelBookmarksRequests = append(tr.fake.novelBookmarksRequests, req)
		if tr.fake.novelBookmarksErr != nil {
			return wireErrorResponse(tr.fake.novelBookmarksErr)
		}
		status, body, err = tr.wireNovelPage(sdk.Page[pixiv.Novel]{Items: tr.fake.novelBookmarks})
	case "/v1/user/following":
		req := pixiv.UserFollowingRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixiv.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.followingRequest = req
		page, callErr := callUserPageFunc(tr.fake.userFollowing, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.userFollowing == nil && len(tr.fake.following) > 0 {
			page = sdk.Page[pixiv.UserPreview]{Items: tr.fake.following}
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/follower":
		req := pixiv.UserFollowersRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixiv.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.followersRequest = req
		page, callErr := callUserPageFunc(tr.fake.userFollowers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/related":
		req := pixiv.RelatedUsersRequest{UserID: queryInt64(request.URL.Query(), "seed_user_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.relatedUsersRequest = req
		tr.fake.relatedRequest = req
		page, callErr := callUserPageFunc(tr.fake.relatedUsers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.relatedUsers == nil && len(tr.fake.relatedPage.Items) > 0 {
			page = tr.fake.relatedPage
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v2/user/list":
		req := pixiv.UserBlockedUsersRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.blockedUsersRequest = req
		page, callErr := callUserPageFunc(tr.fake.userBlockedUsers, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		status, body, err = tr.wireUserPreviewPage(page)
	case "/v1/user/bookmark-tags/illust":
		req := pixiv.UserArtworkBookmarkTagsRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixiv.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.bookmarkTagsRequest = req
		status, body, err = wireBookmarkTags(tr.fake.bookmarkTagsPage.Items)
	case "/v1/user/bookmark-tags/novel":
		req := pixiv.UserNovelBookmarkTagsRequest{UserID: queryInt64(request.URL.Query(), "user_id"), Restrict: pixiv.Restrict(request.URL.Query().Get("restrict")), Cursor: cursorFromOffset(queryInt(request.URL.Query(), "offset"))}
		tr.fake.novelBookmarkTagsRequest = req
		if tr.fake.novelBookmarkTagsErr != nil {
			return wireErrorResponse(tr.fake.novelBookmarkTagsErr)
		}
		status, body, err = wireBookmarkTags(tr.fake.novelBookmarkTagsPage.Items)
	case "/v2/illust/bookmark/detail":
		req := pixiv.ArtworkBookmarkRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id")}
		tr.fake.artworkBookmarkRequest = req
		tr.fake.bookmarkDetailRequest = req
		status, body, err = wireBookmarkDetail(tr.fake.bookmarkDetailResult)
	case "/v2/novel/bookmark/detail":
		req := pixiv.NovelBookmarkRequest{NovelID: queryInt64(request.URL.Query(), "novel_id")}
		tr.fake.novelBookmarkRequest = req
		if tr.fake.novelBookmarkDetailErr != nil {
			return wireErrorResponse(tr.fake.novelBookmarkDetailErr)
		}
		status, body, err = wireNovelBookmarkDetail(tr.fake.novelBookmarkDetailResult)
	case "/v2/novel/detail":
		id := queryInt64(request.URL.Query(), "novel_id")
		tr.fake.novelDetailRequest = id
		tr.fake.novelRequest = pixiv.NovelRequest{NovelID: id}
		novel, callErr := callNovelDetail(tr.fake.novelDetail, id)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.novelDetail == nil && tr.fake.novelDetailResult.ID > 0 {
			novel = tr.fake.novelDetailResult
		}
		status, body, err = wireNovel(novel)
	case "/v2/novel/series":
		id := queryInt64(request.URL.Query(), "series_id")
		req := pixiv.NovelSeriesRequest{SeriesID: id}
		tr.fake.novelSeriesRequest = req
		result, callErr := callNovelSeries(tr.fake.novelSeries, req)
		if callErr != nil {
			return wireErrorResponse(callErr)
		}
		if tr.fake.novelSeries == nil && tr.fake.novelSeriesResult.Series.ID > 0 {
			result = tr.fake.novelSeriesResult
		}
		status, body, err = tr.wireNovelSeries(result)
	case "/v1/novel/content":
		id := queryInt64(request.URL.Query(), "novel_id")
		tr.fake.novelContentRequest = pixiv.NovelContentRequest{NovelID: id}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(tr.fake.novelContentHTML))}, nil
	case "/v3/illust/comments":
		req := pixiv.ArtworkCommentsRequest{ArtworkID: queryInt64(request.URL.Query(), "illust_id")}
		tr.fake.illustCommentsRequest = req
		tr.fake.artworkCommentsRequest = req
		status, body, err = wireCommentPageResult(tr.fake.artworkCommentsResult)
	case "/v2/novel/comments":
		req := pixiv.NovelCommentsRequest{NovelID: queryInt64(request.URL.Query(), "novel_id")}
		tr.fake.novelCommentsRequest = req
		status, body, err = wireCommentPageResult(tr.fake.novelCommentsResult)
	case "/v1/illust/comment/add":
		tr.fake.artworkCommentWireCalls++
		artworkID := formInt64(request, "illust_id")
		comment := formValue(request, "comment")
		switch {
		case formValue(request, "parent_comment_id") != "":
			tr.fake.replyArtworkCommentRequest = pixiv.ReplyArtworkCommentRequest{
				ArtworkID:       artworkID,
				Comment:         comment,
				ParentCommentID: formInt64(request, "parent_comment_id"),
			}
			if tr.fake.replyArtworkCommentErr != nil {
				return wireErrorResponse(tr.fake.replyArtworkCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":902}`), nil
		case formValue(request, "stamp_id") != "":
			tr.fake.stampArtworkCommentRequest = pixiv.StampArtworkCommentRequest{
				ArtworkID: artworkID,
				Comment:   comment,
				StampID:   formInt64(request, "stamp_id"),
			}
			if tr.fake.stampArtworkCommentErr != nil {
				return wireErrorResponse(tr.fake.stampArtworkCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":903}`), nil
		default:
			tr.fake.createArtworkCommentRequest = pixiv.PostArtworkCommentRequest{ArtworkID: artworkID, Comment: comment}
			if tr.fake.createArtworkCommentErr != nil {
				return wireErrorResponse(tr.fake.createArtworkCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":901}`), nil
		}
	case "/v1/illust/comment/delete":
		tr.fake.deleteArtworkCommentRequest = pixiv.DeleteArtworkCommentRequest{CommentID: formInt64(request, "comment_id")}
		if tr.fake.deleteArtworkCommentErr != nil {
			return wireErrorResponse(tr.fake.deleteArtworkCommentErr)
		}
		status, body, err = http.StatusOK, []byte(`{}`), nil
	case "/v1/novel/comment/add":
		tr.fake.novelCommentWireCalls++
		novelID := formInt64(request, "novel_id")
		comment := formValue(request, "comment")
		switch {
		case formValue(request, "parent_comment_id") != "":
			tr.fake.replyNovelCommentRequest = pixiv.ReplyNovelCommentRequest{
				NovelID:         novelID,
				Comment:         comment,
				ParentCommentID: formInt64(request, "parent_comment_id"),
			}
			if tr.fake.replyNovelCommentErr != nil {
				return wireErrorResponse(tr.fake.replyNovelCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":902}`), nil
		case formValue(request, "stamp_id") != "":
			tr.fake.stampNovelCommentRequest = pixiv.StampNovelCommentRequest{
				NovelID: novelID,
				Comment: comment,
				StampID: formInt64(request, "stamp_id"),
			}
			if tr.fake.stampNovelCommentErr != nil {
				return wireErrorResponse(tr.fake.stampNovelCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":903}`), nil
		default:
			tr.fake.createNovelCommentRequest = pixiv.PostNovelCommentRequest{NovelID: novelID, Comment: comment}
			if tr.fake.createNovelCommentErr != nil {
				return wireErrorResponse(tr.fake.createNovelCommentErr)
			}
			status, body, err = http.StatusOK, []byte(`{"comment_id":901}`), nil
		}
	case "/v1/novel/comment/delete":
		tr.fake.deleteNovelCommentRequest = pixiv.DeleteNovelCommentRequest{CommentID: formInt64(request, "comment_id")}
		if tr.fake.deleteNovelCommentErr != nil {
			return wireErrorResponse(tr.fake.deleteNovelCommentErr)
		}
		status, body, err = http.StatusOK, []byte(`{}`), nil
	case "/v1/trending-tags/illust":
		status, body, err = wireTrendingTags(tr.fake.trendingTags)
	case "/v2/illust/bookmark/add":
		req := pixiv.AddArtworkBookmarkRequest{ArtworkID: formInt64(request, "illust_id"), Restrict: pixiv.Restrict(formValue(request, "restrict"))}
		if tags := formValues(request, "tags[]"); len(tags) > 0 {
			req.Tags = tags
		}
		tr.fake.addBookmarkRequest = req
		if tr.fake.addBookmarkErr != nil {
			return wireErrorResponse(tr.fake.addBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/illust/bookmark/delete":
		req := pixiv.RemoveArtworkBookmarkRequest{ArtworkID: formInt64(request, "illust_id")}
		tr.fake.removeBookmarkRequest = req
		if tr.fake.removeBookmarkErr != nil {
			return wireErrorResponse(tr.fake.removeBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v2/novel/bookmark/add":
		req := pixiv.AddNovelBookmarkRequest{NovelID: formInt64(request, "novel_id"), Restrict: pixiv.Restrict(formValue(request, "restrict"))}
		if tags := formValues(request, "tags[]"); len(tags) > 0 {
			req.Tags = tags
		}
		tr.fake.addNovelBookmarkRequest = req
		if tr.fake.addNovelBookmarkErr != nil {
			return wireErrorResponse(tr.fake.addNovelBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/novel/bookmark/delete":
		req := pixiv.RemoveNovelBookmarkRequest{NovelID: formInt64(request, "novel_id")}
		tr.fake.removeNovelBookmarkRequest = req
		if tr.fake.removeNovelBookmarkErr != nil {
			return wireErrorResponse(tr.fake.removeNovelBookmarkErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/user/follow/add":
		req := pixiv.FollowUserRequest{UserID: formInt64(request, "user_id"), Restrict: pixiv.Restrict(formValue(request, "restrict"))}
		tr.fake.followUserRequest = req
		if tr.fake.followUserErr != nil {
			return wireErrorResponse(tr.fake.followUserErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	case "/v1/user/follow/delete":
		req := pixiv.UnfollowUserRequest{UserID: formInt64(request, "user_id")}
		tr.fake.unfollowUserRequest = req
		if tr.fake.unfollowUserErr != nil {
			return wireErrorResponse(tr.fake.unfollowUserErr)
		}
		status, body, err = http.StatusOK, []byte("{}"), nil
	default:
		tr.t.Fatalf("wire transport has no handler for %s", request.URL.Path)
		return nil, nil
	}
	if err != nil {
		tr.t.Fatalf("wire transport encode %s: %v", request.URL.Path, err)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

// wireErrorResponse 把 typed fake 错误编码为 App API error envelope，使
// classifyAppError 能还原 reason/detail。
func wireErrorResponse(cause error) (*http.Response, error) {
	status, code, message := http.StatusInternalServerError, "error", cause.Error()
	var typed *sdk.Error
	if ok := errorAs(cause, &typed); ok && typed != nil && typed.Reason == sdk.MalformedUpstreamResponse {
		body := []byte(`{}`)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	if ok := errorAs(cause, &typed); ok && typed != nil {
		switch typed.Reason {
		case sdk.Unauthorized:
			status, code = http.StatusUnauthorized, "unauthorized"
		case sdk.Forbidden:
			status, code = http.StatusForbidden, "forbidden"
		case sdk.NotFound:
			status, code = http.StatusNotFound, "not_found"
		case sdk.InvalidArgument:
			status, code = http.StatusBadRequest, "invalid_argument"
		case sdk.RateLimited:
			status, code = http.StatusTooManyRequests, "rate_limited"
		}
		if typed.Detail != "" {
			message = typed.Detail
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"user_message": message, "message": message, "reason": code}})
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func errorAs(err error, target **sdk.Error) bool {
	for current := err; current != nil; {
		typed, ok := current.(*sdk.Error)
		if ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := current.(unwrapper)
		if !ok {
			return false
		}
		current = next.Unwrap()
	}
	return false
}

// testSDKPorts 构造使用 wire responder 的真实 public SDK client 端口。
func testSDKPorts(t *testing.T, fake *fakeSDKClient) pixivmcpserver.SDKPorts {
	t.Helper()
	ports, _ := newTestSDKPorts(t, fake)
	return ports
}

func newTestSDKPorts(t *testing.T, fake *fakeSDKClient) (pixivmcpserver.SDKPorts, pixivmcpserver.Account) {
	t.Helper()
	client := openWireClient(t, fake)
	return pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) { return client, nil },
		Execute: func(ctx context.Context, _ pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	}, pixivmcpserver.Account{}
}

func callIllustPage(fn func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error), req pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}
	return fn(context.Background(), req)
}

func callIllustPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixiv.Artwork], error), req T) (sdk.Page[pixiv.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}
	return fn(context.Background(), req)
}

func callNovelPage(fn func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error), req pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if fn == nil {
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
	}
	return fn(context.Background(), req)
}

func callNovelPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixiv.Novel], error), req T) (sdk.Page[pixiv.Novel], error) {
	if fn == nil {
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
	}
	return fn(context.Background(), req)
}

func callUserPage(fn func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error), req pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if fn == nil {
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
	}
	return fn(context.Background(), req)
}

func callUserPageFunc[T any](fn func(context.Context, T) (sdk.Page[pixiv.UserPreview], error), req T) (sdk.Page[pixiv.UserPreview], error) {
	if fn == nil {
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
	}
	return fn(context.Background(), req)
}

func callIllustDetail(fn func(context.Context, int64) (pixiv.Artwork, error), id int64) (pixiv.Artwork, error) {
	if fn == nil {
		return pixiv.Artwork{}, nil
	}
	return fn(context.Background(), id)
}

func callNovelDetail(fn func(context.Context, int64) (pixiv.Novel, error), id int64) (pixiv.Novel, error) {
	if fn == nil {
		return pixiv.Novel{}, nil
	}
	return fn(context.Background(), id)
}

func callNovelSeries(fn func(context.Context, pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error), req pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error) {
	if fn == nil {
		return pixiv.NovelSeriesResult{}, nil
	}
	return fn(context.Background(), req)
}

func callRecommendedArtworks(fn func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error), req pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}
	return fn(context.Background(), req, call)
}

func callUserBookmarks(fn func(pixiv.UserArtworkBookmarksRequest, int) (sdk.Page[pixiv.Artwork], error), req pixiv.UserArtworkBookmarksRequest, call int, fallback []pixiv.Artwork) (sdk.Page[pixiv.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixiv.Artwork]{Items: fallback}, nil
	}
	return fn(req, call)
}

func callUserArtworks(fn func(pixiv.UserArtworksRequest, int) (sdk.Page[pixiv.Artwork], error), fallback []pixiv.Artwork, req pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
	if fn == nil {
		return sdk.Page[pixiv.Artwork]{Items: fallback}, nil
	}
	return fn(req, call)
}

func queryInt64(query url.Values, key string) int64 {
	value, _ := strconv.ParseInt(query.Get(key), 10, 64)
	return value
}

func queryInt(query url.Values, key string) int {
	value, _ := strconv.Atoi(query.Get(key))
	return value
}

func formInt64(request *http.Request, key string) int64 {
	_ = request.ParseForm()
	value, _ := strconv.ParseInt(request.PostForm.Get(key), 10, 64)
	return value
}

func formValue(request *http.Request, key string) string {
	_ = request.ParseForm()
	return request.PostForm.Get(key)
}

func formValues(request *http.Request, key string) []string {
	_ = request.ParseForm()
	return request.PostForm[key]
}

func parseSearchArtworks(query url.Values) pixiv.SearchArtworksRequest {
	req := pixiv.SearchArtworksRequest{
		Word:      query.Get("word"),
		Sort:      pixiv.SortMode(query.Get("sort")),
		StartDate: query.Get("start_date"),
		EndDate:   query.Get("end_date"),
		Cursor:    cursorFromOffset(queryInt(query, "offset")),
	}
	req.Target = pixiv.SearchTarget(query.Get("search_target"))
	req.Duration = pixiv.DurationFilter(query.Get("duration"))
	req.ContentType = pixiv.SearchContentType(query.Get("content_type"))
	req.AIMode = pixiv.SearchAIMode(query.Get("search_ai_type"))
	req.AspectRatio = pixiv.SearchAspectRatio(query.Get("ratio_pattern"))
	req.Resolution = pixiv.SearchResolution(query.Get("resolution"))
	req.Tool = query.Get("tool")
	if value := query.Get("bookmark_num_min"); value != "" {
		parsed, _ := strconv.Atoi(value)
		req.BookmarkMin = &parsed
	}
	if value := query.Get("bookmark_num_max"); value != "" {
		parsed, _ := strconv.Atoi(value)
		req.BookmarkMax = &parsed
	}
	return req
}

func parseSearchNovels(query url.Values) pixiv.SearchNovelsRequest {
	return pixiv.SearchNovelsRequest{
		Word:     query.Get("word"),
		Sort:     pixiv.SortMode(query.Get("sort")),
		Target:   pixiv.SearchTarget(query.Get("search_target")),
		Duration: pixiv.DurationFilter(query.Get("duration")),
		Cursor:   cursorFromOffset(queryInt(query, "offset")),
	}
}

func parseSearchUsers(query url.Values) pixiv.SearchUsersRequest {
	return pixiv.SearchUsersRequest{
		Word:   query.Get("word"),
		Cursor: cursorFromOffset(queryInt(query, "offset")),
	}
}

func parseArtworkRanking(query url.Values) pixiv.ArtworkRankingRequest {
	return pixiv.ArtworkRankingRequest{
		Mode:   pixiv.RankingMode(query.Get("mode")),
		Date:   query.Get("date"),
		Cursor: cursorFromOffset(queryInt(query, "offset")),
	}
}

// kindFromType 把 App API type 查询参数映射回 SDK ArtworkKind。
func kindFromType(value string) pixiv.ArtworkKind {
	switch value {
	case "manga":
		return pixiv.ArtworkKindManga
	case "ugoira":
		return pixiv.ArtworkKindUgoira
	default:
		return pixiv.ArtworkKindIllustration
	}
}

// cursorFromOffset 为 captured request 重建一个非零 cursor；offset=0 表示首页。
func cursorFromOffset(offset int) sdk.Cursor {
	if offset <= 0 {
		return sdk.Cursor{}
	}
	cursor, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "q", []byte(strconv.Itoa(offset)))
	if err != nil {
		panic(err)
	}
	return cursor
}

type wireImageURLs struct {
	SquareMedium string `json:"square_medium,omitempty"`
	Medium       string `json:"medium,omitempty"`
	Large        string `json:"large,omitempty"`
	Original     string `json:"original,omitempty"`
}

type wireUser struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
}

type wireTag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name,omitempty"`
}

func wireArtworkValue(a pixiv.Artwork) map[string]any {
	tags := make([]wireTag, 0, len(a.Tags))
	for _, tag := range a.Tags {
		tags = append(tags, wireTag{Name: tag.Name, TranslatedName: tag.TranslatedName})
	}
	var imageURLs wireImageURLs
	if a.Cover.Resource.URL != "" {
		imageURLs.Original = a.Cover.Resource.URL
	}
	kind := "illust"
	switch a.Kind {
	case pixiv.ArtworkKindManga:
		kind = "manga"
	case pixiv.ArtworkKindUgoira:
		kind = "ugoira"
	}
	createDate := "2024-05-01T01:00:00Z"
	if !a.PublishedAt.IsZero() {
		createDate = a.PublishedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"id":              a.ID,
		"title":           a.Title,
		"caption":         a.Caption,
		"type":            kind,
		"user":            wireUser{ID: a.User.ID, Name: a.User.Name, Account: a.User.Account},
		"tags":            tags,
		"image_urls":      imageURLs,
		"create_date":     createDate,
		"page_count":      a.PageCount,
		"total_bookmarks": a.TotalBookmarks,
		"total_view":      a.TotalViews,
		"width":           a.Width,
		"height":          a.Height,
		"x_restrict":      a.XRestrict,
		"tools":           a.Tools,
	}
}

func wireNovelValue(n pixiv.Novel) map[string]any {
	tags := make([]wireTag, 0, len(n.Tags))
	for _, tag := range n.Tags {
		tags = append(tags, wireTag{Name: tag.Name, TranslatedName: tag.TranslatedName})
	}
	var imageURLs wireImageURLs
	if n.Cover.Resource.URL != "" {
		imageURLs.Original = n.Cover.Resource.URL
	}
	createDate := "2024-05-01T01:00:00Z"
	if !n.PublishedAt.IsZero() {
		createDate = n.PublishedAt.UTC().Format(time.RFC3339)
	}
	value := map[string]any{
		"id":          n.ID,
		"title":       n.Title,
		"caption":     n.Caption,
		"user":        wireUser{ID: n.User.ID, Name: n.User.Name, Account: n.User.Account},
		"tags":        tags,
		"image_urls":  imageURLs,
		"create_date": createDate,
	}
	value["text_length"] = n.TextLength
	value["x_restrict"] = n.XRestrict
	value["is_original"] = n.IsOriginal
	return value
}

func wireUserPreviewValue(p pixiv.UserPreview) map[string]any {
	value := map[string]any{
		"user":        wireUser{ID: p.User.ID, Name: p.User.Name, Account: p.User.Account},
		"is_followed": p.User.IsFollowed,
	}
	if len(p.Illusts) > 0 {
		illusts := make([]map[string]any, 0, len(p.Illusts))
		for _, item := range p.Illusts {
			illusts = append(illusts, wireArtworkValue(item))
		}
		value["illusts"] = illusts
	}
	if len(p.Novels) > 0 {
		novels := make([]map[string]any, 0, len(p.Novels))
		for _, item := range p.Novels {
			novels = append(novels, wireNovelValue(item))
		}
		value["novels"] = novels
	}
	return value
}

func (tr *testSDKTransport) wireArtworkPage(page sdk.Page[pixiv.Artwork]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireArtworkValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		Illusts []map[string]any `json:"illusts"`
		NextURL *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

func (tr *testSDKTransport) wireNovelPage(page sdk.Page[pixiv.Novel]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireNovelValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		Novels  []map[string]any `json:"novels"`
		NextURL *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

func (tr *testSDKTransport) wireNovelSeries(result pixiv.NovelSeriesResult) (int, []byte, error) {
	items := make([]map[string]any, 0, len(result.Novels.Items))
	for _, item := range result.Novels.Items {
		items = append(items, wireNovelValue(item))
	}
	next := tr.nextPageURL(result.Novels.Next)
	body, err := json.Marshal(struct {
		SeriesDetail map[string]any   `json:"novel_series_detail"`
		Novels       []map[string]any `json:"novels"`
		NextURL      *string          `json:"next_url"`
	}{
		SeriesDetail: map[string]any{
			"id":           result.Series.ID,
			"title":        result.Series.Title,
			"caption":      result.Series.Caption,
			"is_concluded": result.Series.IsConcluded,
			"user":         wireUser{ID: result.Series.User.ID, Name: result.Series.User.Name, Account: result.Series.User.Account},
		},
		Novels:  items,
		NextURL: next,
	})
	return http.StatusOK, body, err
}

func (tr *testSDKTransport) wireUserPreviewPage(page sdk.Page[pixiv.UserPreview]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, wireUserPreviewValue(item))
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		UserPreviews []map[string]any `json:"user_previews"`
		NextURL      *string          `json:"next_url"`
	}{items, next})
	return http.StatusOK, body, err
}

// nextPageURL 为 wire page 编码 continuation。cursor 文本稳定派生出续传值，
// 相同的 cursor 产生相同的值（供 pagination 周期检测），不同的 cursor 产生
// 不同的值（避免把正常续页误判为重复）；key 随 operation path 使用冻结语义。
func (tr *testSDKTransport) nextPageURL(cursor sdk.Cursor) *string {
	if cursor.IsZero() {
		return nil
	}
	text := cursor.String()
	sum := 0
	for _, r := range text {
		sum = (sum*31 + int(r)) % 100000
	}
	if sum < 0 {
		sum = -sum
	}
	key := "offset"
	switch tr.path {
	case "/v1/illust/new":
		key = "max_illust_id"
	case "/v1/novel/new":
		key = "max_novel_id"
	case "/v1/illust/series", "/v2/novel/series":
		key = "last_order"
	case "/v1/user/bookmarks/illust", "/v1/user/bookmarks/novel":
		key = "max_bookmark_id"
	}
	value := "https://app-api.pixiv.net" + tr.path + "?" + key + "=" + strconv.Itoa(1+sum)
	if tr.path == "/v2/illust/related" {
		value += "&illust_id=" + strconv.FormatInt(tr.fake.relatedArtworksRequest.ArtworkID, 10)
	}
	return &value
}

func wireArtwork(artwork pixiv.Artwork) (int, []byte, error) {
	body, err := json.Marshal(struct {
		Illust map[string]any `json:"illust"`
	}{wireArtworkValue(artwork)})
	return http.StatusOK, body, err
}

func wireNovel(novel pixiv.Novel) (int, []byte, error) {
	body, err := json.Marshal(struct {
		Novel map[string]any `json:"novel"`
	}{wireNovelValue(novel)})
	return http.StatusOK, body, err
}

func wireUserDetail(user pixiv.UserDetail) (int, []byte, error) {
	body, err := json.Marshal(struct {
		User             map[string]any `json:"user"`
		Profile          map[string]any `json:"profile"`
		ProfilePublicity map[string]any `json:"profile_publicity"`
		Workspace        map[string]any `json:"workspace"`
	}{
		map[string]any{"id": user.User.ID, "name": user.User.Name, "account": user.User.Account},
		wireUserProfile(user.Profile),
		wireUserProfilePublicity(user.ProfilePublicity),
		wireUserWorkspace(user.Workspace),
	})
	return http.StatusOK, body, err
}

func wireUserProfile(profile pixiv.UserProfile) map[string]any {
	return map[string]any{
		"webpage":            profile.Webpage,
		"region":             profile.Region,
		"country_code":       profile.CountryCode,
		"job":                profile.Job,
		"total_illusts":      profile.TotalIllusts,
		"total_manga":        profile.TotalManga,
		"total_novels":       profile.TotalNovels,
		"total_follow_users": profile.TotalFollowUsers,
	}
}

func wireUserProfilePublicity(publicity pixiv.UserProfilePublicity) map[string]any {
	return map[string]any{
		"gender":     publicity.Gender,
		"region":     publicity.Region,
		"birth_day":  publicity.BirthDay,
		"birth_year": publicity.BirthYear,
		"job":        publicity.Job,
		"pawoo":      publicity.Pawoo,
	}
}

func wireUserWorkspace(workspace pixiv.UserWorkspace) map[string]any {
	return map[string]any{
		"pc":                  workspace.PC,
		"tool":                workspace.Tool,
		"workspace_image_url": workspace.WorkspaceImageURL,
	}
}

func wireTrendingTags(tags []pixiv.TrendingTag) (int, []byte, error) {
	items := make([]map[string]any, 0, len(tags))
	for _, item := range tags {
		wireTags := make([]map[string]any, 0, len(item.Artwork.Tags))
		for _, tag := range item.Artwork.Tags {
			wireTags = append(wireTags, map[string]any{"tag": tag.Name, "translated_name": tag.TranslatedName})
		}
		tools := item.Artwork.Tools
		if tools == nil {
			tools = []string{}
		}
		illust := map[string]any{
			"id":              item.Artwork.ID,
			"title":           item.Artwork.Title,
			"caption":         item.Artwork.Caption,
			"user":            map[string]any{"id": item.Artwork.User.ID, "name": item.Artwork.User.Name},
			"tags":            wireTags,
			"create_date":     item.Artwork.PublishedAt.UTC().Format(time.RFC3339),
			"updated_at":      item.Artwork.PublishedAt.UTC().Format(time.RFC3339),
			"type":            string(item.Artwork.RawKind),
			"page_count":      item.Artwork.PageCount,
			"total_view":      item.Artwork.TotalViews,
			"total_bookmarks": item.Artwork.TotalBookmarks,
			"width":           item.Artwork.Width,
			"height":          item.Artwork.Height,
			"tools":           tools,
			"image_urls":      map[string]any{"large": "https://i.pximg.net/artworks/" + strconv.FormatInt(item.Artwork.ID, 10) + "/1.png"},
			"meta_pages":      []any{},
		}
		items = append(items, map[string]any{"tag": item.Tag, "translated_name": item.TranslatedName, "illust": illust})
	}
	body, err := json.Marshal(struct {
		TrendTags []map[string]any `json:"trend_tags"`
	}{items})
	return http.StatusOK, body, err
}

func wireBookmarkTags(tags []pixiv.BookmarkTag) (int, []byte, error) {
	items := make([]map[string]any, 0, len(tags))
	for _, item := range tags {
		items = append(items, map[string]any{"name": item.Name, "count": item.Count})
	}
	body, err := json.Marshal(struct {
		BookmarkTags []map[string]any `json:"bookmark_tags"`
	}{items})
	return http.StatusOK, body, err
}

func wireBookmarkDetail(detail pixiv.ArtworkBookmarkDetail) (int, []byte, error) {
	restrict := ""
	switch detail.Restrict {
	case pixiv.RestrictPublic:
		restrict = "public"
	case pixiv.RestrictPrivate:
		restrict = "private"
	}
	tags := make([]map[string]any, 0, len(detail.Tags))
	for _, tag := range detail.Tags {
		tags = append(tags, map[string]any{"name": tag, "is_registered": true})
	}
	body, err := json.Marshal(struct {
		BookmarkDetail map[string]any `json:"bookmark_detail"`
	}{map[string]any{"is_bookmarked": restrict != "", "restrict": restrict, "tags": tags}})
	return http.StatusOK, body, err
}

func wireNovelBookmarkDetail(detail pixiv.NovelBookmarkDetail) (int, []byte, error) {
	restrict := ""
	switch detail.Restrict {
	case pixiv.RestrictPublic:
		restrict = "public"
	case pixiv.RestrictPrivate:
		restrict = "private"
	}
	tags := make([]map[string]any, 0, len(detail.Tags))
	for _, tag := range detail.Tags {
		tags = append(tags, map[string]any{"name": tag, "is_registered": true})
	}
	body, err := json.Marshal(struct {
		BookmarkDetail map[string]any `json:"bookmark_detail"`
	}{map[string]any{"is_bookmarked": restrict != "", "restrict": restrict, "tags": tags}})
	return http.StatusOK, body, err
}

func wireCommentPageResult(page pixiv.CommentPage) (int, []byte, error) {
	dtos := make([]pixiv.Comment, 0, len(page.Page.Items))
	for _, item := range page.Page.Items {
		dtos = append(dtos, item)
	}
	var total *int64
	if page.Total != nil {
		value := *page.Total
		total = &value
	}
	var accessControl *pixiv.CommentAccessControl
	var numericAccessControl *int64
	if page.AccessControl != nil {
		if page.AccessControl.NumericValue != nil {
			value := *page.AccessControl.NumericValue
			numericAccessControl = &value
		} else {
			converted := *page.AccessControl
			accessControl = &converted
		}
	}
	body, err := json.Marshal(struct {
		Comments             []pixiv.Comment             `json:"comments"`
		NextURL              *string                     `json:"next_url"`
		TotalComments        *int64                      `json:"total_comments"`
		AccessControl        *pixiv.CommentAccessControl `json:"access_control"`
		CommentAccessControl *int64                      `json:"comment_access_control"`
	}{dtos, nil, total, accessControl, numericAccessControl})
	return http.StatusOK, body, err
}

// openWireClient 直接打开一个 wire-responder 的 public SDK client（独立 transport）。
func openWireClient(t *testing.T, fake *fakeSDKClient) *pixiv.Client {
	t.Helper()
	if fake == nil {
		fake = &fakeSDKClient{userID: 42}
	}
	client, _, err := pixiv.OpenWith(context.Background(), "test-refresh-token", pixiv.Options{
		HTTPClient: &http.Client{Transport: &testSDKTransport{t: t, fake: fake}},
	})
	if err != nil {
		t.Fatalf("open test pixiv client: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func (tr *testSDKTransport) wireArtworkSeriesPage(page sdk.Page[pixiv.Artwork]) (int, []byte, error) {
	items := make([]map[string]any, 0, len(page.Items))
	userID := int64(0)
	for _, item := range page.Items {
		items = append(items, wireArtworkValue(item))
		if userID == 0 {
			userID = item.User.ID
		}
	}
	if userID == 0 {
		userID = 1
	}
	next := tr.nextPageURL(page.Next)
	body, err := json.Marshal(struct {
		SeriesDetail map[string]any   `json:"illust_series_detail"`
		Illusts      []map[string]any `json:"illusts"`
		NextURL      *string          `json:"next_url"`
	}{
		SeriesDetail: map[string]any{"user": wireUser{ID: userID, Name: "series-author"}},
		Illusts:      items,
		NextURL:      next,
	})
	return http.StatusOK, body, err
}

// TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords 保留各时间线的专属
// App API 请求断言，并固定所有实体结果使用共享 records 契约。

func TestMCPStdioKeepsJSONRPCOnStdout(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), "PIXIV_MCP_STDIO_HELPER=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	for _, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pixiv_search_illust","arguments":{"word":"stdio-secret-canary"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pixiv_bookmark_add","arguments":{"illust_id":41}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"pixiv_reverse_search","arguments":{"source":"https://stdio-source-secret.example/image.png?token=stdio-key"}}}`,
	} {
		if _, err := io.WriteString(stdin, message+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	scanner := bufio.NewScanner(stdout)
	lines := make([]string, 0, 4)
	for range 4 {
		if !scanner.Scan() {
			t.Fatalf("stdio server ended before responses: %v; stderr=%s", scanner.Err(), stderr.String())
		}
		lines = append(lines, scanner.Text())
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("stdio helper: %v\nstdout=%s\nstderr=%s", err, strings.Join(lines, "\n"), stderr.String())
	}
	protocol := strings.Join(lines, "\n")
	if !strings.Contains(protocol, `"jsonrpc":"2.0"`) || !strings.Contains(protocol, `"isError":true`) {
		t.Fatalf("stdout is not protocol-only: %s; stderr=%s", protocol, stderr.String())
	}
	for _, secret := range []string{"stdio-source-secret.example", "stdio-key"} {
		if strings.Contains(protocol, secret) {
			t.Fatalf("MCP stdout leaked %q: %s", secret, protocol)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must remain empty without project logging: %s", stderr.String())
	}
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("PIXIV_MCP_STDIO_HELPER") != "1" {
		return
	}
	client := &fakeSDKClient{addBookmarkErr: &sdk.Error{Product: "pixiv", Operation: "AddBookmark", Reason: sdk.UpstreamError}}
	server := newTestServer(testSDKPorts(t, client), pixivmcpserver.Account{})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestToolErrorResultPreservesStructuredContent(t *testing.T) {
	app := toolhost.NewApp(pixivmcpserver.SDKPorts{}, pixivmcpserver.Account{})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	toolhost.AddTool(app, server, &mcp.Tool{Name: "structured_error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]string, error) {
		return &mcp.CallToolResult{
			IsError:           true,
			Content:           []mcp.Content{&mcp.TextContent{Text: "structured failure"}},
			StructuredContent: map[string]string{"reason": "preserved"},
		}, map[string]string{"reason": "preserved"}, nil
	})
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
	result := callTool(t, session, "structured_error", map[string]any{})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("error result changed: %+v", result)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["reason"] != "preserved" {
		t.Fatalf("structured content lost: %#v", result.StructuredContent)
	}
}

func TestOpenClientPassesCanceledContextAndClosesLease(t *testing.T) {
	var calls atomic.Int32
	var closeCalls atomic.Int32
	client := openWireClient(t, &fakeSDKClient{userID: 42})
	app := toolhost.NewApp(pixivmcpserver.SDKPorts{
		OpenLease: func(ctx context.Context, _ pixivmcpserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
			calls.Add(1)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return lifecycle.NewLease(client, func() error {
				closeCalls.Add(1)
				return nil
			}), nil
		},
		Execute: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return nil
		},
	}, pixivmcpserver.Account{})
	lease, err := app.OpenClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.OpenClient(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("second open error=%v, want context canceled", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("SDK factory calls=%d, want 2", calls.Load())
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close lease: %v", err)
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("lease close calls=%d, want 1", closeCalls.Load())
	}
}

func TestSDKToolsWithoutSDKReturnStructuredConfigurationError(t *testing.T) {
	session, closeSession := newTestSession(t)
	defer closeSession()
	result := callTool(t, session, "pixiv_bookmark_add", map[string]any{"kind": "artwork", "id": 1})
	if !result.IsError {
		t.Fatalf("SDK configuration failure must be an MCP error result: %+v", result)
	}
	var out outputs.Mutation
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "sdk pooled operation is not configured") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSearchIllustCombinesBookmarkAndStructuredFiltersBeforePagination(t *testing.T) {
	for _, strategy := range []string{"auto", "local", "best_effort"} {
		for _, withRange := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/range=%t", strategy, withRange), func(t *testing.T) {
				calls := 0
				client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					calls++
					if request.Cursor.IsZero() {
						item := testSDKIllust(1, "wrong type", 7)
						item.TotalBookmarks = 20
						return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}, Next: testPageCursor(1)}, nil
					}
					items := []pixiv.Artwork{}
					for _, id := range []int64{2, 3, 4, 5} {
						item := testSDKIllust(id, "manga", 7)
						item.Kind = pixiv.ArtworkKindManga
						item.TotalBookmarks = 20
						if id == 2 {
							item.TotalBookmarks = 1
						}
						items = append(items, item)
					}
					return sdk.Page[pixiv.Artwork]{Items: items}, nil
				}}
				session, closeSession := newSDKTestSession(t, client)
				defer closeSession()
				args := map[string]any{"word": "cat", "bookmark_strategy": strategy, "illust_filter": map[string]any{"type": "manga"}, "page": 2, "limit": 1}
				want := "3"
				if withRange {
					args["bookmark_min"] = 10
					want = "4"
				}
				result := callTool(t, session, "pixiv_search_illust", args)
				if result.IsError {
					t.Fatalf("combined search failed: %+v", result)
				}
				var out outputs.Records
				decodeStructured(t, result, &out)
				if len(out.Records) != 1 || out.Records[0].ID() != want || calls != 2 {
					t.Fatalf("records=%v calls=%d, want ID %s after filtering and skip", out.Records, calls, want)
				}
			})
		}
	}
}

func TestSearchIllustBookmarkCompletenessAtExhaustion(t *testing.T) {
	for _, tc := range []struct {
		strategy  string
		withRange bool
	}{{"auto", true}, {"local", true}, {"best_effort", true}, {"best_effort", false}} {
		strategy := tc.strategy
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/range=%t/empty=%t", strategy, tc.withRange, empty), func(t *testing.T) {
				client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					items := []pixiv.Artwork{}
					if !empty {
						items = append(items, testSDKIllust(1, "match", 7))
					}
					return sdk.Page[pixiv.Artwork]{Items: items}, nil
				}}
				session, closeSession := newSDKTestSession(t, client)
				defer closeSession()
				args := map[string]any{"word": "cat", "bookmark_strategy": strategy}
				if tc.withRange {
					args["bookmark_min"] = 0
				}
				result := callTool(t, session, "pixiv_search_illust", args)
				if result.IsError {
					t.Fatalf("search failed: %+v", result)
				}
				var out outputs.Records
				decodeStructured(t, result, &out)
				want := "complete_for_source"
				if strategy == "best_effort" {
					want = "partial"
				}
				if out.Filter == nil || out.Filter.Completeness != want {
					t.Fatalf("filter=%+v, want %s", out.Filter, want)
				}
			})
		}
	}
}

func TestTypedBookmarkReadsRouteNovelOperations(t *testing.T) {
	client := &fakeSDKClient{
		novelBookmarks:        []pixiv.Novel{{ID: 101, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "story", Count: 3}}},
		novelBookmarkDetailResult: pixiv.NovelBookmarkDetail{
			Restrict: pixiv.RestrictPrivate,
			Tags:     []string{"story"},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	listResult := callTool(t, session, "pixiv_user_bookmarks", map[string]any{"kind": "novel",
		"user_id": 90, "restrict": "public", "tag": "story", "limit": 1,
	})
	if listResult.IsError || client.novelBookmarksRequest.UserID != 90 || client.novelBookmarksRequest.Restrict != pixiv.RestrictPublic || client.novelBookmarksRequest.Tag != "story" {
		t.Fatalf("novel bookmarks result=%+v request=%+v", listResult, client.novelBookmarksRequest)
	}
	var listOut outputs.Records
	decodeStructured(t, listResult, &listOut)
	if len(listOut.Records) != 1 || listOut.Records[0].ID() != "101" || listOut.Records[0].Type() != "novel" {
		t.Fatalf("novel bookmarks records=%+v", listOut.Records)
	}

	tagsResult := callTool(t, session, "pixiv_bookmark_tags", map[string]any{"kind": "novel",
		"user_id": 90, "restrict": "private", "limit": 1,
	})
	if tagsResult.IsError || client.novelBookmarkTagsRequest.UserID != 90 || client.novelBookmarkTagsRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("novel bookmark tags result=%+v request=%+v", tagsResult, client.novelBookmarkTagsRequest)
	}
	var tagsOut outputs.BookmarkTags
	decodeStructured(t, tagsResult, &tagsOut)
	if len(tagsOut.Tags) != 1 || tagsOut.Tags[0].Name != "story" || tagsOut.Tags[0].Count != 3 {
		t.Fatalf("novel bookmark tags=%+v", tagsOut)
	}

	defaultTags := callTool(t, session, "pixiv_bookmark_tags", map[string]any{"kind": "novel", "user_id": 0, "restrict": "", "page": nil, "limit": nil})
	if defaultTags.IsError || client.novelBookmarkTagsRequest.UserID != 42 {
		t.Fatal("merged tags must preserve zero/default user, empty visibility and null paging")
	}

	detailResult := callTool(t, session, "pixiv_novel_bookmark_detail", map[string]any{"novel_id": 101})
	if detailResult.IsError || client.novelBookmarkRequest.NovelID != 101 {
		t.Fatalf("novel bookmark detail result=%+v request=%+v", detailResult, client.novelBookmarkRequest)
	}
	var detailOut outputs.BookmarkDetail
	decodeStructured(t, detailResult, &detailOut)
	if !detailOut.Bookmarked || detailOut.Restrict != string(pixiv.RestrictPrivate) || !slices.Equal(detailOut.Tags, []string{"story"}) {
		t.Fatalf("novel bookmark detail=%+v", detailOut)
	}
}
func TestBookmarkDetailEmptyTagsSatisfyOutputSchema(t *testing.T) {
	for _, test := range []struct {
		name    string
		tool    string
		args    map[string]any
		client  *fakeSDKClient
		wantVis string
	}{
		{name: "artwork unbookmarked", tool: "pixiv_bookmark_detail", args: map[string]any{"illust_id": 101}, client: &fakeSDKClient{bookmarkDetailResult: pixiv.ArtworkBookmarkDetail{}}},
		{name: "artwork public", tool: "pixiv_bookmark_detail", args: map[string]any{"illust_id": 101}, client: &fakeSDKClient{bookmarkDetailResult: pixiv.ArtworkBookmarkDetail{Restrict: pixiv.RestrictPublic}}, wantVis: "public"},
		{name: "novel", tool: "pixiv_novel_bookmark_detail", args: map[string]any{"novel_id": 101}, client: &fakeSDKClient{novelBookmarkDetailResult: pixiv.NovelBookmarkDetail{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.args})
			if err != nil {
				t.Fatalf("schema validation: %v", err)
			}
			var out outputs.BookmarkDetail
			decodeStructured(t, result, &out)
			if result.IsError || out.Tags == nil || len(out.Tags) != 0 || out.Bookmarked != (test.wantVis != "") || out.Restrict != test.wantVis {
				t.Fatalf("result=%+v output=%+v, want tags=[] and visibility %q", result, out, test.wantVis)
			}
		})
	}
}
