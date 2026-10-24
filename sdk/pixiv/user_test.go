package pixiv_test

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestUserRelationshipDefaultsRestrictAndContinuesWithNormalizedQuery(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		call func(*Client, sdk.Cursor) (sdk.Page[UserPreview], error)
	}{
		{
			name: "following",
			path: "/v1/user/following",
			call: func(client *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return client.UserFollowing(context.Background(), UserFollowingRequest{UserID: 7, Cursor: cursor})
			},
		},
		{
			name: "followers",
			path: "/v1/user/follower",
			call: func(client *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return client.UserFollowers(context.Background(), UserFollowersRequest{UserID: 7, Cursor: cursor})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Path != test.path {
					t.Errorf("path = %q, want %q", req.URL.Path, test.path)
				}
				query := req.URL.Query()
				if query.Get("restrict") != string(RestrictPublic) {
					t.Errorf("restrict = %q, want %q", query.Get("restrict"), RestrictPublic)
				}
				wantOffset := ""
				if calls == 2 {
					wantOffset = "20"
				}
				if query.Get("offset") != wantOffset {
					t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
				}
				body := `{"user_previews":[{"user":{"id":41,"name":"user"}}],"next_url":"https://app-api.pixiv.net` + test.path + `?user_id=7&restrict=public&offset=20"}`
				if calls == 2 {
					body = `{"user_previews":[],"next_url":null}`
				}
				return jsonResponse(body), nil
			})
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			page, err := test.call(client, sdk.Cursor{})
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if page.Next.IsZero() {
				t.Fatal("expected continuation cursor")
			}
			page, err = test.call(client, page.Next)
			if err != nil {
				t.Fatalf("continuation: %v", err)
			}
			if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
				t.Fatalf("second page = %#v calls=%d", page, calls)
			}
		})
	}
}

// userFamily 是 user 端点族的表驱动基线：路由、query、续页与响应校验。
type userFamily struct {
	name    string
	path    string
	body    string
	call    func(*Client, sdk.Cursor) (sdk.Page[UserPreview], error)
	query   map[string]string
	next    int64
	itemIDs []int64
}

func TestUserListFamilyRoutesAndContinuation(t *testing.T) {
	tests := []userFamily{
		{
			name: "search",
			path: "/v1/search/user",
			body: `{"user_previews":[{"user":{"id":41,"name":"artist","account":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist", Cursor: cursor})
			},
			query:   map[string]string{"word": "artist"},
			next:    20,
			itemIDs: []int64{41},
		},
		{
			name: "following",
			path: "/v1/user/following",
			body: `{"user_previews":[{"user":{"id":71}}],"next_url":"https://app-api.pixiv.net/v1/user/following?user_id=8&restrict=private&offset=20"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserFollowing(context.Background(), UserFollowingRequest{UserID: 8, Restrict: RestrictPrivate, Cursor: cursor})
			},
			query:   map[string]string{"user_id": "8", "restrict": "private"},
			next:    20,
			itemIDs: []int64{71},
		},
		{
			name: "followers",
			path: "/v1/user/follower",
			body: `{"user_previews":[{"user":{"id":81}}],"next_url":"https://app-api.pixiv.net/v1/user/follower?user_id=8&restrict=public&offset=20"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserFollowers(context.Background(), UserFollowersRequest{UserID: 8, Restrict: RestrictPublic, Cursor: cursor})
			},
			query:   map[string]string{"user_id": "8", "restrict": "public"},
			next:    20,
			itemIDs: []int64{81},
		},
		{
			name: "related",
			path: "/v1/user/related",
			body: `{"user_previews":[{"user":{"id":61,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/user/related?seed_user_id=9&offset=20"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 9, Cursor: cursor})
			},
			query:   map[string]string{"seed_user_id": "9"},
			next:    20,
			itemIDs: []int64{61},
		},
		{
			name: "mypixiv",
			path: "/v1/user/mypixiv",
			body: `{"user_previews":[{"user":{"id":101}}],"next_url":"https://app-api.pixiv.net/v1/user/mypixiv?user_id=10&filter=for_android&offset=40"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.MyPixivUsers(context.Background(), MyPixivUsersRequest{Cursor: cursor})
			},
			query:   map[string]string{"user_id": "10", "filter": "for_android"},
			next:    40,
			itemIDs: []int64{101},
		},
		{
			name: "blocked",
			path: "/v2/user/list",
			body: `{"users":[{"id":91}],"next_url":"https://app-api.pixiv.net/v2/user/list?user_id=9&filter=for_android&offset=20"}`,
			call: func(c *Client, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
				return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9, Cursor: cursor})
			},
			query:   map[string]string{"user_id": "9", "filter": "for_android"},
			next:    20,
			itemIDs: []int64{91},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 上一轮的用户列表响应；MyPixivUsers 需要已验证身份。
			appCalls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "oauth.secure.pixiv.net" {
					return jsonResponse(`{"access_token":"access","refresh_token":"refresh","expires_in":3600,"user":{"id":10,"name":"tester"}}`), nil
				}
				appCalls++
				if req.URL.Path != test.path {
					t.Errorf("path = %q, want %q", req.URL.Path, test.path)
				}
				query := req.URL.Query()
				for key, want := range test.query {
					if query.Get(key) != want {
						t.Errorf("%s = %q, want %q", key, query.Get(key), want)
					}
				}
				if appCalls == 1 {
					if query.Get("offset") != "" {
						t.Errorf("initial offset = %q, want none", query.Get("offset"))
					}
					return jsonResponse(test.body), nil
				}
				if query.Get("offset") != strconv.FormatInt(test.next, 10) {
					t.Errorf("continuation offset = %q, want %d", query.Get("offset"), test.next)
				}
				return jsonResponse(`{"user_previews":[],"users":[],"next_url":null}`), nil
			})
			client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("OpenWith: %v", err)
			}
			page, err := test.call(client, sdk.Cursor{})
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if len(page.Items) != len(test.itemIDs) || page.Next.IsZero() {
				t.Fatalf("first page = %#v", page)
			}
			for index, id := range test.itemIDs {
				if page.Items[index].User.ID != id {
					t.Fatalf("item %d user = %d, want %d", index, page.Items[index].User.ID, id)
				}
			}
			page, err = test.call(client, page.Next)
			if err != nil {
				t.Fatalf("continuation: %v", err)
			}
			if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() {
				t.Fatalf("second page = %#v", page)
			}
		})
	}
}

// TestUserListFamilyRejectsMalformedResponses 保留原 endpoint 测试的空列表、
// null 列表、非法 user ID 与 continuation allowlist 断言。
func TestUserListFamilyRejectsMalformedResponses(t *testing.T) {
	cases := []struct {
		name string
		call func(*Client) error
		body string
	}{
		{name: "search null list", body: `{"user_previews":null}`, call: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search invalid user", body: `{"user_previews":[{"user":{"id":0}}]}`, call: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search unknown continuation key", body: `{"user_previews":[{"user":{"id":41}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20&max_novel_id=9"}`, call: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search wrong continuation path", body: `{"user_previews":[{"user":{"id":41}}],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=artist&offset=20"}`, call: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "search wrong continuation host", body: `{"user_previews":[{"user":{"id":41}}],"next_url":"https://example.invalid/v1/search/user?word=artist&offset=20"}`, call: func(c *Client) error {
			_, err := c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
			return err
		}},
		{name: "related null list", body: `{"user_previews":null}`, call: func(c *Client) error {
			_, err := c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 1})
			return err
		}},
		{name: "related invalid user", body: `{"user_previews":[{"user":{"id":0}}]}`, call: func(c *Client) error {
			_, err := c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 1})
			return err
		}},
		{name: "following missing list", body: `{}`, call: func(c *Client) error {
			_, err := c.UserFollowing(context.Background(), UserFollowingRequest{UserID: 8, Restrict: RestrictPrivate})
			return err
		}},
		{name: "following null list", body: `{"user_previews":null}`, call: func(c *Client) error {
			_, err := c.UserFollowing(context.Background(), UserFollowingRequest{UserID: 8, Restrict: RestrictPrivate})
			return err
		}},
		{name: "followers invalid user", body: `{"user_previews":[{"user":{"id":0}}]}`, call: func(c *Client) error {
			_, err := c.UserFollowers(context.Background(), UserFollowersRequest{UserID: 8, Restrict: RestrictPublic})
			return err
		}},
		{name: "mypixiv null list", body: `{"user_previews":null}`, call: func(c *Client) error {
			_, err := c.MyPixivUsers(context.Background(), MyPixivUsersRequest{})
			return err
		}},
		{name: "blocked missing list", body: `{}`, call: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "blocked null list", body: `{"users":null}`, call: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "blocked invalid user", body: `{"users":[{"id":0}]}`, call: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "blocked empty continuation", body: `{"users":[],"next_url":""}`, call: func(c *Client) error {
			_, err := c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
			return err
		}},
		{name: "recommended invalid nested artwork", body: `{"user_previews":[{"user":{"id":111},"illusts":[{"id":0}],"novels":[]}]}`, call: func(c *Client) error {
			_, err := c.RecommendedUsers(context.Background(), RecommendedUsersRequest{})
			return err
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "oauth.secure.pixiv.net" {
					return jsonResponse(`{"access_token":"access","refresh_token":"refresh","expires_in":3600,"user":{"id":10}}`), nil
				}
				return jsonResponse(test.body), nil
			})
			client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("OpenWith: %v", err)
			}
			if err := test.call(client); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse, err)
			}
		})
	}

	// 成功空列表必须保留非 nil 切片与零游标。
	for _, test := range []struct {
		name string
		body string
		call func(*Client) (any, error)
	}{
		{name: "search empty", body: `{"user_previews":[]}`, call: func(c *Client) (any, error) {
			return c.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist"})
		}},
		{name: "related empty", body: `{"user_previews":[]}`, call: func(c *Client) (any, error) {
			return c.RelatedUsers(context.Background(), RelatedUsersRequest{UserID: 1})
		}},
		{name: "following empty", body: `{"user_previews":[]}`, call: func(c *Client) (any, error) {
			return c.UserFollowing(context.Background(), UserFollowingRequest{UserID: 8, Restrict: RestrictPrivate})
		}},
		{name: "blocked empty", body: `{"users":[]}`, call: func(c *Client) (any, error) {
			return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
		}},
		{name: "blocked preview envelope", body: `{"user_previews":[{"user":{"id":92}}]}`, call: func(c *Client) (any, error) {
			return c.UserBlockedUsers(context.Background(), UserBlockedUsersRequest{UserID: 9})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(test.body), nil })
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			result, err := test.call(client)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			page := result.(sdk.Page[UserPreview])
			if page.Items == nil || len(page.Items) != 0 && test.name != "blocked preview envelope" || !page.Next.IsZero() {
				t.Fatalf("empty result = %#v", page)
			}
			if test.name == "blocked preview envelope" && (len(page.Items) != 1 || page.Items[0].User.ID != 92) {
				t.Fatalf("legacy envelope = %#v", page)
			}
		})
	}
}

func TestUserDetailAndRecommendations(t *testing.T) {
	const detailBody = `{"user":{"id":51,"name":"artist","account":"artist","profile_image_urls":{"medium":"https://i.pximg.net/profile.jpg"}},"profile":{"webpage":"https://example.invalid","total_novels":3},"profile_publicity":{"gender":"public","region":"private"},"workspace":{"pc":"mac"}}`
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v1/user/detail" {
			return jsonResponse(detailBody), nil
		}
		return jsonResponse(`{"user_previews":[{"user":{"id":111,"name":"artist"},"illusts":[{"id":112,"title":"art","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}],"novels":[{"id":113,"title":"story","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}]}],"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=0"}`), nil
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
	if len(page.Items) != 1 || page.Items[0].User.ID != 111 || len(page.Items[0].Illusts) != 1 || page.Items[0].Illusts[0].ID != 112 || len(page.Items[0].Novels) != 1 || page.Items[0].Novels[0].ID != 113 || page.Next.IsZero() {
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
		badClient, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(body), nil
		})}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		if _, err := badClient.User(context.Background(), UserRequest{UserID: 51}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
			t.Fatalf("body %s ReasonOf = %q (err=%v)", body, sdk.ReasonOf(err), err)
		}
	}
}

func TestUserRequestsRejectInvalidInputBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
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
	if calls != 0 {
		t.Fatalf("invalid input reached transport %d time(s)", calls)
	}
}
