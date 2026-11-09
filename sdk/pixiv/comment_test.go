package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

const commentMetadata = `{"id":31,"comment":"reply","user":{"id":11},"date":"2026-11-01T10:00:00+09:00","parent_comment":{"id":30,"caption":"parent","user":{"id":12},"created_at":"2026-11-01T00:00:00Z"}}`

var commentEndpoints = []endpointContract{
	{"artwork_comments", api.PathIllustComments, `{"comments":[` + commentMetadata + `],"total_comments":4,"access_control":{"can_comment":true}}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.Artwork(23).Comments(ctx, pixiv.CommentsOptions{})
		if e == nil {
			require.Len(t, p.Page.Items, 1)
			require.EqualValues(t, 4, *p.Total)
			require.True(t, p.AccessControl.CanComment)
			require.Equal(t, "reply", p.Page.Items[0].Text)
			require.Equal(t, "parent", p.Page.Items[0].Parent.Text)
			require.Equal(t, time.UTC, p.Page.Items[0].CreatedAt.Location())
		}
		return e
	}},
	{"novel_comments", api.PathNovelComments, `{"comments":[` + commentMetadata + `],"comment_access_control":7}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.Novel(23).Comments(ctx, pixiv.CommentsOptions{})
		if e == nil {
			require.Nil(t, p.Total)
			require.EqualValues(t, 7, *p.AccessControl.NumericValue)
			require.EqualValues(t, 11, p.Page.Items[0].Author.ID)
		}
		return e
	}},
	{"unidentified_author", api.PathIllustComments, `{"comments":[{"id":31,"user":{},"date":"2026-11-01T00:00:00Z"}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.Artwork(23).Comments(ctx, pixiv.CommentsOptions{})
		if e == nil {
			require.Zero(t, p.Page.Items[0].Author.ID)
		}
		return e
	}},
}

func TestCommentEndpointContracts(t *testing.T) { testEndpointContracts(t, commentEndpoints) }

func TestCommentContinuation(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, api.PathIllustComments, r.URL.Path)
		require.Equal(t, "23", r.URL.Query().Get(api.ParamIllustID))
		if calls == 1 {
			fmt.Fprint(w, `{"comments":[],"next_url":"https://app-api.pixiv.net/v3/illust/comments?illust_id=23&offset=2"}`)
		} else {
			require.Equal(t, "2", r.URL.Query().Get(api.ParamOffset))
			fmt.Fprint(w, `{"comments":[],"total_comments":8,"access_control":{"is_locked":true}}`)
		}
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	page, err := client.Artwork(23).Comments(t.Context(), pixiv.CommentsOptions{})
	require.NoError(t, err)
	_, err = client.Artwork(24).Comments(t.Context(), pixiv.CommentsOptions{Cursor: page.Page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = client.Novel(23).Comments(t.Context(), pixiv.CommentsOptions{Cursor: page.Page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	next, err := page.Next(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 8, *next.Total)
	require.True(t, next.AccessControl.IsLocked)
	data, err := json.Marshal(page)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &page))
	_, err = page.Next(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = next.Next(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestMalformedCommentResponses(t *testing.T) {
	for _, body := range []string{
		`{"comments":null}`, `{"comments":[{"id":0}]}`,
		`{"comments":[{"id":31,"user":{"id":11},"date":"invalid"}]}`,
		`{"comments":[{"id":31,"user":{"id":11},"date":"2026-11-01T00:00:00Z","parent_comment":{"id":0}}]}`,
		`{"comments":[],"next_url":"https://evil.example/v3/illust/comments?offset=2"}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			_, err = client.Artwork(23).Comments(t.Context(), pixiv.CommentsOptions{})
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestCommentDecodeClearsBinding(t *testing.T) {
	client := &pixiv.Client{}
	comment := pixiv.Comment{CommentRef: client.Artwork(23).Comment(31)}
	require.NoError(t, json.Unmarshal([]byte(`{"id":31,"text":"decoded","author":{"id":11},"parent_comment":{"id":30}}`), &comment))
	require.Equal(t, "decoded", comment.Text)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(comment.Delete(t.Context())))
	_, err := comment.Reply(t.Context(), pixiv.CommentOptions{Text: "reply"})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(comment.Parent.Delete(t.Context())))
	_, err = comment.Author.Profile(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestCommentAccessControlJSON(t *testing.T) {
	for _, body := range []string{`{"comment_access_control":7}`, `{"can_comment":true,"is_locked":false}`} {
		t.Run(body, func(t *testing.T) {
			var control pixiv.CommentAccessControl
			require.NoError(t, json.Unmarshal([]byte(body), &control))
			encoded, err := json.Marshal(control)
			require.NoError(t, err)
			require.JSONEq(t, body, string(encoded))
		})
	}
}

func TestListedCommentOperations(t *testing.T) {
	for _, novel := range []bool{false, true} {
		t.Run(fmt.Sprint(novel), func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method == http.MethodGet {
					require.Equal(t, map[bool]string{false: api.PathIllustComments, true: api.PathNovelComments}[novel], r.URL.Path)
					fmt.Fprint(w, `{"comments":[`+commentMetadata+`]}`)
					return
				}
				action := "reply"
				if calls == 3 {
					action = "delete"
				}
				assertCommentMutationRequest(t, r, novel, action)
				fmt.Fprint(w, `{"comment_id":31}`)
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			read := client.Artwork(23).Comments
			if novel {
				read = client.Novel(23).Comments
			}
			page, err := read(t.Context(), pixiv.CommentsOptions{})
			require.NoError(t, err)
			posted, err := page.Page.Items[0].Parent.Reply(t.Context(), pixiv.CommentOptions{Text: "text"})
			require.NoError(t, err)
			require.NoError(t, posted.Delete(t.Context()))
			require.Equal(t, 3, calls)
		})
	}
}
