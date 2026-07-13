package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerListsExpectedTools(t *testing.T) {
	server := New(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	var names []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	for _, want := range []string{"set_download_path", "download", "refresh_token", "set_refresh_token", "download_random_from_recommendation", "search_illust", "search_user", "trending_tags_illust", "illust_related", "illust_recommended", "illust_follow", "user_artworks", "user_bookmarks", "user_following", "add_bookmark", "remove_bookmark", "follow_user", "unfollow_user", "illust_detail", "illust_ranking", "get_thumbnail_base64"} {
		if !slices.Contains(names, want) {
			t.Fatalf("tool %q missing from %v", want, names)
		}
	}
}

func TestSetDownloadPathValidation(t *testing.T) {
	server := New(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "set_download_path", Arguments: map[string]any{"path": ""}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("expected content")
	}
}

func TestSDKToolsWithoutSDKReturnStructuredConfigurationError(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()
	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 1})
	var out mutationOut
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "sdk is not configured") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestDownloadDefaultsToLocalPathResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []download.DownloadedArtwork{{
		IllustID: 42,
		Title:    "title",
		Author:   "artist",
		Type:     "illust",
		Files:    []download.DownloadedFile{{Path: path}},
	}}}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "download",
		Arguments: map[string]any{"illust_ids": []int64{42, 42, -1}},
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
	if out.Delivery != deliveryLocalPath || len(out.Files) != 1 {
		t.Fatalf("unexpected structured output: %+v", out)
	}
	if out.Files[0].MIMEType != "image/jpeg" || out.Files[0].SizeBytes != 4 || !strings.HasPrefix(out.Files[0].FileURI, "file://") {
		t.Fatalf("unexpected file output: %+v", out.Files[0])
	}
	if !slices.Equal(downloads.downloadIDs, []int64{42, 42, -1}) {
		t.Fatalf("download IDs = %v", downloads.downloadIDs)
	}
}

func TestDownloadImageContentResult(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "1.png")
	second := filepath.Join(dir, "2.gif")
	if err := os.WriteFile(first, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("gif"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []download.DownloadedArtwork{{
		IllustID: 1,
		Title:    "multi",
		Author:   "artist",
		Type:     "illust",
		Files: []download.DownloadedFile{
			{Path: first},
			{Path: second, Page: 1},
		},
	}}}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "download",
		Arguments: map[string]any{"illust_id": 1, "delivery": deliveryImageContent},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 3 {
		t.Fatalf("content len = %d, want text + 2 images", len(result.Content))
	}
	if _, ok := result.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	firstImage, ok := result.Content[1].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[1] = %T, want ImageContent", result.Content[1])
	}
	secondImage, ok := result.Content[2].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[2] = %T, want ImageContent", result.Content[2])
	}
	if firstImage.MIMEType != "image/png" || string(firstImage.Data) != "png" {
		t.Fatalf("first image = %+v", firstImage)
	}
	if secondImage.MIMEType != "image/gif" || string(secondImage.Data) != "gif" {
		t.Fatalf("second image = %+v", secondImage)
	}
	out := decodeDownloadOut(t, result)
	if out.Delivery != deliveryImageContent || len(out.Files) != 2 {
		t.Fatalf("unexpected structured output: %+v", out)
	}
}

func TestSetRefreshTokenRejectsCookieWithoutRefreshToken(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "set_refresh_token",
		Arguments: map[string]any{"refresh_token": "PHPSESSID=web; device_token=device; yuid_b=user"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len = %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "没有 refresh_token") || !strings.Contains(text.Text, "PHPSESSID/device_token") {
		t.Fatalf("unexpected text: %s", text.Text)
	}
}

func TestSetRefreshTokenSuccessIncludesUserName(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "set_refresh_token",
		Arguments: map[string]any{"refresh_token": "good-token"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "用户 ID: 1") || !strings.Contains(text.Text, "用户名: alice") {
		t.Fatalf("unexpected success text: %s", text.Text)
	}
}

func TestSetRefreshTokenFailureSaysSessionOnly(t *testing.T) {
	server := New(&failingRefreshAPI{}, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "set_refresh_token",
		Arguments: map[string]any{"refresh_token": "bad-token"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T", result.Content[0])
	}
	if strings.Contains(text.Text, "已保存") {
		t.Fatalf("failure text claims token was saved: %s", text.Text)
	}
	if !strings.Contains(text.Text, "当前会话") {
		t.Fatalf("failure text should clarify session-only scope: %s", text.Text)
	}
}

func TestSDKUserToolsResolveIdentityKeepLegacyInputAndReturnStructuredOutput(t *testing.T) {
	client := &fakeSDKClient{
		userID:    71,
		artworks:  []sdk.Illust{testSDKIllust(11, "work", 71)},
		bookmarks: []sdk.Illust{testSDKIllust(12, "saved", 99)},
		following: []sdk.UserPreview{{User: sdk.User{ID: 33, Name: "followed", Account: "f"}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	artworks := callTool(t, session, "user_artworks", map[string]any{"limit": 1})
	var artworksOut illustListOut
	decodeStructured(t, artworks, &artworksOut)
	if client.artworksRequest.UserID != 71 || artworksOut.UserID != 71 || len(artworksOut.Items) != 1 || artworksOut.Pagination.Returned != 1 {
		t.Fatalf("user artworks = request=%+v output=%+v", client.artworksRequest, artworksOut)
	}

	bookmarks := callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 99, "tag": "tag", "limit": 0})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarks, &bookmarksOut)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Tag != "tag" || bookmarksOut.UserID != 99 || len(bookmarksOut.Items) != 1 || bookmarksOut.Pagination.HasMore {
		t.Fatalf("bookmarks = request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}
	if !strings.Contains(bookmarksOut.Text, "找到用户 99 的 1 个收藏") {
		t.Fatalf("legacy bookmark text missing: %q", bookmarksOut.Text)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id_to_check": 99, "offset": 0})
	var followingOut userListOut
	decodeStructured(t, following, &followingOut)
	if client.followingRequest.UserID != 99 || followingOut.UserID != 99 || len(followingOut.Items) != 1 {
		t.Fatalf("following = request=%+v output=%+v", client.followingRequest, followingOut)
	}
	if !strings.Contains(followingOut.Text, "用户 99 关注了 1 位用户") {
		t.Fatalf("legacy following text missing: %q", followingOut.Text)
	}
}

func testSDKIllust(id int64, title string, userID int64) sdk.Illust {
	return sdk.Illust{
		ID:        id,
		Title:     title,
		User:      sdk.User{ID: userID, Name: "artist"},
		Tags:      []sdk.Tag{},
		MetaPages: []sdk.MetaPage{},
	}
}

func TestSDKMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"add_bookmark", map[string]any{"illust_id": 9, "restrict": "private", "tags": []string{"one"}}, "add_bookmark"},
		{"remove_bookmark", map[string]any{"illust_id": 9}, "remove_bookmark"},
		{"follow_user", map[string]any{"user_id": 8, "restrict": "private"}, "follow_user"},
		{"unfollow_user", map[string]any{"user_id": 8}, "unfollow_user"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			var out mutationOut
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.want || !strings.Contains(out.Text, "已") {
				t.Fatalf("mutation output = %+v", out)
			}
		})
	}
	if client.addBookmarkRequest.IllustID != 9 || client.addBookmarkRequest.Restrict != sdk.RestrictPrivate || !slices.Equal(client.addBookmarkRequest.Tags, []string{"one"}) {
		t.Fatalf("add bookmark request = %+v", client.addBookmarkRequest)
	}
	if client.removeBookmarkRequest.IllustID != 9 || client.followUserRequest.UserID != 8 || client.followUserRequest.Restrict != sdk.RestrictPrivate || client.unfollowUserRequest.UserID != 8 {
		t.Fatalf("mutation requests = remove=%+v follow=%+v unfollow=%+v", client.removeBookmarkRequest, client.followUserRequest, client.unfollowUserRequest)
	}
}

func TestUserBookmarksAcceptsLegacyMaxBookmarkID(t *testing.T) {
	legacyAPI := &legacyBookmarkAPI{illusts: []pixiv.Illust{{
		ID:        15,
		Title:     "legacy",
		Tags:      []pixiv.Tag{},
		MetaPages: []pixiv.MetaPage{},
	}}}
	session, closeSession := newSDKTestSessionWithAPI(t, legacyAPI, &fakeSDKClient{})
	defer closeSession()
	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 99, "max_bookmark_id": 101})
	var out illustListOut
	decodeStructured(t, result, &out)
	if legacyAPI.userID != 99 || legacyAPI.maxBookmarkID != 101 || len(out.Items) != 1 || out.Items[0].ID != 15 {
		t.Fatalf("legacy compatibility = request=(%d,%d) output=%+v", legacyAPI.userID, legacyAPI.maxBookmarkID, out)
	}
}

func TestSDKListPageRequiresPositiveLimitAndPositiveValue(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
	defer closeSession()
	for _, arguments := range []map[string]any{
		{"page": 0, "limit": 1},
		{"page": 1},
	} {
		result := callTool(t, session, "user_artworks", arguments)
		var out illustListOut
		decodeStructured(t, result, &out)
		if !strings.Contains(out.Text, "page") {
			t.Fatalf("arguments=%v output=%+v", arguments, out)
		}
	}
}

func TestSDKListsFollowOpaqueCursorForLimitAndRejectCycles(t *testing.T) {
	first := testSDKIllust(1, "first", 7)
	second := testSDKIllust(2, "second", 7)
	paged := &fakeSDKClient{userID: 7, artworkResults: map[sdk.Cursor]sdk.IllustListResult{
		"": {Illusts: []sdk.Illust{first}, NextCursor: "next"},
	}}
	pagedSession, closePagedSession := newSDKTestSession(t, paged)
	defer closePagedSession()
	result := callTool(t, pagedSession, "user_artworks", map[string]any{"limit": 1})
	var out illustListOut
	decodeStructured(t, result, &out)
	if !out.Pagination.HasMore || out.Pagination.NextPage == nil || *out.Pagination.NextPage != 2 {
		t.Fatalf("single-page pagination=%+v", out.Pagination)
	}

	client := &fakeSDKClient{userID: 7, artworkResults: map[sdk.Cursor]sdk.IllustListResult{
		"":     {Illusts: []sdk.Illust{first}, NextCursor: "next"},
		"next": {Illusts: []sdk.Illust{second}},
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result = callTool(t, session, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if len(out.Items) != 2 || out.Pagination.HasMore || len(client.artworksRequests) != 2 || client.artworksRequests[1].Cursor != "next" {
		t.Fatalf("all-pages output=%+v requests=%+v", out, client.artworksRequests)
	}

	cyclic := &fakeSDKClient{userID: 7, artworkResults: map[sdk.Cursor]sdk.IllustListResult{
		"":     {Illusts: []sdk.Illust{first}, NextCursor: "loop"},
		"loop": {NextCursor: "loop"},
	}}
	cycleSession, closeCycleSession := newSDKTestSession(t, cyclic)
	defer closeCycleSession()
	result = callTool(t, cycleSession, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if !strings.Contains(out.Text, "cursor repeated") || len(cyclic.artworksRequests) != 2 {
		t.Fatalf("cycle output=%+v requests=%+v", out, cyclic.artworksRequests)
	}
}

func TestSDKToolsUseExistingMCPSessionRefreshToken(t *testing.T) {
	var got application.SDKClientRequest
	service := application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
		got = request
		return &fakeSDKClient{}, nil
	}}
	session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
	defer closeSession()
	_ = callTool(t, session, "add_bookmark", map[string]any{"illust_id": 1})
	if got.RefreshToken != "refresh" {
		t.Fatalf("SDK request did not receive MCP session token: %+v", got)
	}
}

type fakeAPI struct{}

func (fakeAPI) Refresh(context.Context) error { return nil }
func (fakeAPI) SetRefreshToken(string)        {}
func (fakeAPI) RefreshTokenValue() string     { return "refresh" }
func (fakeAPI) UserID() int64                 { return 1 }
func (fakeAPI) UserName() string              { return "alice" }
func (fakeAPI) IsAuthenticated() bool         { return true }
func (fakeAPI) SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (fakeAPI) IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error) {
	return &pixiv.IllustDetail{}, nil
}
func (fakeAPI) IllustRelated(context.Context, int64, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (fakeAPI) IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (fakeAPI) SearchUser(context.Context, string, int) (*pixiv.UserPreviewList, error) {
	return &pixiv.UserPreviewList{}, nil
}
func (fakeAPI) IllustRecommended(context.Context, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{Illusts: []pixiv.Illust{{ID: 1}}}, nil
}
func (fakeAPI) TrendingTagsIllust(context.Context) (*pixiv.TrendTags, error) {
	return &pixiv.TrendTags{}, nil
}
func (fakeAPI) IllustFollow(context.Context, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (fakeAPI) UserBookmarks(context.Context, int64, string, string, int64) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (fakeAPI) UserFollowing(context.Context, int64, string, int) (*pixiv.UserPreviewList, error) {
	return &pixiv.UserPreviewList{}, nil
}
func (fakeAPI) Download(context.Context, string, io.Writer) error {
	return nil
}

type failingRefreshAPI struct {
	fakeAPI
}

func (failingRefreshAPI) Refresh(context.Context) error {
	return errors.New("invalid token")
}

func newTestSession(t *testing.T, downloads *fakeDownloads) (*mcp.ClientSession, func()) {
	t.Helper()
	server := New(&fakeAPI{}, downloads, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
		session.Close()
		cancel()
	}
}

func newSDKTestSession(t *testing.T, sdkClient application.SDKClient) (*mcp.ClientSession, func()) {
	return newSDKTestSessionWithAPI(t, &fakeAPI{}, sdkClient)
}

func newSDKTestSessionWithAPI(t *testing.T, api PixivAPI, sdkClient application.SDKClient) (*mcp.ClientSession, func()) {
	t.Helper()
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return sdkClient, nil
	}}
	return newSDKTestSessionWithService(t, api, service)
}

func newSDKTestSessionWithService(t *testing.T, api PixivAPI, service application.SDKService) (*mcp.ClientSession, func()) {
	t.Helper()
	server := NewWithSDK(api, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{})
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
		session.Close()
		cancel()
	}
}

type legacyBookmarkAPI struct {
	fakeAPI
	illusts       []pixiv.Illust
	userID        int64
	maxBookmarkID int64
}

func (a *legacyBookmarkAPI) UserBookmarks(_ context.Context, userID int64, _ string, _ string, maxBookmarkID int64) (*pixiv.IllustList, error) {
	a.userID = userID
	a.maxBookmarkID = maxBookmarkID
	return &pixiv.IllustList{Illusts: a.illusts}, nil
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

type fakeSDKClient struct {
	userID                int64
	artworks              []sdk.Illust
	bookmarks             []sdk.Illust
	following             []sdk.UserPreview
	artworksRequest       sdk.UserArtworksRequest
	artworksRequests      []sdk.UserArtworksRequest
	artworkResults        map[sdk.Cursor]sdk.IllustListResult
	bookmarksRequest      sdk.UserBookmarksRequest
	followingRequest      sdk.UserFollowingRequest
	addBookmarkRequest    sdk.AddBookmarkRequest
	removeBookmarkRequest sdk.RemoveBookmarkRequest
	followUserRequest     sdk.FollowUserRequest
	unfollowUserRequest   sdk.UnfollowUserRequest
}

func (f *fakeSDKClient) CurrentUserID(context.Context) (int64, error) { return f.userID, nil }
func (*fakeSDKClient) SearchIllust(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) IllustDetail(context.Context, int64) (*sdk.IllustDetail, error) {
	return &sdk.IllustDetail{}, nil
}
func (*fakeSDKClient) IllustRanking(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) IllustRecommended(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (f *fakeSDKClient) UserArtworks(_ context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
	f.artworksRequest = request
	f.artworksRequests = append(f.artworksRequests, request)
	if f.artworkResults != nil {
		result := f.artworkResults[request.Cursor]
		return &result, nil
	}
	return &sdk.IllustListResult{Illusts: f.artworks}, nil
}
func (f *fakeSDKClient) UserBookmarks(_ context.Context, request sdk.UserBookmarksRequest) (*sdk.IllustListResult, error) {
	f.bookmarksRequest = request
	return &sdk.IllustListResult{Illusts: f.bookmarks}, nil
}
func (f *fakeSDKClient) UserFollowing(_ context.Context, request sdk.UserFollowingRequest) (*sdk.UserListResult, error) {
	f.followingRequest = request
	return &sdk.UserListResult{UserPreviews: f.following}, nil
}
func (f *fakeSDKClient) AddBookmark(_ context.Context, request sdk.AddBookmarkRequest) error {
	f.addBookmarkRequest = request
	return nil
}
func (f *fakeSDKClient) RemoveBookmark(_ context.Context, request sdk.RemoveBookmarkRequest) error {
	f.removeBookmarkRequest = request
	return nil
}
func (f *fakeSDKClient) FollowUser(_ context.Context, request sdk.FollowUserRequest) error {
	f.followUserRequest = request
	return nil
}
func (f *fakeSDKClient) UnfollowUser(_ context.Context, request sdk.UnfollowUserRequest) error {
	f.unfollowUserRequest = request
	return nil
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

type fakeDownloads struct {
	artworks    []download.DownloadedArtwork
	downloadIDs []int64
}

func (fakeDownloads) SetDownloadPath(string) error         { return nil }
func (fakeDownloads) Enqueue(context.Context, []int64) int { return 1 }
func (d *fakeDownloads) Download(_ context.Context, ids []int64) ([]download.DownloadedArtwork, error) {
	d.downloadIDs = append([]int64(nil), ids...)
	return d.artworks, nil
}
