package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestEntityToolsExposeOnlyRecordsContract 固定所有实体读取 tool 的共同输出：
// structured content 只承载 records 和分页信息，Content 只用于短摘要而不复制实体。
func TestEntityToolsExposeOnlyRecordsContract(t *testing.T) {
	client := &fakeSDKClient{
		userID:    90,
		artworks:  []sdk.Illust{testSDKIllust(21, "artwork", 90)},
		bookmarks: []sdk.Illust{testSDKIllust(22, "bookmark", 90)},
		following: []sdk.UserPreview{{User: sdk.User{ID: 23, Name: "following"}}},
		searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(1, "search-illust", 1)}}, nil
		},
		searchNovel: func(context.Context, sdk.SearchNovelRequest) (*sdk.NovelListResult, error) {
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 2, URL: "https://www.pixiv.net/novel/show.php?id=2", Title: "search-novel", User: sdk.User{ID: 2}}}}, nil
		},
		searchUser: func(context.Context, sdk.SearchUserRequest) (*sdk.UserListResult, error) {
			return &sdk.UserListResult{Source: sdk.UserSearchSourceApp, UserPreviews: []sdk.UserPreview{{User: sdk.User{ID: 3, Name: "search-user"}}}}, nil
		},
		illustDetail: func(context.Context, int64) (*sdk.IllustDetail, error) {
			return &sdk.IllustDetail{Illust: testSDKIllust(4, "detail", 4)}, nil
		},
		illustRanking: func(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(5, "ranking", 5)}}, nil
		},
		illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(6, "recommended", 6)}}, nil
		},
		userDetailResult: &sdk.UserDetailResult{User: sdk.User{ID: 7, Name: "detail-user"}, Profile: sdk.Profile{Region: "Tokyo"}},
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
		{"illust_follow", map[string]any{}},
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

func TestRecommendedAllFlattensRecordsInStableKindOrder(t *testing.T) {
	client := &fakeSDKClient{
		illustRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(11, "illust", 1)}}, nil
		},
		mangaRecommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			manga := testSDKIllust(12, "manga", 2)
			manga.Type = string(sdk.IllustTypeManga)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{manga}}, nil
		},
		novelRecommended: func(context.Context, sdk.NovelRecommendedRequest) (*sdk.NovelListResult, error) {
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 13, URL: "https://www.pixiv.net/novel/show.php?id=13", Title: "novel", User: sdk.User{ID: 3}}}}, nil
		},
		userRecommended: func(context.Context, sdk.UserRecommendedRequest) (*sdk.UserRecommendedResult, error) {
			return &sdk.UserRecommendedResult{UserPreviews: []sdk.RecommendedUserPreview{{User: sdk.User{ID: 14, Name: "user"}}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "recommended", map[string]any{"kind": "all"})
	if result.IsError {
		t.Fatalf("recommended returned error: %+v", result)
	}
	structured := assertRecordsOnlyStructuredOutput(t, result)
	records := structured["records"].([]any)
	var gotIDs, gotTypes []string
	for _, raw := range records {
		record := raw.(map[string]any)
		gotIDs = append(gotIDs, record["id"].(string))
		gotTypes = append(gotTypes, record["type"].(string))
	}
	if !slices.Equal(gotIDs, []string{"11", "12", "13", "14"}) || !slices.Equal(gotTypes, []string{"illust", "manga", "novel", "user"}) {
		t.Fatalf("flattened records = ids %v types %v", gotIDs, gotTypes)
	}
}

func TestEntityRecordPreservesUserDetailEnvelopeAndErrorHasEmptyRecords(t *testing.T) {
	profileURL := "https://example.test/profile"
	client := &fakeSDKClient{
		userDetailResult: &sdk.UserDetailResult{
			User:             sdk.User{ID: 55, Name: "artist", Account: "artist-account"},
			Profile:          sdk.Profile{Region: "Tokyo", Webpage: &profileURL, TotalIllusts: 9},
			ProfilePublicity: sdk.ProfilePublicity{Region: true},
			Workspace:        sdk.Workspace{PC: "desktop", Tool: "pen"},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 55})
	structured := assertRecordsOnlyStructuredOutput(t, result)
	records := structured["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("user detail records=%#v", records)
	}
	record := records[0].(map[string]any)
	if record["id"] != "55" || record["type"] != "user" || record["url"] != "https://www.pixiv.net/users/55" {
		t.Fatalf("user detail identity=%#v", record)
	}
	if record["profile"].(map[string]any)["webpage"] != profileURL || record["workspace"].(map[string]any)["tool"] != "pen" {
		t.Fatalf("user detail envelope was lost: %#v", record)
	}

	failed, closeFailed := newSDKTestSession(t, &fakeSDKClient{searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		return nil, errors.New("upstream failed")
	}})
	defer closeFailed()
	errorResult := callTool(t, failed, "search_illust", map[string]any{"word": "record"})
	if !errorResult.IsError {
		t.Fatalf("error result=%+v", errorResult)
	}
	structured = assertRecordsOnlyStructuredOutput(t, errorResult)
	if len(structured["records"].([]any)) != 0 {
		t.Fatalf("error records=%#v", structured["records"])
	}
}

func TestServerImplementationVersionIsProtocolOnlyException(t *testing.T) {
	server := New(&fakeAPI{}, &fakeDownloads{}, nil)
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
