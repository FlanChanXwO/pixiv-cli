package pixiv_test

import (
	"context"
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

type commentWriteCase struct {
	name   string
	status int
	reason sdk.Reason
}

func TestCommentMutations(t *testing.T) {
	for _, novel := range []bool{false, true} {
		for _, action := range []string{"post", "reply", "stamp", "stamp_text", "delete"} {
			for _, tc := range []commentWriteCase{
				{"normal", 200, ""}, {"forbidden", 403, sdk.Forbidden}, {"unauthorized", 401, sdk.CredentialsExpired}, {"rate_limited", 429, sdk.RateLimited}, {"truncated", 200, sdk.UpstreamUnavailable}, {"cancel", 200, sdk.UpstreamUnavailable},
				{"service_unavailable", http.StatusServiceUnavailable, sdk.UpstreamError},
			} {
				t.Run(fmt.Sprint(novel, "/", action, "/", tc.name), func(t *testing.T) {
					testCommentMutation(t, novel, action, tc)
				})
			}
		}
	}
}

func testCommentMutation(t *testing.T, novel bool, action string, tc commentWriteCase) {
	t.Helper()
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assertCommentMutationRequest(t, r, novel, action)
		if tc.name == "truncated" {
			w.Header().Set("Content-Length", "100")
		}
		if tc.name == "rate_limited" {
			w.Header().Set("Retry-After", "0")
		}
		w.WriteHeader(tc.status)
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if tc.name == "cancel" {
		cancel()
	}
	err = runCommentMutation(t, ctx, client, novel, action)
	if tc.name == "cancel" {
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, calls)
	} else {
		require.Equal(t, tc.reason, sdk.ReasonOf(err))
		require.Equal(t, 1, calls)
	}
}

func assertCommentMutationRequest(t *testing.T, r *http.Request, novel bool, action string) {
	t.Helper()
	require.Equal(t, http.MethodPost, r.Method)
	require.NoError(t, r.ParseForm())
	path, parameter := api.PathIllustCommentAdd, api.ParamIllustID
	if novel {
		path, parameter = api.PathNovelCommentAdd, api.ParamNovelID
	}
	if action == "delete" {
		path = api.PathIllustCommentDelete
		if novel {
			path = api.PathNovelCommentDelete
		}
		require.Equal(t, "31", r.Form.Get(api.ParamCommentID))
		require.Empty(t, r.Form.Get(parameter))
		require.Equal(t, path, r.URL.Path)
		return
	}
	require.Equal(t, path, r.URL.Path)
	require.Equal(t, "23", r.Form.Get(parameter))
	parent, stamp, text := "", "", "text"
	if action == "reply" {
		parent = "30"
	}
	if action == "stamp" {
		stamp, text = "5", ""
	}
	if action == "stamp_text" {
		stamp, text = "5", "with stamp"
	}
	require.Equal(t, parent, r.Form.Get(api.ParamParentCommentID))
	require.Equal(t, stamp, r.Form.Get(api.ParamStampID))
	require.Equal(t, text, r.Form.Get(api.ParamComment))
	_, present := r.Form[api.ParamComment]
	require.True(t, present)
}

func runCommentMutation(t *testing.T, ctx context.Context, c *pixiv.Client, novel bool, action string) error {
	t.Helper()
	post, parent, remove := c.Artwork(23).PostComment, c.Artwork(23).Comment(30), c.ArtworkComment(31).Delete
	if novel {
		post, parent, remove = c.Novel(23).PostComment, c.Novel(23).Comment(30), c.NovelComment(31).Delete
	}
	if action == "delete" {
		return remove(ctx)
	}
	options := pixiv.CommentOptions{Text: "text"}
	var ref *pixiv.CommentRef
	var err error
	switch action {
	case "reply":
		ref, err = parent.Reply(ctx, options)
	case "stamp":
		ref, err = post(ctx, pixiv.CommentOptions{StampID: 5})
	case "stamp_text":
		ref, err = post(ctx, pixiv.CommentOptions{StampID: 5, Text: "with stamp"})
	default:
		ref, err = post(ctx, options)
	}
	if err == nil {
		require.EqualValues(t, 31, ref.ID)
	}
	return err
}

func TestCommentMutationResponses(t *testing.T) {
	for _, body := range []string{`{"comment":{"id":31}}`, `{}`, `{"comment_id":0}`, `{"comment_id":-1,"comment":{"id":31}}`, `{"comment":{}}`, `{`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, body) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ref, err := client.Artwork(23).PostComment(t.Context(), pixiv.CommentOptions{Text: "text"})
			if body == `{"comment":{"id":31}}` {
				require.NoError(t, err)
				require.EqualValues(t, 31, ref.ID)
			} else {
				require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
				require.Nil(t, ref)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestCommentMutationArguments(t *testing.T) {
	client := &pixiv.Client{}
	for _, call := range []func() error{
		func() error {
			_, e := client.Artwork(0).PostComment(t.Context(), pixiv.CommentOptions{Text: "text"})
			return e
		},
		func() error {
			_, e := client.Novel(0).PostComment(t.Context(), pixiv.CommentOptions{Text: "text"})
			return e
		},
		func() error { _, e := client.Artwork(23).PostComment(t.Context(), pixiv.CommentOptions{}); return e },
		func() error {
			_, e := client.Novel(23).PostComment(t.Context(), pixiv.CommentOptions{StampID: -1, Text: "text"})
			return e
		},
		func() error {
			_, e := client.ArtworkComment(31).Reply(t.Context(), pixiv.CommentOptions{Text: "text"})
			return e
		},
		func() error {
			_, e := client.Novel(23).Comment(0).Reply(t.Context(), pixiv.CommentOptions{Text: "text"})
			return e
		},
		func() error { return client.NovelComment(0).Delete(t.Context()) },
	} {
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(call()))
	}
}
