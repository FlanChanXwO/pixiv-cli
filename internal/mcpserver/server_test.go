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
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
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
	for _, want := range []string{"set_download_path", "download", "refresh_token", "set_refresh_token", "download_random_from_recommendation", "search_illust", "search_user", "trending_tags_illust", "illust_related", "illust_recommended", "recommended", "illust_follow", "user_artworks", "user_bookmarks", "user_following", "add_bookmark", "remove_bookmark", "follow_user", "unfollow_user", "illust_detail", "illust_ranking", "get_thumbnail_base64"} {
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

func TestSDKUserBookmarksFailureReturnsMCPErrorWithStructuredOutput(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{
		userBookmarksErr: errors.New("bookmarks upstream failed"),
	})
	defer closeSession()

	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 9})
	if !result.IsError {
		t.Fatalf("SDK failure must be an MCP error result: %+v", result)
	}
	var out illustListOut
	decodeStructured(t, result, &out)
	if out.UserID != 9 || len(out.Items) != 0 || !strings.Contains(out.Text, "bookmarks upstream failed") {
		t.Fatalf("structured SDK error = %+v", out)
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

func TestSetRefreshTokenRejectsCookieInput(t *testing.T) {
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
	if !strings.Contains(text.Text, "cookie input is not supported; provide a Pixiv App API refresh token") {
		t.Fatalf("unexpected text: %s", text.Text)
	}
}

func TestRefreshTokenOpenFailuresAreSafeAndDiagnostic(t *testing.T) {
	const sensitiveFactoryError = "proxy http://proxy-user:proxy-password@127.0.0.1:7890 token=secret-token config=/secret/config.json"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unknown factory or config error",
			err:  errors.New(sensitiveFactoryError),
			want: "Token刷新失败：无法初始化 Pixiv SDK。请检查本地配置或代理设置。",
		},
		{name: "canceled", err: context.Canceled, want: "Token刷新已取消。"},
		{name: "deadline", err: context.DeadlineExceeded, want: "Token刷新失败：操作超时。"},
		{
			name: "public SDK unauthorized is not treated as missing token",
			err:  &sdk.Error{Code: sdk.CodeUnauthorized, Operation: sdk.OperationSnapshot},
			want: "Token刷新失败：无法初始化 Pixiv SDK：pixiv unauthorized operation=snapshot。",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
				return nil, tt.err
			}}
			session, cleanup := newSDKTestSessionWithService(t, &fakeAPI{}, service)
			defer cleanup()

			result := callTool(t, session, "refresh_token", map[string]any{})
			var out textOut
			decodeStructured(t, result, &out)
			if out.Text != tt.want {
				t.Fatalf("refresh_token output=%q, want %q", out.Text, tt.want)
			}
			for _, secret := range []string{"proxy-user", "proxy-password", "127.0.0.1:7890", "secret-token", "/secret/config.json"} {
				if strings.Contains(out.Text, secret) {
					t.Fatalf("refresh_token output leaked %q: %q", secret, out.Text)
				}
			}
		})
	}
}

func TestRefreshTokenFailureClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unauthorized keeps missing token hint",
			err:  &sdk.Error{Code: sdk.CodeUnauthorized, Operation: sdk.OperationRefresh},
			want: "错误：未设置 refresh token。请先使用 set_refresh_token 工具设置 token。",
		},
		{name: "canceled", err: context.Canceled, want: "Token刷新已取消。"},
		{name: "deadline", err: context.DeadlineExceeded, want: "Token刷新失败：操作超时。"},
		{
			name: "public SDK error keeps safe cause",
			err: &sdk.Error{
				Code:      sdk.CodeUpstreamUnavailable,
				Operation: sdk.OperationRefresh,
				Backend:   sdk.BackendOAuth,
			},
			want: "Token刷新失败：pixiv upstream_unavailable operation=refresh backend=oauth。",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, cleanup := newSDKTestSession(t, &failingRefreshSDKClient{err: tt.err})
			defer cleanup()

			result := callTool(t, session, "refresh_token", map[string]any{})
			var out textOut
			decodeStructured(t, result, &out)
			if out.Text != tt.want {
				t.Fatalf("refresh_token output=%q, want %q", out.Text, tt.want)
			}
		})
	}
}

func TestSetRefreshTokenSuccessIncludesUserName(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
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
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{importAccountErr: errors.New("invalid token")})
	defer closeSession()

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
		t.Fatalf("bookmark text missing: %q", bookmarksOut.Text)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id_to_check": 99, "offset": 0})
	var followingOut userListOut
	decodeStructured(t, following, &followingOut)
	if client.followingRequest.UserID != 99 || followingOut.UserID != 99 || len(followingOut.Items) != 1 {
		t.Fatalf("following = request=%+v output=%+v", client.followingRequest, followingOut)
	}
	if !strings.Contains(followingOut.Text, "用户 99 关注了 1 位用户") {
		t.Fatalf("following text missing: %q", followingOut.Text)
	}
}

func TestSDKUserDetailReturnsStructuredSDKResult(t *testing.T) {
	webpage := "https://example.test/artist"
	workspaceImage := "https://example.test/workspace.png"
	want := &sdk.UserDetailResult{
		User:             sdk.User{ID: 42, Name: "artist", Account: "artist_account", Comment: "hello"},
		Profile:          sdk.Profile{Webpage: &webpage, Region: "Tokyo", CountryCode: "JP", Job: "illustrator", TotalIllusts: 10, TotalManga: 2, TotalNovels: 3, TotalFollowUsers: 4},
		ProfilePublicity: sdk.ProfilePublicity{Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true},
		Workspace:        sdk.Workspace{PC: "desktop", Tool: "pen", WorkspaceImageURL: &workspaceImage},
	}
	client := &fakeSDKClient{userDetailResult: want}
	openCalls := 0
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		openCalls++
		return client, nil
	}}
	session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if result.IsError {
		t.Fatalf("user_detail returned error: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("user_detail text content = %+v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "用户 42") {
		t.Fatalf("user_detail text content = %+v", result.Content)
	}
	var out sdk.UserDetailResult
	decodeStructured(t, result, &out)
	if !reflect.DeepEqual(out, *want) || client.userDetailRequest != (sdk.UserDetailRequest{UserID: 42}) || openCalls != 1 {
		t.Fatalf("user_detail output=%+v request=%+v open calls=%d", out, client.userDetailRequest, openCalls)
	}
}

func TestSDKUserDetailRejectsInvalidInputAndReturnsSDKFailuresAsMCPError(t *testing.T) {
	for _, input := range []map[string]any{{"user_id": 0}, {"user_id": -1}} {
		openCalls := 0
		service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
			openCalls++
			return &fakeSDKClient{}, nil
		}}
		session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
		result := callTool(t, session, "user_detail", input)
		closeSession()
		if !result.IsError || openCalls != 0 {
			t.Fatalf("input=%v result=%+v open calls=%d", input, result, openCalls)
		}
	}
	for _, input := range []map[string]any{{}, {"user_id": "not-an-integer"}} {
		openCalls := 0
		service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
			openCalls++
			return &fakeSDKClient{}, nil
		}}
		session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "user_detail", Arguments: input})
		closeSession()
		if err == nil || openCalls != 0 {
			t.Fatalf("input=%v error=%v open calls=%d", input, err, openCalls)
		}
	}

	typed := &sdk.Error{Code: sdk.CodeMalformedUpstreamResponse, Operation: sdk.OperationUserDetail, Backend: sdk.BackendAppAPI, UserID: 42}
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userDetailErr: typed})
	defer closeSession()
	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("typed SDK failure result=%+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, typed.Error()) {
		t.Fatalf("typed SDK failure content=%+v", result.Content)
	}
	var typedOut sdk.UserDetailResult
	decodeStructured(t, result, &typedOut)
	if typedOut != (sdk.UserDetailResult{}) {
		t.Fatalf("typed SDK failure structured output=%+v", typedOut)
	}

	noSDKSession, closeNoSDKSession := newTestSession(t, &fakeDownloads{})
	defer closeNoSDKSession()
	result = callTool(t, noSDKSession, "user_detail", map[string]any{"user_id": 42})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("unconfigured SDK result=%+v", result)
	}
	text, ok = result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "pixiv sdk is not configured") {
		t.Fatalf("unconfigured SDK content=%+v", result.Content)
	}
	var unconfiguredOut sdk.UserDetailResult
	decodeStructured(t, result, &unconfiguredOut)
	if unconfiguredOut != (sdk.UserDetailResult{}) {
		t.Fatalf("unconfigured SDK structured output=%+v", unconfiguredOut)
	}
}

func TestSDKRecommendedAllReturnsEveryStreamAndPagination(t *testing.T) {
	client := &fakeSDKClient{}
	var order []string
	client.illustRecommended = func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
		order = append(order, "illust")
		return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "illust", 10)}, NextCursor: "illust-next"}, nil
	}
	client.mangaRecommended = func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
		order = append(order, "manga")
		return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "manga", 20)}, NextCursor: "manga-next"}, nil
	}
	client.novelRecommended = func(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
		order = append(order, "novel")
		return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 3, User: sdk.User{ID: 30}, Tags: []sdk.Tag{}}}, NextCursor: "novel-next"}, nil
	}
	client.userRecommended = func(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
		order = append(order, "user")
		return &sdk.UserRecommendedResult{UserPreviews: []sdk.RecommendedUserPreview{{User: sdk.User{ID: 4}, Illusts: []sdk.Illust{}, Novels: []sdk.Novel{{ID: 5}}}}, NextCursor: "user-next"}, nil
	}
	openCalls := 0
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		openCalls++
		return client, nil
	}}
	session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
	defer closeSession()

	result := callTool(t, session, "recommended", map[string]any{"kind": "all", "limit": 1})
	if result.IsError || openCalls != 1 || !slices.Equal(order, []string{"illust", "manga", "novel", "user"}) {
		t.Fatalf("recommended all result=%+v opens=%d order=%v", result, openCalls, order)
	}
	var structured map[string]any
	decodeStructured(t, result, &structured)
	pagination, ok := structured["pagination"].(map[string]any)
	if !ok || len(pagination) != 4 {
		t.Fatalf("missing independent pagination: %#v", structured)
	}
	for _, key := range []string{"illusts", "manga", "novels", "user_previews"} {
		items, ok := structured[key].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("%s = %#v", key, structured[key])
		}
	}
	novels := structured["novels"].([]any)
	if tags := novels[0].(map[string]any)["tags"]; tags == nil {
		t.Fatalf("top-level novel tags must be an array")
	}
	previews := structured["user_previews"].([]any)
	if tags := previews[0].(map[string]any)["novels"].([]any)[0].(map[string]any)["tags"]; tags == nil {
		t.Fatalf("preview novel tags must be an array")
	}
	raw, err := json.Marshal(structured)
	if err != nil || strings.Contains(string(raw), "cursor") || strings.Contains(string(raw), "next_url") {
		t.Fatalf("structured output leaks continuation: %s, err=%v", raw, err)
	}
}

func TestSDKRecommendedSingleKindsAndInputFailures(t *testing.T) {
	for _, test := range []struct {
		kind string
		want string
	}{
		{kind: "illust", want: "illust"},
		{kind: "manga", want: "manga"},
		{kind: "novel", want: "novel"},
		{kind: "user", want: "user"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			var calls []string
			client := &fakeSDKClient{
				illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
					calls = append(calls, "illust")
					return &sdk.IllustListResult{}, nil
				},
				mangaRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
					calls = append(calls, "manga")
					return &sdk.IllustListResult{}, nil
				},
				novelRecommended: func(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
					calls = append(calls, "novel")
					return &sdk.NovelListResult{}, nil
				},
				userRecommended: func(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
					calls = append(calls, "user")
					return &sdk.UserRecommendedResult{}, nil
				},
			}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()
			result := callTool(t, session, "recommended", map[string]any{"kind": test.kind})
			if result.IsError || !slices.Equal(calls, []string{test.want}) {
				t.Fatalf("kind=%s result=%+v calls=%v", test.kind, result, calls)
			}
		})
	}

	openCalls := 0
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		openCalls++
		return &fakeSDKClient{}, nil
	}}
	session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "unknown"})
	if !result.IsError || openCalls != 0 {
		t.Fatalf("invalid kind result=%+v open calls=%d", result, openCalls)
	}
	for _, input := range []map[string]any{{}, {"kind": 9}} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "recommended", Arguments: input})
		if err == nil || openCalls != 0 {
			t.Fatalf("input=%v error=%v open calls=%d", input, err, openCalls)
		}
	}

	noSDKSession, closeNoSDKSession := newTestSession(t, &fakeDownloads{})
	defer closeNoSDKSession()
	result = callTool(t, noSDKSession, "recommended", map[string]any{"kind": "illust"})
	if !result.IsError {
		t.Fatalf("unconfigured SDK result=%+v", result)
	}
}

func TestSDKRecommendedAllFailureDoesNotExposePartialStructuredOutput(t *testing.T) {
	client := &fakeSDKClient{
		illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "first", 1)}}, nil
		},
		mangaRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return nil, sdk.ErrMalformedUpstreamResponse
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "all"})
	if !result.IsError {
		t.Fatalf("all failure result=%+v", result)
	}
	var out recommendedOut
	decodeStructured(t, result, &out)
	if out.Kind != "" || len(out.Illusts) != 0 || len(out.Manga) != 0 || len(out.Novels) != 0 || len(out.UserPreviews) != 0 || out.Pagination != (recommendedPaginationOut{}) {
		t.Fatalf("partial structured output=%+v", out)
	}
}

func TestSDKRecommendedAllAppliesPageTwoIndependently(t *testing.T) {
	var illust, manga, novel, users []sdk.Cursor
	client := &fakeSDKClient{
		illustRecommended: func(_ context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			illust = append(illust, request.Cursor)
			if request.Cursor == "" {
				return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "first", 1)}, NextCursor: "i"}, nil
			}
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(11, "second", 1)}}, nil
		},
		mangaRecommended: func(_ context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			manga = append(manga, request.Cursor)
			if request.Cursor == "" {
				return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "first", 2)}, NextCursor: "m"}, nil
			}
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(12, "second", 2)}}, nil
		},
		novelRecommended: func(_ context.Context, request sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
			novel = append(novel, request.Cursor)
			if request.Cursor == "" {
				return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 3, User: sdk.User{ID: 3}}}, NextCursor: "n"}, nil
			}
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 13, User: sdk.User{ID: 3}}}}, nil
		},
		userRecommended: func(_ context.Context, request sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
			users = append(users, request.Cursor)
			if request.Cursor == "" {
				return &sdk.UserRecommendedResult{UserPreviews: []sdk.RecommendedUserPreview{{User: sdk.User{ID: 4}}}, NextCursor: "u"}, nil
			}
			return &sdk.UserRecommendedResult{UserPreviews: []sdk.RecommendedUserPreview{{User: sdk.User{ID: 14}}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "all", "page": 2, "limit": 1})
	if result.IsError || !slices.Equal(illust, []sdk.Cursor{"", "i"}) || !slices.Equal(manga, []sdk.Cursor{"", "m"}) || !slices.Equal(novel, []sdk.Cursor{"", "n"}) || !slices.Equal(users, []sdk.Cursor{"", "u"}) {
		t.Fatalf("result=%+v cursors=%v/%v/%v/%v", result, illust, manga, novel, users)
	}
	var structured map[string]any
	decodeStructured(t, result, &structured)
	previews, ok := structured["user_previews"].([]any)
	if !ok || len(previews) != 1 {
		t.Fatalf("user previews=%#v", structured["user_previews"])
	}
	preview, ok := previews[0].(map[string]any)
	if !ok {
		t.Fatalf("user preview=%#v", previews[0])
	}
	if illusts, ok := preview["illusts"].([]any); !ok || len(illusts) != 0 {
		t.Fatalf("nested illusts=%#v", preview["illusts"])
	}
	if novels, ok := preview["novels"].([]any); !ok || len(novels) != 0 {
		t.Fatalf("nested novels=%#v", preview["novels"])
	}
}

func TestIllustRecommendedUsesSDKAndPreservesLegacyOffset(t *testing.T) {
	var requests []sdk.IllustRecommendedRequest
	client := &fakeSDKClient{
		illustRecommended: func(_ context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			requests = append(requests, request)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				testSDKIllust(11, "first", 1),
				testSDKIllust(77, "after-offset", 1),
			}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "illust_recommended", map[string]any{"offset": 1})
	var out textOut
	decodeStructured(t, result, &out)
	if result.IsError || len(requests) != 1 || requests[0].Cursor != "" || !strings.Contains(out.Text, "77") || strings.Contains(out.Text, "11") {
		t.Fatalf("result=%+v requests=%+v", out, requests)
	}
}

func TestDownloadRandomFromRecommendationUsesSDKAndPreservesCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommended.jpg")
	if err := os.WriteFile(path, []byte("recommended"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []download.DownloadedArtwork{{IllustID: 77, Files: []download.DownloadedFile{{Path: path}}}}}
	var requests []sdk.IllustRecommendedRequest
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &fakeSDKClient{illustRecommended: func(_ context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			requests = append(requests, request)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(77, "recommended", 1)}}, nil
		}}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, downloads, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{})
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
	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 1})
	if result.IsError || len(requests) != 1 || requests[0] != (sdk.IllustRecommendedRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestSDKDownloadFactoryUsesSelectedAccountAfterTokenSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.jpg")
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	accountB := &fakeSDKClient{userID: 2, illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
		return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 77}}}, nil
	}}
	accountA := &fakeSDKClient{userID: 1, importAccount: func(context.Context, string) (*sdk.Account, error) {
		return &sdk.Account{UserID: 2, Username: "b"}, nil
	}}
	service := application.SDKService{NewClient: func(request application.SDKClientRequest) (application.SDKClient, error) {
		if request.UserID == 2 {
			return accountB, nil
		}
		return accountA, nil
	}}
	var managers []application.SDKClient
	server := NewWithSDKDownloadFactory(&fakeDownloads{}, func(client application.SDKClient) DownloadManager {
		managers = append(managers, client)
		return &fakeDownloads{artworks: []download.DownloadedArtwork{{IllustID: 77, Files: []download.DownloadedFile{{Path: path}}}}}
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{UserID: 1})
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
	_ = callTool(t, session, "set_refresh_token", map[string]any{"refresh_token": "switch"})
	_ = callTool(t, session, "download", map[string]any{"illust_id": 77})
	_ = callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 1})
	if len(managers) != 2 || managers[0] != accountB || managers[1] != accountB {
		t.Fatalf("download managers=%v want account B twice", managers)
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
	client := &fakeSDKClient{bookmarks: []sdk.Illust{testSDKIllust(15, "saved", 99)}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 99, "max_bookmark_id": 101})
	var out illustListOut
	decodeStructured(t, result, &out)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Cursor != "bookmark-101" || len(out.Items) != 1 || out.Items[0].ID != 15 {
		t.Fatalf("max_bookmark_id compatibility = request=%+v output=%+v", client.bookmarksRequest, out)
	}
}

func TestSDKUserListToolsPreserveLegacyParameters(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks: []sdk.Illust{testSDKIllust(15, "saved", 9)},
		following: []sdk.UserPreview{
			{User: sdk.User{ID: 30, Name: "before-offset"}},
			{User: sdk.User{ID: 31, Name: "after-offset"}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	bookmarkResult := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id_to_check": 9, "restrict": "private", "tag": "legacy-tag", "max_bookmark_id": 101,
	})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if client.bookmarksRequest.UserID != 9 || client.bookmarksRequest.Restrict != sdk.RestrictPrivate || client.bookmarksRequest.Tag != "legacy-tag" || client.bookmarksRequest.Cursor != "bookmark-101" || !strings.Contains(bookmarksOut.Text, "找到用户 9 的 1 个收藏") {
		t.Fatalf("bookmarks request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id_to_check": 8, "restrict": "private", "offset": 12,
	})
	var followingOut userListOut
	decodeStructured(t, followingResult, &followingOut)
	if client.followingRequest.UserID != 8 || client.followingRequest.Restrict != sdk.RestrictPrivate || len(followingOut.Items) != 0 || followingOut.Text != "用户 8 没有关注任何人。" {
		t.Fatalf("following offset request=%+v output=%+v", client.followingRequest, followingOut)
	}

	client.bookmarks = []sdk.Illust{}
	client.following = []sdk.UserPreview{}
	bookmarkResult = callTool(t, session, "user_bookmarks", map[string]any{"user_id_to_check": 9})
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	followingResult = callTool(t, session, "user_following", map[string]any{"user_id_to_check": 8})
	decodeStructured(t, followingResult, &followingOut)
	if bookmarksOut.Text != "找不到用户 9 的收藏。" || followingOut.Text != "用户 8 没有关注任何人。" {
		t.Fatalf("empty text bookmarks=%q following=%q", bookmarksOut.Text, followingOut.Text)
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
	// set_refresh_token 先由 public SDK 的 ImportAccount 消耗 r0；mock 按下标
	// 返回 r2。其后四个 SDK operation 必须被 gate 串行化，依次消费 r2..r5。
	expected := []string{"r0", "r2", "r3", "r4", "r5"}
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
		case "/v2/illust/bookmark/add":
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
	// 构造器保留的首参只是兼容占位；会话认证与所有 operation 都来自 public SDK。
	session, closeSession := newSDKTestSessionWithServiceRequest(t, &fakeAPI{}, service, application.SDKClientRequest{AuthFilePath: authPath, UserID: 1})
	defer closeSession()
	set := callTool(t, session, "set_refresh_token", map[string]any{"refresh_token": "r0"})
	var setOut textOut
	decodeStructured(t, set, &setOut)
	if !strings.Contains(setOut.Text, "完成认证") {
		t.Fatalf("set_refresh_token=%q", setOut.Text)
	}
	for _, result := range []*mcp.CallToolResult{
		callTool(t, session, "user_artworks", map[string]any{"user_id": 7}),
		callTool(t, session, "user_artworks", map[string]any{"user_id": 7}),
	} {
		if result.IsError {
			t.Fatalf("SDK artwork operation failed: %+v", result)
		}
	}

	errCh := make(chan error, 2)
	for _, call := range []*mcp.CallToolParams{
		{Name: "add_bookmark", Arguments: map[string]any{"illust_id": 9}},
		{Name: "download", Arguments: map[string]any{"illust_id": 9}},
	} {
		go func(call *mcp.CallToolParams) {
			result, err := session.CallTool(context.Background(), call)
			if err == nil && result.IsError {
				err = fmt.Errorf("SDK mutation failed: %+v", result)
			}
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
	if err != nil || !strings.Contains(string(stored), `"refresh_token": "r6"`) || strings.Contains(string(stored), `"refresh_token": "r2"`) {
		t.Fatalf("auth store did not retain latest rotation: %q err=%v", stored, err)
	}
}

// fakeAPI 只占据 New/NewWithSDK 保留的已废弃兼容参数；MCP 不得调用它。
type fakeAPI struct{}

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

func newSDKTestSessionWithAPI(t *testing.T, api any, sdkClient application.SDKClient) (*mcp.ClientSession, func()) {
	t.Helper()
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return sdkClient, nil
	}}
	return newSDKTestSessionWithService(t, api, service)
}

func newSDKTestSessionWithService(t *testing.T, api any, service application.SDKService) (*mcp.ClientSession, func()) {
	return newSDKTestSessionWithServiceRequest(t, api, service, application.SDKClientRequest{})
}

func newSDKTestSessionWithServiceRequest(t *testing.T, api any, service application.SDKService, request application.SDKClientRequest) (*mcp.ClientSession, func()) {
	t.Helper()
	server := NewWithSDKDownloadFactory(&fakeDownloads{}, func(application.SDKClient) DownloadManager { return &fakeDownloads{} }, slog.New(slog.NewTextHandler(io.Discard, nil)), service, request)
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
	illustRecommended     func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	mangaRecommended      func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	novelRecommended      func(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error)
	userRecommended       func(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error)
	userDetailResult      *sdk.UserDetailResult
	userDetailErr         error
	importAccountErr      error
	importAccount         func(context.Context, string) (*sdk.Account, error)
	userDetailRequest     sdk.UserDetailRequest
	artworksRequest       sdk.UserArtworksRequest
	artworksRequests      []sdk.UserArtworksRequest
	artworkResults        map[sdk.Cursor]sdk.IllustListResult
	bookmarksRequest      sdk.UserBookmarksRequest
	userBookmarksErr      error
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

type failingRefreshSDKClient struct {
	fakeSDKClient
	err error
}

func (f *failingRefreshSDKClient) Refresh(context.Context) (*sdk.Account, error) {
	return nil, f.err
}

func (f *failingMutationSDKClient) AddBookmark(context.Context, sdk.AddBookmarkRequest) error {
	return f.err
}

func (f *fakeSDKClient) CurrentUserID(context.Context) (int64, error) { return f.userID, nil }
func (f *fakeSDKClient) ImportAccount(ctx context.Context, token string) (*sdk.Account, error) {
	if f.importAccount != nil {
		return f.importAccount(ctx, token)
	}
	if f.importAccountErr != nil {
		return nil, f.importAccountErr
	}
	return &sdk.Account{UserID: f.userID, Username: "alice"}, nil
}
func (*fakeSDKClient) ListAccounts() (*sdk.AccountsResult, error) {
	return &sdk.AccountsResult{Accounts: []sdk.Account{}}, nil
}
func (*fakeSDKClient) SelectAccount(int64) error { return nil }
func (*fakeSDKClient) RemoveAccount(int64) error { return nil }
func (f *fakeSDKClient) CheckAccount(context.Context, int64) (*sdk.Account, error) {
	return &sdk.Account{UserID: f.userID, Username: "alice"}, nil
}
func (f *fakeSDKClient) CheckRefreshToken(context.Context, string) (*sdk.Account, error) {
	return nil, errors.New("unexpected refresh token check")
}
func (f *fakeSDKClient) Refresh(context.Context) (*sdk.Account, error) {
	return &sdk.Account{UserID: f.userID, Username: "alice"}, nil
}
func (*fakeSDKClient) StartLogin() (*sdk.LoginSession, error) {
	return nil, errors.New("login is not configured")
}
func (*fakeSDKClient) CompleteLogin(context.Context, *sdk.LoginSession, string, sdk.LoginOptions) (*sdk.Account, error) {
	return nil, errors.New("login is not configured")
}
func (*fakeSDKClient) SearchIllust(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) IllustDetail(context.Context, int64) (*sdk.IllustDetail, error) {
	return &sdk.IllustDetail{}, nil
}
func (*fakeSDKClient) IllustRelated(context.Context, sdk.IllustRelatedRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) IllustRanking(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) FollowingIllusts(context.Context, sdk.FollowingIllustsRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}
func (*fakeSDKClient) SearchUser(context.Context, sdk.SearchUserRequest) (*sdk.UserListResult, error) {
	return &sdk.UserListResult{}, nil
}
func (*fakeSDKClient) TrendingTagsIllust(context.Context) (*sdk.TrendingTagsIllustResult, error) {
	return &sdk.TrendingTagsIllustResult{}, nil
}
func (*fakeSDKClient) UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error) {
	return &sdk.UgoiraMetadataResult{}, nil
}
func (*fakeSDKClient) ParseResourceRef(rawURL string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{URL: rawURL}, nil
}
func (*fakeSDKClient) OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	return nil, errors.New("resource is not configured")
}
func (*fakeSDKClient) Download(context.Context, sdk.ResourceRef, string) error {
	return errors.New("resource is not configured")
}

func (f *fakeSDKClient) IllustRecommended(ctx context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
	if f.illustRecommended != nil {
		return f.illustRecommended(ctx, request)
	}
	return &sdk.IllustListResult{}, nil
}
func (f *fakeSDKClient) MangaRecommended(ctx context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
	if f.mangaRecommended != nil {
		return f.mangaRecommended(ctx, request)
	}
	return &sdk.IllustListResult{}, nil
}
func (f *fakeSDKClient) NovelRecommended(ctx context.Context, request sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
	if f.novelRecommended != nil {
		return f.novelRecommended(ctx, request)
	}
	return &sdk.NovelListResult{}, nil
}
func (f *fakeSDKClient) UserRecommended(ctx context.Context, request sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
	if f.userRecommended != nil {
		return f.userRecommended(ctx, request)
	}
	return &sdk.UserRecommendedResult{}, nil
}

// UserDetail 记录 MCP 到公开 SDK 的完整请求，供结构化输出与错误路径断言。
func (f *fakeSDKClient) UserDetail(_ context.Context, request sdk.UserDetailRequest) (*sdk.UserDetailResult, error) {
	f.userDetailRequest = request
	if f.userDetailErr != nil {
		return nil, f.userDetailErr
	}
	if f.userDetailResult != nil {
		return f.userDetailResult, nil
	}
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
	if f.userBookmarksErr != nil {
		return nil, f.userBookmarksErr
	}
	return &sdk.IllustListResult{Illusts: f.bookmarks}, nil
}
func (*fakeSDKClient) UserBookmarksCursor(_ context.Context, _ sdk.UserBookmarksRequest, maxBookmarkID int64) (sdk.Cursor, error) {
	return sdk.Cursor(fmt.Sprintf("bookmark-%d", maxBookmarkID)), nil
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
