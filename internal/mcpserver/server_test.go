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
	var searchIllustTool *mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		names = append(names, tool.Name)
		if tool.Name == "search_illust" {
			searchIllustTool = tool
		}
	}
	want := []string{
		"set_download_path", "download", "refresh_token", "set_refresh_token",
		"download_random_from_recommendation", "search_illust", "search_illust_options", "illust_detail",
		"illust_related", "illust_ranking", "search_user", "illust_recommended",
		"recommended", "trending_tags_illust", "illust_follow", "user_detail",
		"user_artworks", "user_bookmarks", "user_following", "add_bookmark",
		"remove_bookmark", "follow_user", "unfollow_user", "get_thumbnail_base64",
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("tools=%v, want exact registration set %v", names, want)
	}
	if searchIllustTool == nil {
		t.Fatal("search_illust tool is not registered")
	}
	schema, err := json.Marshal(searchIllustTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal search_illust input schema: %v", err)
	}
	for _, field := range []string{"rating", "content_type", "ai_mode", "aspect_ratio", "resolution", "tool"} {
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
		"rating":       {"all", "sfw", "r18", "r18g", "mature"},
		"content_type": {"all", "illust-and-ugoira", "illust", "manga", "ugoira"},
		"ai_mode":      {"all", "exclude", "only"},
		"aspect_ratio": {"all", "landscape", "portrait", "square"},
		"resolution":   {"all", "high", "medium", "low"},
	}
	for field, enum := range wantEnums {
		property := schemaObject.Properties[field]
		if property.Description == "" || !slices.Equal(property.Enum, enum) {
			t.Fatalf("search_illust schema %s = %+v, want enum %v with description", field, property, enum)
		}
	}
	if schemaObject.Properties["tool"].Description == "" {
		t.Fatal("search_illust schema tool is missing description")
	}
}

func TestSearchIllustMapsStableFiltersToPublicSDK(t *testing.T) {
	var got sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		got = request
		return &sdk.IllustListResult{}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{
		"word": "cat", "rating": "r18g", "content_type": "manga", "ai_mode": "only",
		"aspect_ratio": "landscape", "resolution": "high", "tool": "CLIP STUDIO PAINT",
	})
	if result.IsError {
		t.Fatalf("search_illust returned MCP error: %+v", result)
	}
	want := sdk.SearchIllustFilters{
		Rating: sdk.SearchRatingR18G, ContentType: sdk.SearchContentTypeManga,
		AIMode: sdk.SearchAIModeOnly, AspectRatio: sdk.SearchAspectRatioLandscape,
		Resolution: sdk.SearchResolutionHigh, Tool: "CLIP STUDIO PAINT",
	}
	if got.Word != "cat" || got.Filters != want {
		t.Fatalf("SearchIllust request = %+v, want word=cat filters=%+v", got, want)
	}
}

func TestSearchIllustSchemaRejectsRemovedLegacyWireFields(t *testing.T) {
	calls := 0
	client := &fakeSDKClient{searchIllust: func(_ context.Context, _ sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		calls++
		return &sdk.IllustListResult{}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	for _, args := range []map[string]any{
		{"word": "cat", "search_r18": true},
		{"word": "cat", "offset": 1},
		{"word": "cat", "include_thumbnail": true},
	} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_illust", Arguments: args})
		if err == nil || !strings.Contains(err.Error(), "additional properties") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
	if calls != 0 {
		t.Fatalf("legacy wire fields opened SDK %d times", calls)
	}
}

func TestSearchIllustMapsRatingR18WithoutChangingWord(t *testing.T) {
	var got sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		got = request
		return &sdk.IllustListResult{}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	callTool(t, session, "search_illust", map[string]any{"word": "cat", "rating": "r18"})
	if got.Word != "cat" || got.Filters.Rating != sdk.SearchRatingR18 {
		t.Fatalf("rating r18 request = %+v", got)
	}
}

func TestSearchIllustKeepsFiltersAcrossLogicalPagePagination(t *testing.T) {
	var requests []sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "first", 7)}, NextCursor: "next"}, nil
		case "next":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "second", 7)}}, nil
		default:
			return &sdk.IllustListResult{}, nil
		}
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 2, "limit": 1, "ai_mode": "exclude"})
	var out textOut
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor != "next" || requests[0].Filters.AIMode != sdk.SearchAIModeExclude || requests[1].Filters.AIMode != sdk.SearchAIModeExclude || !strings.Contains(out.Text, `"second"`) {
		t.Fatalf("requests=%+v output=%+v", requests, out)
	}
}

func TestSearchIllustPageLimitFillsLogicalResultsAcrossEmptyBatches(t *testing.T) {
	var requests []sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "a", 7)}, NextCursor: "empty"}, nil
		case "empty":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{}, NextCursor: "more"}, nil
		case "more":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "b", 7), testSDKIllust(3, "c", 7)}, NextCursor: "tail"}, nil
		default:
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(4, "d", 7)}}, nil
		}
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "limit": 3})
	var out textOut
	decodeStructured(t, result, &out)
	if len(requests) != 3 || requests[1].Cursor != "empty" || requests[2].Cursor != "more" {
		t.Fatalf("requests=%+v", requests)
	}
	if !strings.Contains(out.Text, `"a"`) || !strings.Contains(out.Text, `"b"`) || !strings.Contains(out.Text, `"c"`) || strings.Contains(out.Text, `"d"`) {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustPageTwoUsesLogicalLimit(t *testing.T) {
	var requests []sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "a", 7), testSDKIllust(2, "b", 7)}, NextCursor: "next"}, nil
		case "next":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(3, "c", 7), testSDKIllust(4, "d", 7)}}, nil
		default:
			return &sdk.IllustListResult{}, nil
		}
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 2, "limit": 2})
	var out textOut
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor != "next" {
		t.Fatalf("requests=%+v", requests)
	}
	if !strings.Contains(out.Text, `"c"`) || !strings.Contains(out.Text, `"d"`) || strings.Contains(out.Text, `"a"`) {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustRejectsPageWithoutPositiveLimit(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		t.Fatal("SDK should not open for invalid page plan")
		return nil, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 1})
	var out textOut
	decodeStructured(t, result, &out)
	if !strings.Contains(out.Text, "page") || !strings.Contains(out.Text, "limit") {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustContinuesAfterFilteredEmptyBatch(t *testing.T) {
	var requests []sdk.SearchIllustRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		if request.Cursor == "" {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{}, NextCursor: "filtered-next"}, nil
		}
		return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "visible", 7)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "rating": "r18"})
	var out textOut
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor != "filtered-next" || !strings.Contains(out.Text, `"visible"`) {
		t.Fatalf("requests=%+v output=%+v", requests, out)
	}
}

func TestSearchIllustSchemaRejectsInvalidEnumsBeforeOpeningSDK(t *testing.T) {
	for _, test := range []struct {
		field string
		value string
	}{
		{"rating", "adult"},
		{"content_type", "novel"},
		{"ai_mode", "maybe"},
		{"aspect_ratio", "wide"},
		{"resolution", "ultra"},
	} {
		t.Run(test.field, func(t *testing.T) {
			factoryCalls := 0
			service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
				factoryCalls++
				return &fakeSDKClient{}, nil
			}}
			session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
			defer closeSession()
			_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_illust", Arguments: map[string]any{"word": "cat", test.field: test.value}})
			if err == nil || factoryCalls != 0 {
				t.Fatalf("invalid %s=%q error=%v factoryCalls=%d", test.field, test.value, err, factoryCalls)
			}
		})
	}
}

func TestSearchIllustOptionsReturnsStructuredToolsFromPublicSDK(t *testing.T) {
	var got sdk.SearchIllustOptionsRequest
	client := &fakeSDKClient{searchIllustOptions: func(_ context.Context, request sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		got = request
		return &sdk.SearchIllustOptionsResult{Tools: []string{"CLIP STUDIO PAINT", "Photoshop"}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust_options", map[string]any{"word": "cat"})
	if result.IsError {
		t.Fatalf("search_illust_options returned MCP error: %+v", result)
	}
	var out searchIllustOptionsOut
	decodeStructured(t, result, &out)
	first := strings.Index(out.Text, "CLIP STUDIO PAINT")
	second := strings.Index(out.Text, "Photoshop")
	if got.Word != "cat" || !slices.Equal(out.Tools, []string{"CLIP STUDIO PAINT", "Photoshop"}) || first < 0 || second <= first {
		t.Fatalf("request=%+v output=%+v", got, out)
	}
}

func TestSearchIllustOptionsExplainsEmptyToolList(t *testing.T) {
	client := &fakeSDKClient{searchIllustOptions: func(_ context.Context, _ sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		return &sdk.SearchIllustOptionsResult{Tools: nil}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust_options", map[string]any{"word": "cat"})
	var out searchIllustOptionsOut
	decodeStructured(t, result, &out)
	if result.IsError || out.Tools == nil || len(out.Tools) != 0 || out.Text != "当前没有可用的绘图工具。" {
		t.Fatalf("empty options output=%+v result=%+v", out, result)
	}
}

func TestSearchIllustOptionsEscapesControlCharactersOnlyInCompatibilityText(t *testing.T) {
	rawTools := []string{"safe\nforged\rline\x1b[31m\x01", "second"}
	client := &fakeSDKClient{searchIllustOptions: func(_ context.Context, _ sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		return &sdk.SearchIllustOptionsResult{Tools: rawTools}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust_options", map[string]any{"word": "cat"})
	var out searchIllustOptionsOut
	decodeStructured(t, result, &out)
	if !slices.Equal(out.Tools, rawTools) {
		t.Fatalf("structured tools changed: got %q want %q", out.Tools, rawTools)
	}
	for _, control := range []string{"safe\nforged", "\r", "\x1b", "\x01"} {
		if strings.Contains(out.Text, control) {
			t.Fatalf("compatibility text contains raw control %q: %q", control, out.Text)
		}
	}
	for _, escaped := range []string{`\n`, `\r`, `\x1b`, `\x01`} {
		if !strings.Contains(out.Text, escaped) {
			t.Fatalf("compatibility text missing escape %q: %q", escaped, out.Text)
		}
	}
}

func TestSearchIllustOptionsSchemaRequiresWord(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{})
	defer closeSession()

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_illust_options", Arguments: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "word") {
		t.Fatalf("missing word error = %v", err)
	}
}

func TestSearchIllustOptionsExposesSDKErrorAndKeepsArgumentsOutOfLogs(t *testing.T) {
	const wordCanary = "query-secret-canary"
	typed := &sdk.Error{Code: sdk.CodeUnsupported, Operation: sdk.OperationSearchIllustOptions, Backend: sdk.BackendAppAPI}
	client := &fakeSDKClient{searchIllustOptions: func(_ context.Context, _ sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		return nil, typed
	}}
	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) { return client, nil }}
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

	result := callTool(t, session, "search_illust_options", map[string]any{"word": wordCanary})
	var out searchIllustOptionsOut
	decodeStructured(t, result, &out)
	if !result.IsError || !strings.Contains(out.Text, string(sdk.CodeUnsupported)) || out.Tools == nil {
		t.Fatalf("error output=%+v result=%+v", out, result)
	}
	logText := logs.String()
	for _, secret := range []string{wordCanary, typed.Error()} {
		if strings.Contains(logText, secret) {
			t.Fatalf("MCP log leaked %q: %s", secret, logText)
		}
	}
	for _, want := range []string{`"operation":"search_illust_options"`, `"result":"error"`, `"error_code":"unsupported"`} {
		if !strings.Contains(logText, want) {
			t.Fatalf("MCP log missing %q: %s", want, logText)
		}
	}
}

func TestLegacyToolLogsSafelyOutsideMCPProtocol(t *testing.T) {
	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &fakeSDKClient{}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
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
	result := callTool(t, session, "search_illust", map[string]any{"word": "query-secret-canary", "tool": "tool-secret-canary"})
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
	for _, secret := range []string{"query-secret-canary", "tool-secret-canary"} {
		if strings.Contains(got, secret) {
			t.Fatalf("MCP log leaked tool argument %q: %s", secret, got)
		}
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
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_illust_options","arguments":{"word":"options-stdio-secret-canary"}}}`,
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
	if !strings.Contains(protocol, `"jsonrpc":"2.0"`) || !strings.Contains(protocol, `"isError":true`) || strings.Contains(protocol, `"component":"mcp"`) {
		t.Fatalf("stdout is not protocol-only: %s; stderr=%s", protocol, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"component":"mcp"`) || !strings.Contains(stderr.String(), `"operation":"search_illust"`) || !strings.Contains(stderr.String(), `"operation":"search_illust_options"`) || !strings.Contains(stderr.String(), `"operation":"add_bookmark"`) || !strings.Contains(stderr.String(), `"result":"error"`) {
		t.Fatalf("stderr lacks MCP event: %s", stderr.String())
	}
	for _, secret := range []string{"stdio-secret-canary", "options-stdio-secret-canary"} {
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("stderr leaked tool input %q: %s", secret, stderr.String())
		}
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

	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 9})
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
	const wantText = "错误：必须提供 illust_id (单个ID) 或 illust_ids (ID列表) 参数之一。"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
	if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadInvalidDeliveryPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"illust_id": 42,
			"delivery":  "invalid-delivery",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = `错误：delivery 仅支持 "local_path" 或 "image_content"。`
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
	if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadManagerErrorPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{err: errors.New("download sentinel")}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"illust_id": 42,
			"delivery":  deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "下载失败: download sentinel"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadBuildErrorPreservesBusinessErrorShape(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	_, statErr := os.Stat(missing)
	if statErr == nil {
		t.Fatal("missing test file unexpectedly exists")
	}
	downloads := &fakeDownloads{artworks: []download.DownloadedArtwork{{
		IllustID: 42,
		Files:    []download.DownloadedFile{{Path: missing}},
	}}}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"illust_id": 42,
			"delivery":  deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "整理下载结果失败: " + statErr.Error()
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadImageReadErrorPreservesBusinessErrorShape(t *testing.T) {
	directory := t.TempDir()
	_, readErr := os.ReadFile(directory)
	if readErr == nil {
		t.Fatal("reading test directory unexpectedly succeeded")
	}
	downloads := &fakeDownloads{artworks: []download.DownloadedArtwork{{
		IllustID: 42,
		Files:    []download.DownloadedFile{{Path: directory}},
	}}}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"illust_id": 42,
			"delivery":  deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "读取下载文件失败: " + readErr.Error()
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
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

func TestRefreshTokenUnknownFailureIsRedacted(t *testing.T) {
	const rawFailure = "refresh failed: proxy=http://proxy-user:proxy-password@127.0.0.1:7890/oauth?query_token=query-secret Cookie=PHPSESSID=cookie-secret refresh_token=refresh-secret config=/Users/private/.config/pixiv/config.yaml"
	const want = "Token刷新失败。请检查 refresh token 是否有效，以及网络连接或代理设置。"

	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &failingRefreshSDKClient{err: errors.New(rawFailure)}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
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

	result := callTool(t, session, "refresh_token", map[string]any{})
	if result.IsError {
		t.Fatalf("legacy refresh_token error shape changed: %+v", result)
	}
	var out textOut
	decodeStructured(t, result, &out)
	if out.Text != want {
		t.Fatalf("structured output=%q, want %q", out.Text, want)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len=%d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]=%T, want *mcp.TextContent", result.Content[0])
	}
	var contentOut textOut
	if err := json.Unmarshal([]byte(text.Text), &contentOut); err != nil {
		t.Fatalf("decode text content %q: %v", text.Text, err)
	}
	if contentOut != out {
		t.Fatalf("text content=%+v, structured output=%+v", contentOut, out)
	}
	if logs.Len() == 0 {
		t.Fatal("injected MCP logger produced no refresh_token event")
	}
	for _, secret := range []string{
		"proxy-user", "proxy-password", "127.0.0.1:7890", "query-secret",
		"PHPSESSID", "cookie-secret", "refresh-secret", "/Users/private/.config/pixiv/config.yaml",
	} {
		if strings.Contains(text.Text, secret) || strings.Contains(out.Text, secret) {
			t.Fatalf("MCP output leaked %q: content=%q structured=%q", secret, text.Text, out.Text)
		}
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("MCP log leaked %q: %s", secret, logs.String())
		}
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

func TestSDKUserToolsResolveIdentityAndReturnStructuredOutput(t *testing.T) {
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

	bookmarks := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 99, "tag": "tag", "limit": 0})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarks, &bookmarksOut)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Tag != "tag" || bookmarksOut.UserID != 99 || len(bookmarksOut.Items) != 1 || bookmarksOut.Pagination.HasMore {
		t.Fatalf("bookmarks = request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}
	if !strings.Contains(bookmarksOut.Text, "找到用户 99 的 1 个收藏") {
		t.Fatalf("bookmark text missing: %q", bookmarksOut.Text)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id": 99, "limit": 1})
	var followingOut userListOut
	decodeStructured(t, following, &followingOut)
	if client.followingRequest.UserID != 99 || followingOut.UserID != 99 || len(followingOut.Items) != 1 {
		t.Fatalf("following = request=%+v output=%+v", client.followingRequest, followingOut)
	}
	if !strings.Contains(followingOut.Text, "用户 99 关注了 1 位用户") {
		t.Fatalf("following text missing: %q", followingOut.Text)
	}
}

func TestUserArtworksNormalizesNilToolsAndPreservesNonNilToolsAtMCPBoundary(t *testing.T) {
	withNilTools := testSDKIllust(11, "without-tools", 71)
	withTools := testSDKIllust(12, "with-tools", 71)
	withTools.Tools = []string{"CLIP STUDIO PAINT", "Photoshop"}
	client := &fakeSDKClient{userID: 71, artworks: []sdk.Illust{withNilTools, withTools}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 71})
	if result.IsError {
		t.Fatalf("user_artworks result=%+v", result)
	}
	var out illustListOut
	decodeStructured(t, result, &out)
	if out.Items[0].Tools == nil || len(out.Items[0].Tools) != 0 {
		t.Fatalf("nil tools were not normalized to an empty array: %#v", out.Items[0].Tools)
	}
	if !slices.Equal(out.Items[1].Tools, withTools.Tools) {
		t.Fatalf("non-nil tools changed: got %q want %q", out.Items[1].Tools, withTools.Tools)
	}
	if withNilTools.Tools != nil {
		t.Fatalf("MCP normalization mutated the SDK value: %#v", withNilTools.Tools)
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

func TestRecommendedNormalizesToolsAcrossTopLevelAndNestedIllusts(t *testing.T) {
	withoutTools := testSDKIllust(1, "without-tools", 10)
	withTools := testSDKIllust(2, "with-tools", 10)
	withTools.Tools = []string{"SAI", "Photoshop"}
	client := &fakeSDKClient{
		illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{withoutTools, withTools}}, nil
		},
		mangaRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{withoutTools}}, nil
		},
		novelRecommended: func(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
			return &sdk.NovelListResult{Novels: []sdk.Novel{}}, nil
		},
		userRecommended: func(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
			return &sdk.UserRecommendedResult{UserPreviews: []sdk.RecommendedUserPreview{{
				User:    sdk.User{ID: 10},
				Illusts: []sdk.Illust{withoutTools, withTools},
				Novels:  []sdk.Novel{},
			}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "recommended", map[string]any{"kind": "all"})
	if result.IsError {
		t.Fatalf("recommended result=%+v", result)
	}
	var out recommendedOut
	decodeStructured(t, result, &out)
	for label, items := range map[string][]sdk.Illust{
		"illusts": out.Illusts,
		"manga":   out.Manga,
		"nested":  out.UserPreviews[0].Illusts,
	} {
		if items[0].Tools == nil || len(items[0].Tools) != 0 {
			t.Fatalf("%s nil tools were not normalized: %#v", label, items[0].Tools)
		}
	}
	if !slices.Equal(out.Illusts[1].Tools, withTools.Tools) || !slices.Equal(out.UserPreviews[0].Illusts[1].Tools, withTools.Tools) {
		t.Fatalf("non-nil tools changed: top=%q nested=%q want=%q", out.Illusts[1].Tools, out.UserPreviews[0].Illusts[1].Tools, withTools.Tools)
	}
	if withoutTools.Tools != nil {
		t.Fatalf("MCP normalization mutated the SDK value: %#v", withoutTools.Tools)
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

func TestIllustRecommendedUsesSDKAndLogicalPageSkip(t *testing.T) {
	var requests []sdk.IllustRecommendedRequest
	client := &fakeSDKClient{
		illustRecommended: func(_ context.Context, request sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			requests = append(requests, request)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				testSDKIllust(11, "first", 1),
				testSDKIllust(77, "after-skip", 1),
			}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "illust_recommended", map[string]any{"page": 2, "limit": 1})
	var out textOut
	decodeStructured(t, result, &out)
	if result.IsError || len(requests) != 1 || requests[0].Cursor != "" || !strings.Contains(out.Text, "77") || strings.Contains(out.Text, "11") {
		t.Fatalf("result=%+v requests=%+v", out, requests)
	}
}

func TestIllustRecommendedTextIncludesAllTagsInUpstreamOrder(t *testing.T) {
	illust := testSDKIllust(77, "tagged", 9)
	illust.Tags = []sdk.Tag{
		{Name: "tag-1"}, {Name: "tag-2"}, {Name: "tag-3"}, {Name: "tag-4"},
		{Name: "tag-5"}, {Name: "tag-6"}, {Name: "tag-7"},
	}
	client := &fakeSDKClient{illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
		return &sdk.IllustListResult{Illusts: []sdk.Illust{illust}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_recommended", map[string]any{})
	var out textOut
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("illust_recommended returned MCP error: %+v", result)
	}
	want := "标签: tag-1, tag-2, tag-3, tag-4, tag-5, tag-6, tag-7"
	if !strings.Contains(out.Text, want) {
		t.Fatalf("illust_recommended text = %q, want substring %q", out.Text, want)
	}
}

func TestIllustRankingUsesStableLabelAndPreservesRequestAndRank(t *testing.T) {
	var requests []sdk.IllustRankingRequest
	client := &fakeSDKClient{illustRanking: func(_ context.Context, request sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		return &sdk.IllustListResult{Illusts: []sdk.Illust{
			testSDKIllust(11, "first", 1),
			testSDKIllust(12, "second", 1),
			testSDKIllust(13, "third", 1),
		}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_ranking", map[string]any{
		"mode": "day_male", "date": "2025-02-03", "page": 3, "limit": 1,
	})
	var out textOut
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("illust_ranking returned MCP error: %+v", result)
	}
	if len(requests) != 1 || requests[0].Mode != sdk.RankingModeDayMale || requests[0].Date != "2025-02-03" || requests[0].Cursor != "" {
		t.Fatalf("ranking requests = %+v", requests)
	}
	if !strings.HasPrefix(out.Text, "男性向每日排行榜:\n\n") || !strings.Contains(out.Text, "第 3 名: ID: 13") {
		t.Fatalf("illust_ranking text = %q", out.Text)
	}
}

func TestIllustRankingUsesStableLabelsForAllModesAndFutureFallback(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: "day", want: "每日排行榜"},
		{mode: "day_male", want: "男性向每日排行榜"},
		{mode: "day_female", want: "女性向每日排行榜"},
		{mode: "week", want: "每周排行榜"},
		{mode: "week_original", want: "原创作品排行榜"},
		{mode: "week_rookie", want: "新人排行榜"},
		{mode: "month", want: "每月排行榜"},
		{mode: "future_mode", want: "future_mode 排行榜"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			client := &fakeSDKClient{illustRanking: func(_ context.Context, request sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
				if string(request.Mode) != test.mode {
					t.Fatalf("ranking mode = %q, want %q", request.Mode, test.mode)
				}
				return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(13, "ranked", 1)}}, nil
			}}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			result := callTool(t, session, "illust_ranking", map[string]any{"mode": test.mode})
			var out textOut
			decodeStructured(t, result, &out)
			if result.IsError || !strings.HasPrefix(out.Text, test.want+":\n\n") {
				t.Fatalf("illust_ranking(%q) text = %q", test.mode, out.Text)
			}
		})
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
	if result.IsError || len(result.Content) != 1 || len(requests) != 1 || requests[0] != (sdk.IllustRecommendedRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestDownloadRandomSDKOpenErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		openCalls++
		return nil, errors.New("open sentinel")
	}}
	server := NewWithSDKDownloadFactory(downloads, func(application.SDKClient) DownloadManager {
		managerFactoryCalls++
		return downloads
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{})
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

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "获取推荐列表失败: open sentinel"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if openCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d manager_factory=%d download_ids=%v", openCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomRecommendationErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, recommendationCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		openCalls++
		return &fakeSDKClient{illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			recommendationCalls++
			return nil, errors.New("recommendation sentinel")
		}}, nil
	}}
	server := NewWithSDKDownloadFactory(downloads, func(application.SDKClient) DownloadManager {
		managerFactoryCalls++
		return downloads
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{})
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

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "获取推荐列表失败: recommendation sentinel"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if openCalls != 1 || recommendationCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d download_ids=%v", openCalls, recommendationCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomEmptyRecommendationPreservesBusinessErrorShape(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, nil)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "无法获取推荐内容，列表为空。"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 0 || len(probe.downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomManagerErrorPreservesBusinessErrorShape(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{77})
	defer closeSession()
	probe.downloads.err = errors.New("download sentinel")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "下载失败: download sentinel"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 1 || probe.downloads.downloadCalls != 1 || !slices.Equal(probe.downloads.downloadIDs, []int64{77}) {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomBuildErrorPreservesBusinessErrorShape(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	_, statErr := os.Stat(missing)
	if statErr == nil {
		t.Fatal("missing test file unexpectedly exists")
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{77})
	defer closeSession()
	probe.downloads.artworks = []download.DownloadedArtwork{{
		IllustID: 77,
		Files:    []download.DownloadedFile{{Path: missing}},
	}}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryImageContent,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "整理下载结果失败: " + statErr.Error()
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 1 || probe.downloads.downloadCalls != 1 || !slices.Equal(probe.downloads.downloadIDs, []int64{77}) {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomRejectsExplicitZeroBeforeOpeningSDK(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 0})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomCountErrorPreservesImageContentDelivery(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": deliveryImageContent,
	})
	const wantText = "错误：count 必须是 1 到 20 之间的整数。"
	assertEmptyDownloadResult(t, result, deliveryImageContent, wantText)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomInvalidDeliveryPrecedesCountValidation(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": "invalid-delivery",
	})
	const wantText = `错误：delivery 仅支持 "local_path" 或 "image_content"。`
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomRejectsExplicitNegativeCountBeforeOpeningSDK(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": -1})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomRejectsCountAboveMaximumBeforeOpeningSDK(t *testing.T) {
	ids := make([]int64, 21)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, ids)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 21})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func assertDownloadRandomCountError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	const wantText = "错误：count 必须是 1 到 20 之间的整数。"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
}

func assertNoDownloadRandomDownstream(t *testing.T, probe *downloadRandomProbe) {
	t.Helper()
	if probe.openCalls != 0 || probe.recommendationCalls != 0 || probe.managerFactoryCalls != 0 || probe.downloads.downloadCalls != 0 || len(probe.downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomOmittedCountDefaultsToFive(t *testing.T) {
	recommendationIDs := []int64{1, 2, 3, 4, 5, 6}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != downloadRandomDefaultCount {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != downloadRandomDefaultCount {
		t.Fatalf("download IDs are not unique: %v", probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomNullCountDefaultsToFive(t *testing.T) {
	recommendationIDs := []int64{1, 2, 3, 4, 5, 6}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": nil})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != downloadRandomDefaultCount {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomToolSchemaDocumentsOptionalCountContract(t *testing.T) {
	session, closeSession, _ := newDownloadRandomProbeSession(t, nil)
	defer closeSession()

	var inputSchema any
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "download_random_from_recommendation" {
			inputSchema = tool.InputSchema
			break
		}
	}
	if inputSchema == nil {
		t.Fatal("download_random_from_recommendation tool not found")
	}
	raw, err := json.Marshal(inputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(schema.Required, "count") {
		t.Fatalf("count must remain optional: schema=%s", raw)
	}
	description := schema.Properties["count"].Description
	if !strings.Contains(description, "defaults to 5") || !strings.Contains(description, "1 to 20") {
		t.Fatalf("count schema does not document default/range: %s", raw)
	}
}

func TestDownloadRandomAcceptsMaximumCount(t *testing.T) {
	recommendationIDs := make([]int64, downloadRandomMaxCount+1)
	for i := range recommendationIDs {
		recommendationIDs[i] = int64(i + 1)
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": downloadRandomMaxCount})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != downloadRandomMaxCount {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != downloadRandomMaxCount {
		t.Fatalf("download IDs are not unique: %v", probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomUsesAvailableRecommendationsWhenListIsShorter(t *testing.T) {
	recommendationIDs := []int64{11, 12, 13}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 5})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != len(recommendationIDs) {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	got := append([]int64(nil), probe.downloads.downloadIDs...)
	slices.Sort(got)
	if !slices.Equal(got, recommendationIDs) {
		t.Fatalf("download IDs=%v want available recommendations %v", got, recommendationIDs)
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
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		probe.openCalls++
		return &fakeSDKClient{illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			probe.recommendationCalls++
			illusts := make([]sdk.Illust, len(recommendationIDs))
			for i, id := range recommendationIDs {
				illusts[i] = testSDKIllust(id, "recommended", 1)
			}
			return &sdk.IllustListResult{Illusts: illusts}, nil
		}}, nil
	}}
	server := NewWithSDKDownloadFactory(probe.downloads, func(application.SDKClient) DownloadManager {
		probe.managerFactoryCalls++
		return probe.downloads
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), service, application.SDKClientRequest{})
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
		session.Close()
		cancel()
	}, probe
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

func TestUserArtworksTextIncludesAllTagsInUpstreamOrder(t *testing.T) {
	illust := testSDKIllust(15, "tagged", 9)
	illust.Tags = []sdk.Tag{
		{Name: "tag-1"}, {Name: "tag-2"}, {Name: "tag-3"}, {Name: "tag-4"},
		{Name: "tag-5"}, {Name: "tag-6"}, {Name: "tag-7"},
	}
	client := &fakeSDKClient{artworks: []sdk.Illust{illust}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 9})
	var out illustListOut
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("user_artworks returned MCP error: %+v", result)
	}
	want := "标签: tag-1, tag-2, tag-3, tag-4, tag-5, tag-6, tag-7"
	if !strings.Contains(out.Text, want) {
		t.Fatalf("user_artworks text = %q, want substring %q", out.Text, want)
	}
}

func TestSDKUserListToolsUseCanonicalUserIDAndFilters(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks: []sdk.Illust{testSDKIllust(15, "saved", 9)},
		following: []sdk.UserPreview{
			{User: sdk.User{ID: 30, Name: "first"}},
			{User: sdk.User{ID: 31, Name: "second"}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	bookmarkResult := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id": 9, "restrict": "private", "tag": "tag-a", "limit": 1,
	})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if client.bookmarksRequest.UserID != 9 || client.bookmarksRequest.Restrict != sdk.RestrictPrivate || client.bookmarksRequest.Tag != "tag-a" || client.bookmarksRequest.Cursor != "" || !strings.Contains(bookmarksOut.Text, "找到用户 9 的 1 个收藏") {
		t.Fatalf("bookmarks request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id": 8, "restrict": "private", "page": 2, "limit": 1,
	})
	var followingOut userListOut
	decodeStructured(t, followingResult, &followingOut)
	if client.followingRequest.UserID != 8 || client.followingRequest.Restrict != sdk.RestrictPrivate || len(followingOut.Items) != 1 || followingOut.Items[0].User.ID != 31 {
		t.Fatalf("following page request=%+v output=%+v", client.followingRequest, followingOut)
	}

	client.bookmarks = []sdk.Illust{}
	client.following = []sdk.UserPreview{}
	bookmarkResult = callTool(t, session, "user_bookmarks", map[string]any{"user_id": 9})
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	followingResult = callTool(t, session, "user_following", map[string]any{"user_id": 8})
	decodeStructured(t, followingResult, &followingOut)
	if bookmarksOut.Text != "找不到用户 9 的收藏。" || followingOut.Text != "用户 8 没有关注任何人。" {
		t.Fatalf("empty text bookmarks=%q following=%q", bookmarksOut.Text, followingOut.Text)
	}
}

func TestSDKUserListToolsSchemaRejectsRemovedLegacyFields(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
	defer closeSession()
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"user_bookmarks", map[string]any{"user_id_to_check": 9}},
		{"user_bookmarks", map[string]any{"user_id": 9, "max_bookmark_id": 1}},
		{"user_following", map[string]any{"user_id_to_check": 8}},
		{"user_following", map[string]any{"user_id": 8, "offset": 1}},
	} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err == nil || !strings.Contains(err.Error(), "additional properties") {
			t.Fatalf("%s args=%v err=%v", call.name, call.args, err)
		}
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
	legacyDefault := &fakeSDKClient{userID: 7, artworkResults: map[sdk.Cursor]sdk.IllustListResult{
		"":     {Illusts: []sdk.Illust{first}, NextCursor: "next"},
		"next": {Illusts: []sdk.Illust{second}},
	}}
	defaultSession, closeDefaultSession := newSDKTestSession(t, legacyDefault)
	defer closeDefaultSession()
	result := callTool(t, defaultSession, "user_artworks", map[string]any{})
	var out illustListOut
	decodeStructured(t, result, &out)
	if len(out.Items) != 1 || !out.Pagination.HasMore || out.Pagination.Limit != nil || out.Pagination.NextPage != nil || len(legacyDefault.artworksRequests) != 1 {
		t.Fatalf("default single-batch output=%+v requests=%+v", out, legacyDefault.artworksRequests)
	}

	paged := &fakeSDKClient{userID: 7, artworkResults: map[sdk.Cursor]sdk.IllustListResult{
		"": {Illusts: []sdk.Illust{first}, NextCursor: "next"},
	}}
	pagedSession, closePagedSession := newSDKTestSession(t, paged)
	defer closePagedSession()
	result = callTool(t, pagedSession, "user_artworks", map[string]any{"limit": 1})
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
	searchIllust          func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error)
	searchIllustOptions   func(context.Context, sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error)
	illustDetail          func(context.Context, int64) (*sdk.IllustDetail, error)
	artworks              []sdk.Illust
	bookmarks             []sdk.Illust
	following             []sdk.UserPreview
	illustRecommended     func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	illustRanking         func(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error)
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
func (*fakeSDKClient) ExportAccountRefreshToken(int64) (string, error) {
	return "", errors.New("unexpected account token export")
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

func (f *fakeSDKClient) SearchIllust(ctx context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
	if f.searchIllust != nil {
		return f.searchIllust(ctx, request)
	}
	return &sdk.IllustListResult{}, nil
}

func (f *fakeSDKClient) SearchIllustOptions(ctx context.Context, request sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
	if f.searchIllustOptions != nil {
		return f.searchIllustOptions(ctx, request)
	}
	return &sdk.SearchIllustOptionsResult{Tools: []string{}}, nil
}

func (f *fakeSDKClient) IllustDetail(ctx context.Context, illustID int64) (*sdk.IllustDetail, error) {
	if f.illustDetail != nil {
		return f.illustDetail(ctx, illustID)
	}
	return &sdk.IllustDetail{}, nil
}
func (*fakeSDKClient) IllustRelated(context.Context, sdk.IllustRelatedRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}

func (f *fakeSDKClient) IllustRanking(ctx context.Context, request sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
	if f.illustRanking != nil {
		return f.illustRanking(ctx, request)
	}
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

func assertEmptyDownloadResult(t *testing.T, result *mcp.CallToolResult, wantDelivery, wantText string) {
	t.Helper()
	out := decodeDownloadOut(t, result)
	if result.IsError || out.Delivery != wantDelivery || out.Text != wantText || out.Items == nil || len(out.Items) != 0 || out.Files == nil || len(out.Files) != 0 {
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
	artworks      []download.DownloadedArtwork
	downloadCalls int
	downloadIDs   []int64
	err           error
}

func (fakeDownloads) SetDownloadPath(string) error { return nil }
func (d *fakeDownloads) Download(_ context.Context, ids []int64) ([]download.DownloadedArtwork, error) {
	d.downloadCalls++
	d.downloadIDs = append([]int64(nil), ids...)
	return d.artworks, d.err
}
