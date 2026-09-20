package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/searchfilter"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSearchArtworksLocalRatingContextDoesNotSendUnconfirmedServerRating(t *testing.T) {
	filter, err := searchfilter.NormalizeFilter("r18", "illustration")
	require.NoError(t, err)

	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		require.Empty(t, query.Get("rating"))
		require.Empty(t, query.Get("x_restrict"))
		require.Empty(t, query.Get("cursor_context"))
		return jsonResponse(`{"illusts":[],"next_url":null}`), nil
	})}})
	require.NoError(t, err)

	_, err = client.SearchArtworks(context.Background(), SearchArtworksRequest{
		Word:          "cat",
		ContentType:   SearchContentTypeIllust,
		CursorContext: filter.CursorContext(),
	})
	require.NoError(t, err)
}
