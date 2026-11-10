package fanbox_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestMediaSave(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/post.info" {
			fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":{"images":[{"id":"i","originalUrl":"https://downloads.fanbox.cc/image"}]}}}}`)
			return
		}
		if r.Host != "downloads.fanbox.cc" || r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture" || r.Header.Get("Referer") != "https://www.fanbox.cc/" {
			t.Error("incorrect media headers")
		}
		fmt.Fprint(w, "fixture bytes")
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	post, err := client.Post("42").Get(t.Context())
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "nested", "image")
	require.NoError(t, post.Body.Images[0].Save(t.Context(), path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "fixture bytes", string(data))
}

type mediaSaveFailure struct {
	name   string
	status int
	reason sdk.Reason
}

func TestMediaSaveFailures(t *testing.T) {
	for _, tc := range []mediaSaveFailure{
		{"expired", 401, sdk.CredentialsExpired}, {"forbidden", 403, sdk.Forbidden},
		{"challenge", 403, sdk.ChallengeRequired}, {"missing", 404, sdk.NotFound},
		{"partial stream", 200, sdk.UpstreamUnavailable}, {"partial response", 206, sdk.UpstreamError},
		{"not modified", 304, sdk.UpstreamError}, {"upstream", 503, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/post.info" {
					fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":{"files":[{"id":"f","url":"https://downloads.fanbox.cc/file"}]}}}}`)
					return
				}
				if tc.name == "challenge" {
					w.Header().Set("Cf-Mitigated", "challenge")
				}
				if tc.name == "partial stream" {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, "partial")
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			post, err := client.Post("42").Get(t.Context())
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "existing")
			require.NoError(t, os.WriteFile(path, []byte("old bytes"), 0o600))
			err = post.Body.Files[0].Save(t.Context(), path)
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "old bytes", string(data))
		})
	}
}

func serveConditionalMedia(t *testing.T, method string, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.URL.Path == "/post.info" {
		fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":{"files":[{"id":"f","name":"notes.ZIP","extension":"zip","url":"https://downloads.fanbox.cc/file"}]}}}}`)
		return
	}
	if r.Method != method || r.Header.Get("Range") != "bytes=0-3" || r.Header.Get("If-None-Match") != `"version"` || r.Header.Get("If-Modified-Since") != "Tue, 10 Nov 2026 00:00:00 GMT" || r.Header.Get("If-Range") != `"range"` {
		t.Error("media conditions were lost")
	}
	w.Header().Set("ETag", `"version"`)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", "4")
	w.Header().Set("Set-Cookie", "session-fixture")
	w.Header().Set("Location", "https://secret.invalid/")
	if method == http.MethodHead {
		w.WriteHeader(http.StatusNotModified)
	} else {
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, "data")
	}
}

func TestMediaConditionalRead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveConditionalMedia(t, method, w, r) }))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			post, err := client.Post("42").Get(t.Context())
			require.NoError(t, err)
			response, err := post.Body.Files[0].Open(t.Context(), fanbox.OpenRequest{Method: method, Range: "bytes=0-3", IfNoneMatch: `"version"`, IfModifiedSince: "Tue, 10 Nov 2026 00:00:00 GMT", IfRange: `"range"`})
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, "notes.ZIP", response.Filename)
			require.Equal(t, `"version"`, response.Header.Get("ETag"))
			require.Empty(t, response.Header.Get("Set-Cookie"))
			require.Empty(t, response.Header.Get("Location"))
			if method == http.MethodHead {
				require.Equal(t, http.StatusNotModified, response.StatusCode)
				require.Empty(t, data)
			} else {
				require.Equal(t, http.StatusPartialContent, response.StatusCode)
				require.Equal(t, "data", string(data))
			}
		})
	}
}

func TestMediaRedirectPrivacy(t *testing.T) {
	for _, host := range []string{"downloads.fanbox.cc", "pixiv.pximg.net", "api.fanbox.cc", "artist.fanbox.cc"} {
		t.Run(host, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/creator.get" {
					fmt.Fprintf(w, `{"body":{"user":{"name":"fixture","iconUrl":"https://%s/media"}}}`, host)
					return
				}
				if r.Header.Get("Referer") != "https://www.fanbox.cc/" || r.Header.Get("Range") != "bytes=0-3" {
					t.Error("missing media headers")
				}
				wantCookie := ""
				if host == "downloads.fanbox.cc" && r.URL.Path == "/media" {
					wantCookie = "FANBOXSESSID=session-fixture"
				}
				if r.Header.Get("Cookie") != wantCookie {
					t.Error("media credentials escaped their host or redirect boundary")
				}
				if r.URL.Path == "/media" {
					w.Header().Set("Location", "https://downloads.fanbox.cc/redirected")
					w.WriteHeader(http.StatusFound)
					return
				}
				fmt.Fprint(w, "data")
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			creator, err := client.Creator("artist").Get(t.Context())
			require.NoError(t, err)
			response, err := creator.Icon.Open(t.Context(), fanbox.OpenRequest{Range: "bytes=0-3"})
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
		})
	}
}
