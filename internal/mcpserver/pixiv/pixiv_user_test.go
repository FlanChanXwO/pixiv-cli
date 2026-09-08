package pixiv_test

import (
	"context"
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
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 1, Title: "follow", User: pixiv.User{ID: 10, Name: "writer"}}}, Next: testPageCursor(2)}, nil
		},
		latestIllusts: func(_ context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			latestIllust = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "latest", 8)}, Next: testPageCursor(3)}, nil
		},
		latestNovels: func(_ context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			latestNovel = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 3, Title: "latest novel", User: pixiv.User{ID: 10, Name: "writer"}}}}, nil
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
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 6, Title: "mypixiv novel", User: pixiv.User{ID: 10, Name: "writer"}}}}, nil
		},
		userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			userNovels = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 7, Title: "user novel", User: pixiv.User{ID: 10, Name: "writer"}}}}, nil
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

	illustNew := assertRecords("timeline_illust_latest", map[string]any{"content_type": "manga", "limit": 1}, "2", "illust")
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

	assertRecords("mypixiv_illusts", map[string]any{}, "5", "illust")
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
func TestTrendingTagsIllustReturnsTagsAndText(t *testing.T) {
	client := &fakeSDKClient{trendingTags: []pixiv.TrendingTag{
		{Tag: "miku", TranslatedName: "Hatsune Miku", Artwork: testSDKIllust(101, "miku-art", 1)},
		{Tag: "frieren", TranslatedName: "", Artwork: testSDKIllust(102, "frieren-art", 2)},
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "trending_tags_illust", map[string]any{})
	if result.IsError {
		t.Fatalf("trending_tags_illust returned error: %+v", result)
	}
	var out outputs.TrendingTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 2 || out.Tags[0].Tag != "miku" || out.Tags[0].TranslatedName != "Hatsune Miku" {
		t.Fatalf("trending tags=%+v", out.Tags)
	}
	if !resultHasText(result, "Trending tags:") || !resultHasText(result, "- miku (translation: Hatsune Miku)") || !resultHasText(result, "- frieren (translation: none)") {
		t.Fatalf("trending text=%+v", result.Content)
	}
}

func TestTrendingTagsIllustSDKErrorIsStructured(t *testing.T) {
	client := openWireClient(t, &fakeSDKClient{trendingTags: []pixiv.TrendingTag{{Tag: "never", TranslatedName: ""}}})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Execute: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return errors.New("trending pool failure")
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "trending_tags_illust", map[string]any{})
	if !result.IsError || !resultHasText(result, "Error: trending pool failure") {
		t.Fatalf("trending SDK failure result=%+v", result)
	}
	var out outputs.TrendingTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 0 {
		t.Fatalf("structured error must carry empty tags: %+v", out)
	}
}

// 用户类 tool 的 owner 契约：身份解析、列表分页/游标、schema 与错误形状。
func TestSDKListValidationReturnsMCPErrorWithStructuredOutput(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{})
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 9, "page": 0, "limit": 1})
	if !result.IsError {
		t.Fatalf("invalid page must be an MCP error result: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("error result must retain text content: %+v", result.Content)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "page must be a positive integer") {
		t.Fatalf("structured validation error = %+v", out)
	}
}

func TestSDKUserDetailReturnsStructuredSDKResult(t *testing.T) {
	webpage := "https://example.test/artist"
	workspaceImage := "https://example.test/workspace.png"
	want := pixiv.UserDetail{
		User:             pixiv.User{ID: 42, Name: "artist", Account: "artist_account", Comment: "hello"},
		Profile:          pixiv.UserProfile{Webpage: webpage, Region: "Tokyo", CountryCode: "JP", Job: "illustrator", TotalIllusts: 10, TotalManga: 2, TotalNovels: 3, TotalFollowUsers: 4},
		ProfilePublicity: pixiv.UserProfilePublicity{Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true},
		Workspace:        pixiv.UserWorkspace{PC: "desktop", Tool: "pen", WorkspaceImageURL: workspaceImage},
	}
	client := &fakeSDKClient{userDetailResult: want}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if result.IsError {
		t.Fatalf("user_detail returned error: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("user_detail text content = %+v", result.Content)
	}
	var out outputs.UserDetail
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "42" || client.userDetailRequest != (pixiv.UserRequest{UserID: 42}) {
		t.Fatalf("user_detail output=%+v request=%+v", out, client.userDetailRequest)
	}
}

func TestSDKUserDetailRejectsInvalidInputAndReturnsSDKFailuresAsMCPError(t *testing.T) {
	for _, input := range []map[string]any{{"user_id": 0}, {"user_id": -1}} {
		client := &fakeSDKClient{}
		session, closeSession := newSDKTestSession(t, client)
		result := callTool(t, session, "user_detail", input)
		closeSession()
		if !result.IsError || client.userDetailRequest != (pixiv.UserRequest{}) {
			t.Fatalf("input=%v result=%+v captured=%+v", input, result, client.userDetailRequest)
		}
	}
	for _, input := range []map[string]any{{}, {"user_id": "not-an-integer"}} {
		client := &fakeSDKClient{}
		session, closeSession := newSDKTestSession(t, client)
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "user_detail", Arguments: input})
		closeSession()
		if err == nil || client.userDetailRequest != (pixiv.UserRequest{}) {
			t.Fatalf("input=%v error=%v captured=%+v", input, err, client.userDetailRequest)
		}
	}

	typed := &sdk.Error{Product: "pixiv", Operation: "User", Reason: sdk.MalformedUpstreamResponse}
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userDetailErr: typed})
	defer closeSession()
	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("typed SDK failure result=%+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, typed.Error()) {
		t.Fatalf("typed SDK failure content=%+v", result.Content)
	}
	var typedOut outputs.UserDetail
	decodeStructured(t, result, &typedOut)
	if len(typedOut.Records) != 0 {
		t.Fatalf("typed SDK failure structured output=%+v", typedOut)
	}

	noSDKSession, closeNoSDKSession := newTestSession(t, &fakeDownloads{})
	defer closeNoSDKSession()
	result = callTool(t, noSDKSession, "user_detail", map[string]any{"user_id": 42})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("unconfigured SDK result=%+v", result)
	}
	text, ok = result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "sdk pooled operation is not configured") {
		t.Fatalf("unconfigured SDK content=%+v", result.Content)
	}
	var unconfiguredOut outputs.UserDetail
	decodeStructured(t, result, &unconfiguredOut)
	if len(unconfiguredOut.Records) != 0 {
		t.Fatalf("unconfigured SDK structured output=%+v", unconfiguredOut)
	}
}

func TestSDKUserListToolsUseCanonicalUserIDAndFilters(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks: []pixiv.Artwork{testSDKIllust(15, "saved", 9)},
		following: []pixiv.UserPreview{
			{User: pixiv.User{ID: 30, Name: "first"}},
			{User: pixiv.User{ID: 31, Name: "second"}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	bookmarkResult := callTool(t, session, "user_bookmarks", map[string]any{
		"user_id": 9, "restrict": "private", "tag": "tag-a", "limit": 1,
	})
	var bookmarksOut outputs.Records
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	if client.bookmarksRequest.UserID != 9 || client.bookmarksRequest.Restrict != pixiv.RestrictPrivate || client.bookmarksRequest.Tag != "tag-a" || !client.bookmarksRequest.Cursor.IsZero() || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "15" {
		t.Fatalf("bookmarks request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	followingResult := callTool(t, session, "user_following", map[string]any{
		"user_id": 8, "restrict": "private", "page": 2, "limit": 1,
	})
	var followingOut outputs.Records
	decodeStructured(t, followingResult, &followingOut)
	if client.followingRequest.UserID != 8 || client.followingRequest.Restrict != pixiv.RestrictPrivate || len(followingOut.Records) != 1 || followingOut.Records[0].ID() != "31" {
		t.Fatalf("following page request=%+v output=%+v", client.followingRequest, followingOut)
	}

	client.bookmarks = []pixiv.Artwork{}
	client.following = []pixiv.UserPreview{}
	bookmarkResult = callTool(t, session, "user_bookmarks", map[string]any{"user_id": 9})
	decodeStructured(t, bookmarkResult, &bookmarksOut)
	followingResult = callTool(t, session, "user_following", map[string]any{"user_id": 8})
	decodeStructured(t, followingResult, &followingOut)
	if len(bookmarksOut.Records) != 0 || len(followingOut.Records) != 0 {
		t.Fatalf("empty records bookmarks=%+v following=%+v", bookmarksOut, followingOut)
	}
}

func TestSDKUserListToolsSchemaRejectsRemovedLegacyFields(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
	defer closeSession()
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"user_bookmarks", map[string]any{"user_id_to_check": 9}},
		{"user_bookmarks", map[string]any{"user_id": 9, "max_bookmark_id": 1}},
		{"user_following", map[string]any{"user_id_to_check": 8}},
		{"user_following", map[string]any{"user_id": 8, "offset": 1}},
	} {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err == nil || !strings.Contains(err.Error(), "additional properties") {
			t.Fatalf("%s args=%v err=%v", call.name, call.args, err)
		}
	}
}

func TestSDKListsFollowOpaqueCursorForLimitAndRejectCycles(t *testing.T) {
	first := testSDKIllust(1, "first", 7)
	second := testSDKIllust(2, "second", 7)
	legacyDefault := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	defaultSession, closeDefaultSession := newSDKTestSession(t, legacyDefault)
	defer closeDefaultSession()
	result := callTool(t, defaultSession, "user_artworks", map[string]any{})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || !out.Pagination.HasMore || out.Pagination.Limit != nil || out.Pagination.NextPage != nil || len(legacyDefault.artworksRequests) != 1 {
		t.Fatalf("default single-batch output=%+v requests=%+v", out, legacyDefault.artworksRequests)
	}

	paged := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	pagedSession, closePagedSession := newSDKTestSession(t, paged)
	defer closePagedSession()
	result = callTool(t, pagedSession, "user_artworks", map[string]any{"limit": 1})
	decodeStructured(t, result, &out)
	if !out.Pagination.HasMore || out.Pagination.NextPage == nil || *out.Pagination.NextPage != 2 {
		t.Fatalf("single-page pagination=%+v", out.Pagination)
	}

	client := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: testPageCursor(1)}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{second}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()
	result = callTool(t, session, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if len(out.Records) != 2 || out.Pagination.HasMore || len(client.artworksRequests) != 2 || client.artworksRequests[1].Cursor.IsZero() {
		t.Fatalf("all-pages output=%+v requests=%+v", out, client.artworksRequests)
	}

	cycleCursor := testPageCursor(9)
	cyclic := &fakeSDKClient{userID: 7, userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
		if request.Cursor.IsZero() {
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{first}, Next: cycleCursor}, nil
		}
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{}, Next: cycleCursor}, nil
	}}
	cycleSession, closeCycleSession := newSDKTestSession(t, cyclic)
	defer closeCycleSession()
	result = callTool(t, cycleSession, "user_artworks", map[string]any{"limit": 0})
	decodeStructured(t, result, &out)
	if !resultHasText(result, "cursor repeated") || len(cyclic.artworksRequests) != 2 {
		t.Fatalf("cycle output=%+v requests=%+v", out, cyclic.artworksRequests)
	}
}

func TestBookmarkTagsMapsRequestAndReturnsTags(t *testing.T) {
	client := &fakeSDKClient{
		bookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "blue", Count: 4}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "bookmark_tags", map[string]any{"user_id": 90, "restrict": "private"})
	if result.IsError || client.bookmarkTagsRequest.UserID != 90 || client.bookmarkTagsRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("bookmark tags result=%+v request=%+v", result, client.bookmarkTagsRequest)
	}
	var out outputs.BookmarkTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 1 || out.Tags[0].Name != "blue" {
		t.Fatalf("bookmark tags=%+v", out)
	}
}

func TestBookmarkDetailMapsRequestAndReturnsState(t *testing.T) {
	client := &fakeSDKClient{
		bookmarkDetailResult: pixiv.ArtworkBookmarkDetail{Restrict: pixiv.RestrictPrivate, Tags: []string{"blue"}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "bookmark_detail", map[string]any{"illust_id": 11})
	if result.IsError || client.bookmarkDetailRequest.ArtworkID != 11 {
		t.Fatalf("bookmark detail result=%+v request=%+v", result, client.bookmarkDetailRequest)
	}
	var out outputs.BookmarkDetail
	decodeStructured(t, result, &out)
	if !out.Bookmarked || out.Restrict != string(pixiv.RestrictPrivate) || !slices.Equal(out.Tags, []string{"blue"}) {
		t.Fatalf("bookmark detail=%+v", out)
	}
}

func TestUserDetailUsesApplicationExecuteService(t *testing.T) {
	var pooled bool
	client := openWireClient(t, &fakeSDKClient{userID: 42})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Execute: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			return errors.New("application pool failure")
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if !pooled || !result.IsError || !resultHasText(result, "application pool failure") {
		t.Fatalf("pooled=%v result=%+v", pooled, result)
	}
}
