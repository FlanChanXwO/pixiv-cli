package pixiv

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
	"github.com/stretchr/testify/require"
)

func TestUserDecodeClearsBinding(t *testing.T) {
	client := &Client{}
	user := &User{UserRef: client.User(11), userData: userData{Name: "old"}}
	require.NoError(t, json.Unmarshal([]byte(`{"id":12,"name":"decoded"}`), user))
	require.Nil(t, user.client)
	require.Equal(t, "decoded", user.Name)
	_, err := user.Profile(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	err = user.Follow(t.Context(), FollowOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	page := ArtworkPage{Artwork: &Artwork{ArtworkRef: client.Artwork(23)}}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &page))
	require.Nil(t, page.Artwork)
	archive := UgoiraArchive{Image: Image{client: client}, Quality: UgoiraQualityMedium}
	require.NoError(t, json.Unmarshal([]byte(`{"url":"https://i.pximg.net/f.zip","quality":"original"}`), &archive))
	require.Equal(t, UgoiraQualityOriginal, archive.Quality)
	_, err = archive.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))

}

type endpointContract struct {
	name, path, body string
	call             func(*testing.T, context.Context, *Client) error
}

var userEndpoints = []endpointContract{
	{"current", api.PathUserDetail, `{"user":{"id":11},"profile":{},"profile_publicity":{},"workspace":{}}`, func(t *testing.T, ctx context.Context, c *Client) error { _, err := c.CurrentUser(ctx); return err }},
	{"profile", api.PathUserDetail, `{"user":{"id":11},"profile":{"total_illusts":7},"profile_publicity":{},"workspace":{}}`, func(t *testing.T, ctx context.Context, c *Client) error {
		p, e := c.User(11).Profile(ctx)
		if e == nil {
			require.Equal(t, 7, p.TotalIllusts)
			require.EqualValues(t, 11, p.User.ID)
		}
		return e
	}},
	{"related", api.PathUserRelated, `{"user_previews":[{"user":{"id":12}}]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		p, e := c.User(11).Related(ctx, RelatedUsersOptions{})
		if e == nil {
			require.EqualValues(t, 12, p.Items[0].ID)
		}
		return e
	}},
	{"following", api.PathUserFollowing, `{"user_previews":[]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		_, e := c.User(11).Following(ctx, RelationshipOptions{})
		return e
	}},
	{"followers", api.PathUserFollower, `{"user_previews":[]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		_, e := c.User(11).Followers(ctx, RelationshipOptions{})
		return e
	}},
	{"blocked", api.PathUserBlocked, `{"users":[{"id":12}]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		p, e := c.User(11).BlockedUsers(ctx, BlockedUsersOptions{})
		if e == nil {
			require.EqualValues(t, 12, p.Items[0].ID)
		}
		return e
	}},
	{"artworks", api.PathUserIllusts, `{"illusts":[]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		_, e := c.User(11).Artworks(ctx, UserArtworksOptions{})
		return e
	}},
	{"related_artworks", api.PathIllustRelated, `{"illusts":[]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		_, e := c.Artwork(23).Related(ctx, RelatedArtworksOptions{})
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
						if mode == "upstream" {
							w.WriteHeader(403)
						}

						switch {
						case mode == "normal":
							fmt.Fprint(w, endpoint.body)
						case endpoint.path == api.PathIllustBookmarkDetail:
							fmt.Fprint(w, `{"bookmark_detail":[]}`)
						default:
							fmt.Fprint(w, `{}`)
						}

					}))
					defer server.Close()
					value := NewCredentials("fixture-refresh")
					value.SetTokens("fixture-access", "fixture-refresh")
					value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
					store, err := NewMemoryCredentials(value)
					require.NoError(t, err)
					client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
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
			value := NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			followErr := client.User(12).Follow(ctx, FollowOptions{})
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
