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

func TestSDKUserBookmarksFailureReturnsMCPErrorWithStructuredOutput(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{
		userBookmarksErr: errors.New("bookmarks upstream failed"),
	})
	defer closeSession()

	result := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 9})
	if !result.IsError {
		t.Fatalf("SDK failure must be an MCP error result: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !resultHasText(result, "UserArtworkBookmarks") || !resultHasText(result, "upstream_error") {
		t.Fatalf("structured SDK error = %+v", out)
	}
}

func TestSDKUserToolsResolveIdentityAndReturnStructuredOutput(t *testing.T) {
	client := &fakeSDKClient{
		userID:    71,
		artworks:  []pixiv.Artwork{testSDKIllust(11, "work", 71)},
		bookmarks: []pixiv.Artwork{testSDKIllust(12, "saved", 99)},
		following: []pixiv.UserPreview{{User: pixiv.User{ID: 33, Name: "followed", Account: "f"}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	artworks := callTool(t, session, "user_artworks", map[string]any{"limit": 1})
	var artworksOut outputs.Records
	decodeStructured(t, artworks, &artworksOut)
	if client.artworksRequest.UserID != 71 || len(artworksOut.Records) != 1 || artworksOut.Records[0].ID() != "11" || artworksOut.Pagination.Returned != 1 {
		t.Fatalf("user artworks = request=%+v output=%+v", client.artworksRequest, artworksOut)
	}

	bookmarks := callTool(t, session, "user_bookmarks", map[string]any{"user_id": 99, "tag": "tag", "limit": 0})
	var bookmarksOut outputs.Records
	decodeStructured(t, bookmarks, &bookmarksOut)
	if client.bookmarksRequest.UserID != 99 || client.bookmarksRequest.Tag != "tag" || len(bookmarksOut.Records) != 1 || bookmarksOut.Records[0].ID() != "12" || bookmarksOut.Pagination.HasMore {
		t.Fatalf("bookmarks = request=%+v output=%+v", client.bookmarksRequest, bookmarksOut)
	}

	following := callTool(t, session, "user_following", map[string]any{"user_id": 99, "limit": 1})
	var followingOut outputs.Records
	decodeStructured(t, following, &followingOut)
	if client.followingRequest.UserID != 99 || len(followingOut.Records) != 1 || followingOut.Records[0].ID() != "33" {
		t.Fatalf("following = request=%+v output=%+v", client.followingRequest, followingOut)
	}
}

func TestUserArtworksPreservesArtworkRecordsAtMCPBoundary(t *testing.T) {
	withNilTools := testSDKIllust(11, "without-tools", 71)
	withTools := testSDKIllust(12, "with-tools", 71)
	withTools.Tools = []string{"CLIP STUDIO PAINT", "Photoshop"}
	client := &fakeSDKClient{userID: 71, artworks: []pixiv.Artwork{withNilTools, withTools}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 71})
	if result.IsError {
		t.Fatalf("user_artworks result=%+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 2 || out.Records[0].ID() != "11" || out.Records[1].ID() != "12" {
		t.Fatalf("artwork records=%+v", out)
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

func TestUserArtworksReturnsTaggedArtworkRecord(t *testing.T) {
	illust := testSDKIllust(15, "tagged", 9)
	illust.Tags = []pixiv.Tag{
		{Name: "tag-1"}, {Name: "tag-2"}, {Name: "tag-3"}, {Name: "tag-4"},
		{Name: "tag-5"}, {Name: "tag-6"}, {Name: "tag-7"},
	}
	client := &fakeSDKClient{artworks: []pixiv.Artwork{illust}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_artworks", map[string]any{"user_id": 9})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("user_artworks returned MCP error: %+v", result)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "15" {
		t.Fatalf("user_artworks records=%+v", out)
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

func TestSDKListPageRequiresPositiveLimitAndPositiveValue(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userID: 1})
	defer closeSession()
	for _, arguments := range []map[string]any{
		{"page": 0, "limit": 1},
		{"page": 1},
	} {
		result := callTool(t, session, "user_artworks", arguments)
		var out outputs.Records
		decodeStructured(t, result, &out)
		if !resultHasText(result, "page") {
			t.Fatalf("arguments=%v output=%+v", arguments, out)
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

func TestUserNovelBookmarksMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		novelBookmarksPage: sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 31, Title: "bookmark-novel", User: pixiv.User{ID: 6}}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_novel_bookmarks", map[string]any{"user_id": 91, "restrict": "public", "tag": "blue"})
	if result.IsError || client.novelBookmarksRequest.UserID != 91 || client.novelBookmarksRequest.Tag != "blue" {
		t.Fatalf("novel bookmarks result=%+v request=%+v", result, client.novelBookmarksRequest)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "31" {
		t.Fatalf("novel bookmarks=%+v", out)
	}
}

func TestUserFollowersMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		followersPage: sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 41, Name: "follower"}}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "user_followers", map[string]any{"user_id": 92, "restrict": "private"})
	if result.IsError || client.followersRequest.UserID != 92 || client.followersRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("followers result=%+v request=%+v", result, client.followersRequest)
	}
}

func TestRelatedUsersMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		relatedPage: sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 42, Name: "related"}}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "related_users", map[string]any{"user_id": 93})
	if result.IsError || client.relatedRequest.UserID != 93 {
		t.Fatalf("related result=%+v request=%+v", result, client.relatedRequest)
	}
}

func TestBlockedUsersMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		blockedPage: sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 43, Name: "blocked"}}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "blocked_users", map[string]any{"user_id": 94})
	if result.IsError || client.blockedRequest.UserID != 94 {
		t.Fatalf("blocked result=%+v request=%+v", result, client.blockedRequest)
	}
}

func TestUserDetailUsesApplicationPooledService(t *testing.T) {
	var pooled bool
	client := openWireClient(t, &fakeSDKClient{userID: 42})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
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
