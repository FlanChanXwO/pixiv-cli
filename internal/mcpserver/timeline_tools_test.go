package mcpserver

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords 保留各时间线的专属
// App API 请求断言，并固定所有实体结果使用共享 records 契约。
func TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords(t *testing.T) {
	var followingNovel pixiv.FollowingNovelsRequest
	var latestIllust pixiv.LatestArtworksRequest
	var latestNovel pixiv.LatestNovelsRequest
	var myPixivUsers pixiv.MyPixivUsersRequest
	var myPixivIllusts pixiv.MyPixivArtworksRequest
	var myPixivNovels pixiv.MyPixivNovelsRequest
	var userNovels pixiv.UserNovelsRequest
	client := &fakeSDKClient{
		userID: 71,
		followingNovels: func(_ context.Context, request pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			followingNovel = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 1, Title: "follow", User: pixiv.User{Name: "writer"}}}, Next: testPageCursor(2)}, nil
		},
		latestIllusts: func(_ context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			latestIllust = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "latest", 8)}, Next: testPageCursor(3)}, nil
		},
		latestNovels: func(_ context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			latestNovel = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, Title: "latest novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
		myPixivUsers: func(_ context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			myPixivUsers = request
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4, Name: "friend"}}}}, nil
		},
		myPixivIllusts: func(_ context.Context, request pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			myPixivIllusts = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(5, "mypixiv", 4)}}, nil
		},
		myPixivNovels: func(_ context.Context, request pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			myPixivNovels = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 6, Title: "mypixiv novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
		userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			userNovels = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 7, Title: "user novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	assertRecords := func(name string, args map[string]any, wantID, wantType string) map[string]any {
		t.Helper()
		result := callTool(t, session, name, args)
		if result.IsError {
			t.Fatalf("%s=%+v", name, result)
		}
		structured := assertRecordsOnlyStructuredOutput(t, result)
		records := structured["records"].([]any)
		if len(records) != 1 {
			t.Fatalf("%s records=%#v", name, records)
		}
		record := records[0].(map[string]any)
		if record["id"] != wantID || record["type"] != wantType {
			t.Fatalf("%s record=%#v, want id=%s type=%s", name, record, wantID, wantType)
		}
		return structured
	}

	following := assertRecords("timeline_novel_following", map[string]any{"restrict": "private", "limit": 1}, "1", "novel")
	if followingNovel.Restrict != pixiv.RestrictPrivate || !paginationHasMore(t, following) {
		t.Fatalf("timeline_novel_following request=%+v structured=%#v", followingNovel, following)
	}

	illustNew := assertRecords("timeline_illust_latest", map[string]any{"content_type": "manga", "limit": 1}, "2", "illustration")
	if latestIllust.ContentType != pixiv.SearchContentTypeManga || !paginationHasMore(t, illustNew) {
		t.Fatalf("timeline_illust_latest request=%+v structured=%#v", latestIllust, illustNew)
	}

	assertRecords("timeline_novel_latest", map[string]any{}, "3", "novel")
	if !latestNovel.Cursor.IsZero() {
		t.Fatalf("timeline_novel_latest request=%+v", latestNovel)
	}

	assertRecords("mypixiv_users", map[string]any{}, "4", "user")
	if !myPixivUsers.Cursor.IsZero() {
		t.Fatalf("mypixiv_users request=%+v", myPixivUsers)
	}

	assertRecords("mypixiv_illusts", map[string]any{}, "5", "illustration")
	if !myPixivIllusts.Cursor.IsZero() {
		t.Fatalf("mypixiv_illusts request=%+v", myPixivIllusts)
	}

	assertRecords("mypixiv_novels", map[string]any{}, "6", "novel")
	if !myPixivNovels.Cursor.IsZero() {
		t.Fatalf("mypixiv_novels request=%+v", myPixivNovels)
	}

	assertRecords("user_novels", map[string]any{"user_id": 88}, "7", "novel")
	if userNovels.UserID != 88 {
		t.Fatalf("user_novels request=%+v", userNovels)
	}
}

func TestTimelineToolsValidateInputAndExposeSDKErrors(t *testing.T) {
	upstream := errors.New("latest upstream failed")
	client := &fakeSDKClient{latestIllusts: func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{}, upstream
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"timeline_illust_latest", map[string]any{"content_type": "ugoira"}},
		{"mypixiv_novels", map[string]any{"page": 0, "limit": 1}},
		{"timeline_illust_latest", map[string]any{"content_type": "illust"}},
	} {
		result := callTool(t, session, tool.name, tool.args)
		if !result.IsError {
			t.Fatalf("%s must return MCP error: %+v", tool.name, result)
		}
		structured := assertRecordsOnlyStructuredOutput(t, result)
		if len(structured["records"].([]any)) != 0 {
			t.Fatalf("%s error records=%#v", tool.name, structured["records"])
		}
	}
}

func paginationHasMore(t *testing.T, structured map[string]any) bool {
	t.Helper()
	pagination, ok := structured["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("pagination=%#v", structured["pagination"])
	}
	hasMore, _ := pagination["has_more"].(bool)
	return hasMore
}
