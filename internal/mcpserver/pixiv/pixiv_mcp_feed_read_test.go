package pixiv_test

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFeedRecommendationSchemasMatchLegacyContracts(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	for _, test := range []struct {
		name   string
		fields []string
	}{
		{name: "illust_ranking", fields: []string{"date", "illust_filter", "limit", "mode", "page"}},
		{name: "illust_recommended", fields: []string{"illust_filter", "limit", "page"}},
		{name: "recommended", fields: []string{"illust_filter", "kind", "limit", "novel_filter", "page", "user_filter"}},
		{name: "timeline_illust_following", fields: []string{"illust_filter", "limit", "page", "restrict"}},
		{name: "timeline_novel_following", fields: []string{"limit", "novel_filter", "page", "restrict"}},
		{name: "timeline_illust_latest", fields: []string{"content_type", "illust_filter", "limit", "page"}},
		{name: "timeline_novel_latest", fields: []string{"limit", "novel_filter", "page"}},
		{name: "trending_tags_illust", fields: []string{}},
	} {
		t.Run("input/"+test.name, func(t *testing.T) {
			tool, ok := byName[test.name]
			if !ok {
				t.Fatalf("tool %q is not registered", test.name)
			}
			schema := feedSchemaObject(t, tool.Name+" input", tool.InputSchema)
			assertSchemaFields(t, schema, test.fields)
		})
	}

	assertSchemaEnum(t, feedSchemaProperty(t, byName["illust_ranking"].InputSchema, "mode"), []string{
		"day", "day_male", "day_female", "week", "week_original", "week_rookie", "month",
		"day_manga", "week_manga", "month_manga", "week_rookie_manga", "day_r18", "day_male_r18",
		"day_female_r18", "week_r18", "week_r18g",
	})
	assertSchemaEnum(t, feedSchemaProperty(t, byName["recommended"].InputSchema, "kind"), []string{"all", "illust", "manga", "novel", "user"})
	assertSchemaEnum(t, feedSchemaProperty(t, byName["timeline_illust_following"].InputSchema, "restrict"), []string{"public", "private"})
	assertSchemaEnum(t, feedSchemaProperty(t, byName["timeline_novel_following"].InputSchema, "restrict"), []string{"public", "private"})
	assertSchemaEnum(t, feedSchemaProperty(t, byName["timeline_illust_latest"].InputSchema, "content_type"), []string{"illust", "manga"})

	recommendedOutput := feedSchemaObject(t, "recommended output", byName["recommended"].OutputSchema)
	assertSchemaFields(t, recommendedOutput, []string{"pagination", "records"})
	assertSchemaRequired(t, recommendedOutput, []string{"pagination", "records"})
	pagination := feedSchemaProperty(t, recommendedOutput, "pagination")
	assertSchemaFields(t, pagination, []string{"illust", "manga", "novel", "user"})

	trendingOutput := feedSchemaObject(t, "trending_tags_illust output", byName["trending_tags_illust"].OutputSchema)
	assertSchemaFields(t, trendingOutput, []string{"tags", "text"})
	assertSchemaRequired(t, trendingOutput, []string{"tags", "text"})
}

func TestRecommendedKindSelectsArtworkSubtype(t *testing.T) {
	for _, test := range []struct {
		kind           string
		wantType       pixiv.ArtworkKind
		wantPagination string
	}{
		{kind: "illust", wantType: pixiv.ArtworkKindIllustration, wantPagination: "illust"},
		{kind: "manga", wantType: pixiv.ArtworkKindManga, wantPagination: "manga"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			illust := testSDKIllust(101, "illust", 1)
			manga := testSDKIllust(102, "manga", 2)
			manga.Kind = pixiv.ArtworkKindManga
			client := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{illust, manga}}, nil
			}}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			result := callTool(t, session, "recommended", map[string]any{"kind": test.kind})
			if result.IsError {
				t.Fatalf("recommended %s returned error: %+v", test.kind, result)
			}
			var structured map[string]any
			decodeStructured(t, result, &structured)
			records, ok := structured["records"].([]any)
			if !ok || len(records) != 1 {
				t.Fatalf("recommended %s records=%#v", test.kind, structured["records"])
			}
			record, ok := records[0].(map[string]any)
			if !ok || record["type"] != string(test.wantType) {
				t.Fatalf("recommended %s record=%#v, want type %q", test.kind, records[0], test.wantType)
			}
			pagination, ok := structured["pagination"].(map[string]any)
			if !ok || len(pagination) != 1 {
				t.Fatalf("recommended %s pagination=%#v", test.kind, structured["pagination"])
			}
			if _, ok := pagination[test.wantPagination]; !ok {
				t.Fatalf("recommended %s pagination=%#v, missing %q", test.kind, pagination, test.wantPagination)
			}
		})
	}
}

func TestRecommendedRejectsKindConflictingFiltersBeforeSDKExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{name: "illust rejects novel filter", args: map[string]any{"kind": "illust", "novel_filter": map[string]any{"id": 201}}},
		{name: "manga rejects conflicting artwork type", args: map[string]any{"kind": "manga", "illust_filter": map[string]any{"type": "illust"}}},
		{name: "novel rejects user filter", args: map[string]any{"kind": "novel", "user_filter": map[string]any{"id": 401}}},
		{name: "user rejects artwork filter", args: map[string]any{"kind": "user", "illust_filter": map[string]any{"type": "illust"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var executions int
			client := &fakeSDKClient{}
			ports := testSDKPorts(t, client)
			baseExecute := ports.Execute
			ports.Execute = func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				executions++
				return baseExecute(ctx, account, attempt)
			}
			session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
			defer closeSession()

			result := callTool(t, session, "recommended", test.args)
			if !result.IsError || executions != 0 {
				t.Fatalf("conflicting filter result=%+v executions=%d", result, executions)
			}
			var out outputs.Recommended
			decodeStructured(t, result, &out)
			if len(out.Records) != 0 {
				t.Fatalf("conflicting filter returned records=%+v", out.Records)
			}
		})
	}
}

func TestIllustRankingRejectsInvalidInputBeforeSDKExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{name: "unknown mode", args: map[string]any{"mode": "not-a-ranking"}},
		{name: "invalid calendar date", args: map[string]any{"date": "2025-02-30"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var executions int
			ports := testSDKPorts(t, &fakeSDKClient{})
			baseExecute := ports.Execute
			ports.Execute = func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				executions++
				return baseExecute(ctx, account, attempt)
			}
			session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
			defer closeSession()

			result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "illust_ranking", Arguments: test.args})
			if callErr == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid input unexpectedly succeeded: result=%+v err=%v", result, callErr)
			}
			if executions != 0 {
				t.Fatalf("invalid input entered SDK execution: %d", executions)
			}
		})
	}
}

func TestFeedRecommendationLegacyJSONReplayPreservesStructuredContracts(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		args   map[string]any
		client *fakeSDKClient
		check  func(*testing.T, *mcp.CallToolResult)
	}{
		{
			name: "ranking defaults mode",
			tool: "illust_ranking",
			args: map[string]any{},
			client: &fakeSDKClient{illustRanking: func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(11, "ranking", 1)}}, nil
			}},
			check: func(t *testing.T, result *mcp.CallToolResult) { assertFeedRecordID(t, result, "11") },
		},
		{
			name: "recommended all",
			tool: "recommended",
			args: map[string]any{"kind": "all"},
			client: func() *fakeSDKClient {
				call := 0
				client := &fakeSDKClient{}
				client.recommendedArtworks = func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
					call++
					item := testSDKIllust(int64(20+call), "artwork", int64(call))
					if call == 2 {
						item.Kind = pixiv.ArtworkKindManga
					}
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}}, nil
				}
				client.novelRecommended = func(context.Context, pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
					return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 23, User: pixiv.User{ID: 3}, Tags: []pixiv.Tag{}}}}, nil
				}
				client.userRecommended = func(context.Context, pixiv.RecommendedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 24}, Illusts: []pixiv.Artwork{}, Novels: []pixiv.Novel{}}}}, nil
				}
				return client
			}(),
			check: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Fatalf("recommended all returned error: %+v", result)
				}
				var structured map[string]any
				decodeStructured(t, result, &structured)
				records, ok := structured["records"].([]any)
				if !ok || len(records) != 4 {
					t.Fatalf("recommended all records=%#v", structured["records"])
				}
				pagination, ok := structured["pagination"].(map[string]any)
				if !ok || len(pagination) != 4 {
					t.Fatalf("recommended all pagination=%#v", structured["pagination"])
				}
			},
		},
		{
			name: "illust following defaults public",
			tool: "timeline_illust_following",
			args: map[string]any{},
			client: &fakeSDKClient{followingIllusts: func(context.Context, pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(31, "following", 3)}}, nil
			}},
			check: func(t *testing.T, result *mcp.CallToolResult) { assertFeedRecordID(t, result, "31") },
		},
		{
			name: "novel following defaults public",
			tool: "timeline_novel_following",
			args: map[string]any{},
			client: &fakeSDKClient{followingNovels: func(context.Context, pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 32, User: pixiv.User{ID: 3}, Tags: []pixiv.Tag{}}}}, nil
			}},
			check: func(t *testing.T, result *mcp.CallToolResult) { assertFeedRecordID(t, result, "32") },
		},
		{
			name: "latest illust preserves content type",
			tool: "timeline_illust_latest",
			args: map[string]any{"content_type": "manga"},
			client: &fakeSDKClient{latestIllusts: func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(33, "latest", 3)}}, nil
			}},
			check: func(t *testing.T, result *mcp.CallToolResult) { assertFeedRecordID(t, result, "33") },
		},
		{
			name: "latest novel",
			tool: "timeline_novel_latest",
			args: map[string]any{},
			client: &fakeSDKClient{latestNovels: func(context.Context, pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 34, User: pixiv.User{ID: 3}, Tags: []pixiv.Tag{}}}}, nil
			}},
			check: func(t *testing.T, result *mcp.CallToolResult) { assertFeedRecordID(t, result, "34") },
		},
		{
			name:   "trending tags",
			tool:   "trending_tags_illust",
			args:   map[string]any{},
			client: &fakeSDKClient{trendingTags: []pixiv.TrendingTag{{Tag: "miku", TranslatedName: "Hatsune Miku", Artwork: testSDKIllust(35, "miku", 3)}}},
			check: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Fatalf("trending tags returned error: %+v", result)
				}
				var out outputs.TrendingTags
				decodeStructured(t, result, &out)
				if len(out.Tags) != 1 || out.Tags[0].Tag != "miku" {
					t.Fatalf("trending tags=%+v", out.Tags)
				}
			},
		},
		{
			name:   "trending tags empty",
			tool:   "trending_tags_illust",
			args:   map[string]any{},
			client: &fakeSDKClient{trendingTags: []pixiv.TrendingTag{}},
			check: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Fatalf("empty trending tags returned error: %+v", result)
				}
				var out outputs.TrendingTags
				decodeStructured(t, result, &out)
				if out.Tags == nil || len(out.Tags) != 0 || out.Text != "No trending tags found." {
					t.Fatalf("empty trending tags=%+v", out)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()
			result := callTool(t, session, test.tool, test.args)
			test.check(t, result)
		})
	}
}

func TestTimelineIllustFilterFillsLogicalPageAcrossBatches(t *testing.T) {
	calls := 0
	client := &fakeSDKClient{latestIllusts: func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		calls++
		if calls == 1 {
			wrong := testSDKIllust(41, "manga", 4)
			wrong.Kind = pixiv.ArtworkKindManga
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{wrong}, Next: testPageCursor(41)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(42, "illust", 4)}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "timeline_illust_latest", map[string]any{
		"content_type":  "illust",
		"illust_filter": map[string]any{"type": "illust"},
		"limit":         1,
	})
	if result.IsError {
		t.Fatalf("filtered timeline returned error: %s", feedResultText(result))
	}
	assertFeedRecordID(t, result, "42")
	if calls != 2 {
		t.Fatalf("filtered logical page calls=%d, want 2", calls)
	}
}

func feedSchemaObject(t *testing.T, name string, raw any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal %s schema: %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("decode %s schema: %v", name, err)
	}
	if schema["type"] != "object" {
		t.Fatalf("%s schema type=%#v", name, schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("%s schema must be closed: %#v", name, schema["additionalProperties"])
	}
	return schema
}

func feedSchemaProperty(t *testing.T, raw any, name string) map[string]any {
	t.Helper()
	schema := feedSchemaObject(t, name+" parent", raw)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s parent properties=%#v", name, schema["properties"])
	}
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q=%#v", name, properties[name])
	}
	return property
}

func assertSchemaFields(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%#v", schema["properties"])
	}
	got := make([]string, 0, len(properties))
	for field := range properties {
		got = append(got, field)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("schema fields=%v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("schema fields=%v, want %v", got, want)
		}
	}
}

func assertSchemaRequired(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	encoded, err := json.Marshal(schema["required"])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if string(encoded) != "null" {
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("schema required=%v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("schema required=%v, want %v", got, want)
		}
	}
}

func assertSchemaEnum(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	encoded, err := json.Marshal(schema["enum"])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("schema enum=%#v: %v", schema["enum"], err)
	}
	if len(got) != len(want) {
		t.Fatalf("schema enum=%v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("schema enum=%v, want %v", got, want)
		}
	}
}

func assertFeedRecordID(t *testing.T, result *mcp.CallToolResult, want string) {
	t.Helper()
	if result.IsError {
		t.Fatalf("feed result returned error: %+v", result)
	}
	var structured map[string]any
	decodeStructured(t, result, &structured)
	records, ok := structured["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("feed records=%#v", structured["records"])
	}
	record, ok := records[0].(map[string]any)
	if !ok || record["id"] != want {
		t.Fatalf("feed record=%#v, want id %q", records[0], want)
	}
}

func feedResultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return "<no text content>"
}
