package pixiv

import (
	"context"
	"slices"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func TestTopLevelFilterExpressionIsIgnored(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		first := filteredTestIllust(1, 100, 1, "keep")
		first.TotalBookmarks = 1
		second := filteredTestIllust(2, 100, 1, "other")
		second.TotalBookmarks = 2
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first, second}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result := callTool(t, session, "search_illust", map[string]any{
		"word": "cat", "filter": "bookmarkCount >= 2", "illust_filter": map[string]any{"tags": []string{"keep"}},
	})
	if result.IsError {
		t.Fatalf("search_illust result=%+v", result)
	}
	var out illustQueryOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "1" {
		t.Fatalf("top-level expression must be ignored and structured filter applied: %+v", out.Records)
	}
}

func filteredTestIllust(id int64, views, pages int, tag string) pixiv.Artwork {
	illust := testSDKIllust(id, "work", 7)
	illust.TotalViews = views
	illust.PageCount = pages
	illust.Tags = []pixiv.Tag{{Name: tag}}
	return illust
}
