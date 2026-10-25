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

// TestSearchArtworksCheckpointRoundTrip 覆盖 logical page checkpoint 的
// round-trip、初始 offset 与 AI 过滤后的跨批恢复。
func TestSearchArtworksCheckpointRoundTrip(t *testing.T) {
	var offsets []string
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		offset := req.URL.Query().Get("offset")
		offsets = append(offsets, offset)
		first := 1
		if offset == "270" {
			first = 271
		}
		ids := []int{first, first + 1, first + 2}
		var records []string
		for _, id := range ids {
			records = append(records, fmt.Sprintf(`{"id":%d,"type":"illust","ai_type":2,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}}`, id))
		}
		return jsonResponse(`{"illusts":[` + strings.Join(records, ",") + `]}`), nil
	})}})
	require.NoError(t, err)

	request := SearchArtworksRequest{Word: "test", Offset: 270, AIMode: SearchAIModeOnly}
	page, err := client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	require.EqualValues(t, 271, page.Items[0].ID)

	request.Cursor, err = client.CheckpointSearchArtworks(request, 2)
	require.NoError(t, err)
	page, err = client.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 273, page.Items[0].ID)
	require.True(t, page.Next.IsZero())
	require.Equal(t, []string{"270", "270"}, offsets)
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
		{"client", func(*SearchArtworksRequest) {}, makeClient()},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			changed.Cursor = cursor
			test.change(&changed)
			_, err := test.client.CheckpointSearchArtworks(changed, 1)
			require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
		})
	}

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

func TestSearchArtworksCheckpointVerifiedAccount(t *testing.T) {
	open := func(id int) *Client {
		client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
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
