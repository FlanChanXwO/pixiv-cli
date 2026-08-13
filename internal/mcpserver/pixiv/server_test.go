package pixiv_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func TestSearchIllustMapsStableFiltersToPublicSDK(t *testing.T) {
	var got pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{
		"word": "cat", "search_target": "keyword", "start_date": "2026-01-01", "end_date": "2026-01-31",
		"content_type": "manga", "ai_mode": "only",
		"aspect_ratio": "landscape", "resolution": "high", "tool": "CLIP STUDIO PAINT",
		"bookmark_min": 1000, "bookmark_max": 10000, "bookmark_strategy": "best_effort",
	})
	if result.IsError {
		t.Fatalf("search_illust returned MCP error: %+v", result)
	}
	if got.Word != "cat" || got.Target != pixiv.SearchTargetKeyword || got.StartDate != "2026-01-01" || got.EndDate != "2026-01-31" ||
		got.ContentType != pixiv.SearchContentTypeManga ||
		got.AspectRatio != pixiv.SearchAspectRatioLandscape || got.Tool != "CLIP STUDIO PAINT" ||
		got.BookmarkMin == nil || *got.BookmarkMin != 1000 || got.BookmarkMax == nil || *got.BookmarkMax != 10000 {
		t.Fatalf("SearchArtworks request = %+v, want word=cat stable filters", got)
	}
}

func TestSearchIllustExpandsLongQuickDurationToTokyoDateRange(t *testing.T) {
	var got pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "duration": "within_half_year"})
	if result.IsError {
		t.Fatalf("search_illust returned MCP error: %+v", result)
	}
	if got.Duration != "" || got.StartDate == "" || got.EndDate == "" || got.StartDate > got.EndDate {
		t.Fatalf("SearchArtworks request = %+v, want date range without duration", got)
	}
}

func TestSearchIllustReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(42, "work", 7)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat"})
	if result.IsError {
		t.Fatalf("search_illust returned MCP error: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "42" || out.Pagination.Returned != 1 {
		t.Fatalf("search_illust output = %+v", out)
	}
}

func TestSearchNovelMapsStableFiltersAndReturnsStructuredOutput(t *testing.T) {
	var got pixiv.SearchNovelsRequest
	client := &fakeSDKClient{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		got = request
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 12, Title: "novel", TextLength: 120, IsOriginal: true, User: pixiv.User{ID: 9, Name: "author"}}}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_novel", map[string]any{
		"word": "miku", "search_target": "title_and_caption", "sort": "date_asc", "duration": "within_last_week",
		"limit": 1,
	})
	if result.IsError {
		t.Fatalf("search_novel returned MCP error: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if got.Word != "miku" || got.Target != pixiv.SearchTargetTitleAndCaption || got.Sort != pixiv.SortModeDateAsc || got.Duration != pixiv.DurationFilter("within_last_week") {
		t.Fatalf("SearchNovels request = %+v, want stable parameters", got)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "12" || out.Pagination.Returned != 1 {
		t.Fatalf("search_novel output = %+v", out)
	}
}

func TestIllustDetailAcceptsArtworkURLAndReturnsStructuredOutput(t *testing.T) {
	var gotID int64
	client := &fakeSDKClient{illustDetail: func(_ context.Context, id int64) (pixiv.Artwork, error) {
		gotID = id
		return testSDKIllust(id, "work", 7), nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_detail", map[string]any{"url": "https://www.pixiv.net/en/artworks/42?from=share"})
	if result.IsError {
		t.Fatalf("illust_detail returned MCP error: %+v", result)
	}
	var out outputs.UserDetail
	decodeStructured(t, result, &out)
	if gotID != 42 || len(out.Records) != 1 || out.Records[0].ID() != "42" {
		t.Fatalf("illust_detail id=%d output=%+v, want URL-resolved artwork", gotID, out)
	}

	invalid := callTool(t, session, "illust_detail", map[string]any{"illust_id": 42, "url": "https://www.pixiv.net/artworks/42"})
	if !invalid.IsError {
		t.Fatalf("illust_detail accepted both references: %+v", invalid)
	}
	text, ok := invalid.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "exactly one") || strings.Contains(text.Text, "https://") {
		t.Fatalf("illust_detail invalid input = %#v", invalid.Content)
	}
}

func TestSearchNovelContinuesAfterLocallyFilteredEmptyBatch(t *testing.T) {
	var requests []pixiv.SearchNovelsRequest
	client := &fakeSDKClient{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 19, Title: "visible", User: pixiv.User{ID: 5}, Tags: []pixiv.Tag{}}}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_novel", map[string]any{"word": "miku"})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor.IsZero() || len(out.Records) != 1 || out.Records[0].ID() != "19" {
		t.Fatalf("requests=%+v output=%+v", requests, out)
	}
}

func TestSearchUserReturnsSourceAndStructuredPreviews(t *testing.T) {
	var got pixiv.SearchUsersRequest
	client := &fakeSDKClient{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		got = request
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 7, Name: "author", Account: "author_account"}}}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_user", map[string]any{"word": "author", "limit": 1})
	if result.IsError {
		t.Fatalf("search_user returned MCP error: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if got.Word != "author" || len(out.Records) != 1 || out.Records[0].ID() != "7" || out.Pagination.Returned != 1 {
		t.Fatalf("search_user request=%+v output=%+v", got, out)
	}
}

func TestSearchIllustSchemaRejectsRemovedLegacyWireFields(t *testing.T) {
	calls := 0
	client := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		calls++
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	for _, args := range []map[string]any{
		{"word": "cat", "search_r18": true},
		{"word": "cat", "offset": 1},
		{"word": "cat", "include_thumbnail": true},
		{"word": "cat", "filter": "bookmarkCount >= 2"},
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

func TestSearchIllustRejectsUnsupportedRatingBeforeOpeningSDK(t *testing.T) {
	calls := 0
	client := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		calls++
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_illust", Arguments: map[string]any{"word": "cat", "rating": "r18"}})
	if err == nil || !strings.Contains(err.Error(), "additional properties") || calls != 0 {
		t.Fatalf("unsupported rating error=%v calls=%d", err, calls)
	}
}

func TestSearchIllustKeepsFiltersAcrossLogicalPagePagination(t *testing.T) {
	var requests []pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 7)}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "second", 7)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 2, "limit": 1, "ai_mode": "exclude"})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor.IsZero() || len(out.Records) != 1 || out.Records[0].ID() != "2" {
		t.Fatalf("requests=%+v output=%+v", requests, out)
	}
}

func TestSearchIllustPageLimitFillsLogicalResultsAcrossEmptyBatches(t *testing.T) {
	var requests []pixiv.SearchArtworksRequest
	var searchCalls int
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		searchCalls++
		switch searchCalls {
		case 1:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "a", 7)}, Next: testPageCursor(1)}, nil
		case 2:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}, Next: testPageCursor(2)}, nil
		case 3:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "b", 7), testSDKIllust(3, "c", 7)}, Next: testPageCursor(3)}, nil
		default:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(4, "d", 7)}}, nil
		}
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "limit": 3})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(requests) != 3 || requests[1].Cursor.IsZero() || requests[2].Cursor.IsZero() {
		t.Fatalf("requests=%+v", requests)
	}
	if !slices.Equal([]string{out.Records[0].ID(), out.Records[1].ID(), out.Records[2].ID()}, []string{"1", "2", "3"}) {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustPageTwoUsesLogicalLimit(t *testing.T) {
	var requests []pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "a", 7), testSDKIllust(2, "b", 7)}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(3, "c", 7), testSDKIllust(4, "d", 7)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 2, "limit": 2})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor.IsZero() {
		t.Fatalf("requests=%+v", requests)
	}
	if !slices.Equal([]string{out.Records[0].ID(), out.Records[1].ID()}, []string{"3", "4"}) {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustRejectsPageWithoutPositiveLimit(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		t.Fatal("SDK should not open for invalid page plan")
		return sdk.Page[pixiv.Artwork]{}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "page": 1})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "page") || !resultHasText(result, "limit") {
		t.Fatalf("output=%+v", out)
	}
}

func TestSearchIllustContinuesAfterFilteredEmptyBatch(t *testing.T) {
	var requests []pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "visible", 7)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat"})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor.IsZero() || len(out.Records) != 1 || out.Records[0].ID() != "2" {
		t.Fatalf("requests=%+v output=%+v", requests, out)
	}
}

func TestSearchIllustSchemaRejectsInvalidEnumsBeforeOpeningSDK(t *testing.T) {
	for _, test := range []struct {
		field string
		value string
	}{
		{"content_type", "novel"},
		{"ai_mode", "maybe"},
		{"aspect_ratio", "wide"},
		{"resolution", "ultra"},
		{"search_target", "tags_and_caption"},
		{"duration", "within_last_decade"},
	} {
		t.Run(test.field, func(t *testing.T) {
			client := &fakeSDKClient{}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()
			_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_illust", Arguments: map[string]any{"word": "cat", test.field: test.value}})
			if err == nil || client.searchIllustRequest != (pixiv.SearchArtworksRequest{}) {
				t.Fatalf("invalid %s=%q error=%v captured=%+v", test.field, test.value, err, client.searchIllustRequest)
			}
		})
	}
}

func TestSearchIllustRejectsInvalidCrossFieldFiltersBeforeOpeningSDK(t *testing.T) {
	for _, arguments := range []map[string]any{
		{"word": "cat", "duration": "within_last_week", "start_date": "2026-01-01"},
		{"word": "cat", "start_date": "2026-02-30"},
		{"word": "cat", "start_date": "2026-02-01", "end_date": "2026-01-31"},
		{"word": "cat", "bookmark_min": 10000, "bookmark_max": 1000},
	} {
		client := &fakeSDKClient{}
		session, closeSession := newSDKTestSession(t, client)
		result := callTool(t, session, "search_illust", arguments)
		closeSession()
		if !result.IsError || client.searchIllustRequest != (pixiv.SearchArtworksRequest{}) {
			t.Fatalf("arguments=%v result=%+v captured=%+v", arguments, result, client.searchIllustRequest)
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
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "page must be a positive integer") {
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
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "UserArtworkBookmarks") || !resultHasText(result, "upstream_error") {
		t.Fatalf("structured SDK error = %+v", out)
	}
}

func TestSDKMutationTypedErrorIsMCPError(t *testing.T) {
	client := &fakeSDKClient{addBookmarkErr: &sdk.Error{
		Product:    "pixiv",
		Operation:  "AddBookmark",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 41})
	if !result.IsError {
		t.Fatalf("typed SDK mutation failure must be an MCP error: %+v", result)
	}
	var out outputs.Mutation
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured mutation error = %+v", out)
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
	const wantText = "Error: provide src (one source) or srcs (a source list)"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
			"src":      "42",
			"delivery": "invalid-delivery",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = `Error: delivery supports only "local_path".`
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if downloads.downloadCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("download calls=%d IDs=%v want no downstream call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadManagerErrorPreservesBusinessErrorShape(t *testing.T) {
	downloads := &fakeDownloads{err: errors.New("download sentinel")}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	out := decodeDownloadOut(t, result)
	if !result.IsError || len(out.Failures) != 1 || out.Failures[0].Message != "download sentinel" {
		t.Fatalf("result=%+v output=%+v", result, out)
	}
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
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42,
		Files:    []downloader.DownloadedFile{{Path: missing}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if downloads.downloadCalls != 1 || !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download calls=%d IDs=%v want one manager call", downloads.downloadCalls, downloads.downloadIDs)
	}
}

func TestDownloadRejectsImageContentDelivery(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": "image_content",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertEmptyDownloadResult(t, result, "local_path", `Error: delivery supports only "local_path".`)
	if downloads.downloadCalls != 0 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
}

func TestDownloadPassesPagesAndQualityToManager(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "9.png")
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 9,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: path, Page: 1}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":     "9",
			"pages":   "1,3-4",
			"quality": "small",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("download failed: %+v", result)
	}
	if downloads.downloadCalls != 1 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
	got := downloads.lastRequest
	if len(got.IllustIDs) != 1 || got.IllustIDs[0] != 9 {
		t.Fatalf("ids=%v", got.IllustIDs)
	}
	if got.Quality != downloader.DownloadQualitySmall {
		t.Fatalf("quality=%q", got.Quality)
	}
	if len(got.Pages) != 3 || got.Pages[0] != 1 || got.Pages[1] != 3 || got.Pages[2] != 4 {
		t.Fatalf("pages=%v", got.Pages)
	}
	out := decodeDownloadOut(t, result)
	if out.Delivery != "local_path" || len(out.Files) != 1 || out.Files[0].Path == "" || out.Files[0].FileURI == "" || out.Files[0].MIMEType == "" {
		t.Fatalf("local delivery output=%+v", out)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks=%d want text only", len(result.Content))
	}
}

func TestDownloadRejectsInvalidPagesAndQualityBeforeManager(t *testing.T) {
	downloads := &fakeDownloads{}
	session, closeSession := newTestSession(t, downloads)
	defer closeSession()
	for _, args := range []map[string]any{
		{"src": "1", "pages": "0"},
		{"src": "1", "quality": "huge"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "download", Arguments: args})
		if err != nil {
			t.Fatalf("call tool: %v", err)
		}
		out := decodeDownloadOut(t, result)
		if out.Delivery != "local_path" || len(out.Files) != 0 || !strings.HasPrefix(out.Text, "Error: ") {
			t.Fatalf("args=%v output=%+v", args, out)
		}
	}
	if downloads.downloadCalls != 0 {
		t.Fatalf("download calls=%d", downloads.downloadCalls)
	}
}

func TestDownloadDefaultsToLocalPathResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42,
		Title:    "title",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: path}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "download",
		Arguments: map[string]any{"srcs": []string{"42", "42"}},
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
	if out.Delivery != "local_path" || len(out.Files) != 1 {
		t.Fatalf("unexpected structured output: %+v", out)
	}
	if out.Files[0].MIMEType != "image/jpeg" || out.Files[0].SizeBytes != 4 || !strings.HasPrefix(out.Files[0].FileURI, "file://") {
		t.Fatalf("unexpected file output: %+v", out.Files[0])
	}
	if !slices.Equal(downloads.downloadIDs, []int64{42}) {
		t.Fatalf("download IDs = %v", downloads.downloadIDs)
	}
}

func TestDownloadAcceptsArtworkURLsAndIncludesCanonicalURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloader.DownloadedFile{{Path: path, Page: 1}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result := callTool(t, session, "download", map[string]any{"src": "https://www.pixiv.net/en/artworks/42?from=share"})
	if result.IsError {
		t.Fatalf("download result=%+v", result)
	}
	out := decodeDownloadOut(t, result)
	if len(out.Items) != 1 || out.Items[0].URL != "https://www.pixiv.net/artworks/42" || downloads.downloadIDs[0] != 42 {
		t.Fatalf("download output=%+v ids=%v", out, downloads.downloadIDs)
	}
}

func TestDownloadUserURLExpandsEveryVisualArtworkType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "42.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloader.DownloadedFile{{Path: path}},
	}}}
	client := &fakeSDKClient{artworks: []pixiv.Artwork{testSDKIllust(42, "work", 7)}}
	session, closeSession := newSDKDownloadTestSession(t, client, downloads)
	defer closeSession()

	result := callTool(t, session, "download", map[string]any{"src": "https://www.pixiv.net/users/7/artworks"})
	if result.IsError {
		t.Fatalf("download result=%+v", result)
	}
	out := decodeDownloadOut(t, result)
	if len(out.Items) != 1 || downloads.downloadCalls != 1 {
		t.Fatalf("download output=%+v calls=%d", out, downloads.downloadCalls)
	}
	gotTypes := make([]pixiv.ArtworkKind, 0, len(client.artworksRequests))
	for _, request := range client.artworksRequests {
		gotTypes = append(gotTypes, request.Kind)
	}
	if !slices.Equal(gotTypes, []pixiv.ArtworkKind{pixiv.ArtworkKindIllustration, pixiv.ArtworkKindManga, pixiv.ArtworkKindUgoira}) {
		t.Fatalf("UserArtworks types = %v", gotTypes)
	}
}

func TestSDKUserToolsResolveIdentityAndReturnStructuredOutput(t *testing.T) {
	client := &fakeSDKClient{
		userID:    71,
		artworks:  []pixiv.Artwork{testSDKIllust(11, "work", 71)},
		bookmarks: []pixiv.Artwork{testSDKIllust(12, "saved", 99)},
		following: []pixiv.UserPreview{{User: pixiv.User{ID: 33, Name: "followed", Account: "f"}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	artworks := callTool(t, session, "user_artworks", map[string]any{"limit": 1})
	var artworksOut outputs.Records
	decodeStructured(t, artworks, &artworksOut)
	if client.artworksRequest.UserID != 71 || len(artworksOut.Records) != 1 || artworksOut.Records[0].ID() != "11" || artworksOut.Pagination.Returned != 1 {
		t.Fatalf("user artworks = request=%+v output=%+v", client.artworksRequest, artworksOut)
	}

	bookmarks := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 99, "tag": "tag", "limit": 0})
	var bookmarksOut outputs.Records
	decodeStructured(t, bookmarks, &bookmarksOut)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Tag != "tag" || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "12" || bookmarksOut.Pagination.HasMore {
		t.Fatalf("bookmarks = request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id": 99, "limit": 1})
	var followingOut outputs.Records
	decodeStructured(t, following, &followingOut)
	if client.followingRequest.UserID != 99 || len(followingOut.Records) != 1 || followingOut.Records[0].ID() != "33" {
		t.Fatalf("following = request=%+v output=%+v", client.followingRequest, followingOut)
	}
}

func TestUserArtworksPreservesArtworkRecordsAtMCPBoundary(t *testing.T) {
	withNilTools := testSDKIllust(11, "without-tools", 71)
	withTools := testSDKIllust(12, "with-tools", 71)
	withTools.Tools = []string{"CLIP STUDIO PAINT", "Photoshop"}
	client := &fakeSDKClient{userID: 71, artworks: []pixiv.Artwork{withNilTools, withTools}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 71})
	if result.IsError {
		t.Fatalf("user_artworks result=%+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 2 || out.Records[0].ID() != "11" || out.Records[1].ID() != "12" {
		t.Fatalf("artwork records=%+v", out)
	}
}

func TestSDKUserDetailReturnsStructuredSDKResult(t *testing.T) {
	webpage := "https://example.test/artist"
	workspaceImage := "https://example.test/workspace.png"
	want := pixiv.UserDetail{
		User:             pixiv.User{ID: 42, Name: "artist", Account: "artist_account", Comment: "hello"},
		Profile:          pixiv.UserProfile{Webpage: webpage, Region: "Tokyo", CountryCode: "JP", Job: "illustrator", TotalIllusts: 10, TotalManga: 2, TotalNovels: 3, TotalFollowUsers: 4},
		ProfilePublicity: pixiv.UserProfilePublicity{Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true},
		Workspace:        pixiv.UserWorkspace{PC: "desktop", Tool: "pen", WorkspaceImageURL: workspaceImage},
	}
	client := &fakeSDKClient{userDetailResult: want}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if result.IsError {
		t.Fatalf("user_detail returned error: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("user_detail text content = %+v", result.Content)
	}
	var out outputs.UserDetail
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "42" || client.userDetailRequest != (pixiv.UserRequest{UserID: 42}) {
		t.Fatalf("user_detail output=%+v request=%+v", out, client.userDetailRequest)
	}
}

func TestSDKUserDetailRejectsInvalidInputAndReturnsSDKFailuresAsMCPError(t *testing.T) {
	for _, input := range []map[string]any{{"user_id": 0}, {"user_id": -1}} {
		client := &fakeSDKClient{}
		session, closeSession := newSDKTestSession(t, client)
		result := callTool(t, session, "user_detail", input)
		closeSession()
		if !result.IsError || client.userDetailRequest != (pixiv.UserRequest{}) {
			t.Fatalf("input=%v result=%+v captured=%+v", input, result, client.userDetailRequest)
		}
	}
	for _, input := range []map[string]any{{}, {"user_id": "not-an-integer"}} {
		client := &fakeSDKClient{}
		session, closeSession := newSDKTestSession(t, client)
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "user_detail", Arguments: input})
		closeSession()
		if err == nil || client.userDetailRequest != (pixiv.UserRequest{}) {
			t.Fatalf("input=%v error=%v captured=%+v", input, err, client.userDetailRequest)
		}
	}

	typed := &sdk.Error{Product: "pixiv", Operation: "User", Reason: sdk.MalformedUpstreamResponse}
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
	var typedOut outputs.UserDetail
	decodeStructured(t, result, &typedOut)
	if len(typedOut.Records) != 0 {
		t.Fatalf("typed SDK failure structured output=%+v", typedOut)
	}

	noSDKSession, closeNoSDKSession := newTestSession(t, &fakeDownloads{})
	defer closeNoSDKSession()
	result = callTool(t, noSDKSession, "user_detail", map[string]any{"user_id": 42})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("unconfigured SDK result=%+v", result)
	}
	text, ok = result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "sdk pooled operation is not configured") {
		t.Fatalf("unconfigured SDK content=%+v", result.Content)
	}
	var unconfiguredOut outputs.UserDetail
	decodeStructured(t, result, &unconfiguredOut)
	if len(unconfiguredOut.Records) != 0 {
		t.Fatalf("unconfigured SDK structured output=%+v", unconfiguredOut)
	}
}

func TestSDKRecommendedAllReturnsEveryStreamAndPagination(t *testing.T) {
	client := &fakeSDKClient{}
	var order []string
	client.recommendedArtworks = func(_ context.Context, _ pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if call == 1 {
			order = append(order, "illust")
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "illust", 10)}, Next: testPageCursor(1)}, nil
		}
		order = append(order, "manga")
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "manga", 20)}, Next: testPageCursor(2)}, nil
	}
	client.novelRecommended = func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		order = append(order, "novel")
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 30}, Tags: []pixiv.Tag{}}}, Next: testPageCursor(3)}, nil
	}
	client.userRecommended = func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		order = append(order, "user")
		return sdk.Page[pixiv.UserPreview]{
			Items: []pixiv.UserPreview{{
				User:    pixiv.User{ID: 4},
				Illusts: []pixiv.Artwork{},
				Novels:  []pixiv.Novel{{ID: 5, User: pixiv.User{ID: 40}}},
			}},
			Next: testPageCursor(4),
		}, nil
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "recommended", map[string]any{"kind": "all", "limit": 1})
	if result.IsError || !slices.Equal(order, []string{"illust", "manga", "novel", "user"}) {
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

func TestRecommendedPreservesAllTopLevelAndUserPreviewRecords(t *testing.T) {
	withoutTools := testSDKIllust(1, "without-tools", 10)
	withTools := testSDKIllust(2, "with-tools", 10)
	withTools.Tools = []string{"SAI", "Photoshop"}
	client := &fakeSDKClient{
		recommendedArtworks: func(_ context.Context, _ pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
			if call == 1 {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{withoutTools, withTools}}, nil
			}
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{withoutTools}}, nil
		},
		novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
		},
		userRecommended: func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{
				User:    pixiv.User{ID: 10},
				Illusts: []pixiv.Artwork{withoutTools, withTools},
				Novels:  []pixiv.Novel{},
			}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "recommended", map[string]any{"kind": "all"})
	if result.IsError {
		t.Fatalf("recommended result=%+v", result)
	}
	var out outputs.Recommended
	decodeStructured(t, result, &out)
	if len(out.Records) != 4 || !slices.Equal([]string{out.Records[0].ID(), out.Records[1].ID(), out.Records[2].ID(), out.Records[3].ID()}, []string{"1", "2", "1", "10"}) {
		t.Fatalf("recommended records=%+v", out)
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
				recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
					calls = append(calls, "visual")
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
				},
				novelRecommended: func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
					calls = append(calls, "novel")
					return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
				},
				userRecommended: func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
					calls = append(calls, "user")
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
				},
			}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()
			result := callTool(t, session, "recommended", map[string]any{"kind": test.kind})
			wantCall := test.want
			if wantCall == "illust" || wantCall == "manga" {
				wantCall = "visual"
			}
			if result.IsError || !slices.Equal(calls, []string{wantCall}) {
				t.Fatalf("kind=%s result=%+v calls=%v", test.kind, result, calls)
			}
		})
	}

	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "unknown"})
	if !result.IsError {
		t.Fatalf("invalid kind result=%+v", result)
	}
	for _, input := range []map[string]any{{}, {"kind": 9}} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "recommended", Arguments: input})
		if err == nil {
			t.Fatalf("input=%v error=%v", input, err)
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
		recommendedArtworks: func(_ context.Context, _ pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
			if call == 1 {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 1)}}, nil
			}
			return sdk.Page[pixiv.Artwork]{}, errors.New("malformed upstream response")
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "all"})
	if !result.IsError {
		t.Fatalf("all failure result=%+v", result)
	}
	var out outputs.Recommended
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || out.Pagination != (outputs.RecommendedPagination{}) {
		t.Fatalf("partial structured output=%+v", out)
	}
}

func TestSDKRecommendedAllAppliesPageTwoIndependently(t *testing.T) {
	var illust, manga, novel, users []bool
	client := &fakeSDKClient{
		recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
			if call <= 2 {
				illust = append(illust, request.Cursor.IsZero())
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 1)}, Next: testPageCursor(1)}, nil
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(11, "second", 1)}}, nil
			}
			manga = append(manga, request.Cursor.IsZero())
			if request.Cursor.IsZero() {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "first", 2)}, Next: testPageCursor(2)}, nil
			}
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(12, "second", 2)}}, nil
		},
		novelRecommended: func(_ context.Context, request pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			novel = append(novel, request.Cursor.IsZero())
			if request.Cursor.IsZero() {
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, User: pixiv.User{ID: 3}}}, Next: testPageCursor(3)}, nil
			}
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 13, User: pixiv.User{ID: 3}}}}, nil
		},
		userRecommended: func(_ context.Context, request pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			users = append(users, request.Cursor.IsZero())
			if request.Cursor.IsZero() {
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}}, Next: testPageCursor(4)}, nil
			}
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 14}}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "recommended", map[string]any{"kind": "all", "page": 2, "limit": 1})
	if result.IsError || !slices.Equal(illust, []bool{true, false}) || !slices.Equal(manga, []bool{true, false}) || !slices.Equal(novel, []bool{true, false}) || !slices.Equal(users, []bool{true, false}) {
		t.Fatalf("result=%+v cursors=%v/%v/%v/%v", result, illust, manga, novel, users)
	}
	var structured map[string]any
	decodeStructured(t, result, &structured)
	records, ok := structured["records"].([]any)
	if !ok || len(records) != 4 {
		t.Fatalf("records=%#v", structured["records"])
	}
}

func TestIllustRecommendedUsesSDKAndLogicalPageSkip(t *testing.T) {
	var requests []pixiv.RecommendedArtworksRequest
	client := &fakeSDKClient{
		recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
			requests = append(requests, request)
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
				testSDKIllust(11, "first", 1),
				testSDKIllust(77, "after-skip", 1),
			}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "illust_recommended", map[string]any{"page": 2, "limit": 1})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError || len(requests) != 1 || !requests[0].Cursor.IsZero() || len(out.Records) != 1 || out.Records[0].ID() != "77" {
		t.Fatalf("result=%+v requests=%+v", out, requests)
	}
}

func TestIllustRecommendedReturnsTaggedRecord(t *testing.T) {
	illust := testSDKIllust(77, "tagged", 9)
	illust.Tags = []pixiv.Tag{
		{Name: "tag-1"}, {Name: "tag-2"}, {Name: "tag-3"}, {Name: "tag-4"},
		{Name: "tag-5"}, {Name: "tag-6"}, {Name: "tag-7"},
	}
	client := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{illust}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_recommended", map[string]any{})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("illust_recommended returned MCP error: %+v", result)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "77" {
		t.Fatalf("illust_recommended records = %+v", out)
	}
}

func TestIllustRankingPassesRequestAndReturnsRecord(t *testing.T) {
	var requests []pixiv.ArtworkRankingRequest
	client := &fakeSDKClient{illustRanking: func(_ context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
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
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("illust_ranking returned MCP error: %+v", result)
	}
	if len(requests) != 1 || requests[0].Mode != pixiv.RankingModeDayMale || requests[0].Date != "2025-02-03" || !requests[0].Cursor.IsZero() {
		t.Fatalf("ranking requests = %+v", requests)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "13" {
		t.Fatalf("illust_ranking records = %+v", out)
	}
}

func TestIllustRankingSupportsAllModes(t *testing.T) {
	tests := []struct {
		mode string
	}{
		{mode: "day"},
		{mode: "day_male"},
		{mode: "day_female"},
		{mode: "week"},
		{mode: "week_original"},
		{mode: "week_rookie"},
		{mode: "month"},
		{mode: "day_manga"},
		{mode: "week_manga"},
		{mode: "month_manga"},
		{mode: "week_rookie_manga"},
		{mode: "day_r18"},
		{mode: "day_male_r18"},
		{mode: "day_female_r18"},
		{mode: "week_r18"},
		{mode: "week_r18g"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			client := &fakeSDKClient{illustRanking: func(_ context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
				if string(request.Mode) != test.mode {
					t.Fatalf("ranking mode = %q, want %q", request.Mode, test.mode)
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(13, "ranked", 1)}}, nil
			}}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			result := callTool(t, session, "illust_ranking", map[string]any{"mode": test.mode})
			var out outputs.Records
			decodeStructured(t, result, &out)
			if result.IsError || len(out.Records) != 1 || out.Records[0].ID() != "13" {
				t.Fatalf("illust_ranking(%q) output = %+v", test.mode, out)
			}
		})
	}
}

func TestDownloadRandomFromRecommendationUsesSDKAndPreservesCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommended.jpg")
	if err := os.WriteFile(path, []byte("recommended"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{IllustID: 77, Files: []downloader.DownloadedFile{{Path: path}}}}}
	var requests []pixiv.RecommendedArtworksRequest
	sdkClient := &fakeSDKClient{recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(77, "recommended", 1)}}, nil
	}}
	server := pixivmcpserver.NewWithSDK(&fakeAPI{}, downloads, testSDKPorts(t, sdkClient), pixivmcpserver.Account{})
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
	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 1})
	if result.IsError || len(result.Content) != 1 || len(requests) != 1 || requests[0] != (pixiv.RecommendedArtworksRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestDownloadRandomSDKOpenErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	server := pixivmcpserver.NewWithSDKDownloadFactory(downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		managerFactoryCalls++
		return downloads
	}, pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			openCalls++
			return nil, errors.New("open sentinel")
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return nil
		},
	}, pixivmcpserver.Account{})
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

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: open sentinel"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if openCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d manager_factory=%d download_ids=%v", openCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomRecommendationErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, recommendationCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	sdkClient := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		recommendationCalls++
		return sdk.Page[pixiv.Artwork]{}, errors.New("recommendation sentinel")
	}}
	wireClient := openWireClient(t, sdkClient)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			openCalls++
			return wireClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, wireClient)
			return err
		},
	}
	server := pixivmcpserver.NewWithSDKDownloadFactory(downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		managerFactoryCalls++
		return downloads
	}, ports, pixivmcpserver.Account{})
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

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: pixiv:RecommendedArtworks: upstream_error"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: the list is empty."
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Download failed: download sentinel"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
	probe.downloads.artworks = []downloader.DownloadedArtwork{{
		IllustID: 77,
		Files:    []downloader.DownloadedFile{{Path: missing}},
	}}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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

func TestDownloadRandomCountErrorPreservesLocalPathDelivery(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": "local_path",
	})
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomInvalidDeliveryPrecedesCountValidation(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": "invalid-delivery",
	})
	const wantText = `Error: delivery supports only "local_path".`
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
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
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 5 {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 5 {
		t.Fatalf("download IDs are not unique: %v", probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomNullCountDefaultsToFive(t *testing.T) {
	recommendationIDs := []int64{1, 2, 3, 4, 5, 6}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": nil})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 5 {
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
	recommendationIDs := make([]int64, 21)
	for i := range recommendationIDs {
		recommendationIDs[i] = int64(i + 1)
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 20})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 20 {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 20 {
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
		{"add_bookmark", map[string]any{"illust_id": 9, "restrict": "private", "tags": []string{"one"}}, "add_bookmark", "Bookmarked artwork 9."},
		{"remove_bookmark", map[string]any{"illust_id": 9}, "remove_bookmark", "Removed bookmark from artwork 9."},
		{"follow_user", map[string]any{"user_id": 8, "restrict": "private"}, "follow_user", "Followed user 8."},
		{"unfollow_user", map[string]any{"user_id": 8}, "unfollow_user", "Unfollowed user 8."},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			var out outputs.Mutation
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.want || out.Text != test.wantText {
				t.Fatalf("mutation output = %+v", out)
			}
		})
	}
	if client.addBookmarkRequest.ArtworkID != 9 || client.addBookmarkRequest.Restrict != pixiv.RestrictPrivate || !slices.Equal(client.addBookmarkRequest.Tags, []string{"one"}) {
		t.Fatalf("add bookmark request = %+v", client.addBookmarkRequest)
	}
	if client.removeBookmarkRequest.ArtworkID != 9 || client.followUserRequest.UserID != 8 || client.followUserRequest.Restrict != pixiv.RestrictPrivate || client.unfollowUserRequest.UserID != 8 {
		t.Fatalf("mutation requests = remove=%+v follow=%+v unfollow=%+v", client.removeBookmarkRequest, client.followUserRequest, client.unfollowUserRequest)
	}
}

func TestUserArtworksReturnsTaggedArtworkRecord(t *testing.T) {
	illust := testSDKIllust(15, "tagged", 9)
	illust.Tags = []pixiv.Tag{
		{Name: "tag-1"}, {Name: "tag-2"}, {Name: "tag-3"}, {Name: "tag-4"},
		{Name: "tag-5"}, {Name: "tag-6"}, {Name: "tag-7"},
	}
	client := &fakeSDKClient{artworks: []pixiv.Artwork{illust}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 9})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("user_artworks returned MCP error: %+v", result)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "15" {
		t.Fatalf("user_artworks records=%+v", out)
	}
}

func TestSDKUserListToolsUseCanonicalUserIDAndFilters(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks: []pixiv.Artwork{testSDKIllust(15, "saved", 9)},
		following: []pixiv.UserPreview{
			{User: pixiv.User{ID: 30, Name: "first"}},
			{User: pixiv.User{ID: 31, Name: "second"}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	bookmarkResult := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id": 9, "restrict": "private", "tag": "tag-a", "limit": 1,
	})
	var bookmarksOut outputs.Records
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if client.bookmarksRequest.UserID != 9 || client.bookmarksRequest.Restrict != pixiv.RestrictPrivate || client.bookmarksRequest.Tag != "tag-a" || !client.bookmarksRequest.Cursor.IsZero() || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "15" {
		t.Fatalf("bookmarks request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id": 8, "restrict": "private", "page": 2, "limit": 1,
	})
	var followingOut outputs.Records
	decodeStructured(t, followingResult, &followingOut)
	if client.followingRequest.UserID != 8 || client.followingRequest.Restrict != pixiv.RestrictPrivate || len(followingOut.Records) != 1 || followingOut.Records[0].ID() != "31" {
		t.Fatalf("following page request=%+v output=%+v", client.followingRequest, followingOut)
	}

	client.bookmarks = []pixiv.Artwork{}
	client.following = []pixiv.UserPreview{}
	bookmarkResult = callTool(t, session, "user_bookmarks", map[string]any{"user_id": 9})
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	followingResult = callTool(t, session, "user_following", map[string]any{"user_id": 8})
	decodeStructured(t, followingResult, &followingOut)
	if len(bookmarksOut.Records) != 0 || len(followingOut.Records) != 0 {
		t.Fatalf("empty records bookmarks=%+v following=%+v", bookmarksOut, followingOut)
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
		var out outputs.Records
		decodeStructured(t, result, &out)
		if !resultHasText(result, "page") {
			t.Fatalf("arguments=%v output=%+v", arguments, out)
		}
	}
}

func TestSDKListsFollowOpaqueCursorForLimitAndRejectCycles(t *testing.T) {
	first := testSDKIllust(1, "first", 7)
	second := testSDKIllust(2, "second", 7)
	legacyDefault := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	defaultSession, closeDefaultSession := newSDKTestSession(t, legacyDefault)
	defer closeDefaultSession()
	result := callTool(t, defaultSession, "user_artworks", map[string]any{})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || !out.Pagination.HasMore || out.Pagination.Limit != nil || out.Pagination.NextPage != nil || len(legacyDefault.artworksRequests) != 1 {
		t.Fatalf("default single-batch output=%+v requests=%+v", out, legacyDefault.artworksRequests)
	}

	paged := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	pagedSession, closePagedSession := newSDKTestSession(t, paged)
	defer closePagedSession()
	result = callTool(t, pagedSession, "user_artworks", map[string]any{"limit": 1})
	decodeStructured(t, result, &out)
	if !out.Pagination.HasMore || out.Pagination.NextPage == nil || *out.Pagination.NextPage != 2 {
		t.Fatalf("single-page pagination=%+v", out.Pagination)
	}

	client := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result = callTool(t, session, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if len(out.Records) != 2 || out.Pagination.HasMore || len(client.artworksRequests) != 2 || client.artworksRequests[1].Cursor.IsZero() {
		t.Fatalf("all-pages output=%+v requests=%+v", out, client.artworksRequests)
	}

	cycleCursor := testPageCursor(9)
	cyclic := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: cycleCursor}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}, Next: cycleCursor}, nil
	}}
	cycleSession, closeCycleSession := newSDKTestSession(t, cyclic)
	defer closeCycleSession()
	result = callTool(t, cycleSession, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if !resultHasText(result, "cursor repeated") || len(cyclic.artworksRequests) != 2 {
		t.Fatalf("cycle output=%+v requests=%+v", out, cyclic.artworksRequests)
	}
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
