package pixiv_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 搜索类 tool 的 owner 契约：过滤器映射、逻辑分页、schema 拒绝与结构化输出。
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

func TestSearchIllustBookmarkRangeUsesSearchWorkflow(t *testing.T) {
	var pooled bool
	fake := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		low := testSDKIllust(51, "low", 1)
		low.TotalBookmarks = 2
		high := testSDKIllust(52, "high", 1)
		high.TotalBookmarks = 20
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{low, high}}, nil
	}}
	sdkClient := openWireClient(t, fake)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return sdkClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			_, err := attempt(ctx, sdkClient)
			return err
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "miku", "bookmark_min": 10, "limit": 1})
	if !pooled || result.IsError {
		t.Fatalf("pooled=%v result=%+v", pooled, result)
	}
	var out map[string]any
	decodeStructured(t, result, &out)
	if _, ok := out["filter"]; !ok {
		t.Fatalf("search output lacks bookmark filter metadata: %#v", out)
	}
	if resultHasText(result, "51") || !resultHasText(result, "Retrieved 1 records.") {
		t.Fatalf("search result=%+v", result)
	}
}

func TestSearchFailurePreservesStructuredErrorResult(t *testing.T) {
	typedErr := &sdk.Error{
		Product:    "pixiv",
		Operation:  "SearchArtworks",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}
	client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{}, typedErr
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "ordinary-query"})
	if !result.IsError || !resultHasText(result, "Error: "+typedErr.Error()) {
		t.Fatalf("structured search failure changed: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("structured output=%+v, want empty records", out)
	}
}
