package fanbox_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestImageLocatorRevalidation(t *testing.T) {
	var mediaCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/creator.get" {
			fmt.Fprint(w, `{"body":{"user":{"name":"fixture","iconUrl":"https://pixiv.pximg.net/icon"}}}`)
			return
		}
		mediaCalls.Add(1)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	creator, err := client.Creator("artist").Get(t.Context())
	require.NoError(t, err)
	for _, raw := range []string{"", "http://downloads.fanbox.cc/image", "https://foreign.invalid/image", "https://downloads.fanbox.cc/", "https://user:secret-fixture@downloads.fanbox.cc/image", "https://downloads.fanbox.cc.evil.invalid/image", "https://pximg.net/image", "https://www.pixiv.net/image"} {
		creator.Icon.URL = raw
		_, err = creator.Icon.Open(t.Context(), fanbox.OpenRequest{})
		require.Equal(t, sdk.ResourceForbidden, sdk.ReasonOf(err))
		require.NotContains(t, err.Error(), "secret-fixture")
	}
	require.Zero(t, mediaCalls.Load())
}

func TestImageConditionRevalidation(t *testing.T) {
	var mediaCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/creator.get" {
			fmt.Fprint(w, `{"body":{"user":{"name":"fixture","iconUrl":"https://pixiv.pximg.net/icon"}}}`)
			return
		}
		mediaCalls.Add(1)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	creator, err := client.Creator("artist").Get(t.Context())
	require.NoError(t, err)
	for _, request := range []fanbox.OpenRequest{{Method: "POST"}, {Method: "GET\r\n"}, {Range: "bytes=0-1\r\nsecret-fixture"}, {IfNoneMatch: "secret-fixture\x00"}, {IfModifiedSince: "secret-fixture\n"}, {IfRange: "secret-fixture\x7f"}} {
		_, err = creator.Icon.Open(t.Context(), request)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		require.NotContains(t, err.Error(), "secret-fixture")
	}
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(creator.Icon.Save(t.Context(), " ")))
	require.Zero(t, mediaCalls.Load())
}

func TestDecodedMediaIsUnbound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","coverImageUrl":"https://pixiv.pximg.net/cover","body":{"blocks":[{"type":"image","imageId":"i"},{"type":"file","fileId":"f"}],"imageMap":{"i":{"id":"i","originalUrl":"https://downloads.fanbox.cc/image","thumbnailUrl":"https://pixiv.pximg.net/thumb"}},"fileMap":{"f":{"id":"f","url":"https://downloads.fanbox.cc/file"}}}}}}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	post, err := client.Post("42").Get(t.Context())
	require.NoError(t, err)
	image := post.Body.Images[0]
	file := post.Body.Files[0]
	require.NoError(t, json.Unmarshal([]byte(`{"url":"https://downloads.fanbox.cc/image"}`), &image))
	require.NoError(t, json.Unmarshal([]byte(`{"url":"https://downloads.fanbox.cc/file"}`), &file))
	data, err := json.Marshal(post)
	require.NoError(t, err)
	require.NotContains(t, string(data), "session-fixture")
	require.NoError(t, json.Unmarshal(data, post))
	for _, decoded := range []fanbox.Image{image, post.Cover, post.Body.Images[0], *post.Body.Images[0].Thumbnail, *post.Body.Blocks[0].Image, {}} {
		_, err = decoded.Open(t.Context(), fanbox.OpenRequest{})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(decoded.Save(t.Context(), filepath.Join(t.TempDir(), "image"))))
	}
	for _, decoded := range []fanbox.File{file, post.Body.Files[0], *post.Body.Blocks[1].File, {}} {
		_, err = decoded.Open(t.Context(), fanbox.OpenRequest{})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(decoded.Save(t.Context(), filepath.Join(t.TempDir(), "file"))))
	}
}
