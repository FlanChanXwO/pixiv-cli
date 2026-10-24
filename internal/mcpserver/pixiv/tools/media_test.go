package tools

import (
	"context"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAppAnimationUsesPublishedArtifactWithoutChangingLegacyContent(t *testing.T) {
	original := &mcp.ImageContent{MIMEType: "image/gif", Data: []byte("complete-original")}
	preview := &mcp.ImageContent{MIMEType: "image/png", Data: []byte("preview")}
	contents := []mcp.Content{original, preview}
	page := outputs.MediaPage{Page: 1, Filename: "42.gif", MIMEType: "image/gif", Size: len(original.Data), ContentIndex: 0, PreviewContentIndex: new(1)}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"io.modelcontextprotocol/clientCapabilities": map[string]any{"extensions": map[string]any{"io.modelcontextprotocol/ui": map[string]any{"mimeTypes": []string{"text/html;profile=mcp-app"}}}}}}}
	url := "https://media.test/media/animation/opaque.animation.signature"
	downloads := map[string]any{"media_origin": "https://media.test", "pages": []map[string]any{{"page": 1, "url": url}}}
	result := &mcp.CallToolResult{Content: contents, Meta: mcp.Meta{"pixiv/mediaDownloads": downloads}}
	appAnimationContent(request, result, outputs.ArtworkMedia{Complete: true, Pages: []outputs.MediaPage{page}})
	require.Equal(t, []mcp.Content{preview}, result.Content)
	appContents, ok := result.Meta["pixiv/mediaContents"].([]mcp.Content)
	require.True(t, ok)
	link, ok := appContents[0].(*mcp.ResourceLink)
	require.True(t, ok, "published App media must use protected HTTP instead of another large control payload")
	require.Equal(t, url, link.URI)
	require.Equal(t, "42.gif", link.Name)
	require.Equal(t, "image/gif", link.MIMEType)
	require.NotNil(t, link.Size)
	require.EqualValues(t, len(original.Data), *link.Size)
	require.Same(t, original, contents[0], "negotiated delivery must not mutate the legacy content slice")
	require.Same(t, preview, appContents[1])
	require.Equal(t, downloads, result.Meta["pixiv/mediaDownloads"])
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGalleryQueriesConcurrentlyAndPreservesOrder(t *testing.T) {
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	client, err := pixiv.NewWith("fixture", pixiv.Options{HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		id := r.URL.Query().Get("illust_id")
		if id == "99" {
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"illust":{"id":%s,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":7}}}`, id)))}, nil
	})}})
	require.NoError(t, err)
	app := toolhost.NewApp(toolhost.SDKPorts{Execute: func(ctx context.Context, _ toolhost.Account, fn func(context.Context, *pixiv.Client) (bool, error)) error {
		_, e := fn(ctx, client)
		return e
	}}, toolhost.Account{})
	result := make(chan galleryOut, 1)
	go func() {
		_, out, _ := pixivTools{app: app}.showGalleryHandler(t.Context(), showGalleryIn{IllustIDs: []int64{43, 99, 42, 43}})
		result <- out
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			<-result
			t.Fatal("metadata queries were serialized")
		}
	}
	close(release)
	out := <-result
	require.Len(t, out.Records, 2)
	require.Equal(t, "43", out.Records[0].ID())
	require.Equal(t, "42", out.Records[1].ID())
	require.Len(t, out.Failures, 3)
	require.EqualValues(t, 99, out.Failures[1].IllustID)
	require.Len(t, entered, 1, "duplicate artwork queried only once")
}

func TestGalleryCancellationReportsUndispatchedArtworks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tools := pixivTools{app: toolhost.NewApp(toolhost.SDKPorts{}, toolhost.Account{})}
	reported := map[int]bool{}
	tools.forEachGalleryItem(ctx, 3, func(index int) {
		t.Errorf("dispatch %d ran after cancellation", index)
	}, func(index int) {
		reported[index] = true
	})
	require.Equal(t, map[int]bool{0: true, 1: true, 2: true}, reported)
}

func TestAnimationThumbnailRequiresImageProxy(t *testing.T) {
	client, err := pixiv.NewWith("fixture", pixiv.Options{HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7}}}`))}, nil
	})}})
	require.NoError(t, err)
	app := toolhost.NewApp(toolhost.SDKPorts{Execute: func(ctx context.Context, _ toolhost.Account, fn func(context.Context, *pixiv.Client) (bool, error)) error {
		_, e := fn(ctx, client)
		return e
	}}, toolhost.Account{})
	_, out, _ := pixivTools{app: app}.showGalleryHandler(t.Context(), showGalleryIn{IllustIDs: []int64{42}})
	require.Len(t, out.Records, 1)
	require.Equal(t, []galleryFailure{{IllustID: 42, Error: "image_proxy_unavailable"}}, out.Failures, "animation thumbnails follow the authorized image path")
}
