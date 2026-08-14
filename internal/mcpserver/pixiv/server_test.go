package pixiv_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// server_test.go 只保留产品级聚合契约：registry 完整性、stdio JSON-RPC
// stdout 纯净性、SDK 操作门控与 tool error 结果形状。单 owner 的契约测试位于
// 同包的 <owner>_tools_test.go。

func TestServerListsExpectedTools(t *testing.T) {
	server := pixivmcpserver.New(&fakeAPI{}, &fakeDownloads{})
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
	defer func() { _ = session.Close() }()

	var names []string
	var searchIllustTool, searchNovelTool *mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		names = append(names, tool.Name)
		if tool.Name == "search_illust" {
			searchIllustTool = tool
		}
		if tool.Name == "search_novel" {
			searchNovelTool = tool
		}
	}
	want := []string{
		"download", "download_random_from_recommendation", "search_illust", "search_novel", "illust_detail",
		"illust_related", "illust_ranking", "search_user", "illust_recommended", "novel_detail", "novel_content",
		"illust_series", "novel_series", "illust_comments", "novel_comments",
		"recommended", "trending_tags_illust", "timeline_illust_following", "timeline_novel_following",
		"timeline_illust_latest", "timeline_novel_latest", "mypixiv_users", "mypixiv_illusts", "mypixiv_novels",
		"user_detail", "user_artworks", "user_novels", "user_bookmarks", "user_novel_bookmarks", "user_following", "user_followers", "related_users", "blocked_users", "bookmark_tags", "bookmark_detail", "add_bookmark",
		"remove_bookmark", "follow_user", "unfollow_user",
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("tools=%v, want exact registration set %v", names, want)
	}
	if searchIllustTool == nil {
		t.Fatal("search_illust tool is not registered")
	}
	if searchNovelTool == nil {
		t.Fatal("search_novel tool is not registered")
	}
	schema, err := json.Marshal(searchIllustTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal search_illust input schema: %v", err)
	}
	for _, field := range []string{"search_target", "duration", "start_date", "end_date", "content_type", "ai_mode", "aspect_ratio", "resolution", "tool", "bookmark_min", "bookmark_max", "bookmark_strategy", "illust_filter"} {
		if !strings.Contains(string(schema), `"`+field+`"`) {
			t.Fatalf("search_illust input schema missing %q: %s", field, schema)
		}
	}
	var schemaObject struct {
		Properties map[string]struct {
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &schemaObject); err != nil {
		t.Fatalf("decode search_illust input schema: %v", err)
	}
	wantEnums := map[string][]string{
		"search_target": {"partial_match_for_tags", "exact_match_for_tags", "title_and_caption", "keyword"},
		"duration":      {"within_last_day", "within_last_week", "within_last_month", "within_half_year", "within_year"},
		"content_type":  {"all", "illust-and-ugoira", "illust", "manga", "ugoira"},
		"ai_mode":       {"all", "exclude", "only"},
		"aspect_ratio":  {"all", "landscape", "portrait", "square"},
		"resolution":    {"all", "high", "medium", "low"},
	}
	for field, enum := range wantEnums {
		property := schemaObject.Properties[field]
		if property.Description == "" || !slices.Equal(property.Enum, enum) {
			t.Fatalf("search_illust schema %s = %+v, want enum %v with description", field, property, enum)
		}
	}
	for _, field := range []string{"start_date", "end_date", "tool", "bookmark_min", "bookmark_max"} {
		if schemaObject.Properties[field].Description == "" {
			t.Fatalf("search_illust schema %s is missing description", field)
		}
	}
	novelSchema, err := json.Marshal(searchNovelTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal search_novel input schema: %v", err)
	}
	for _, field := range []string{"page", "limit", "novel_filter"} {
		if !strings.Contains(string(novelSchema), `"`+field+`"`) {
			t.Fatalf("search_novel input schema missing %q: %s", field, novelSchema)
		}
	}
	for _, field := range []string{"rating", "min_text_length", "max_text_length", "original_only"} {
		if strings.Contains(string(novelSchema), `"`+field+`"`) {
			t.Fatalf("search_novel input schema publishes unsupported field %q: %s", field, novelSchema)
		}
	}
}

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
	if !strings.Contains(protocol, `"jsonrpc":"2.0"`) || !strings.Contains(protocol, `"isError":true`) {
		t.Fatalf("stdout is not protocol-only: %s; stderr=%s", protocol, stderr.String())
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
	server := pixivmcpserver.NewWithSDK(&fakeAPI{}, &fakeDownloads{}, testSDKPorts(t, client), pixivmcpserver.Account{})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestToolErrorResultPreservesStructuredContent(t *testing.T) {
	app := runtime.NewApp(nil, nil, pixivmcpserver.SDKPorts{}, pixivmcpserver.Account{})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	runtime.AddTool(app, server, &mcp.Tool{Name: "structured_error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]string, error) {
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

func TestOpenSDKOperationReturnsConfigurationError(t *testing.T) {
	app := runtime.NewApp(nil, nil, pixivmcpserver.SDKPorts{}, pixivmcpserver.Account{})
	if _, _, err := app.OpenSDKOperation(context.Background()); err == nil || err.Error() != "pixiv sdk is not configured" {
		t.Fatalf("open SDK error = %v", err)
	}
}

func TestSDKOperationGateRespectsCanceledContext(t *testing.T) {
	var calls atomic.Int32
	client := openWireClient(t, &fakeSDKClient{userID: 42})
	app := runtime.NewApp(nil, nil, pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			calls.Add(1)
			return client, nil
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return nil
		},
	}, pixivmcpserver.Account{})
	_, release, err := app.OpenSDKOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := app.OpenSDKOperation(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("second open error=%v, want context canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("SDK factory calls=%d, want 1", calls.Load())
	}
}

func TestSDKToolsWithoutSDKReturnStructuredConfigurationError(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()
	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 1})
	if !result.IsError {
		t.Fatalf("SDK configuration failure must be an MCP error result: %+v", result)
	}
	var out outputs.Mutation
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "sdk pooled operation is not configured") {
		t.Fatalf("unexpected output: %+v", out)
	}
}
