package pixiv_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
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

func TestArtworkMediaUgoiraDeliversAnimationAndPreview(t *testing.T) {
	for _, tc := range []struct{ format, mime, quality string }{{"", "image/gif", "original"}, {"apng", "image/apng", "original"}, {"gif", "image/gif", "medium"}} {
		t.Run(tc.format+tc.quality, func(t *testing.T) {
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			for i, c := range []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
				frame, err := zw.Create(fmt.Sprintf("%d.png", i))
				require.NoError(t, err)
				img := image.NewRGBA(image.Rect(0, 0, 2, 2))
				for y := 0; y < 2; y++ {
					for x := 0; x < 2; x++ {
						img.SetRGBA(x, y, c)
					}
				}
				require.NoError(t, png.Encode(frame, img))
			}
			require.NoError(t, zw.Close())
			var closed, archiveReads atomic.Int32
			client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch {
				case r.URL.Host == "app-api.pixiv.net" && strings.Contains(r.URL.Path, "ugoira"):
					urls := map[string]string{"medium": "https://i.pximg.net/medium.zip"}
					if tc.quality == "original" {
						urls["original"] = "https://i.pximg.net/original.zip"
					}
					raw, _ := json.Marshal(map[string]any{"ugoira_metadata": map[string]any{"zip_urls": urls, "frames": []any{map[string]any{"file": "0.png", "delay": 80}, map[string]any{"file": "1.png", "delay": 120}}}})
					body = string(raw)
				case r.URL.Host == "app-api.pixiv.net":
					body = `{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`
				case r.URL.Host == "i.pximg.net":
					archiveReads.Add(1)
					require.Equal(t, "/"+tc.quality+".zip", r.URL.Path)
					require.NotEmpty(t, r.Header.Get("Referer"))
					require.Empty(t, r.Header.Get("Authorization"))
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/zip"}}, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
				default:
					return nil, fmt.Errorf("unexpected fixture host")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}})
			require.NoError(t, err)
			session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{OpenLease: func(context.Context, pixivserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				return lifecycle.NewLease(client, func() error { closed.Add(1); return nil }), nil
			}}, pixivserver.Account{})
			defer closeSession()
			args := map[string]any{"illust_id": 42}
			if tc.format != "" {
				args["animation_format"] = tc.format
			}
			result := callTool(t, session, "pixiv_artwork_media", args)
			require.False(t, result.IsError, "missing full Ugoira delivery: %+v", result)
			var out struct {
				Complete bool `json:"complete"`
				Pages    []struct {
					Filename       string `json:"filename"`
					MIME           string `json:"mime_type"`
					Size           int    `json:"size"`
					ContentIndex   int    `json:"content_index"`
					PreviewIndex   *int   `json:"preview_content_index"`
					ArchiveQuality string `json:"archive_quality"`
				} `json:"pages"`
			}
			decodeStructured(t, result, &out)
			require.True(t, out.Complete)
			require.Len(t, out.Pages, 1)
			p := out.Pages[0]
			require.Equal(t, tc.mime, p.MIME)
			require.Equal(t, tc.quality, p.ArchiveQuality)
			format := tc.format
			if format == "" {
				format = "gif"
			}
			require.Equal(t, "42."+format, p.Filename)
			blob, ok := result.Content[p.ContentIndex].(*mcp.EmbeddedResource)
			require.True(t, ok)
			require.Equal(t, p.Size, len(blob.Resource.Blob))
			require.Equal(t, tc.mime, blob.Resource.MIMEType)
			if format == "gif" {
				decoded, err := gif.DecodeAll(bytes.NewReader(blob.Resource.Blob))
				require.NoError(t, err)
				require.Len(t, decoded.Image, 2)
				require.Equal(t, []int{8, 12}, decoded.Delay)
			} else {
				require.True(t, bytes.HasPrefix(blob.Resource.Blob, []byte("\x89PNG\r\n\x1a\n")))
				require.Contains(t, string(blob.Resource.Blob), "acTL")
			}
			require.NotNil(t, p.PreviewIndex)
			require.NotEqual(t, p.ContentIndex, *p.PreviewIndex)
			preview, ok := result.Content[*p.PreviewIndex].(*mcp.ImageContent)
			require.True(t, ok)
			require.Equal(t, "image/png", preview.MIMEType)
			first, err := png.Decode(bytes.NewReader(preview.Data))
			require.NoError(t, err)
			require.Equal(t, image.Rect(0, 0, 2, 2), first.Bounds())
			require.Equal(t, int32(1), closed.Load())
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries, "temporary encoder files remain")
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(raw), temp)
			require.NotContains(t, string(raw), "fixture-token")
			for _, args := range []map[string]any{{"illust_id": 42, "pages": []int{1}}, {"illust_id": 42, "quality": "regular"}} {
				invalid := callTool(t, session, "pixiv_artwork_media", args)
				require.True(t, invalid.IsError)
				var rejected mediaOutput
				decodeStructured(t, invalid, &rejected)
				require.Equal(t, "static_parameters_not_applicable", rejected.Error)
			}
			require.Equal(t, int32(1), archiveReads.Load(), "invalid static parameters fetched animation bytes")
		})
	}
}

func TestArtworkMediaUgoiraFailureAndCancellationCleanTemporaryFiles(t *testing.T) {
	for _, mode := range []string{"corrupt", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			entered := make(chan struct{})
			var bodyClosed, leaseClosed atomic.Bool
			client, err := pixiv.NewWith("fixture-token", pixiv.Options{HTTPClient: &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "app-api.pixiv.net" {
					body := `{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`
					if strings.Contains(r.URL.Path, "ugoira") {
						body = `{"ugoira_metadata":{"zip_urls":{"original":"https://i.pximg.net/a.zip"},"frames":[{"file":"0.png","delay":80}]}}`
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				}
				require.Equal(t, "i.pximg.net", r.URL.Host)
				var body io.ReadCloser = io.NopCloser(strings.NewReader("fixture-private-corrupt-zip"))
				if mode == "cancel" {
					body = &mediaWaitingBody{ctx: r.Context(), entered: entered, closed: &bodyClosed}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/zip"}}, Body: body}, nil
			})}})
			require.NoError(t, err)
			session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{OpenLease: func(context.Context, pixivserver.Account) (*lifecycle.Lease[*pixiv.Client], error) {
				return lifecycle.NewLease(client, func() error { leaseClosed.Store(true); return nil }), nil
			}}, pixivserver.Account{})
			defer closeSession()
			if mode == "cancel" {
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
					t.Fatal("archive read did not start")
				}
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("canceled tool did not return")
				}
				require.Eventually(t, func() bool { return bodyClosed.Load() && leaseClosed.Load() }, 5*time.Second, time.Millisecond)
			} else {
				result := callTool(t, session, "pixiv_artwork_media", map[string]any{"illust_id": 42})
				require.True(t, result.IsError)
				var out mediaOutput
				decodeStructured(t, result, &out)
				require.False(t, out.Complete)
				require.Empty(t, out.DeliveredPages)
				raw, err := json.Marshal(result)
				require.NoError(t, err)
				require.NotContains(t, string(raw), temp)
				require.NotContains(t, string(raw), "fixture-private-corrupt-zip")
				require.True(t, leaseClosed.Load())
			}
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			require.Empty(t, entries, "failed/canceled encode left temporary files")
		})
	}
}
