package pixiv_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestUserListFamily(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		body      string
		query     map[string]string
		nextQuery map[string]string
		invoke    func(*Client, sdk.Cursor) (sdk.Page[UserPreview], error)
	}{
		{
			name:      "search",
			path:      "/v1/search/user",
			body:      `{"user_previews":[{"user":{"id":41}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`,
			query:     map[string]string{"word": "artist"},
			nextQuery: map[string]string{"offset": "20", "word": "artist"},
			invoke: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist", Cursor: cursor})
			},
		},
		{
			name:      "following",
			path:      "/v1/user/following",
			body:      `{"user_previews":[{"user":{"id":71}}],"next_url":"https://app-api.pixiv.net/v1/user/following?user_id=8&restrict=private&offset=20"}`,
			query:     map[string]string{"user_id": "8", "restrict": "private"},
			nextQuery: map[string]string{"offset": "20", "restrict": "private"},
			invoke: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserFollowing(context.Background(), UserFollowingRequest{UserID: 8, Restrict: RestrictPrivate, Cursor: cursor})
			},
		},
		{
			name:      "followers",
			path:      "/v1/user/follower",
			body:      `{"user_previews":[{"user":{"id":81}}],"next_url":"https://app-api.pixiv.net/v1/user/follower?user_id=8&restrict=public&offset=20"}`,
			query:     map[string]string{"user_id": "8", "restrict": "public"},
			nextQuery: map[string]string{"offset": "20", "restrict": "public"},
			invoke: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserFollowers(context.Background(), UserFollowersRequest{UserID: 8, Restrict: RestrictPublic, Cursor: cursor})
			},
		},
		{
			name:      "related",
			path:      "/v1/user/related",
			body:      `{"user_previews":[{"user":{"id":61}}],"next_url":"https://app-api.pixiv.net/v1/user/related?seed_user_id=9&offset=20"}`,
			query:     map[string]string{"seed_user_id": "9"},
			nextQuery: map[string]string{"offset": "20", "seed_user_id": "9"},
			invoke: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 9, Cursor: cursor})
			},
		},
		{
			name:      "blocked",
			path:      "/v2/user/list",
			body:      `{"users":[{"id":91}],"next_url":"https://app-api.pixiv.net/v2/user/list?user_id=9&filter=for_android&offset=20"}`,
			query:     map[string]string{"user_id": "9", "filter": "for_android"},
			nextQuery: map[string]string{"offset": "20", "user_id": "9"},
			invoke: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9, Cursor: cursor})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body, `{"user_previews":[],"users":[],"next_url":null}`)
			page, err := test.invoke(client, sdk.Cursor{})
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if len(page.Items) != 1 || page.Next.IsZero() {
				t.Fatalf("page = %#v", page)
			}
			if request := recorder.request(t, 0); request.URL.Path != test.path {
				t.Fatalf("path = %q, want %q", request.URL.Path, test.path)
			}
			for key, want := range test.query {
				if got := recorder.request(t, 0).URL.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if _, err := test.invoke(client, page.Next); err != nil {
				t.Fatalf("continuation: %v", err)
			}
			for key, want := range test.nextQuery {
				if got := recorder.request(t, 1).URL.Query().Get(key); got != want {
					t.Errorf("continuation %s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestUserFollowingDefaultsPublicRestrict(t *testing.T) {
	client, recorder := newWireClient(t, `{"user_previews":[{"user":{"id":41}}],"next_url":"https://app-api.pixiv.net/v1/user/following?user_id=7&restrict=public&offset=20"}`)
	page, err := client.UserFollowing(context.Background(), UserFollowingRequest{UserID: 7})
	if err != nil {
		t.Fatalf("UserFollowing: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation")
	}
	if got := recorder.request(t, 0).URL.Query().Get("restrict"); got != string(RestrictPublic) {
		t.Fatalf("restrict = %q, want public", got)
	}
}

func TestMyPixivUsersUsesVerifiedIdentity(t *testing.T) {
	var query url.Values
	client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			return jsonResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":10}}`), nil
		}
		query = req.URL.Query()
		return jsonResponse(`{"user_previews":[{"user":{"id":101}}],"next_url":"https://app-api.pixiv.net/v1/user/mypixiv?user_id=10&filter=for_android&offset=40"}`), nil
	})}})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	page, err := client.MyPixivUsers(context.Background(), MyPixivUsersRequest{})
	if err != nil {
		t.Fatalf("MyPixivUsers: %v", err)
	}
	if len(page.Items) != 1 || page.Next.IsZero() {
		t.Fatalf("page = %#v", page)
	}
	if query.Get("user_id") != "10" || query.Get("filter") != "for_android" {
		t.Fatalf("query = %v", query)
	}
}

func TestUserListFamilyRejectsMalformedResponses(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		invoke func(*Client) error
	}{
		{name: "search missing list", body: `{}`, invoke: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search null list", body: `{"user_previews":null}`, invoke: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search zero user", body: `{"user_previews":[{"user":{"id":0}}]}`, invoke: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "related zero user", body: `{"user_previews":[{"user":{"id":0}}]}`, invoke: func(c *Client) error {
			_, err := c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 1})
			return err
		}},
		{name: "blocked missing list", body: `{"users":null}`, invoke: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "blocked invalid user", body: `{"users":[{"id":0}]}`, invoke: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "blocked empty continuation", body: `{"users":[],"next_url":""}`, invoke: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "recommended nested artwork", body: `{"user_previews":[{"user":{"id":111},"illusts":[{"id":0}]}]}`, invoke: func(c *Client) error {
			_, err := c.RecommendedUsers(context.Background(), RecommendedUsersRequest{})
			return err
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse, err)
			}
		})
	}
}

func TestUserListFamilyEmptyTerminalPage(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		invoke func(*Client) (sdk.Page[UserPreview], error)
	}{
		"search": {body: `{"user_previews":[]}`, invoke: func(c *Client) (sdk.Page[UserPreview], error) {
			return c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
		}},
		"related": {body: `{"user_previews":[]}`, invoke: func(c *Client) (sdk.Page[UserPreview], error) {
			return c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 1})
		}},
		"blocked": {body: `{"users":[]}`, invoke: func(c *Client) (sdk.Page[UserPreview], error) {
			return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
		}},
		"blocked preview envelope": {body: `{"user_previews":[{"user":{"id":92}}]}`, invoke: func(c *Client) (sdk.Page[UserPreview], error) {
			return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newWireClient(t, test.body)
			page, err := test.invoke(client)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if name == "blocked preview envelope" {
				if len(page.Items) != 1 || page.Items[0].User.ID != 92 {
					t.Fatalf("legacy envelope = %#v", page)
				}
				return
			}
			if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() {
				t.Fatalf("empty result = %#v", page)
			}
		})
	}
}

func TestUserListItemsKeepNullSampleFields(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"user_previews":[{"user":{"id":41,"name":"artist"}}],"next_url":null}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	encoded, err := json.Marshal(page.Items[0])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"illusts":null`) || !strings.Contains(string(encoded), `"novels":null`) {
		t.Fatalf("list item JSON = %s, want null sample fields", encoded)
	}
}

func TestUserDetailAndRecommendations(t *testing.T) {
	const detailBody = `{"user":{"id":51,"name":"artist","account":"artist","profile_image_urls":{"medium":"https://i.pximg.net/profile.jpg"}},"profile":{"webpage":"https://example.invalid","total_novels":3},"profile_publicity":{"gender":"public","region":"private"},"workspace":{"pc":"mac"}}`
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v1/user/detail" {
			return jsonResponse(detailBody), nil
		}
		return jsonResponse(`{"user_previews":[{"user":{"id":111},"illusts":[{"id":112,"title":"art","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}],"novels":[{"id":113,"title":"story","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}]}],"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=0"}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	detail, err := client.User(context.Background(), UserRequest{UserID: 51})
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if detail.User.ID != 51 || detail.Profile.TotalNovels != 3 || !detail.ProfilePublicity.Gender || detail.ProfilePublicity.Region || detail.Workspace.PC != "mac" || detail.User.ProfileImage.Resource.Ref.IsZero() {
		t.Fatalf("detail = %#v", detail)
	}
	page, err := client.RecommendedUsers(context.Background(), RecommendedUsersRequest{})
	if err != nil {
		t.Fatalf("RecommendedUsers: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].User.ID != 111 || len(page.Items[0].Illusts) != 1 || len(page.Items[0].Novels) != 1 || page.Next.IsZero() {
		t.Fatalf("recommended = %#v", page)
	}

	for _, body := range []string{
		`{}`,
		`{"user":null,"profile":{},"profile_publicity":{},"workspace":{}}`,
		`{"user":{"id":51},"profile":{},"profile_publicity":{},"workspace":null}`,
		`{"user":{"id":51},"profile":null,"profile_publicity":{},"workspace":{}}`,
		`{"user":{"id":51},"profile":{},"profile_publicity":{"gender":"hidden"},"workspace":{}}`,
		`{"user":{"id":0},"profile":{},"profile_publicity":{},"workspace":{}}`,
	} {
		t.Run("malformed/"+body, func(t *testing.T) {
			badClient, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(body), nil
			})}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if _, err := badClient.User(context.Background(), UserRequest{UserID: 51}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("ReasonOf = %q (err=%v)", sdk.ReasonOf(err), err)
			}
		})
	}
}

func TestUserRequestsRejectInvalidInputBeforeNetwork(t *testing.T) {
	client, recorder := newWireClient(t)
	tests := []struct {
		name   string
		reason sdk.Reason
		call   func() error
	}{
		{name: "related user id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.RelatedUsers(context.Background(), RelatedUsersRequest{})
			return err
		}},
		{name: "following user id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserFollowing(context.Background(), UserFollowingRequest{})
			return err
		}},
		{name: "followers restrict", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserFollowers(context.Background(), UserFollowersRequest{UserID: 1, Restrict: Restrict("friends")})
			return err
		}},
		{name: "blocked user id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{})
			return err
		}},
		{name: "search word", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.SearchUsers(context.Background(), SearchUsersRequest{})
			return err
		}},
		{name: "detail user id", reason: sdk.InvalidArgument, call: func() error {
			_, err := client.User(context.Background(), UserRequest{})
			return err
		}},
		{name: "mypixiv unknown identity", reason: sdk.Unauthorized, call: func() error {
			_, err := client.MyPixivUsers(context.Background(), MyPixivUsersRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != test.reason {
				t.Fatalf("ReasonOf = %q, want %q", reason, test.reason)
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid input reached transport %d time(s)", recorder.calls())
	}
}
