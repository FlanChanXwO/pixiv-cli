package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func TestIllustSeriesMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		artworkSeriesPage: sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(11, "series-artwork", 1)}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_series", map[string]any{"series_id": 10})
	if result.IsError || client.artworkSeriesRequest.SeriesID != 10 {
		t.Fatalf("illust series result=%+v request=%+v", result, client.artworkSeriesRequest)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "11" {
		t.Fatalf("illust series records=%+v", out.Records)
	}
}

func TestNovelDetailMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		novelDetailResult: pixiv.Novel{ID: 12, Title: "novel-detail", User: pixiv.User{ID: 2}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_detail", map[string]any{"novel_id": 12})
	if result.IsError || client.novelRequest.NovelID != 12 {
		t.Fatalf("novel detail result=%+v request=%+v", result, client.novelRequest)
	}
	var out outputs.NovelDetail
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "12" {
		t.Fatalf("novel detail records=%+v", out.Records)
	}
}

func TestNovelContentReturnsBlocksWithOpaqueResourceRefs(t *testing.T) {
	client := &fakeSDKClient{
		novelContentHTML: `<!DOCTYPE html><html><body>` +
			`<h1 class="title">novel-content</h1>` +
			`<p class="noveltext">complete body</p>` +
			`<figure class="novel_image"><img src="https://i.pximg.net/novel/12/image"></figure>` +
			`</body></html>`,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_content", map[string]any{"novel_id": 12})
	if result.IsError || client.novelContentRequest.NovelID != 12 {
		t.Fatalf("novel content result=%+v request=%+v", result, client.novelContentRequest)
	}
	var out outputs.NovelContent
	decodeStructured(t, result, &out)
	derivedRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_image","id":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content.Blocks) != 2 || out.Content.Blocks[0].Text != "complete body" || out.Content.Blocks[1].Image == nil || out.Content.Blocks[1].Image.Resource == nil || out.Content.Blocks[1].Image.Resource.Ref != derivedRef.String() {
		t.Fatalf("novel content=%+v", out)
	}
	rawContent, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawContent), "https://private.example") || strings.Contains(strings.ToLower(string(rawContent)), "cookie") || strings.Contains(strings.ToLower(string(rawContent)), "header") {
		t.Fatalf("novel content leaked resource transport data: %s", rawContent)
	}
}

func TestIllustCommentsMapsRequestAndPreservesEnvelope(t *testing.T) {
	total := int64(3)
	client := &fakeSDKClient{
		artworkCommentsResult: pixiv.CommentPage{
			Page:          sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 21, Comment: "artwork comment", User: pixiv.User{ID: 4}}}},
			Total:         &total,
			AccessControl: &pixiv.CommentAccessControl{CanComment: true},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_comments", map[string]any{"id": 20})
	if result.IsError || client.artworkCommentsRequest.ArtworkID != 20 {
		t.Fatalf("artwork comments result=%+v request=%+v", result, client.artworkCommentsRequest)
	}
	var out outputs.Comments
	decodeStructured(t, result, &out)
	if len(out.Comments) != 1 || out.Comments[0].ID != 21 || out.Total == nil || *out.Total != total || out.AccessControl == nil || !out.AccessControl.CanComment {
		t.Fatalf("artwork comments=%+v", out)
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

func TestSearchIllustFilterFillsLogicalLimitAndDeduplicatesAcrossPages(t *testing.T) {
	requests := make([]pixiv.SearchArtworksRequest, 0, 2)
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
				filteredTestIllust(1, 1, 1, "other"),
				filteredTestIllust(2, 10, 2, "keep"),
			}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
			filteredTestIllust(2, 10, 2, "keep"),
			filteredTestIllust(3, 20, 3, "keep"),
		}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{
		"word": "cat", "limit": 2,
		"illust_filter": map[string]any{"tags": []string{"keep"}, "min_views": 10, "min_pages": 2},
	})
	if result.IsError {
		t.Fatalf("search_illust result=%+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	got := make([]string, 0, len(out.Records))
	for _, record := range out.Records {
		got = append(got, record.ID())
	}
	if !slices.Equal(got, []string{"2", "3"}) || out.Pagination.Returned != 2 || len(requests) != 2 {
		t.Fatalf("records=%v pagination=%+v requests=%+v", got, out.Pagination, requests)
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
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(6, "recommended", 6)}}, nil
		},
		userDetailResult: pixiv.UserDetail{User: pixiv.User{ID: 7, Name: "detail-user"}, Profile: pixiv.UserProfile{Region: "Tokyo"}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"search_illust", map[string]any{"word": "record"}},
		{"search_novel", map[string]any{"word": "record"}},
		{"illust_detail", map[string]any{"illust_id": 4}},
		{"illust_related", map[string]any{"illust_id": 4}},
		{"illust_ranking", map[string]any{}},
		{"illust_recommended", map[string]any{}},
		{"timeline_illust_following", map[string]any{}},
		{"search_user", map[string]any{"word": "record"}},
		{"user_detail", map[string]any{"user_id": 7}},
		{"user_artworks", map[string]any{"user_id": 90}},
		{"user_bookmarks", map[string]any{"user_id": 90}},
		{"user_following", map[string]any{"user_id": 90}},
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

func TestServerImplementationVersionIsProtocolOnlyException(t *testing.T) {
	server := pixivmcpserver.New(&fakeAPI{}, &fakeDownloads{})
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
	if session.InitializeResult().ServerInfo.Version != "3.0.0" {
		t.Fatalf("serverInfo.version=%q, want 3.0.0", session.InitializeResult().ServerInfo.Version)
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
