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

	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
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
	for _, want := range []string{"set_download_path", "download", "refresh_token", "set_refresh_token", "download_random_from_recommendation", "search_illust", "search_user", "trending_tags_illust", "illust_related", "illust_recommended", "illust_follow", "user_bookmarks", "user_following", "illust_detail", "illust_ranking", "get_thumbnail_base64"} {
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
