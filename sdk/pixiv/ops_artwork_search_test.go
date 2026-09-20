package pixiv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSearchArtworksCheckpointRoundTrip(t *testing.T) {
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"illusts":[{"id":1,"title":"a","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}},{"id":2,"title":"b","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}},{"id":3,"title":"c","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}}]}`), nil
	})}})
	require.NoError(t, err)
	request := SearchArtworksRequest{Word: "test"}
	page, err := client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	checkpoint, ok := any(client).(interface {
		CheckpointSearchArtworks(SearchArtworksRequest, int) (sdk.Cursor, error)
	})
	require.True(t, ok, "SDK must construct a resumable batch checkpoint")
	cursor, err := checkpoint.CheckpointSearchArtworks(request, 2)
	require.NoError(t, err)
	encoded, err := json.Marshal(cursor)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &request.Cursor))
	page, err = client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 3, page.Items[0].ID)
	require.True(t, page.Next.IsZero())
}

func TestSearchArtworksCheckpointRejectsChangedBindings(t *testing.T) {
	makeClient := func() *Client {
		client, err := NewWith("token", Options{})
		require.NoError(t, err)
		return client
	}
	client := makeClient()
	request := SearchArtworksRequest{Word: "private query", CursorContext: "local filter"}
	cursor, err := client.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		change func(*SearchArtworksRequest)
		client *Client
	}{
		{"word", func(r *SearchArtworksRequest) { r.Word = "other" }, client},
		{"local filter", func(r *SearchArtworksRequest) { r.CursorContext = "changed" }, client},
		{"AI mode", func(r *SearchArtworksRequest) { r.AIMode = SearchAIModeOnly }, client},
		{"content type", func(r *SearchArtworksRequest) { r.ContentType = SearchContentTypeManga }, client},
		{"client", func(r *SearchArtworksRequest) {}, makeClient()},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			changed.Cursor = cursor
			test.change(&changed)
			_, err := test.client.CheckpointSearchArtworks(changed, 1)
			require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
		})
	}
	// 解码 payload 后也不能暴露查询或本地筛选原文。
	payload, err := sdk.CursorPayload(cursor)
	require.NoError(t, err)
	require.NotContains(t, string(payload), request.Word)
	require.NotContains(t, string(payload), request.CursorContext)
	raw, err := base64.RawURLEncoding.DecodeString(cursor.String())
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	envelope["b"] = 1
	raw, err = json.Marshal(envelope)
	require.NoError(t, err)
	old, err := sdk.ParseCursor(base64.RawURLEncoding.EncodeToString(raw))
	require.NoError(t, err)
	request.Cursor = old
	_, err = client.CheckpointSearchArtworks(request, 1)
	require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
}

func TestSearchArtworksCheckpointAIAndLaterBatches(t *testing.T) {
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Empty(t, req.URL.Query().Get("cursor_context"))
		ids := []int{1, 2, 3, 4}
		next := ",\"next_url\":\"https://app-api.pixiv.net/v1/search/illust?offset=30\""
		if req.URL.Query().Get("offset") == "30" {
			ids = []int{5, 6}
			next = ""
		}
		var records []string
		for _, id := range ids {
			ai := 2
			if id == 1 {
				ai = 1
			}
			records = append(records, fmt.Sprintf(`{"id":%d,"illust_ai_type":%d,"type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}}`, id, ai))
		}
		return jsonResponse(`{"illusts":[` + strings.Join(records, ",") + `]` + next + `}`), nil
	})}})
	require.NoError(t, err)
	request := SearchArtworksRequest{Word: "test", AIMode: SearchAIModeOnly, CursorContext: "private filter"}
	cursor, err := client.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	text, err := cursor.MarshalText()
	require.NoError(t, err)
	require.NoError(t, request.Cursor.UnmarshalText(text))
	page, err := client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.EqualValues(t, 3, page.Items[0].ID)
	require.Len(t, page.Items, 2)
	nextBatch := page.Next
	request.Cursor, err = client.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	page, err = client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 4, page.Items[0].ID)
	request.Cursor = nextBatch
	request.Cursor, err = client.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	page, err = client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 6, page.Items[0].ID)
	require.True(t, page.Next.IsZero())
	request.Cursor, err = client.CheckpointSearchArtworks(request, 9)
	require.NoError(t, err)
	_, err = client.SearchArtworks(context.Background(), request)
	require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
}

func TestSearchArtworksCheckpointVerifiedAccount(t *testing.T) {
	open := func(id int) *Client {
		client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(fmt.Sprintf(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":%d}}`, id)), nil
		})}})
		require.NoError(t, err)
		return client
	}
	first, same, other := open(42), open(42), open(43)
	request := SearchArtworksRequest{Word: "test"}
	cursor, err := first.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	request.Cursor = cursor
	_, err = same.CheckpointSearchArtworks(request, 1)
	require.NoError(t, err)
	_, err = other.CheckpointSearchArtworks(request, 1)
	require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
}
