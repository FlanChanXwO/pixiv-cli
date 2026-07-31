package mcpserver

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords 保留各时间线的专属
// App API 请求断言，并固定所有实体结果使用共享 records 契约。
func TestTimelineAndMyPixivToolsRouteAppSDKRequestsWithRecords(t *testing.T) {
	var followingNovel sdk.FollowingNovelsRequest
	var latestIllust sdk.LatestIllustsRequest
	var latestNovel sdk.LatestNovelsRequest
	var myPixivUsers sdk.MyPixivUsersRequest
	var myPixivIllusts sdk.MyPixivIllustsRequest
	var myPixivNovels sdk.MyPixivNovelsRequest
	var userNovels sdk.UserNovelsRequest
	client := &fakeSDKClient{
		userID: 71,
		followingNovels: func(_ context.Context, request sdk.FollowingNovelsRequest) (*sdk.NovelListResult, error) {
			followingNovel = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 1, Title: "follow", User: sdk.User{Name: "writer"}}}, NextCursor: "follow-next"}, nil
		},
		latestIllusts: func(_ context.Context, request sdk.LatestIllustsRequest) (*sdk.IllustListResult, error) {
			latestIllust = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(2, "latest", 8)}, NextCursor: "illust-next"}, nil
		},
		latestNovels: func(_ context.Context, request sdk.LatestNovelsRequest) (*sdk.NovelListResult, error) {
			latestNovel = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 3, Title: "latest novel", User: sdk.User{Name: "writer"}}}}, nil
		},
		myPixivUsers: func(_ context.Context, request sdk.MyPixivUsersRequest) (*sdk.UserListResult, error) {
			myPixivUsers = request
			return &sdk.UserListResult{UserPreviews: []sdk.UserPreview{{User: sdk.User{ID: 4, Name: "friend"}}}}, nil
		},
		myPixivIllusts: func(_ context.Context, request sdk.MyPixivIllustsRequest) (*sdk.IllustListResult, error) {
			myPixivIllusts = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{testSDKIllust(5, "mypixiv", 4)}}, nil
		},
		myPixivNovels: func(_ context.Context, request sdk.MyPixivNovelsRequest) (*sdk.NovelListResult, error) {
			myPixivNovels = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 6, Title: "mypixiv novel", User: sdk.User{Name: "writer"}}}}, nil
		},
		userNovels: func(_ context.Context, request sdk.UserNovelsRequest) (*sdk.NovelListResult, error) {
			userNovels = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 7, Title: "user novel", User: sdk.User{Name: "writer"}}}}, nil
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
	if followingNovel.Restrict != sdk.RestrictPrivate || !paginationHasMore(t, following) {
		t.Fatalf("timeline_novel_following request=%+v structured=%#v", followingNovel, following)
	}

	illustNew := assertRecords("timeline_illust_latest", map[string]any{"content_type": "manga", "limit": 1}, "2", "illust")
	if latestIllust.Type != sdk.IllustTypeManga || !paginationHasMore(t, illustNew) {
		t.Fatalf("timeline_illust_latest request=%+v structured=%#v", latestIllust, illustNew)
	}

	assertRecords("timeline_novel_latest", map[string]any{}, "3", "novel")
	if latestNovel.Cursor != "" {
		t.Fatalf("timeline_novel_latest request=%+v", latestNovel)
	}

	assertRecords("mypixiv_users", map[string]any{}, "4", "user")
	if myPixivUsers.UserID != 71 {
		t.Fatalf("mypixiv_users request=%+v", myPixivUsers)
	}

	assertRecords("mypixiv_illusts", map[string]any{}, "5", "illust")
	if myPixivIllusts.Cursor != "" {
		t.Fatalf("mypixiv_illusts request=%+v", myPixivIllusts)
	}

	assertRecords("mypixiv_novels", map[string]any{}, "6", "novel")
	if myPixivNovels.Cursor != "" {
		t.Fatalf("mypixiv_novels request=%+v", myPixivNovels)
	}

	assertRecords("user_novels", map[string]any{"user_id": 88}, "7", "novel")
	if userNovels.UserID != 88 {
		t.Fatalf("user_novels request=%+v", userNovels)
	}
}

func TestTimelineToolsValidateInputAndExposeSDKErrors(t *testing.T) {
	upstream := errors.New("latest upstream failed")
	client := &fakeSDKClient{latestIllusts: func(context.Context, sdk.LatestIllustsRequest) (*sdk.IllustListResult, error) {
		return nil, upstream
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
