package pixiv_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUserReadSchemasMatchLegacyContracts(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]any, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	for _, test := range []struct {
		name     string
		required []string
		fields   []string
	}{
		{name: "search_user", required: []string{"word"}, fields: []string{"word", "user_filter", "page", "limit"}},
		{name: "mypixiv_users", fields: []string{"user_filter", "page", "limit"}},
		{name: "mypixiv_illusts", fields: []string{"illust_filter", "page", "limit"}},
		{name: "mypixiv_novels", fields: []string{"novel_filter", "page", "limit"}},
		{name: "user_detail", required: []string{"user_id"}, fields: []string{"user_id"}},
		{name: "user_artworks", fields: []string{"user_id", "type", "illust_filter", "page", "limit"}},
		{name: "user_novels", fields: []string{"user_id", "novel_filter", "page", "limit"}},
		{name: "user_following", fields: []string{"user_id", "restrict", "user_filter", "page", "limit"}},
		{name: "user_followers", fields: []string{"user_id", "restrict", "page", "limit"}},
		{name: "related_users", fields: []string{"user_id", "restrict", "user_filter", "page", "limit"}},
		{name: "blocked_users", fields: []string{"user_id", "restrict", "page", "limit"}},
	} {
		t.Run(test.name+"/input", func(t *testing.T) {
			tool, ok := byName[test.name].(*mcp.Tool)
			if !ok {
				t.Fatalf("tool %q is not registered", test.name)
			}
			schema := feedSchemaObject(t, test.name+" input", tool.InputSchema)
			assertSchemaFields(t, schema, test.fields)
			assertSchemaRequired(t, schema, test.required)
			if test.name != "user_detail" {
				assertSchemaMinimum(t, feedSchemaProperty(t, schema, "page"), 1)
				assertSchemaMinimum(t, feedSchemaProperty(t, schema, "limit"), 0)
			}
			if test.name == "search_user" {
				property := feedSchemaProperty(t, schema, "word")
				minLength, ok := property["minLength"].(float64)
				if property["type"] != "string" || !ok || minLength != 1 {
					t.Fatalf("search_user word schema=%#v", property)
				}
			}
			if test.name == "user_detail" {
				assertSchemaMinimum(t, feedSchemaProperty(t, schema, "user_id"), 1)
			}
			for _, name := range []string{"user_artworks", "user_novels", "user_following", "user_followers", "related_users", "blocked_users"} {
				if test.name == name {
					assertSchemaMinimum(t, feedSchemaProperty(t, schema, "user_id"), 1)
				}
			}
			if test.name == "user_artworks" {
				assertSchemaEnum(t, feedSchemaProperty(t, schema, "type"), []string{"illust", "manga", "ugoira"})
			}
			for _, name := range []string{"user_following", "user_followers", "related_users", "blocked_users"} {
				if test.name == name {
					assertSchemaEnum(t, feedSchemaProperty(t, schema, "restrict"), []string{"public", "private"})
				}
			}
		})
	}

	tool, ok := byName["user_detail"].(*mcp.Tool)
	if !ok {
		t.Fatal("user_detail is not registered")
	}
	output := feedSchemaObject(t, "user_detail output", tool.OutputSchema)
	assertSchemaFields(t, output, []string{"records"})
	assertSchemaRequired(t, output, []string{"records"})
}

func TestSearchUserRejectsBlankWordBeforeSDKExecution(t *testing.T) {
	executions := 0
	client := openWireClient(t, &fakeSDKClient{})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Execute: func(ctx context.Context, _ pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			executions++
			_, err := attempt(ctx, client)
			return err
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "search_user", map[string]any{"word": " \t"})
	if !result.IsError || executions != 0 || !resultHasText(result, "search word is required") {
		t.Fatalf("blank search_user result=%+v executions=%d", result, executions)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("blank search_user records=%+v", out.Records)
	}
}

func TestSearchUserSDKFailureRemainsStructured(t *testing.T) {
	typed := sdk.NewError("pixiv", "SearchUsers", sdk.Forbidden)
	calls := 0
	client := &fakeSDKClient{searchUser: func(context.Context, pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		calls++
		return sdk.Page[pixiv.UserPreview]{}, typed
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_user", map[string]any{"word": "miku"})
	if !result.IsError || calls != 1 || !resultHasText(result, typed.Error()) {
		t.Fatalf("search_user failure result=%+v calls=%d", result, calls)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || out.Pagination.Page != 1 {
		t.Fatalf("search_user failure output=%+v", out)
	}
}

func TestUserDetailSDKFailureRemainsStructured(t *testing.T) {
	typed := sdk.NewError("pixiv", "User", sdk.Forbidden)
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{userDetailErr: typed})
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 401})
	if !result.IsError || !resultHasText(result, typed.Error()) {
		t.Fatalf("user_detail failure result=%+v", result)
	}
	var out outputs.UserDetail
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("user_detail failure records=%+v", out.Records)
	}
}

func TestRelatedUsersResolvesCurrentIdentityAndFiltersBeforeLogicalPagination(t *testing.T) {
	client := &fakeSDKClient{
		userID: 71,
		relatedUsers: func(_ context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			if request.UserID != 71 {
				t.Fatalf("related user request=%+v, want current user 71", request)
			}
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{
				{User: pixiv.User{ID: 70, Name: "other"}},
				{User: pixiv.User{ID: 71, Name: "current"}},
			}}, nil
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "related_users", map[string]any{
		"user_filter": map[string]any{"id": 71},
		"limit":       1,
	})
	if result.IsError {
		t.Fatalf("related_users returned error: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "71" {
		t.Fatalf("related_users output=%+v", out)
	}
	if out.Pagination.Returned != 1 || out.Pagination.HasMore {
		t.Fatalf("related_users pagination=%+v", out.Pagination)
	}

	if client.relatedUsersRequest.UserID != 71 {
		t.Fatalf("related_users request=%+v", client.relatedUsersRequest)
	}
}

func TestUserReadLegacyJSONReplayPreservesStructuredContracts(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		client   *fakeSDKClient
		wantID   string
		checkReq func(*testing.T, *fakeSDKClient)
	}{
		{
			name:    "search_user",
			fixture: `{"word":"miku"}`,
			client: &fakeSDKClient{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Word != "miku" {
					return sdk.Page[pixiv.UserPreview]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 101, Name: "miku"}}}}, nil
			}},
			wantID: "101",
		},
		{
			name:    "user_detail",
			fixture: `{"user_id":401}`,
			client:  &fakeSDKClient{userDetailResult: pixiv.UserDetail{User: pixiv.User{ID: 401, Name: "artist"}}},
			wantID:  "401",
			checkReq: func(t *testing.T, client *fakeSDKClient) {
				if client.userDetailRequest.UserID != 401 {
					t.Fatalf("user_detail request=%+v", client.userDetailRequest)
				}
			},
		},
		{
			name:    "user_artworks",
			fixture: `{}`,
			client:  &fakeSDKClient{userID: 401, artworks: []pixiv.Artwork{testSDKIllust(501, "artwork", 401)}},
			wantID:  "501",
			checkReq: func(t *testing.T, client *fakeSDKClient) {
				if client.artworksRequest.UserID != 401 {
					t.Fatalf("user_artworks request=%+v", client.artworksRequest)
				}
			},
		},
		{
			name:    "user_novels",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				if request.UserID != 401 {
					return sdk.Page[pixiv.Novel]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 601, Title: "novel", User: pixiv.User{ID: 401}}}}, nil
			}},
			wantID: "601",
		},
		{
			name:    "mypixiv_users",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, myPixivUsers: func(_ context.Context, _ pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 701, Name: "friend"}}}}, nil
			}},
			wantID: "701",
		},
		{
			name:    "mypixiv_illusts",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, myPixivIllusts: func(_ context.Context, _ pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(801, "friend artwork", 401)}}, nil
			}},
			wantID: "801",
		},
		{
			name:    "mypixiv_novels",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, myPixivNovels: func(_ context.Context, _ pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 901, Title: "friend novel", User: pixiv.User{ID: 401}}}}, nil
			}},
			wantID: "901",
		},
		{
			name:    "user_following",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, userFollowing: func(_ context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.UserID != 401 || request.Restrict != pixiv.RestrictPublic {
					return sdk.Page[pixiv.UserPreview]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1001, Name: "following"}}}}, nil
			}},
			wantID: "1001",
		},
		{
			name:    "user_followers",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, userFollowers: func(_ context.Context, request pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.UserID != 401 || request.Restrict != pixiv.RestrictPublic {
					return sdk.Page[pixiv.UserPreview]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1101, Name: "follower"}}}}, nil
			}},
			wantID: "1101",
		},
		{
			name:    "related_users",
			fixture: `{"user_id":401}`,
			client: &fakeSDKClient{relatedUsers: func(_ context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.UserID != 401 {
					return sdk.Page[pixiv.UserPreview]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1201, Name: "related"}}}}, nil
			}},
			wantID: "1201",
		},
		{
			name:    "blocked_users",
			fixture: `{}`,
			client: &fakeSDKClient{userID: 401, userBlockedUsers: func(_ context.Context, request pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.UserID != 401 {
					return sdk.Page[pixiv.UserPreview]{}, errUnexpectedRequest
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1301, Name: "blocked"}}}}, nil
			}},
			wantID: "1301",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal([]byte(test.fixture), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result := callTool(t, session, test.name, fixture)
			if result.IsError {
				t.Fatalf("%s replay result=%+v", test.name, result)
			}
			if test.name == "user_detail" {
				var out outputs.UserDetail
				decodeStructured(t, result, &out)
				if len(out.Records) != 1 || out.Records[0].ID() != test.wantID {
					t.Fatalf("%s output=%+v", test.name, out)
				}
			} else {
				var out outputs.Records
				decodeStructured(t, result, &out)
				if len(out.Records) != 1 || out.Records[0].ID() != test.wantID {
					t.Fatalf("%s output=%+v", test.name, out)
				}
			}
			if test.checkReq != nil {
				test.checkReq(t, test.client)
			}
		})
	}
}

func TestUserReadFiltersFillLogicalPagesAcrossSDKCursors(t *testing.T) {
	tests := []struct {
		name   string
		args   map[string]any
		client *fakeSDKClient
	}{
		{
			name: "search_user",
			args: map[string]any{"word": "miku", "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(1)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "mypixiv_users",
			args: map[string]any{"user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{myPixivUsers: func(_ context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(2)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "user_novels",
			args: map[string]any{"user_id": 71, "novel_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 1, User: pixiv.User{ID: 71}}}, Next: testPageCursor(3)}, nil
				}
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 2, User: pixiv.User{ID: 71}}}}, nil
			}},
		},
		{
			name: "user_artworks",
			args: map[string]any{"user_id": 71, "illust_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userArtworksFunc: func(request pixiv.UserArtworksRequest, call int) (sdk.Page[pixiv.Artwork], error) {
				if call == 1 {
					return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(1, "first", 71)}, Next: testPageCursor(4)}, nil
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(2, "second", 71)}}, nil
			}},
		},
		{
			name: "user_following",
			args: map[string]any{"user_id": 71, "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{userFollowing: func(_ context.Context, request pixiv.UserFollowingRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(5)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
		{
			name: "related_users",
			args: map[string]any{"user_id": 71, "user_filter": map[string]any{"id": 2}, "limit": 1},
			client: &fakeSDKClient{relatedUsers: func(_ context.Context, request pixiv.RelatedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
				if request.Cursor.IsZero() {
					return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 1}}}, Next: testPageCursor(6)}, nil
				}
				return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 2}}}}, nil
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result := callTool(t, session, test.name, test.args)
			if result.IsError {
				for _, content := range result.Content {
					if text, ok := content.(*mcp.TextContent); ok {
						t.Fatalf("filtered %s returned error: %s", test.name, text.Text)
					}
				}
				t.Fatalf("filtered %s returned error without text content: %+v", test.name, result)
			}
			var out outputs.Records
			decodeStructured(t, result, &out)
			if len(out.Records) != 1 || out.Records[0].ID() != "2" {
				t.Fatalf("filtered %s output=%+v", test.name, out)
			}
			if out.Pagination.Returned != 1 || out.Pagination.HasMore {
				t.Fatalf("filtered %s pagination=%+v", test.name, out.Pagination)
			}
		})
	}
}

func TestBlockedUsersSDKFailureRemainsStructuredAndDoesNotFallback(t *testing.T) {
	typed := &sdk.Error{Product: "pixiv", Operation: "UserBlockedUsers", Reason: sdk.Forbidden}
	calls := 0
	client := &fakeSDKClient{userID: 71, userBlockedUsers: func(_ context.Context, _ pixiv.UserBlockedUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		calls++
		return sdk.Page[pixiv.UserPreview]{}, typed
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "blocked_users", map[string]any{})
	if !result.IsError || calls != 1 || !resultHasText(result, typed.Error()) {
		t.Fatalf("blocked_users failure result=%+v calls=%d", result, calls)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "forbidden") {
		t.Fatalf("blocked_users structured error=%+v", out)
	}
}

var errUnexpectedRequest = &sdk.Error{Product: "test", Operation: "fixture", Reason: sdk.InvalidArgument}
