package pixiv_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pixivserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type mediaTransport func(*http.Request) (*http.Response, error)

func (f mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type mediaOutput struct {
	TotalPages     int    `json:"total_pages"`
	RequestedPages []int  `json:"requested_pages"`
	DeliveredPages []int  `json:"delivered_pages"`
	Complete       bool   `json:"complete"`
	Error          string `json:"error"`
	Pages          []struct {
		Page         int    `json:"page"`
		ContentIndex int    `json:"content_index"`
		MIME         string `json:"mime_type"`
		Size         int    `json:"size"`
	} `json:"pages"`
	Failures []struct {
		Page  int    `json:"page"`
		Error string `json:"error"`
	} `json:"failures"`
}

func TestArtworkMediaDeliversAllPagesAndExplicitSelection(t *testing.T) {
	for _, tc := range []struct {
		name, quality, path string
		pages               []int
		want                []int
		failPage            int
		badMIME             bool
		invalid             bool
		animation           string
	}{
		{name: "default-all", path: "c/1200x1200/", want: []int{1, 2, 3}},
		{name: "original-ordered-deduplicated", quality: "original", path: "img-original/", pages: []int{3, 1, 3}, want: []int{3, 1}},
		{name: "thumbnail", quality: "thumbnail", path: "c/250x250_80_a2/", pages: []int{2}, want: []int{2}},
		{name: "partial", path: "c/1200x1200/", want: []int{1, 2, 3}, failPage: 2},
		{name: "invalid-image", path: "c/1200x1200/", want: []int{1, 2, 3}, failPage: 2, badMIME: true},
		{name: "out-of-range", pages: []int{1, 4}, want: []int{1, 4}, invalid: true},
		{name: "static-animation-parameter", pages: []int{1}, want: []int{1}, animation: "gif", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))
			data := buf.Bytes()
			var opens, closes, reads atomic.Int32
			transport := mediaTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "app-api.pixiv.net" {
					pages := make([]any, 3)
					for i := range pages {
						pages[i] = map[string]any{"image_urls": map[string]string{"original": fmt.Sprintf("https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p%d.png", i)}}
					}
					body, _ := json.Marshal(map[string]any{"illust": map[string]any{"id": 42, "title": "fixture", "type": "manga", "page_count": 3, "create_date": "2026-01-01T00:00:00Z", "user": map[string]any{"id": 7, "name": "artist"}, "meta_pages": pages}})
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
				}
				require.Equal(t, "i.pximg.net", r.URL.Host)
				require.NotEmpty(t, r.Header.Get("Referer"))
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				require.Contains(t, r.URL.Path, tc.path)
				reads.Add(1)
				code := 200
				payload := data
				if tc.failPage > 0 && strings.Contains(r.URL.Path, fmt.Sprintf("42_p%d", tc.failPage-1)) {
					code = 403
					if tc.badMIME {
						code = 200
						payload = []byte("<html>fixture-secret-upstream-error</html>")
					}
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(bytes.NewReader(payload))}, nil
			})
			ports := pixivserver.SDKPorts{OpenLease: func(context.Context, pixivserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				opens.Add(1)
				client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
				return lifecycle.NewLease(client, func() error { closes.Add(1); return nil }), err
			}}
			session, closeSession := newSDKTestSessionWithPorts(t, ports, pixivserver.Account{})
			defer closeSession()
			tools, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)
			found := false
			for _, tool := range tools.Tools {
				if tool.Name == "pixiv_artwork_media" {
					found = true
					require.True(t, tool.Annotations.ReadOnlyHint)
				}
			}
			require.True(t, found, "missing artwork media tool")
			args := map[string]any{"illust_id": 42}
			if tc.quality != "" {
				args["quality"] = tc.quality
			}
			if tc.pages != nil {
				args["pages"] = tc.pages
			}
			if tc.animation != "" {
				args["animation_format"] = tc.animation
			}
			result := callTool(t, session, "pixiv_artwork_media", args)
			var out mediaOutput
			decodeStructured(t, result, &out)
			require.Equal(t, 3, out.TotalPages)
			require.Equal(t, tc.want, out.RequestedPages)
			require.Equal(t, tc.failPage == 0 && !tc.invalid, out.Complete)
			require.Equal(t, tc.failPage != 0 || tc.invalid, result.IsError)
			wantDelivered := []int{}
			for _, p := range tc.want {
				if p != tc.failPage && !tc.invalid {
					wantDelivered = append(wantDelivered, p)
				}
			}
			require.Equal(t, wantDelivered, out.DeliveredPages)
			require.Len(t, out.Pages, len(wantDelivered))
			for i, p := range out.Pages {
				require.Equal(t, wantDelivered[i], p.Page)
				require.Equal(t, "image/png", p.MIME)
				require.Equal(t, len(data), p.Size)
				require.Less(t, p.ContentIndex, len(result.Content))
				img, ok := result.Content[p.ContentIndex].(*mcp.ImageContent)
				require.True(t, ok)
				require.Equal(t, data, img.Data)
				require.Equal(t, p.MIME, img.MIMEType)
			}
			if tc.failPage > 0 {
				require.Len(t, out.Failures, 1)
				require.Equal(t, tc.failPage, out.Failures[0].Page)
			} else {
				require.Empty(t, out.Failures)
			}
			expectedReads := len(tc.want)
			if tc.invalid {
				expectedReads = 0
			}
			require.Equal(t, int32(expectedReads), reads.Load())
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "fixture-secret-upstream-error")
			require.Equal(t, int32(1), opens.Load())
			require.Equal(t, opens.Load(), closes.Load())
		})
	}
}

func TestArtworkMediaRejectsInvalidInputsBeforeOpening(t *testing.T) {
	var opens atomic.Int32
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{Open: func(pixivserver.Account) (*pixiv.Client, error) {
		opens.Add(1)
		return nil, fmt.Errorf("fixture-private-token")
	}}, pixivserver.Account{})
	defer closeSession()
	for _, args := range []map[string]any{{"illust_id": "fixture-secret-argument"}, {"illust_id": 42, "unknown": "fixture-secret-argument"}, {"illust_id": 0}, {"illust_id": 42, "pages": []int{}}, {"illust_id": 42, "pages": []int{0}}, {"illust_id": 42, "quality": "bad"}, {"illust_id": 42, "animation_format": "bad"}} {
		result := callTool(t, session, "pixiv_artwork_media", args)
		require.True(t, result.IsError)
		require.NotNil(t, result.StructuredContent)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "fixture-secret-argument")
	}
	require.Zero(t, opens.Load())
}

// mediaWaitingBody models an in-flight HTTP body, not a detached download job.
type mediaWaitingBody struct {
	ctx     context.Context
	entered chan struct{}
	closed  *atomic.Bool
}

func (b *mediaWaitingBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *mediaWaitingBody) Close() error { b.closed.Store(true); return nil }

func TestArtworkMediaCancellationClosesBodyAndAccountLease(t *testing.T) {
	entered := make(chan struct{})
	var bodyClosed, leaseClosed atomic.Bool
	var mediaReads atomic.Int32
	client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			body := `{"illust":{"id":42,"title":"fixture","type":"illust","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png"}}}`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		require.Equal(t, "i.pximg.net", r.URL.Host)
		mediaReads.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: &mediaWaitingBody{ctx: r.Context(), entered: entered, closed: &bodyClosed}}, nil
	})}})
	require.NoError(t, err)
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{OpenLease: func(context.Context, pixivserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
		return lifecycle.NewLease(client, func() error { leaseClosed.Store(true); return nil }), nil
	}}, pixivserver.Account{})
	defer closeSession()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_artwork_media", Arguments: map[string]any{"illust_id": 42}})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("media read never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tool cancellation did not return")
	}
	require.Eventually(t, func() bool { return bodyClosed.Load() && leaseClosed.Load() }, 5*time.Second, time.Millisecond)
	require.Equal(t, int32(1), mediaReads.Load())
}
