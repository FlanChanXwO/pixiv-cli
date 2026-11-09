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

const userRecommendation = `{"user":{"id":12,"name":"author"},"illusts":[` + searchArtworkFirst + `],"novels":[` + novelMetadata + `]}`

func TestRecommendedUsersEndpoint(t *testing.T) {
	testEndpointContracts(t, []endpointContract{{"user_recommended", api.PathUserRecommended, `{"user_previews":[` + userRecommendation + `]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		page, err := c.RecommendedUsers(ctx, pixiv.RecommendedUsersOptions{})
		if err == nil {
			require.Len(t, page.Items, 1)
			item := page.Items[0]
			require.EqualValues(t, 12, item.User.ID)
			require.EqualValues(t, 23, item.Artworks[0].ID)
			require.EqualValues(t, 23, item.Novels[0].ID)
			require.EqualValues(t, 11, item.Artworks[0].Author.ID)
			require.EqualValues(t, 11, item.Novels[0].Author.ID)
		}
		return err
	}}})
}

func TestRecommendedUserSamplesFailWholePage(t *testing.T) {
	for _, preview := range []string{
		`{"user":{"id":0}}`,
		`{"user":{"id":12},"illusts":[{"id":23,"user":{"id":11},"create_date":"broken"}]}`,
		`{"user":{"id":12},"novels":[{"id":23,"user":{"id":11},"create_date":"broken"}]}`,
		`{"user":{"id":12},"novels":[{"id":0}]}`,
	} {
		t.Run(preview, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"user_previews":[`+userRecommendation+`,`+preview+`]}`)
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			page, err := c.RecommendedUsers(t.Context(), pixiv.RecommendedUsersOptions{})
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
			require.Empty(t, page.Items)
		})
	}
}

func TestRecommendedUsersZeroOffset(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, api.PathUserRecommended, r.URL.Path)
		if calls == 1 {
			require.Empty(t, r.URL.Query())
			fmt.Fprint(w, `{"user_previews":[`+userRecommendation+`],"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=0"}`)
		} else {
			require.Equal(t, []string{"0"}, r.URL.Query()["offset"])
			fmt.Fprint(w, `{"user_previews":[]}`)
		}
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	page, err := c.RecommendedUsers(t.Context(), pixiv.RecommendedUsersOptions{})
	require.NoError(t, err)
	_, err = c.SearchUsers(t.Context(), "author", pixiv.UserSearchOptions{Cursor: page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	next, err := page.Next(t.Context())
	require.NoError(t, err)
	require.Empty(t, next.Items)
	require.Equal(t, 2, calls)
	item := page.Items[0]
	require.NoError(t, json.Unmarshal([]byte(`{}`), &item))
	require.Nil(t, item.User)
	require.Nil(t, item.Artworks)
	require.Nil(t, item.Novels)
}
