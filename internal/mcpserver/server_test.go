package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
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

func TestLegacyToolLogsSafelyOutsideMCPProtocol(t *testing.T) {
	var logs bytes.Buffer
	server := New(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := callTool(t, session, "search_illust", map[string]any{"word": "query-secret-canary"})
	if result.IsError {
		t.Fatalf("unexpected MCP result: %+v", result)
	}
	if strings.Contains(fmt.Sprint(result.Content), "pixiv operation") {
		t.Fatalf("protocol content contains diagnostic log: %+v", result.Content)
	}
	got := logs.String()
	for _, want := range []string{`"component":"mcp"`, `"operation":"search_illust"`, `"result":"success"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "query-secret-canary") {
		t.Fatalf("MCP log leaked tool argument: %s", got)
	}
}

func TestMCPStdioKeepsJSONRPCOnStdoutAndLogsOnStderr(t *testing.T) {
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
	// StdioTransport 使用 newline-delimited JSON-RPC。输入仅含普通搜索词；
	// 断言依然覆盖完整 OS stdout/stderr 边界，而非仅内存 transport。
	for _, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_illust","arguments":{"word":"stdio-secret-canary"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"add_bookmark","arguments":{"illust_id":41}}}`,
	} {
		if _, err := io.WriteString(stdin, message+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	scanner := bufio.NewScanner(stdout)
	lines := make([]string, 0, 3)
	for range 3 {
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
	if !strings.Contains(protocol, `"jsonrpc":"2.0"`) || !strings.Contains(protocol, `"isError":true`) || strings.Contains(protocol, `"component":"mcp"`) {
		t.Fatalf("stdout is not protocol-only: %s; stderr=%s", protocol, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"component":"mcp"`) || !strings.Contains(stderr.String(), `"operation":"search_illust"`) || !strings.Contains(stderr.String(), `"operation":"add_bookmark"`) || !strings.Contains(stderr.String(), `"result":"error"`) {
		t.Fatalf("stderr lacks MCP event: %s", stderr.String())
	}
	if strings.Contains(stderr.String(), "stdio-secret-canary") {
		t.Fatalf("stderr leaked tool input: %s", stderr.String())
	}
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("PIXIV_MCP_STDIO_HELPER") != "1" {
		return
	}
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &failingMutationSDKClient{err: &sdk.Error{Code: sdk.CodeUpstreamError, Backend: sdk.BackendAppAPI, IllustID: 41}}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(os.Stderr, nil)), service, application.SDKClientRequest{})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestToolLoggingWrapperPreservesStructuredErrorResult(t *testing.T) {
	var logs bytes.Buffer
	app := &App{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	addTool(app, server, &mcp.Tool{Name: "structured_error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]string, error) {
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
	defer session.Close()
	result := callTool(t, session, "structured_error", map[string]any{})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("error result changed: %+v", result)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["reason"] != "preserved" {
		t.Fatalf("structured content lost: %#v", result.StructuredContent)
	}
	if !strings.Contains(logs.String(), `"result":"error"`) || !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("error event missing: %s", logs.String())
	}
}

func TestSDKOperationGateRespectsCanceledContext(t *testing.T) {
	var calls atomic.Int32
	app := &App{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		sdkGate: make(chan struct{}, 1),
		sdk: application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
			calls.Add(1)
			return &fakeSDKClient{}, nil
		}},
	}
	_, release, err := app.openSDKOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := app.openSDKOperation(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("second open error=%v, want context canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("SDK factory calls=%d, want 1", calls.Load())
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
	if !result.IsError {
		t.Fatalf("SDK configuration failure must be an MCP error result: %+v", result)
	}
	var out mutationOut
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "sdk is not configured") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSDKListValidationReturnsMCPErrorWithStructuredOutput(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{})
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 9, "page": 0, "limit": 1})
	if !result.IsError {
		t.Fatalf("invalid page must be an MCP error result: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("error result must retain text content: %+v", result.Content)
	}
	var out illustListOut
	decodeStructured(t, result, &out)
	if out.UserID != 9 || len(out.Items) != 0 || !strings.Contains(out.Text, "page must be a positive integer") {
		t.Fatalf("structured validation error = %+v", out)
	}
}

func TestLegacyUserListFailureReturnsMCPErrorWithStructuredOutput(t *testing.T) {
	server := New(&failingLegacyBookmarksAPI{}, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 9})
	if !result.IsError {
		t.Fatalf("legacy failure must be an MCP error result: %+v", result)
	}
	var out illustListOut
	decodeStructured(t, result, &out)
	if out.UserID != 9 || len(out.Items) != 0 || !strings.Contains(out.Text, "legacy failed") {
		t.Fatalf("structured legacy error = %+v", out)
	}
}

func TestSDKMutationTypedErrorIsMCPErrorAndWrapperLogsMetadata(t *testing.T) {
	var logs bytes.Buffer
	client := &failingMutationSDKClient{err: &sdk.Error{
		Code:           sdk.CodeUpstreamError,
		Backend:        sdk.BackendAppAPI,
		UpstreamStatus: http.StatusBadGateway,
		IllustID:       41,
	}}
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return client, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 41})
	if !result.IsError {
		t.Fatalf("typed SDK mutation failure must be an MCP error: %+v", result)
	}
	var out mutationOut
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured mutation error = %+v", out)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["operation"] == "add_bookmark" && event["result"] == "error" {
			found = true
			if event["error_code"] != string(sdk.CodeUpstreamError) || event["backend"] != string(sdk.BackendAppAPI) || event["status"] != float64(http.StatusBadGateway) || event["illust_id"] != float64(41) {
				t.Fatalf("wrapper dropped typed SDK metadata: %v", event)
			}
		}
	}
	if !found {
		t.Fatalf("missing add_bookmark typed error event: %s", logs.String())
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

func TestNewKeepsLegacyUserListToolsAndParameters(t *testing.T) {
	api := &legacyUserToolsAPI{
		bookmarks: []pixiv.Illust{{ID: 15, Tags: []pixiv.Tag{}, MetaPages: []pixiv.MetaPage{}}},
		following: []pixiv.UserPreview{{User: pixiv.User{ID: 31, Name: "legacy"}}},
	}
	server := New(api, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	bookmarkResult := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id_to_check": 9, "restrict": "private", "tag": "legacy-tag", "max_bookmark_id": 101,
	})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if api.bookmarkUserID != 9 || api.bookmarkRestrict != "private" || api.bookmarkTag != "legacy-tag" || api.maxBookmarkID != 101 || !strings.Contains(bookmarksOut.Text, "找到用户 9 的 1 个收藏") {
		t.Fatalf("legacy bookmarks request=(%d,%q,%q,%d) output=%+v", api.bookmarkUserID, api.bookmarkRestrict, api.bookmarkTag, api.maxBookmarkID, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id_to_check": 8, "restrict": "private", "offset": 12,
	})
	var followingOut userListOut
	decodeStructured(t, followingResult, &followingOut)
	if api.followingUserID != 8 || api.followingRestrict != "private" || api.followingOffset != 12 || !strings.Contains(followingOut.Text, "用户 8 关注了 1 位用户") {
		t.Fatalf("legacy following request=(%d,%q,%d) output=%+v", api.followingUserID, api.followingRestrict, api.followingOffset, followingOut)
	}

	api.bookmarks = []pixiv.Illust{}
	api.following = []pixiv.UserPreview{}
	bookmarkResult = callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 9})
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	followingResult = callTool(t, session, "user_following", map[string]any{"user_id_to_check": 8})
	decodeStructured(t, followingResult, &followingOut)
	if bookmarksOut.Text != "找不到用户 9 的收藏。" || followingOut.Text != "用户 8 没有关注任何人。" {
		t.Fatalf("legacy empty text bookmarks=%q following=%q", bookmarksOut.Text, followingOut.Text)
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

func TestSDKToolsPersistRotationAfterSessionTokenAndSerializeConcurrentOperations(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":1,"accounts":[{"user_id":1,"username":"old","refresh_token":"old-a"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := []string{"r1", "r2", "r3", "r4"}
	var oauthMu sync.Mutex
	oauthCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse oauth form: %v", err)
				return
			}
			oauthMu.Lock()
			defer oauthMu.Unlock()
			if oauthCalls >= len(expected) || r.Form.Get("refresh_token") != expected[oauthCalls] {
				http.Error(w, "unexpected rotated token", http.StatusUnauthorized)
				return
			}
			oauthCalls++
			_, _ = fmt.Fprintf(w, `{"access_token":"access-%d","refresh_token":"r%d","user":{"id":7,"name":"alice"}}`, oauthCalls, oauthCalls+1)
		case "/v1/user/illusts":
			_, _ = w.Write([]byte(`{"illusts":[],"next_url":null}`))
		case "/v2/illust/bookmark/add", "/v1/illust/bookmark/delete":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
		return sdk.OpenDefault(sdk.Options{
			AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(),
			OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL,
			UserID: request.UserID, RefreshToken: request.RefreshToken,
		})
	}}
	source := &rotatingSessionAPI{refreshToken: "r0", userID: 7, userName: "alice"}
	// 模拟 RunMCP AutoAuthenticate 已选择旧 UID A；set_refresh_token 随后认证到 UID B。
	session, closeSession := newSDKTestSessionWithServiceRequest(t, source, service, application.SDKClientRequest{AuthFilePath: authPath, UserID: 1})
	defer closeSession()
	set := callTool(t, session, "set_refresh_token", map[string]any{"refresh_token": "r0"})
	var setOut textOut
	decodeStructured(t, set, &setOut)
	if !strings.Contains(setOut.Text, "完成认证") {
		t.Fatalf("set_refresh_token=%q", setOut.Text)
	}
	_ = callTool(t, session, "user_artworks", map[string]any{"user_id": 7})
	_ = callTool(t, session, "user_artworks", map[string]any{"user_id": 7})

	errCh := make(chan error, 2)
	for _, call := range []*mcp.CallToolParams{
		{Name: "add_bookmark", Arguments: map[string]any{"illust_id": 9}},
		{Name: "remove_bookmark", Arguments: map[string]any{"illust_id": 9}},
	} {
		go func(call *mcp.CallToolParams) {
			_, err := session.CallTool(context.Background(), call)
			errCh <- err
		}(call)
	}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent SDK tool: %v", err)
		}
	}
	oauthMu.Lock()
	calls := oauthCalls
	oauthMu.Unlock()
	if calls != len(expected) {
		t.Fatalf("oauth calls=%d want=%d", calls, len(expected))
	}
	stored, err := os.ReadFile(authPath)
	if err != nil || !strings.Contains(string(stored), `"refresh_token": "r5"`) || strings.Contains(string(stored), `"refresh_token": "r1"`) {
		t.Fatalf("auth store did not retain latest rotation: %q err=%v", stored, err)
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
	return newSDKTestSessionWithServiceRequest(t, api, service, application.SDKClientRequest{})
}

func newSDKTestSessionWithServiceRequest(t *testing.T, api PixivAPI, service application.SDKService, request application.SDKClientRequest) (*mcp.ClientSession, func()) {
	t.Helper()
	server := NewWithSDK(api, &fakeDownloads{}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, request)
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

type rotatingSessionAPI struct {
	refreshToken string
	userID       int64
	userName     string
}

func (a *rotatingSessionAPI) Refresh(context.Context) error {
	if a.refreshToken != "r0" {
		return errors.New("unexpected legacy refresh token")
	}
	a.refreshToken = "r1"
	return nil
}
func (a *rotatingSessionAPI) SetRefreshToken(token string) { a.refreshToken = token }
func (a *rotatingSessionAPI) RefreshTokenValue() string    { return a.refreshToken }
func (a *rotatingSessionAPI) UserID() int64                { return a.userID }
func (a *rotatingSessionAPI) UserName() string             { return a.userName }
func (*rotatingSessionAPI) IsAuthenticated() bool          { return false }
func (*rotatingSessionAPI) SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error) {
	return &pixiv.IllustDetail{}, nil
}
func (*rotatingSessionAPI) IllustRelated(context.Context, int64, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) SearchUser(context.Context, string, int) (*pixiv.UserPreviewList, error) {
	return &pixiv.UserPreviewList{}, nil
}
func (*rotatingSessionAPI) IllustRecommended(context.Context, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) TrendingTagsIllust(context.Context) (*pixiv.TrendTags, error) {
	return &pixiv.TrendTags{}, nil
}
func (*rotatingSessionAPI) IllustFollow(context.Context, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) UserBookmarks(context.Context, int64, string, string, int64) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (*rotatingSessionAPI) UserFollowing(context.Context, int64, string, int) (*pixiv.UserPreviewList, error) {
	return &pixiv.UserPreviewList{}, nil
}
func (*rotatingSessionAPI) Download(context.Context, string, io.Writer) error { return nil }

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

type legacyUserToolsAPI struct {
	fakeAPI
	bookmarks         []pixiv.Illust
	following         []pixiv.UserPreview
	bookmarkUserID    int64
	bookmarkRestrict  string
	bookmarkTag       string
	maxBookmarkID     int64
	followingUserID   int64
	followingRestrict string
	followingOffset   int
}

func (a *legacyUserToolsAPI) UserBookmarks(_ context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*pixiv.IllustList, error) {
	a.bookmarkUserID = userID
	a.bookmarkRestrict = restrict
	a.bookmarkTag = tag
	a.maxBookmarkID = maxBookmarkID
	return &pixiv.IllustList{Illusts: a.bookmarks}, nil
}

func (a *legacyUserToolsAPI) UserFollowing(_ context.Context, userID int64, restrict string, offset int) (*pixiv.UserPreviewList, error) {
	a.followingUserID = userID
	a.followingRestrict = restrict
	a.followingOffset = offset
	return &pixiv.UserPreviewList{UserPreviews: a.following}, nil
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

type failingMutationSDKClient struct {
	fakeSDKClient
	err error
}

func (f *failingMutationSDKClient) AddBookmark(context.Context, sdk.AddBookmarkRequest) error {
	return f.err
}

type failingLegacyBookmarksAPI struct{ fakeAPI }

func (*failingLegacyBookmarksAPI) UserBookmarks(context.Context, int64, string, string, int64) (*pixiv.IllustList, error) {
	return nil, errors.New("legacy failed")
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

// UserDetail 仅保持 SDK 测试替身满足窄接口；MCP user_detail tool 将在后续任务单独接入。
func (*fakeSDKClient) UserDetail(context.Context, sdk.UserDetailRequest) (*sdk.UserDetailResult, error) {
	return &sdk.UserDetailResult{}, nil
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
