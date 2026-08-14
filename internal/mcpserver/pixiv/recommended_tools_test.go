package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recommended 与 illust_recommended 的 owner 契约：多流聚合、单一 kind、分页与失败隔离。
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
