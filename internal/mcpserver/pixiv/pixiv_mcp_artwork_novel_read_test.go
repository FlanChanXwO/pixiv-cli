package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArtworkNovelReadOutputSchemasMatchWireEnvelopes(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]map[string]any, len(tools))
	for _, tool := range tools {
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", tool.Name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s output schema: %v", tool.Name, err)
		}
		byName[tool.Name] = schema
	}

	for _, test := range []struct {
		name     string
		required []string
		fields   []string
	}{
		{name: "illust_detail", required: []string{"records"}, fields: []string{"records"}},
		{name: "novel_detail", required: []string{"records"}, fields: []string{"records"}},
		{name: "novel_series", required: []string{"series", "records", "pagination"}, fields: []string{"series", "records", "pagination"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, ok := byName[test.name]
			if !ok {
				t.Fatalf("tool %q is not registered", test.name)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s properties=%#v", test.name, schema["properties"])
			}
			gotFields := make([]string, 0, len(properties))
			for field := range properties {
				gotFields = append(gotFields, field)
			}
			slices.Sort(gotFields)
			wantFields := append([]string(nil), test.fields...)
			slices.Sort(wantFields)
			if !slices.Equal(gotFields, wantFields) {
				t.Fatalf("%s fields=%v, want %v", test.name, gotFields, wantFields)
			}

			gotRequired := make([]string, 0)
			for _, value := range schema["required"].([]any) {
				gotRequired = append(gotRequired, value.(string))
			}
			slices.Sort(gotRequired)
			wantRequired := append([]string(nil), test.required...)
			slices.Sort(wantRequired)
			if !slices.Equal(gotRequired, wantRequired) {
				t.Fatalf("%s required=%v, want %v", test.name, gotRequired, wantRequired)
			}
			if schema["additionalProperties"] != false {
				t.Fatalf("%s output schema=%#v, want a closed envelope", test.name, schema)
			}
		})
	}
}

func TestArtworkNovelReadInputSchemasMatchLegacyWireFields(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]map[string]any, len(tools))
	for _, tool := range tools {
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s input schema: %v", tool.Name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s input schema: %v", tool.Name, err)
		}
		byName[tool.Name] = schema
	}

	for _, test := range []struct {
		name     string
		required []string
		fields   []string
	}{
		{name: "search_illust", required: []string{"word"}, fields: []string{"word", "search_target", "sort", "duration", "start_date", "end_date", "page", "limit", "content_type", "ai_mode", "aspect_ratio", "resolution", "tool", "bookmark_min", "bookmark_max", "bookmark_strategy", "illust_filter"}},
		{name: "search_novel", required: []string{"word"}, fields: []string{"word", "search_target", "sort", "duration", "page", "limit", "novel_filter"}},
		{name: "illust_detail", fields: []string{"illust_id", "url"}},
		{name: "illust_related", required: []string{"illust_id"}, fields: []string{"illust_id", "illust_filter", "page", "limit"}},
		{name: "illust_series", required: []string{"series_id"}, fields: []string{"series_id", "page", "limit"}},
		{name: "novel_detail", required: []string{"novel_id"}, fields: []string{"novel_id"}},
		{name: "novel_content", required: []string{"novel_id"}, fields: []string{"novel_id"}},
		{name: "novel_series", required: []string{"series_id"}, fields: []string{"series_id", "page", "limit"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, ok := byName[test.name]
			if !ok {
				t.Fatalf("tool %q is not registered", test.name)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s properties=%#v", test.name, schema["properties"])
			}
			gotFields := make([]string, 0, len(properties))
			for field := range properties {
				gotFields = append(gotFields, field)
			}
			slices.Sort(gotFields)
			wantFields := append([]string(nil), test.fields...)
			slices.Sort(wantFields)
			if !slices.Equal(gotFields, wantFields) {
				t.Fatalf("%s fields=%v, want %v", test.name, gotFields, wantFields)
			}

			gotRequired := make([]string, 0)
			if rawRequired, ok := schema["required"].([]any); ok {
				for _, value := range rawRequired {
					gotRequired = append(gotRequired, value.(string))
				}
			}
			slices.Sort(gotRequired)
			wantRequired := append([]string(nil), test.required...)
			slices.Sort(wantRequired)
			if !slices.Equal(gotRequired, wantRequired) {
				t.Fatalf("%s required=%v, want %v", test.name, gotRequired, wantRequired)
			}
			if schema["additionalProperties"] != false {
				t.Fatalf("%s input schema=%#v, want a closed object", test.name, schema)
			}
		})
	}
}

func TestArtworkNovelReadLegacyJSONReplayPreservesStructuredContracts(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		client  func() *fakeSDKClient
		assert  func(*testing.T, *fakeSDKClient, *mcp.CallToolResult)
	}{
		{
			name:    "search_illust",
			fixture: `{"word":"cat"}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(101, "cat", 1)}}, nil
				}}
			},
			assert: assertArtworkNovelReplaySuccess("101"),
		},
		{
			name:    "search_novel",
			fixture: `{"word":"cat"}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{searchNovel: func(_ context.Context, _ pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
					return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 201, Title: "cat", User: pixiv.User{ID: 1}}}}, nil
				}}
			},
			assert: assertArtworkNovelReplaySuccess("201"),
		},
		{
			name:    "illust_detail",
			fixture: `{"illust_id":101}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{illustDetail: func(_ context.Context, id int64) (pixiv.Artwork, error) {
					return testSDKIllust(id, "detail", 1), nil
				}}
			},
			assert: assertArtworkNovelReplaySuccess("101"),
		},
		{
			name:    "illust_related",
			fixture: `{"illust_id":101}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{relatedArtworks: func(_ context.Context, request pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					if request.ArtworkID != 101 {
						return sdk.Page[pixiv.Artwork]{}, errors.New("unexpected related artwork ID")
					}
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(102, "related", 1)}}, nil
				}}
			},
			assert: assertArtworkNovelReplaySuccess("102"),
		},
		{
			name:    "illust_series",
			fixture: `{"series_id":301}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{artworkSeriesPage: sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(302, "series", 1)}}}
			},
			assert: assertArtworkNovelReplaySuccess("302"),
		},
		{
			name:    "novel_detail",
			fixture: `{"novel_id":201}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{novelDetailResult: pixiv.Novel{ID: 201, Title: "detail", User: pixiv.User{ID: 1}}}
			},
			assert: assertArtworkNovelReplaySuccess("201"),
		},
		{
			name:    "novel_content",
			fixture: `{"novel_id":201}`,
			client:  func() *fakeSDKClient { return &fakeSDKClient{} },
			assert: func(t *testing.T, client *fakeSDKClient, result *mcp.CallToolResult) {
				if !result.IsError || !resultHasText(result, "content_unavailable") {
					t.Fatalf("novel_content result=%+v", result)
				}
				if client.novelContentRequest.NovelID != 0 {
					t.Fatalf("rejected novel content endpoint received request=%+v", client.novelContentRequest)
				}
			},
		},
		{
			name:    "novel_series",
			fixture: `{"series_id":302}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{novelSeries: func(_ context.Context, request pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error) {
					if request.SeriesID != 302 {
						return pixiv.NovelSeriesResult{}, errors.New("unexpected novel series ID")
					}
					return pixiv.NovelSeriesResult{
						Series: pixiv.NovelSeries{ID: 302, Title: "series", User: pixiv.User{ID: 1}},
						Novels: sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 303, Title: "chapter", User: pixiv.User{ID: 1}}}},
					}, nil
				}}
			},
			assert: assertArtworkNovelReplaySuccess("303"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.client()
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			var fixture map[string]any
			if err := json.Unmarshal([]byte(test.fixture), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			result := callTool(t, session, test.name, fixture)
			test.assert(t, client, result)
		})
	}
}

func TestArtworkNovelReadSDKFailuresPreserveSafeStructuredEnvelopes(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		client  func() *fakeSDKClient
	}{
		{
			name:    "search_illust",
			fixture: `{"word":"cat"}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					return sdk.Page[pixiv.Artwork]{}, errors.New("search artwork failed")
				}}
			},
		},
		{
			name:    "search_novel",
			fixture: `{"word":"cat"}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{searchNovel: func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
					return sdk.Page[pixiv.Novel]{}, errors.New("search novel failed")
				}}
			},
		},
		{
			name:    "illust_detail",
			fixture: `{"illust_id":101}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{illustDetail: func(context.Context, int64) (pixiv.Artwork, error) {
					return pixiv.Artwork{}, errors.New("artwork detail failed")
				}}
			},
		},
		{
			name:    "illust_related",
			fixture: `{"illust_id":101}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{relatedArtworks: func(context.Context, pixiv.RelatedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
					return sdk.Page[pixiv.Artwork]{}, errors.New("related artwork failed")
				}}
			},
		},
		{
			name:    "illust_series",
			fixture: `{"series_id":301}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{artworkSeries: func(context.Context, pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error) {
					return sdk.Page[pixiv.Artwork]{}, errors.New("artwork series failed")
				}}
			},
		},
		{
			name:    "novel_detail",
			fixture: `{"novel_id":201}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{novelDetail: func(context.Context, int64) (pixiv.Novel, error) {
					return pixiv.Novel{}, errors.New("novel detail failed")
				}}
			},
		},
		{
			name:    "novel_series",
			fixture: `{"series_id":302}`,
			client: func() *fakeSDKClient {
				return &fakeSDKClient{novelSeries: func(context.Context, pixiv.NovelSeriesRequest) (pixiv.NovelSeriesResult, error) {
					return pixiv.NovelSeriesResult{}, errors.New("novel series failed")
				}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.client()
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			var fixture map[string]any
			if err := json.Unmarshal([]byte(test.fixture), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			result := callTool(t, session, test.name, fixture)
			if !result.IsError {
				t.Fatalf("failed %s call was not an MCP error: %+v", test.name, result)
			}
			var structured map[string]any
			decodeStructured(t, result, &structured)
			records, ok := structured["records"].([]any)
			if !ok || len(records) != 0 {
				t.Fatalf("failed %s structured output=%#v, want empty records", test.name, structured)
			}
		})
	}
}

func assertArtworkNovelReplaySuccess(wantID string) func(*testing.T, *fakeSDKClient, *mcp.CallToolResult) {
	return func(t *testing.T, _ *fakeSDKClient, result *mcp.CallToolResult) {
		t.Helper()
		if result.IsError {
			t.Fatalf("replayed call returned MCP error: %+v", result)
		}
		var structured map[string]any
		decodeStructured(t, result, &structured)
		records, ok := structured["records"].([]any)
		if !ok || len(records) != 1 {
			t.Fatalf("structured records=%#v", structured["records"])
		}
		record, ok := records[0].(map[string]any)
		if !ok || record["id"] != wantID {
			t.Fatalf("structured record=%#v, want id %q", records[0], wantID)
		}
	}
}

func TestIllustRelatedRejectsNonPositiveIDBeforeSDKExecution(t *testing.T) {
	executions := 0
	ports := pixivmcpserver.SDKPorts{
		Execute: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			executions++
			return errors.New("unexpected SDK execution")
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "illust_related", map[string]any{"illust_id": 0})
	if !result.IsError || !resultHasText(result, "illust_id must be a positive integer") {
		t.Fatalf("invalid illust_related input result=%+v", result)
	}
	if executions != 0 {
		t.Fatalf("invalid illust_related input entered SDK execution %d times", executions)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("invalid illust_related structured output=%+v", out)
	}
}
