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

func TestUserDecodeClearsBinding(t *testing.T) {
	client := &pixiv.Client{}
	profile := pixiv.Profile{User: &pixiv.User{UserRef: client.User(11)}}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &profile))
	require.Nil(t, profile.User)
	user := &pixiv.User{UserRef: client.User(11)}
	require.NoError(t, json.Unmarshal([]byte(`{"id":12,"name":"decoded"}`), user))
	_, err := user.Profile(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	err = user.Follow(t.Context(), pixiv.FollowOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

type endpointContract struct {
	name, path, body string
	call             func(*testing.T, context.Context, *pixiv.Client) error
}

var userEndpoints = []endpointContract{
	{"current", api.PathUserDetail, `{"user":{"id":11},"profile":{},"profile_publicity":{},"workspace":{}}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		_, err := c.CurrentUser(ctx)
		return err
	}},
	{"profile", api.PathUserDetail, `{"user":{"id":12},"profile":{"total_illusts":7},"profile_publicity":{},"workspace":{}}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.User(12).Profile(ctx)
		if e == nil {
			require.Equal(t, 7, p.TotalIllusts)
			require.EqualValues(t, 12, p.User.ID)
		}
		return e
	}},
	{"related", api.PathUserRelated, `{"user_previews":[{"user":{"id":12}}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.User(11).Related(ctx, pixiv.RelatedUsersOptions{})
		if e == nil {
			require.EqualValues(t, 12, p.Items[0].ID)
		}
		return e
	}},
	{"following", api.PathUserFollowing, `{"user_previews":[]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		_, e := c.User(11).Following(ctx, pixiv.RelationshipOptions{})
		return e
	}},
	{"followers", api.PathUserFollower, `{"user_previews":[]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		_, e := c.User(11).Followers(ctx, pixiv.RelationshipOptions{})
		return e
	}},
	{"blocked", api.PathUserBlocked, `{"users":[{"id":12}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.User(11).BlockedUsers(ctx, pixiv.BlockedUsersOptions{})
		if e == nil {
			require.EqualValues(t, 12, p.Items[0].ID)
		}
		return e
	}},
	{"artworks", api.PathUserIllusts, `{"illusts":[]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		_, e := c.User(11).Artworks(ctx, pixiv.UserArtworksOptions{})
		return e
	}},
	{"related_artworks", api.PathIllustRelated, `{"illusts":[]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		_, e := c.Artwork(23).Related(ctx, pixiv.RelatedArtworksOptions{})
		return e
	}},
}

func TestUserEndpointContracts(t *testing.T) { testEndpointContracts(t, userEndpoints) }

func testEndpointContracts(t *testing.T, endpoints []endpointContract) {
	t.Helper()
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
				t.Run(mode, func(t *testing.T) {
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, endpoint.path, r.URL.Path)
						if endpoint.name == "current" || endpoint.name == "profile" {
							id, filter := "12", ""
							if endpoint.name == "current" {
								id, filter = "11", "for_android"
							}
							require.Equal(t, id, r.URL.Query().Get(api.ParamUserID))
							require.Equal(t, filter, r.URL.Query().Get(api.ParamFilter))
						}
						if mode == "upstream" {
							w.WriteHeader(403)
						}
						switch {
						case mode == "normal":
							fmt.Fprint(w, endpoint.body)
						case endpoint.path == api.PathIllustBookmarkDetail || endpoint.path == api.PathNovelBookmarkDetail:
							fmt.Fprint(w, `{"bookmark_detail":{"is_bookmarked":true}}`)
						case endpoint.name == "profile":
							fmt.Fprint(w, `{"user":{"id":13},"profile":{},"profile_publicity":{},"workspace":{}}`)
						default:
							fmt.Fprint(w, `{}`)
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
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if mode == "cancel" {
						cancel()
					}
					err = endpoint.call(t, ctx, client)
					assertEndpointResult(t, mode, err)
				})
			}
		})
	}
}

func assertEndpointResult(t *testing.T, mode string, err error) {
	t.Helper()
	switch mode {
	case "normal":
		require.NoError(t, err)
	case "upstream":
		require.Equal(t, sdk.Forbidden, sdk.ReasonOf(err))
	case "malformed":
		require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
	case "cancel":
		require.ErrorIs(t, err, context.Canceled)
	}
}

func TestFollowWrites(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, []string{api.PathFollowAdd, api.PathFollowDelete}[calls-1], r.URL.Path)
				require.NoError(t, r.ParseForm())
				require.Equal(t, "12", r.Form.Get(api.ParamUserID))
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "malformed" {
					w.Header().Set("Content-Length", "100")
					fmt.Fprint(w, `broken`)
				} else {
					fmt.Fprint(w, `{}`)
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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			followErr := client.User(12).Follow(ctx, pixiv.FollowOptions{})
			unfollowErr := client.User(12).Unfollow(ctx)
			assertFollowResult(t, mode, followErr, unfollowErr, calls)
		})
	}
}

func assertFollowResult(t *testing.T, mode string, followErr, unfollowErr error, calls int) {
	t.Helper()
	if mode == "malformed" {
		require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(followErr))
		require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(unfollowErr))
	} else {
		assertEndpointResult(t, mode, followErr)
		assertEndpointResult(t, mode, unfollowErr)
	}
	if mode != "cancel" {
		require.Equal(t, 2, calls)
	}
}
