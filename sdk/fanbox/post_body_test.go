package fanbox_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestPostBodyBlocks(t *testing.T) {
	body := `{"text":"caption","blocks":[{"type":"p","text":"hello","styles":[{"type":"bold","length":5}]},{"type":"file","fileId":"same","caption":"attachment"},{"type":"image","imageId":"same","caption":"picture"},{"type":"image","imageId":"same"},{"type":"video","provider":"youtube","videoId":"clip"},{"type":"embed","embedId":"embed"},{"type":"future","imageId":"same","nested":{"keep":[1,true]}}],"imageMap":{"same":{"id":"same","extension":"png","width":640,"height":480,"originalUrl":"https://downloads.fanbox.cc/image.png","thumbnailUrl":"https://pixiv.pximg.net/thumb.png"}},"fileMap":{"same":{"id":"same","name":"notes","extension":"zip","url":"https://downloads.fanbox.cc/file.zip"}},"embedMap":{"embed":{"html":"<iframe></iframe>","url":"https://third.invalid/"}}}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":%s}}}`, body)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	post, err := client.Post("42").Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, "caption", post.Body.Text)
	require.JSONEq(t, body, string(post.Body.Payload))
	require.Len(t, post.Body.Images, 1)
	require.Len(t, post.Body.Files, 1)
	require.Len(t, post.Body.Blocks, 7)
	blocks := post.Body.Blocks
	require.Equal(t, fanbox.BlockParagraph, blocks[0].Style)
	require.Equal(t, "hello", blocks[0].Text)
	require.Contains(t, string(blocks[0].Payload), `"styles"`)
	require.Equal(t, "attachment", blocks[1].Caption)
	require.Equal(t, "notes", blocks[1].File.Name)
	require.Equal(t, "zip", blocks[1].File.Extension)
	require.Equal(t, "same", blocks[2].Image.ID)
	require.Equal(t, "picture", blocks[2].Caption)
	require.Same(t, blocks[2].Image, blocks[3].Image)
	require.Equal(t, 640, blocks[2].Image.Width)
	require.Equal(t, 480, blocks[2].Image.Height)
	require.Equal(t, "https://pixiv.pximg.net/thumb.png", blocks[2].Image.Thumbnail.URL)
	require.Contains(t, string(blocks[4].Payload), `"videoId":"clip"`)
	require.Nil(t, blocks[5].Image)
	require.Equal(t, fanbox.BlockStyle("future"), blocks[6].Style)
	require.Same(t, blocks[2].Image, blocks[6].Image)
	require.Contains(t, string(blocks[6].Payload), `"keep":[1,true]`)
	data, err := json.Marshal(post)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, post))
	require.JSONEq(t, body, string(post.Body.Payload))
}

func TestPostBodyFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		reason     sdk.Reason
	}{
		{"wrong shape", `[]`, sdk.MalformedUpstreamResponse},
		{"null block", `{"blocks":[null]}`, sdk.MalformedUpstreamResponse},
		{"missing image", `{"blocks":[{"type":"image","imageId":"i"}]}`, sdk.MalformedUpstreamResponse},
		{"missing file", `{"blocks":[{"type":"future","fileId":"f"}]}`, sdk.MalformedUpstreamResponse},
		{"missing media id", `{"images":[{"originalUrl":"https://downloads.fanbox.cc/i"}]}`, sdk.MalformedUpstreamResponse},
		{"bad original", `{"images":[{"id":"i","originalUrl":"https://foreign.invalid/i"}]}`, sdk.ResourceForbidden},
		{"bad thumbnail", `{"images":[{"id":"i","originalUrl":"https://downloads.fanbox.cc/i","thumbnailUrl":"https://foreign.invalid/i"}]}`, sdk.ResourceForbidden},
		{"bad file", `{"files":[{"id":"f","url":"https://foreign.invalid/f"}]}`, sdk.ResourceForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":%s}}}`, tc.body)
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			_, err = client.Post("42").Get(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
		})
	}
}
