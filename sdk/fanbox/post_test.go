package fanbox_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestPostDetails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/post.info":
			if r.URL.Query().Get("postId") != "42" {
				t.Error("incorrect post request")
			}
			fmt.Fprint(w, `{"body":{"post":{"id":"42","title":"fixture","publishedDatetime":"2026-11-10T12:00:00+09:00","creatorId":"artist","feeRequired":500,"isRestricted":true,"isPinned":true,"restrictedFor":1,"commentCount":2,"body":null}}}`)
		case "/tag.getFeatured":
			fmt.Fprint(w, `{"body":[{"tag":"art"}]}`)
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	post, err := client.Post(" 42 ").Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, "42", post.ID)
	require.Equal(t, "fixture", post.Title)
	require.Equal(t, time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC), post.PublishedAt)
	require.True(t, post.IsRestricted && post.IsPinned)
	require.Equal(t, 500, post.FeeRequired)
	require.Equal(t, 1, post.RestrictedFor)
	require.Equal(t, 2, post.CommentCount)
	require.Nil(t, post.Body)
	tags, err := post.Creator.Tags(t.Context())
	require.NoError(t, err)
	require.Equal(t, "art", tags[0].Name)
	data, err := json.Marshal(post)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, post))
	_, err = post.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = post.Creator.Tags(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestPostDetailFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		reason     sdk.Reason
	}{
		{"missing body", `{}`, 200, sdk.MalformedUpstreamResponse},
		{"null post", `{"body":{"post":null}}`, 200, sdk.MalformedUpstreamResponse},
		{"missing post", `{"body":{}}`, 200, sdk.MalformedUpstreamResponse},
		{"missing id", `{"body":{"post":{"publishedDatetime":"2026-11-10T00:00:00Z"}}}`, 200, sdk.MalformedUpstreamResponse},
		{"wrong id", `{"body":{"post":{"id":"43","publishedDatetime":"2026-11-10T00:00:00Z"}}}`, 200, sdk.MalformedUpstreamResponse},
		{"bad time", `{"body":{"post":{"id":"42","publishedDatetime":"invalid"}}}`, 200, sdk.MalformedUpstreamResponse},
		{"expired", `{}`, 401, sdk.CredentialsExpired},
		{"forbidden", `{}`, 403, sdk.Forbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			_, err = client.Post("42").Get(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
		})
	}
}
