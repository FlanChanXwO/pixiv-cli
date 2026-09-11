package pixiv_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTypedBookmarkReadsRouteNovelOperations(t *testing.T) {
	client := &fakeSDKClient{
		novelBookmarks:        []pixiv.Novel{{ID: 101, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "story", Count: 3}}},
		novelBookmarkDetailResult: pixiv.NovelBookmarkDetail{
			Restrict: pixiv.RestrictPrivate,
			Tags:     []string{"story"},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	listResult := callTool(t, session, "user_novel_bookmarks", map[string]any{
		"user_id": 90, "restrict": "public", "tag": "story", "limit": 1,
	})
	if listResult.IsError || client.novelBookmarksRequest.UserID != 90 || client.novelBookmarksRequest.Restrict != pixiv.RestrictPublic || client.novelBookmarksRequest.Tag != "story" {
		t.Fatalf("novel bookmarks result=%+v request=%+v", listResult, client.novelBookmarksRequest)
	}
	var listOut outputs.Records
	decodeStructured(t, listResult, &listOut)
	if len(listOut.Records) != 1 || listOut.Records[0].ID() != "101" || listOut.Records[0].Type() != "novel" {
		t.Fatalf("novel bookmarks records=%+v", listOut.Records)
	}

	tagsResult := callTool(t, session, "novel_bookmark_tags", map[string]any{
		"user_id": 90, "restrict": "private", "limit": 1,
	})
	if tagsResult.IsError || client.novelBookmarkTagsRequest.UserID != 90 || client.novelBookmarkTagsRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("novel bookmark tags result=%+v request=%+v", tagsResult, client.novelBookmarkTagsRequest)
	}
	var tagsOut outputs.BookmarkTags
	decodeStructured(t, tagsResult, &tagsOut)
	if len(tagsOut.Tags) != 1 || tagsOut.Tags[0].Name != "story" || tagsOut.Tags[0].Count != 3 {
		t.Fatalf("novel bookmark tags=%+v", tagsOut)
	}

	detailResult := callTool(t, session, "novel_bookmark_detail", map[string]any{"novel_id": 101})
	if detailResult.IsError || client.novelBookmarkRequest.NovelID != 101 {
		t.Fatalf("novel bookmark detail result=%+v request=%+v", detailResult, client.novelBookmarkRequest)
	}
	var detailOut outputs.BookmarkDetail
	decodeStructured(t, detailResult, &detailOut)
	if !detailOut.Bookmarked || detailOut.Restrict != string(pixiv.RestrictPrivate) || !slices.Equal(detailOut.Tags, []string{"story"}) {
		t.Fatalf("novel bookmark detail=%+v", detailOut)
	}
}

func TestArtworkBookmarkSubtypeFiltersBeforeLogicalPagination(t *testing.T) {
	first := testSDKIllust(1, "not manga", 90)
	second := testSDKIllust(2, "first manga", 90)
	second.Kind = pixiv.ArtworkKindManga
	third := testSDKIllust(3, "second manga", 90)
	third.Kind = pixiv.ArtworkKindManga
	cursor := testPageCursor(4)
	client := &fakeSDKClient{
		userBookmarksFunc: func(request pixiv.UserArtworkBookmarksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
			if call == 1 {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first, second}, Next: cursor}, nil
			}
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{third}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id":       90,
		"restrict":      "private",
		"illust_filter": map[string]any{"type": "manga"},
		"page":          2,
		"limit":         1,
	})
	if result.IsError {
		t.Fatalf("filtered bookmark result=%+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "3" || out.Pagination.Page != 2 || out.Pagination.Returned != 1 || out.Pagination.HasMore {
		t.Fatalf("filtered bookmark output=%+v", out)
	}
	if len(client.bookmarksRequests) != 2 || client.bookmarksRequests[0].Restrict != pixiv.RestrictPrivate || client.bookmarksRequests[0].Cursor.IsZero() == false || client.bookmarksRequests[1].Cursor.IsZero() {
		t.Fatalf("filtered bookmark requests=%+v", client.bookmarksRequests)
	}
}

func TestTypedBookmarkReadsPreserveEmptyAndTypedErrors(t *testing.T) {
	client := &fakeSDKClient{
		novelBookmarks:       []pixiv.Novel{},
		novelBookmarkTagsErr: errors.New("novel bookmark tags upstream failed"),
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	emptyResult := callTool(t, session, "user_novel_bookmarks", map[string]any{"user_id": 90})
	var emptyOut outputs.Records
	decodeStructured(t, emptyResult, &emptyOut)
	if emptyResult.IsError || emptyOut.Records == nil || len(emptyOut.Records) != 0 {
		t.Fatalf("empty novel bookmarks result=%+v output=%+v", emptyResult, emptyOut)
	}

	errorResult := callTool(t, session, "novel_bookmark_tags", map[string]any{"user_id": 90})
	var errorOut outputs.BookmarkTags
	decodeStructured(t, errorResult, &errorOut)
	if !errorResult.IsError || errorOut.Tags == nil || len(errorOut.Tags) != 0 || !resultHasText(errorResult, "UserNovelBookmarkTags") || !resultHasText(errorResult, "upstream_error") {
		t.Fatalf("novel bookmark tags error result=%+v output=%+v", errorResult, errorOut)
	}

	invalidResult := callTool(t, session, "novel_bookmark_detail", map[string]any{"novel_id": 0})
	var invalidOut outputs.BookmarkDetail
	decodeStructured(t, invalidResult, &invalidOut)
	if !invalidResult.IsError || !resultHasText(invalidResult, "novel_id must be a positive integer") || client.novelBookmarkRequest.NovelID != 0 {
		t.Fatalf("invalid novel bookmark detail result=%+v output=%+v request=%+v", invalidResult, invalidOut, client.novelBookmarkRequest)
	}
}

func TestTypedBookmarkSchemasKeepLegacyFieldsClosed(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
	defer closeSession()
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"user_novel_bookmarks", map[string]any{"user_id_to_check": 9}},
		{"user_novel_bookmarks", map[string]any{"user_id": 9, "max_bookmark_id": 1}},
		{"novel_bookmark_tags", map[string]any{"user_id": 9, "offset": 1}},
		{"novel_bookmark_detail", map[string]any{"illust_id": 9}},
		{"bookmark_detail", map[string]any{"novel_id": 9}},
	} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err == nil || !strings.Contains(err.Error(), "additional properties") {
			t.Fatalf("%s args=%v err=%v", call.name, call.args, err)
		}
	}
}
