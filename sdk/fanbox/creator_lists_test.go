package fanbox_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestCreatorLists(t *testing.T) {
	for _, relation := range []fanbox.CreatorRelation{"", fanbox.CreatorFollowing} {
		t.Run(string(relation), func(t *testing.T) {
			path, key := "/plan.listSupporting", "plans"
			if relation == fanbox.CreatorFollowing {
				path, key = "/creator.listFollowing", "creators"
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
				case "/creator.get":
					fmt.Fprint(w, `{"body":{"creatorId":"artist","user":{"name":"fixture"}}}`)
				case path:
					if r.URL.Query().Get("offset") != "" {
						fmt.Fprint(w, `{"body":[]}`)
						return
					}
					fmt.Fprintf(w, `{"body":{"%s":[{"creatorId":"artist"}],"pageUrls":["https://api.fanbox.cc%s?offset=1"],"nextUrl":"https://foreign.invalid/"}}`, key, path)
				default:
					t.Error("incorrect list request")
				}
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			page, err := client.Creators(t.Context(), fanbox.CreatorsRequest{Relation: relation})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			creator, err := page.Items[0].Get(t.Context())
			require.NoError(t, err)
			require.Equal(t, "fixture", creator.Name)
			next, err := page.Next(t.Context())
			require.NoError(t, err)
			require.Empty(t, next.Items)
		})
	}
}

func TestCreatorListFailures(t *testing.T) {
	for _, relation := range []fanbox.CreatorRelation{fanbox.CreatorSupporting, fanbox.CreatorFollowing} {
		for _, tc := range []struct{ name, body string }{
			{"missing", `{}`}, {"null", `{"body":null}`}, {"missing list", `{"body":{}}`},
			{"null list", `{"body":{"plans":null,"creators":null}}`},
			{"wrong list", `{"body":{"plans":{},"creators":{}}}`},
			{"missing id", `{"body":[{}]}`},
			{"foreign host", `{"body":{"plans":[],"creators":[],"nextUrl":"https://foreign.invalid/list?offset=1"}}`},
			{"foreign route", `{"body":{"plans":[],"creators":[],"nextUrl":"https://api.fanbox.cc/post.info?offset=1"}}`},
		} {
			t.Run(string(relation)+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" {
						fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
						return
					}
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				httpClient := server.Client()
				routeToServer(httpClient, server)
				client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
				require.NoError(t, err)
				_, err = client.Creators(t.Context(), fanbox.CreatorsRequest{Relation: relation})
				require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
			})
		}
	}
}

func TestCreatorListResume(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
			return
		}
		if r.URL.Query().Get("offset") == "1" {
			fmt.Fprint(w, `{"body":[{"creatorId":"second"}]}`)
			return
		}
		fmt.Fprint(w, `{"body":{"plans":[{"creatorId":"artist"}],"nextUrl":"https://api.fanbox.cc/plan.listSupporting?offset=1"}}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	page, err := client.Creators(t.Context(), fanbox.CreatorsRequest{})
	require.NoError(t, err)
	data, err := json.Marshal(page)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &page))
	_, err = page.Next(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = page.Items[0].Tags(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	token, err := sdk.ParseCursor(page.Cursor.String())
	require.NoError(t, err)
	resumed, err := client.Creators(t.Context(), fanbox.CreatorsRequest{Relation: fanbox.CreatorSupporting, Cursor: token})
	require.NoError(t, err)
	require.Len(t, resumed.Items, 1)
	require.Equal(t, "second", resumed.Items[0].ID)
	last, err := resumed.Next(t.Context())
	require.NoError(t, err)
	require.NotNil(t, last.Items)
	require.Empty(t, last.Items)
}
