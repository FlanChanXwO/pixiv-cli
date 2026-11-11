package ascii2d_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/ascii2d"
	"github.com/stretchr/testify/require"
)

const (
	image   = "\x89PNG\r\n\x1a\nfixture"
	hash    = "0123456789abcdef0123456789abcdef"
	form    = `<form id="file_upload" action="/search/file" method="post" enctype="multipart/form-data"><input type="hidden" name="authenticity_token" value="csrf-fixture"><input type="file" name="file"></form>`
	results = `<div class="item-box"></div><div class="item-box">ad</div><div class="item-box"><div class="image-box"></div><div class="info-box"><div class="detail-box"><a href="https://www.pixiv.net/artworks/123">title</a><a href="https://www.pixiv.net/users/45">author</a><small>Pixiv</small></div></div></div>`
)

func routeClient(server *httptest.Server) *http.Client {
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return client
}

func TestSearch(t *testing.T) {
	var solves, uploads, destroys atomic.Int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		switch payload["cmd"] {
		case "sessions.create", "sessions.destroy":
			if payload["cmd"] == "sessions.destroy" {
				destroys.Add(1)
			}
			fmt.Fprint(w, `{"status":"ok"}`)
		case "request.get":
			solves.Add(1)
			if payload["url"] != "https://ascii2d.net/" || r.Header.Get("Cookie") != "" {
				t.Error("unsafe solver request")
			}
			fmt.Fprint(w, `{"status":"ok","solution":{"userAgent":"solved-agent","cookies":[{"name":"cf_clearance","value":"clearance-fixture"},{"name":"other","value":"discard"}]}}`)
		default:
			t.Error("unexpected solver command")
		}
	}))
	defer solver.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveSearch(t, w, r, &uploads) }))
	defer server.Close()
	options := ascii2d.Options{Image: []byte(image), Mode: ascii2d.Both, HTTPClient: routeClient(server), FlareSolverrURL: solver.URL}
	for range 2 {
		result, err := ascii2d.Search(t.Context(), options)
		require.NoError(t, err)
		for _, page := range []*ascii2d.Page{result.Color, result.BOVW} {
			require.NotNil(t, page)
			require.NoError(t, page.Err)
			require.Equal(t, []ascii2d.Match{{Rank: 1, IndexName: "Pixiv", Title: "title", Author: "author", ArtworkID: 123, ExternalURLs: []string{"https://www.pixiv.net/artworks/123", "https://www.pixiv.net/users/45"}}}, page.Matches)
		}
	}
	require.EqualValues(t, 1, solves.Load())
	require.EqualValues(t, 2, uploads.Load())
	require.EqualValues(t, 1, destroys.Load())
}

func serveSearch(t *testing.T, w http.ResponseWriter, r *http.Request, uploads *atomic.Int32) {
	t.Helper()
	if cookie, err := r.Cookie("cf_clearance"); err != nil || cookie.Value != "clearance-fixture" {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.Header.Get("User-Agent") != "solved-agent" || strings.Contains(r.Header.Get("Cookie"), "discard") {
		t.Error("invalid replay state")
	}
	switch r.URL.Path {
	case "/":
		fmt.Fprint(w, form)
	case "/search/file":
		uploads.Add(1)
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil || string(data) != image || header.Header.Get("Content-Type") != "image/png" || r.FormValue("authenticity_token") != "csrf-fixture" {
			t.Error("invalid multipart")
		}
		w.Header().Set("Location", "http://ascii2d.net/search/color/"+hash)
		w.WriteHeader(http.StatusFound)
	default:
		fmt.Fprint(w, results)
	}
}

func TestSearchFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		mode   ascii2d.Mode
		reason sdk.Reason
	}{
		{"limit", 429, "", ascii2d.Color, sdk.RateLimited},
		{"server", 503, "", ascii2d.Color, sdk.UpstreamUnavailable},
		{"format", 200, `<html>changed</html>`, ascii2d.Color, sdk.MalformedUpstreamResponse},
		{"cancel", 0, "", ascii2d.Color, sdk.UpstreamUnavailable},
		{"challenge", 403, `<h1>Just a moment</h1>`, ascii2d.Color, sdk.ChallengeRequired},
		{"partial", 503, "", ascii2d.Both, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) { searchFailure(t, tc.status, tc.body, tc.mode, tc.reason) })
	}
}

func searchFailure(t *testing.T, status int, body string, mode ascii2d.Mode, reason sdk.Reason) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, form)
		case "/search/file":
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Error(err)
			}
			w.Header().Set("Location", "/search/color/"+hash)
			w.WriteHeader(http.StatusFound)
		default:
			if status == 0 {
				cancel()
				<-r.Context().Done()
				return
			}
			if mode == ascii2d.Both && strings.Contains(r.URL.Path, "/bovw/") {
				fmt.Fprint(w, results)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(status)
			fmt.Fprint(w, body)
		}
	}))
	defer server.Close()
	result, err := ascii2d.Search(ctx, ascii2d.Options{Image: []byte(image), Mode: mode, HTTPClient: routeClient(server)})
	require.Equal(t, reason, sdk.ReasonOf(err))
	if status == 0 {
		require.ErrorIs(t, err, context.Canceled)
	}
	if mode == ascii2d.Both {
		require.NoError(t, result.BOVW.Err)
		require.Len(t, result.BOVW.Matches, 1)
	}
}
