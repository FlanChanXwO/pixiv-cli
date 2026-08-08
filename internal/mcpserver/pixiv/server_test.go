package pixiv

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
	"sync/atomic"
	"testing"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerListsExpectedTools(t *testing.T) {
	server := New(&fakeAPI{}, &fakeDownloads{})
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
		"illust_related", "illust_ranking", "search_user", "illust_recommended",
		"recommended", "trending_tags_illust", "timeline_illust_following", "timeline_novel_following",
		"timeline_illust_latest", "timeline_novel_latest", "mypixiv_users", "mypixiv_illusts", "mypixiv_novels",
		"user_detail", "user_artworks", "user_novels", "user_bookmarks", "user_following", "add_bookmark",
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
	for _, field := range []string{"search_target", "duration", "start_date", "end_date", "rating", "content_type", "ai_mode", "aspect_ratio", "resolution", "tool", "bookmark_min", "bookmark_max", "illust_filter"} {
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
		"rating":        {"all", "sfw", "r18", "r18g", "mature"},
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
	for _, field := range []string{"rating", "min_text_length", "max_text_length", "original_only", "page", "limit", "novel_filter"} {
		if !strings.Contains(string(novelSchema), `"`+field+`"`) {
			t.Fatalf("search_novel input schema missing %q: %s", field, novelSchema)
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
		"rating": "r18g", "content_type": "manga", "ai_mode": "only",
		"aspect_ratio": "landscape", "resolution": "high", "tool": "CLIP STUDIO PAINT",
		"bookmark_min": 1000, "bookmark_max": 10000,
	})
	if result.IsError {
		t.Fatalf("search_illust returned MCP error: %+v", result)
	}
	if got.Word != "cat" || got.Target != pixiv.SearchTargetKeyword || got.StartDate != "2026-01-01" || got.EndDate != "2026-01-31" ||
		got.ContentType != pixiv.SearchContentTypeManga || got.AIMode != pixiv.SearchAIModeOnly ||
		got.AspectRatio != pixiv.SearchAspectRatioLandscape || got.Resolution != pixiv.SearchResolutionHigh || got.Tool != "CLIP STUDIO PAINT" ||
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
	var out illustQueryOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "42" || out.Pagination.Returned != 1 {
		t.Fatalf("search_illust output = %+v", out)
	}
}

func TestSearchNovelMapsStableFiltersAndReturnsStructuredOutput(t *testing.T) {
	var got pixiv.SearchNovelsRequest
	client := &fakeSDKClient{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		got = request
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 12, Title: "novel", TextLength: 120, IsOriginal: true}}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_novel", map[string]any{
		"word": "miku", "search_target": "title_and_caption", "sort": "date_asc", "duration": "within_last_week",
		"rating": "r18", "min_text_length": 100, "max_text_length": 1000, "original_only": true, "limit": 1,
	})
	if result.IsError {
		t.Fatalf("search_novel returned MCP error: %+v", result)
	}
	var out novelSearchOut
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
	var out illustDetailOut
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
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 19, Title: "visible", Tags: []pixiv.Tag{}}}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_novel", map[string]any{"word": "miku", "rating": "r18"})
	var out novelSearchOut
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
	var out userSearchOut
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
	var got pixiv.SearchArtworksRequest
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	callTool(t, session, "search_illust", map[string]any{"word": "cat", "rating": "r18"})
	if got.Word != "cat" {
		t.Fatalf("rating r18 request = %+v", got)
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
	var out illustQueryOut
	decodeStructured(t, result, &out)
	if len(requests) != 2 || requests[1].Cursor.IsZero() || requests[0].AIMode != pixiv.SearchAIModeExclude || requests[1].AIMode != pixiv.SearchAIModeExclude || len(out.Records) != 1 || out.Records[0].ID() != "2" {
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
	var out illustQueryOut
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
	var out illustQueryOut
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
	var out illustQueryOut
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

	result := callTool(t, session, "search_illust", map[string]any{"word": "cat", "rating": "r18"})
	var out illustQueryOut
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
		{"rating", "adult"},
		{"content_type", "novel"},
		{"ai_mode", "maybe"},
		{"aspect_ratio", "wide"},
		{"resolution", "ultra"},
		{"search_target", "tags_and_caption"},
		{"duration", "within_last_decade"},
	} {
		t.Run(test.field, func(t *testing.T) {
			factoryCalls := 0
			service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
				factoryCalls++
				return testClientSet(t, &fakeSDKClient{}), nil
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

func TestSearchIllustRejectsInvalidCrossFieldFiltersBeforeOpeningSDK(t *testing.T) {
	for _, arguments := range []map[string]any{
		{"word": "cat", "duration": "within_last_week", "start_date": "2026-01-01"},
		{"word": "cat", "start_date": "2026-02-30"},
		{"word": "cat", "start_date": "2026-02-01", "end_date": "2026-01-31"},
		{"word": "cat", "bookmark_min": 10000, "bookmark_max": 1000},
	} {
		factoryCalls := 0
		service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			factoryCalls++
			return testClientSet(t, &fakeSDKClient{}), nil
		}}
		session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
		result := callTool(t, session, "search_illust", arguments)
		closeSession()
		if !result.IsError || factoryCalls != 0 {
			t.Fatalf("arguments=%v result=%+v factoryCalls=%d", arguments, result, factoryCalls)
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
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, &failingMutationSDKClient{err: &sdk.Error{Product: "pixiv", Operation: "AddBookmark", Reason: sdk.UpstreamError}}), nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, service, pixivapp.SDKClientRequest{})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestToolErrorResultPreservesStructuredContent(t *testing.T) {
	app := &App{}
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
}

func TestOpenSDKOperationReturnsConfigurationError(t *testing.T) {
	app := &App{}
	if _, _, err := app.openSDKOperation(context.Background()); err == nil || err.Error() != "pixiv sdk is not configured" {
		t.Fatalf("open SDK error = %v", err)
	}
}

func TestSDKOperationGateRespectsCanceledContext(t *testing.T) {
	var calls atomic.Int32
	app := &App{
		sdkGate: make(chan struct{}, 1),
		sdk: pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			calls.Add(1)
			return testClientSet(t, &fakeSDKClient{}), nil
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
	var out illustListOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "bookmarks upstream failed") {
		t.Fatalf("structured SDK error = %+v", out)
	}
}

func TestSDKMutationTypedErrorIsMCPError(t *testing.T) {
	client := &failingMutationSDKClient{err: &sdk.Error{
		Product:    "pixiv",
		Operation:  "AddBookmark",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, client), nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, service, pixivapp.SDKClientRequest{})
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
			"src":      "42",
			"delivery": "invalid-delivery",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = `Error: delivery supports only "local_path".`
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
			"delivery": deliveryLocalPath,
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{
		IllustID: 42,
		Files:    []downloadapp.DownloadedFile{{Path: missing}},
	}}}
	session, closeSession := newSDKDownloadTestSession(t, &fakeSDKClient{}, downloads)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"src":      "42",
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
	assertEmptyDownloadResult(t, result, deliveryLocalPath, `Error: delivery supports only "local_path".`)
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{
		IllustID: 9,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloadapp.DownloadedFile{{Path: path, Page: 1}},
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
	if got.Quality != downloadapp.DownloadQualitySmall {
		t.Fatalf("quality=%q", got.Quality)
	}
	if len(got.Pages) != 3 || got.Pages[0] != 1 || got.Pages[1] != 3 || got.Pages[2] != 4 {
		t.Fatalf("pages=%v", got.Pages)
	}
	out := decodeDownloadOut(t, result)
	if out.Delivery != deliveryLocalPath || len(out.Files) != 1 || out.Files[0].Path == "" || out.Files[0].FileURI == "" || out.Files[0].MIMEType == "" {
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
		if out.Delivery != deliveryLocalPath || len(out.Files) != 0 || !strings.HasPrefix(out.Text, "Error: ") {
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{
		IllustID: 42,
		Title:    "title",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloadapp.DownloadedFile{{Path: path}},
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
	if out.Delivery != deliveryLocalPath || len(out.Files) != 1 {
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloadapp.DownloadedFile{{Path: path, Page: 1}},
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{
		IllustID: 42, Title: "work", Author: "artist", Type: "illust", Files: []downloadapp.DownloadedFile{{Path: path}},
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
	var artworksOut illustListOut
	decodeStructured(t, artworks, &artworksOut)
	if client.artworksRequest.UserID != 71 || len(artworksOut.Records) != 1 || artworksOut.Records[0].ID() != "11" || artworksOut.Pagination.Returned != 1 {
		t.Fatalf("user artworks = request=%+v output=%+v", client.artworksRequest, artworksOut)
	}

	bookmarks := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 99, "tag": "tag", "limit": 0})
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarks, &bookmarksOut)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Tag != "tag" || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "12" || bookmarksOut.Pagination.HasMore {
		t.Fatalf("bookmarks = request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id": 99, "limit": 1})
	var followingOut userListOut
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
	var out illustListOut
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
	openCalls := 0
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		openCalls++
		return testClientSet(t, client), nil
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
	var out userDetailOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "42" || client.userDetailRequest != (pixiv.UserRequest{UserID: 42}) || openCalls != 1 {
		t.Fatalf("user_detail output=%+v request=%+v open calls=%d", out, client.userDetailRequest, openCalls)
	}
}

func TestSDKUserDetailRejectsInvalidInputAndReturnsSDKFailuresAsMCPError(t *testing.T) {
	for _, input := range []map[string]any{{"user_id": 0}, {"user_id": -1}} {
		openCalls := 0
		service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			openCalls++
			return testClientSet(t, &fakeSDKClient{}), nil
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
		service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			openCalls++
			return testClientSet(t, &fakeSDKClient{}), nil
		}}
		session, closeSession := newSDKTestSessionWithService(t, &fakeAPI{}, service)
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "user_detail", Arguments: input})
		closeSession()
		if err == nil || openCalls != 0 {
			t.Fatalf("input=%v error=%v open calls=%d", input, err, openCalls)
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
	var typedOut userDetailOut
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
	if !ok || !strings.Contains(text.Text, "pixiv sdk is not configured") {
		t.Fatalf("unconfigured SDK content=%+v", result.Content)
	}
	var unconfiguredOut userDetailOut
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
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}, Illusts: []pixiv.Artwork{}, Novels: []pixiv.Novel{{ID: 5}}}}, Next: testPageCursor(4)}, nil
	}
	openCalls := 0
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		openCalls++
		return testClientSet(t, client), nil
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
	var out recommendedOut
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

	openCalls := 0
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		openCalls++
		return testClientSet(t, &fakeSDKClient{}), nil
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
	var out recommendedOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || out.Pagination != (recommendedPaginationOut{}) {
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
	var out illustQueryOut
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
	var out illustQueryOut
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
	var out illustQueryOut
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
		{mode: "future_mode"},
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
			var out illustQueryOut
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
	downloads := &fakeDownloads{artworks: []downloadapp.DownloadedArtwork{{IllustID: 77, Files: []downloadapp.DownloadedFile{{Path: path}}}}}
	var requests []pixiv.RecommendedArtworksRequest
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, &fakeSDKClient{recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
			requests = append(requests, request)
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(77, "recommended", 1)}}, nil
		}}), nil
	}}
	server := NewWithSDK(&fakeAPI{}, downloads, service, pixivapp.SDKClientRequest{})
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
	if result.IsError || len(result.Content) != 1 || len(requests) != 1 || requests[0] != (pixiv.RecommendedArtworksRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestDownloadRandomSDKOpenErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		openCalls++
		return pixivapp.ClientSet{}, errors.New("open sentinel")
	}}
	server := NewWithSDKDownloadFactory(downloads, func(pixivapp.ClientSet) DownloadManager {
		managerFactoryCalls++
		return downloads
	}, service, pixivapp.SDKClientRequest{})
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
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: open sentinel"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
	if openCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d manager_factory=%d download_ids=%v", openCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomRecommendationErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, recommendationCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		openCalls++
		return testClientSet(t, &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
			recommendationCalls++
			return sdk.Page[pixiv.Artwork]{}, errors.New("recommendation sentinel")
		}}), nil
	}}
	server := NewWithSDKDownloadFactory(downloads, func(pixivapp.ClientSet) DownloadManager {
		managerFactoryCalls++
		return downloads
	}, service, pixivapp.SDKClientRequest{})
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
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: recommendation sentinel"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: the list is empty."
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Download failed: download sentinel"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
	probe.downloads.artworks = []downloadapp.DownloadedArtwork{{
		IllustID: 77,
		Files:    []downloadapp.DownloadedFile{{Path: missing}},
	}}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": deliveryLocalPath,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
		"delivery": deliveryLocalPath,
	})
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
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
	const wantText = "Error: count must be an integer from 1 to 20"
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
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		probe.openCalls++
		return testClientSet(t, &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
			probe.recommendationCalls++
			illusts := make([]pixiv.Artwork, len(recommendationIDs))
			for i, id := range recommendationIDs {
				illusts[i] = testSDKIllust(id, "recommended", 1)
			}
			return sdk.Page[pixiv.Artwork]{Items: illusts}, nil
		}}), nil
	}}
	server := NewWithSDKDownloadFactory(probe.downloads, func(pixivapp.ClientSet) DownloadManager {
		probe.managerFactoryCalls++
		return probe.downloads
	}, service, pixivapp.SDKClientRequest{})
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

func testSDKIllust(id int64, title string, userID int64) pixiv.Artwork {
	return pixiv.Artwork{
		ID:    id,
		Title: title,
		Kind:  pixiv.ArtworkKindIllustration,
		User:  pixiv.User{ID: userID, Name: "artist"},
		Tags:  []pixiv.Tag{},
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
			var out mutationOut
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
	var out illustListOut
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
	var bookmarksOut illustListOut
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if client.bookmarksRequest.UserID != 9 || client.bookmarksRequest.Restrict != pixiv.RestrictPrivate || client.bookmarksRequest.Tag != "tag-a" || !client.bookmarksRequest.Cursor.IsZero() || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "15" {
		t.Fatalf("bookmarks request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id": 8, "restrict": "private", "page": 2, "limit": 1,
	})
	var followingOut userListOut
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
		var out illustListOut
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
	var out illustListOut
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
	server := New(&fakeAPI{}, downloads)
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

func newSDKTestSession(t *testing.T, sdkClient any) (*mcp.ClientSession, func()) {
	return newSDKTestSessionWithAPI(t, &fakeAPI{}, sdkClient)
}

func newSDKDownloadTestSession(t *testing.T, sdkClient any, downloads DownloadManager) (*mcp.ClientSession, func()) {
	t.Helper()
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, sdkClient), nil
	}}
	server := NewWithSDKDownloadFactory(downloads, func(pixivapp.ClientSet) DownloadManager { return downloads }, service, pixivapp.SDKClientRequest{})
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

func newSDKTestSessionWithAPI(t *testing.T, api any, sdkClient any) (*mcp.ClientSession, func()) {
	t.Helper()
	service := pixivapp.SDKService{NewClient: func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, sdkClient), nil
	}}
	return newSDKTestSessionWithService(t, api, service)
}

func testClientSet(t *testing.T, value any) pixivapp.ClientSet {
	t.Helper()
	auth, ok := value.(pixivapp.AuthClient)
	if !ok {
		t.Fatalf("test SDK does not implement AuthClient: %T", value)
	}
	artwork, ok := value.(pixivapp.ArtworkClient)
	if !ok {
		t.Fatalf("test SDK does not implement ArtworkClient: %T", value)
	}
	novel, ok := value.(pixivapp.NovelClient)
	if !ok {
		t.Fatalf("test SDK does not implement NovelClient: %T", value)
	}
	user, ok := value.(pixivapp.UserClient)
	if !ok {
		t.Fatalf("test SDK does not implement UserClient: %T", value)
	}
	mutation, ok := value.(pixivapp.MutationClient)
	if !ok {
		t.Fatalf("test SDK does not implement MutationClient: %T", value)
	}
	resource, ok := value.(pixivapp.ResourceClient)
	if !ok {
		t.Fatalf("test SDK does not implement ResourceClient: %T", value)
	}
	return pixivapp.NewClientSet(auth, artwork, novel, user, mutation, resource)
}

func newSDKTestSessionWithService(t *testing.T, api any, service pixivapp.SDKService) (*mcp.ClientSession, func()) {
	return newSDKTestSessionWithServiceRequest(t, api, service, pixivapp.SDKClientRequest{})
}

func newSDKTestSessionWithServiceRequest(t *testing.T, api any, service pixivapp.SDKService, request pixivapp.SDKClientRequest) (*mcp.ClientSession, func()) {
	t.Helper()
	server := NewWithSDKDownloadFactory(&fakeDownloads{}, func(pixivapp.ClientSet) DownloadManager { return &fakeDownloads{} }, service, request)
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

func resultHasText(result *mcp.CallToolResult, wanted string) bool {
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if ok && strings.Contains(text.Text, wanted) {
			return true
		}
	}
	return false
}

type fakeSDKClient struct {
	pixivapp.ClientSet
	userID                   int64
	searchIllust             func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	searchNovel              func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error)
	searchUser               func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	illustDetail             func(context.Context, int64) (pixiv.Artwork, error)
	artworks                 []pixiv.Artwork
	bookmarks                []pixiv.Artwork
	following                []pixiv.UserPreview
	followingIllusts         func(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	followingNovels          func(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error)
	latestIllusts            func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	latestNovels             func(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error)
	myPixivUsers             func(context.Context, pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	myPixivIllusts           func(context.Context, pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	myPixivNovels            func(context.Context, pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userNovels               func(context.Context, pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error)
	recommendedArtworks      func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	illustRanking            func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error)
	novelRecommended         func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error)
	userRecommended          func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error)
	userDetailResult         pixiv.UserDetail
	userDetailErr            error
	userDetailRequest        pixiv.UserRequest
	artworksRequest          pixiv.UserArtworksRequest
	artworksRequests         []pixiv.UserArtworksRequest
	userArtworksFunc         func(pixiv.UserArtworksRequest, int) (sdk.Page[pixiv.Artwork], error)
	userArtworksCalls        int
	recommendedArtworksCalls int
	bookmarksRequest         pixiv.UserArtworkBookmarksRequest
	userBookmarksErr         error
	followingRequest         pixiv.UserFollowingRequest
	addBookmarkRequest       pixiv.AddBookmarkRequest
	removeBookmarkRequest    pixiv.RemoveBookmarkRequest
	followUserRequest        pixiv.FollowUserRequest
	unfollowUserRequest      pixiv.UnfollowUserRequest
}

type failingMutationSDKClient struct {
	fakeSDKClient
	err error
}

func (f *failingMutationSDKClient) AddBookmark(context.Context, pixiv.AddBookmarkRequest) error {
	return f.err
}

func (f *fakeSDKClient) CurrentUserID(context.Context) (int64, error) { return f.userID, nil }

func (f *fakeSDKClient) SearchArtworks(ctx context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.searchIllust != nil {
		return f.searchIllust(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) SearchNovels(ctx context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.searchNovel != nil {
		return f.searchNovel(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) Artwork(ctx context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	if f.illustDetail != nil {
		return f.illustDetail(ctx, request.ArtworkID)
	}
	return pixiv.Artwork{}, nil
}

func (*fakeSDKClient) RelatedArtworks(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) ArtworkRanking(ctx context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.illustRanking != nil {
		return f.illustRanking(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) RecommendedArtworks(ctx context.Context, request pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	f.recommendedArtworksCalls++
	if f.recommendedArtworks != nil {
		return f.recommendedArtworks(ctx, request, f.recommendedArtworksCalls)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) FollowingArtworks(ctx context.Context, request pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.followingIllusts != nil {
		return f.followingIllusts(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) FollowingNovels(ctx context.Context, request pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.followingNovels != nil {
		return f.followingNovels(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) LatestArtworks(ctx context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.latestIllusts != nil {
		return f.latestIllusts(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) LatestNovels(ctx context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.latestNovels != nil {
		return f.latestNovels(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) MyPixivUsers(ctx context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.myPixivUsers != nil {
		return f.myPixivUsers(ctx, request)
	}
	return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
}

func (f *fakeSDKClient) MyPixivArtworks(ctx context.Context, request pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.myPixivIllusts != nil {
		return f.myPixivIllusts(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}}, nil
}

func (f *fakeSDKClient) MyPixivNovels(ctx context.Context, request pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.myPixivNovels != nil {
		return f.myPixivNovels(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) SearchUsers(ctx context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.searchUser != nil {
		return f.searchUser(ctx, request)
	}
	return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
}

func (*fakeSDKClient) TrendingArtworkTags(context.Context, pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error) {
	return []pixiv.TrendingTag{}, nil
}

func (f *fakeSDKClient) RecommendedNovels(ctx context.Context, request pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.novelRecommended != nil {
		return f.novelRecommended(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) RecommendedUsers(ctx context.Context, request pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
	if f.userRecommended != nil {
		return f.userRecommended(ctx, request)
	}
	return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{}}, nil
}

// User 记录 MCP 到公开 SDK 的完整请求，供结构化输出与错误路径断言。
func (f *fakeSDKClient) User(_ context.Context, request pixiv.UserRequest) (pixiv.UserDetail, error) {
	f.userDetailRequest = request
	if f.userDetailErr != nil {
		return pixiv.UserDetail{}, f.userDetailErr
	}
	return f.userDetailResult, nil
}

func (f *fakeSDKClient) UserArtworks(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	f.artworksRequest = request
	f.artworksRequests = append(f.artworksRequests, request)
	if f.userArtworksFunc != nil {
		f.userArtworksCalls++
		return f.userArtworksFunc(request, f.userArtworksCalls)
	}
	return sdk.Page[pixiv.Artwork]{Items: f.artworks}, nil
}

func (f *fakeSDKClient) UserArtworkBookmarks(_ context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	f.bookmarksRequest = request
	if f.userBookmarksErr != nil {
		return sdk.Page[pixiv.Artwork]{}, f.userBookmarksErr
	}
	return sdk.Page[pixiv.Artwork]{Items: f.bookmarks}, nil
}

func (f *fakeSDKClient) UserFollowing(_ context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
	f.followingRequest = request
	return sdk.Page[pixiv.UserPreview]{Items: f.following}, nil
}

func (f *fakeSDKClient) UserNovels(ctx context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
	if f.userNovels != nil {
		return f.userNovels(ctx, request)
	}
	return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{}}, nil
}

func (f *fakeSDKClient) AddBookmark(_ context.Context, request pixiv.AddBookmarkRequest) error {
	f.addBookmarkRequest = request
	return nil
}

func (f *fakeSDKClient) RemoveBookmark(_ context.Context, request pixiv.RemoveBookmarkRequest) error {
	f.removeBookmarkRequest = request
	return nil
}

func (f *fakeSDKClient) FollowUser(_ context.Context, request pixiv.FollowUserRequest) error {
	f.followUserRequest = request
	return nil
}

func (f *fakeSDKClient) UnfollowUser(_ context.Context, request pixiv.UnfollowUserRequest) error {
	f.unfollowUserRequest = request
	return nil
}

func (*fakeSDKClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, errors.New("resource is not configured")
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
	artworks      []downloadapp.DownloadedArtwork
	downloadCalls int
	downloadIDs   []int64
	lastRequest   downloadapp.DownloadRequest
	err           error
}

func (fakeDownloads) SetDownloadPath(string) error { return nil }
func (d *fakeDownloads) Download(_ context.Context, request downloadapp.DownloadRequest) ([]downloadapp.DownloadedArtwork, error) {
	ids := request.IllustIDs

	d.downloadCalls++
	d.downloadIDs = append([]int64(nil), ids...)
	d.lastRequest = request
	return d.artworks, d.err
}
