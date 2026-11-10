package fanbox_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestCreatorCursorBinding(t *testing.T) {
	var lists atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			id := 21
			if r.Header.Get("Cookie") == "FANBOXSESSID=other-fixture" {
				id = 22
			}
			fmt.Fprintf(w, `<meta name="metadata" content='{"context":{"user":{"userId":%d,"name":"fixture"}}}'>`, id)
			return
		}
		lists.Add(1)
		fmt.Fprint(w, `{"body":{"plans":[],"nextUrl":"https://api.fanbox.cc/plan.listSupporting?offset=1"}}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	other, err := fanbox.New("other-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	page, err := client.Creators(t.Context(), fanbox.CreatorsRequest{})
	require.NoError(t, err)
	_, err = client.Creators(t.Context(), fanbox.CreatorsRequest{Relation: fanbox.CreatorFollowing, Cursor: page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = other.Creators(t.Context(), fanbox.CreatorsRequest{Cursor: page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.EqualValues(t, 1, lists.Load())
}

func TestCreatorCursorPositions(t *testing.T) {
	var lists atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
			return
		}
		lists.Add(1)
		fmt.Fprint(w, `{"body":{"plans":[],"nextUrl":"https://api.fanbox.cc/plan.listSupporting?offset=1"}}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	page, err := client.Creators(t.Context(), fanbox.CreatorsRequest{})
	require.NoError(t, err)
	for _, position := range []url.Values{nil, {}, {"offset": {}}, {"offset": {"1", "2"}}, {"": {"1"}}, {"creatorId": {"foreign"}}, {"postId": {"1"}}, {"tag": {"changed"}}} {
		token := cursorWithPosition(t, page.Cursor, position)
		_, err := client.Creators(t.Context(), fanbox.CreatorsRequest{Cursor: token})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
	require.EqualValues(t, 1, lists.Load())
}

func cursorWithPosition(t *testing.T, token sdk.Cursor, position url.Values) sdk.Cursor {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(token.String())
	require.NoError(t, err)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &envelope))
	envelope["position"], err = json.Marshal(position)
	require.NoError(t, err)
	data, err = json.Marshal(envelope)
	require.NoError(t, err)
	modified, err := sdk.ParseCursor(base64.RawURLEncoding.EncodeToString(data))
	require.NoError(t, err)
	return modified
}

func TestCreatorIdentityCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := client.Creators(ctx, fanbox.CreatorsRequest{}); result <- err }()
	<-started
	waiter, stopWaiter := context.WithCancel(t.Context())
	waiting := make(chan error, 1)
	go func() { _, err := client.Creators(waiter, fanbox.CreatorsRequest{}); waiting <- err }()
	stopWaiter()
	require.ErrorIs(t, <-waiting, context.Canceled)
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}
