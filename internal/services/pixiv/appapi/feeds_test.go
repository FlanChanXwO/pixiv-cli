package appapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIllustNewUsesConfirmedAppAPIQueryAndContinuation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/new" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("content_type") != "manga" || query.Get("filter") != "for_android" || query.Get("offset") != "30" {
			t.Fatalf("query = %v", query)
		}
		_, _ = w.Write([]byte(`{"illusts":[{"id":1,"title":"manga"}],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=manga&filter=for_android&offset=60"}`))
	}))
	defer api.Close()

	result, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).IllustNew(context.Background(), "manga", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Illusts) != 1 || result.Illusts[0].ID != 1 || !result.ContinuationExists || result.NextOffset != 60 {
		t.Fatalf("result = %#v", result)
	}
}

func TestConfirmedFeedMethodsUseExpectedAppAPIPathsAndQueries(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		query map[string]string
		body  string
		call  func(*Client) error
	}{
		{
			name: "novel new", path: "/v1/novel/new",
			query: map[string]string{"filter": "for_android", "offset": "20"},
			body:  `{"novels":[{"id":2,"user":{"id":20}}]}`,
			call: func(client *Client) error {
				result, err := client.NovelNew(context.Background(), 20)
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 2 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "novel following", path: "/v1/novel/follow",
			query: map[string]string{"restrict": "private", "offset": "30"},
			body:  `{"novels":[{"id":3,"user":{"id":30}}]}`,
			call: func(client *Client) error {
				result, err := client.NovelFollow(context.Background(), "private", 30)
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 3 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv users", path: "/v1/user/mypixiv",
			query: map[string]string{"user_id": "44", "filter": "for_android", "offset": "40"},
			body:  `{"user_previews":[{"user":{"id":4}}]}`,
			call: func(client *Client) error {
				result, err := client.UserMyPixiv(context.Background(), 44, 40)
				if err != nil {
					return err
				}
				if len(result.UserPreviews) != 1 || result.UserPreviews[0].User.ID != 4 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv illusts", path: "/v2/illust/mypixiv",
			query: map[string]string{"offset": "50"},
			body:  `{"illusts":[{"id":5}]}`,
			call: func(client *Client) error {
				result, err := client.IllustMyPixiv(context.Background(), 50)
				if err != nil {
					return err
				}
				if len(result.Illusts) != 1 || result.Illusts[0].ID != 5 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "mypixiv novels", path: "/v1/novel/mypixiv",
			query: map[string]string{"offset": "60"},
			body:  `{"novels":[{"id":6,"user":{"id":60}}]}`,
			call: func(client *Client) error {
				result, err := client.NovelMyPixiv(context.Background(), 60)
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 6 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
		{
			name: "user novels", path: "/v1/user/novels",
			query: map[string]string{"user_id": "77", "filter": "for_android", "offset": "70"},
			body:  `{"novels":[{"id":7,"user":{"id":77}}]}`,
			call: func(client *Client) error {
				result, err := client.UserNovels(context.Background(), 77, 70)
				if err != nil {
					return err
				}
				if len(result.Novels) != 1 || result.Novels[0].ID != 7 {
					return fmt.Errorf("result=%#v", result)
				}
				return nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, test.path)
				}
				for key, want := range test.query {
					if got := r.URL.Query().Get(key); got != want {
						t.Fatalf("%s = %q, want %q; query=%v", key, got, want, r.URL.Query())
					}
				}
				if len(r.URL.Query()) != len(test.query) {
					t.Fatalf("query = %v, want exactly %v", r.URL.Query(), test.query)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer api.Close()

			if err := test.call(New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access"))); err != nil {
				t.Fatal(err)
			}
		})
	}
}
