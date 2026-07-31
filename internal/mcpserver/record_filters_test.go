package mcpserver

import (
	"context"
	"slices"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSearchIllustFilterFillsLogicalLimitAndDeduplicatesAcrossPages(t *testing.T) {
	requests := make([]sdk.SearchIllustRequest, 0, 2)
	client := &fakeSDKClient{searchIllust: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				filteredTestIllust(1, 1, 1, "other"),
				filteredTestIllust(2, 10, 2, "keep"),
			}, NextCursor: "second"}, nil
		case "second":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				filteredTestIllust(2, 10, 2, "keep"),
				filteredTestIllust(3, 20, 3, "keep"),
			}}, nil
		default:
			t.Fatalf("unexpected cursor %q", request.Cursor)
			return nil, nil
		}
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
	var out illustQueryOut
	decodeStructured(t, result, &out)
	got := make([]string, 0, len(out.Records))
	for _, record := range out.Records {
		got = append(got, record.ID())
	}
	if !slices.Equal(got, []string{"2", "3"}) || out.Pagination.Returned != 2 || len(requests) != 2 {
		t.Fatalf("records=%v pagination=%+v requests=%+v", got, out.Pagination, requests)
	}
}

func TestMCPRecordFilterSchemasAreEntitySpecific(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"search novel rejects illust filter", map[string]any{"word": "novel", "illust_filter": map[string]any{"id": 1}}},
		{"timeline novel rejects illust filter", map[string]any{"illust_filter": map[string]any{"id": 1}}},
		{"timeline illust rejects novel filter", map[string]any{"content_type": "illust", "novel_filter": map[string]any{"id": 1}}},
		{"search illust rejects unknown filter member", map[string]any{"word": "cat", "illust_filter": map[string]any{"unknown": 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := "search_novel"
			if test.name == "timeline novel rejects illust filter" {
				tool = "timeline_novel_latest"
			}
			if test.name == "timeline illust rejects novel filter" {
				tool = "timeline_illust_latest"
			}
			if test.name == "search illust rejects unknown filter member" {
				tool = "search_illust"
			}
			_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: test.args})
			if err == nil {
				t.Fatalf("%s accepted args=%v", tool, test.args)
			}
		})
	}
}

func filteredTestIllust(id int64, views, pages int, tag string) sdk.Illust {
	illust := testSDKIllust(id, "work", 7)
	illust.TotalView = views
	illust.PageCount = pages
	illust.Tags = []sdk.Tag{{Name: tag}}
	return illust
}
