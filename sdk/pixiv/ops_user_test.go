package pixiv_test

import (
	"context"
	"net/http"
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
