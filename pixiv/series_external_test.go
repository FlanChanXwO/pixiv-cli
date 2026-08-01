package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestIllustSeriesVerifiesOwnerAndBindsLastOrderCursor(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/illust/series", r.URL.Path)
		require.Equal(t, "9", r.URL.Query().Get("illust_series_id"))
		switch calls.Add(1) {
		case 1:
			require.Empty(t, r.URL.Query().Get("last_order"))
			_, _ = fmt.Fprint(w, `{"illust_series_detail":{"user":{"id":7}},"illusts":[{"id":101,"type":"illust","user":{"id":7}}],"next_url":"https://app-api.pixiv.net/v1/illust/series?last_order=10"}`)
		case 2:
			require.Equal(t, "10", r.URL.Query().Get("last_order"))
			_, _ = fmt.Fprint(w, `{"illust_series_detail":{"user":{"id":7}},"illusts":[{"id":102,"type":"illust","user":{"id":7}}],"next_url":null}`)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	require.NoError(t, err)
	first, err := client.IllustSeries(context.Background(), pixiv.IllustSeriesRequest{SeriesID: 9, UserID: 7})
	require.NoError(t, err)
	require.Len(t, first.Illusts, 1)
	require.NotEmpty(t, first.NextCursor)
	second, err := client.IllustSeries(context.Background(), pixiv.IllustSeriesRequest{SeriesID: 9, UserID: 7, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.Illusts, 1)
	require.Empty(t, second.NextCursor)
}

func TestIllustSeriesRejectsUnexpectedOwner(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"illust_series_detail":{"user":{"id":8}},"illusts":[],"next_url":null}`)
	}))
	t.Cleanup(server.Close)
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	require.NoError(t, err)
	_, err = client.IllustSeries(context.Background(), pixiv.IllustSeriesRequest{SeriesID: 9, UserID: 7})
	require.Error(t, err)
	var typed *pixiv.Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, pixiv.CodeMalformedUpstreamResponse, typed.Code)
}
