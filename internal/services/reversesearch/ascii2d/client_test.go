package ascii2d_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/ascii2d"
	"github.com/stretchr/testify/require"
)

const fixtureHash = "0123456789abcdef0123456789abcdef"

var _ reversesearch.ASCII2DClient = (*ascii2d.Client)(nil)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type closeTrackingBody struct {
	io.Reader
	closed atomic.Bool
}

func (body *closeTrackingBody) Close() error {
	body.closed.Store(true)
	return nil
}

func TestUploadPrioritizesHTTPStatusWhenMultipartWriterAlsoFails(t *testing.T) {
	const privateResponse = "private upstream response body"
	responseBody := &closeTrackingBody{Reader: strings.NewReader(privateResponse)}
	client := newTransportClient(t, func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.NoError(t, request.Body.Close())
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       responseBody,
			Request:    request,
		}, nil
	})

	_, err := client.Upload(context.Background(), loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nprivate source payload")))
	require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d returned an unsuccessful HTTP status")
	require.True(t, responseBody.closed.Load())
	require.NotContains(t, errorChainText(err), "private source payload")
	require.NotContains(t, errorChainText(err), privateResponse)
}

func TestUploadPreservesCancellationAndClosesResponseBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responseBody := &closeTrackingBody{Reader: strings.NewReader("private upstream response body")}
	client := newTransportClient(t, func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.NoError(t, request.Body.Close())
		cancel()
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"/search/color/" + fixtureHash}},
			Body:       responseBody,
			Request:    request,
		}, nil
	})

	_, err := client.Upload(ctx, loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nprivate source payload")))
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, responseBody.closed.Load())
	require.NotContains(t, errorChainText(err), "private source payload")
	require.NotContains(t, errorChainText(err), "private upstream response body")
}

func TestUploadAcceptsSupportedMediaAtOfficialSizeBoundary(t *testing.T) {
	tests := []struct {
		name      string
		header    []byte
		mediaType string
		filename  string
	}{
		{name: "jpeg", header: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, mediaType: "image/jpeg", filename: "image.jpg"},
		{name: "png", header: []byte("\x89PNG\r\n\x1a\n"), mediaType: "image/png", filename: "image.png"},
		{name: "webp", header: []byte("RIFF\x04\x00\x00\x00WEBPVP8 "), mediaType: "image/webp", filename: "image.webp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var uploadedType string
			var uploadedFilename string
			server := newASCII2DServer(t, func(w http.ResponseWriter, request *http.Request) {
				if err := request.ParseMultipartForm(ascii2d.MaxImageBytes); err != nil {
					t.Errorf("parse multipart form: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				file, header, err := request.FormFile("file")
				if err != nil {
					t.Errorf("get form file: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer file.Close()
				uploadedType = header.Header.Get("Content-Type")
				uploadedFilename = header.Filename
				_, _ = io.Copy(io.Discard, file)
				http.Redirect(w, request, "/search/color/"+fixtureHash, http.StatusFound)
			})
			defer server.Close()

			snapshot := loadSizedSnapshot(t, test.header, ascii2d.MaxImageBytes)
			client := newClient(t, server)
			_, err := client.Upload(context.Background(), snapshot)
			require.NoError(t, err)
			require.Equal(t, test.mediaType, uploadedType)
			require.Equal(t, test.filename, uploadedFilename)
		})
	}
}

func TestUploadRejectsUnsupportedOrOversizedMediaBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client := newClient(t, server)

	_, err := client.Upload(context.Background(), loadSizedSnapshot(t, []byte("not-an-image"), int64(len("not-an-image"))))
	require.Equal(t, reversesearch.CodeInvalidSource, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d supports only JPEG, PNG, or WEBP images")

	_, err = client.Upload(context.Background(), loadSizedSnapshot(t, []byte("\x89PNG\r\n\x1a\n"), ascii2d.MaxImageBytes+1))
	require.Equal(t, reversesearch.CodeInvalidSource, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d image exceeds the 10 MB limit")
	require.Zero(t, requests.Load())
}

func TestUploadUsesIndependentCookieJarCSRFAndStrictRedirect(t *testing.T) {
	originalJar, err := cookiejar.New(nil)
	require.NoError(t, err)
	baseClient := &http.Client{Jar: originalJar}
	var uploadCount atomic.Int32
	server := newASCII2DServer(t, func(w http.ResponseWriter, request *http.Request) {
		uploadCount.Add(1)
		if !strings.Contains(request.Header.Get("Cookie"), "ascii2d_session=fixture-session") {
			t.Errorf("expected ascii2d_session cookie, got %q", request.Header.Get("Cookie"))
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.FormValue("authenticity_token") != "fixture-csrf" {
			t.Errorf("expected fixture-csrf token, got %q", request.FormValue("authenticity_token"))
		}
		if len(request.MultipartForm.File["file"]) != 1 {
			t.Errorf("expected 1 file, got %d", len(request.MultipartForm.File["file"]))
		}
		http.Redirect(w, request, "/search/color/"+fixtureHash, http.StatusFound)
	})
	defer server.Close()
	client, err := ascii2d.New(ascii2d.Options{HTTPClient: baseClient, Endpoint: server.URL})
	require.NoError(t, err)

	session, err := client.Upload(context.Background(), loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture")))
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, int32(1), uploadCount.Load(), "上传重定向不得被自动跟随")
	parsedURL := serverURL(t, server.URL)
	require.Empty(t, originalJar.Cookies(parsedURL), "adapter 不得复用或污染调用方 cookie jar")
}

func TestConcurrentUploadsUseSeparateCookieSessions(t *testing.T) {
	var homeCount atomic.Int32
	var mismatchCount atomic.Int32
	bothHomes := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			id := homeCount.Add(1)
			if id == 2 {
				close(bothHomes)
			}
			<-bothHomes
			value := fmt.Sprintf("session-%d", id)
			http.SetCookie(w, &http.Cookie{Name: "ascii2d_session", Value: value, Path: "/"})
			_, _ = io.WriteString(w, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", value))
		case "/search/file":
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse multipart form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			cookie, err := request.Cookie("ascii2d_session")
			if err != nil {
				t.Errorf("get cookie: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if cookie.Value != request.FormValue("authenticity_token") {
				mismatchCount.Add(1)
			}
			w.Header().Set("Location", "/search/color/"+fixtureHash)
			w.WriteHeader(http.StatusFound)
		default:
			t.Errorf("unexpected request path: %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := newClient(t, server)

	var group sync.WaitGroup
	errorsByUpload := make([]error, 2)
	snapshots := []*reversesearch.Snapshot{
		loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture-one")),
		loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture-two")),
	}
	group.Add(2)
	for index := range errorsByUpload {
		go func(index int) {
			defer group.Done()
			_, errorsByUpload[index] = client.Upload(context.Background(), snapshots[index])
		}(index)
	}
	group.Wait()
	require.NoError(t, errorsByUpload[0])
	require.NoError(t, errorsByUpload[1])
	require.Zero(t, mismatchCount.Load())
}

func TestSessionRejectsCrossOriginRedirects(t *testing.T) {
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/result" {
			_, _ = io.WriteString(w, resultFixture())
			return
		}
		_, _ = io.WriteString(w, uploadFormFixture())
	}))
	defer attacker.Close()

	t.Run("homepage", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/":
				http.Redirect(w, request, attacker.URL+"/", http.StatusFound)
			case "/search/file":
				w.Header().Set("Location", "/search/color/"+fixtureHash)
				w.WriteHeader(http.StatusFound)
			}
		}))
		defer origin.Close()

		client := newClient(t, origin)
		_, err := client.Upload(context.Background(), loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture")))
		require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
		require.EqualError(t, err, "ascii2d redirected outside its origin")
		require.NotContains(t, err.Error(), attacker.URL)
	})

	t.Run("result", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/":
				_, _ = io.WriteString(w, uploadFormFixture())
			case "/search/file":
				_, _ = io.Copy(io.Discard, request.Body)
				w.Header().Set("Location", "/search/color/"+fixtureHash)
				w.WriteHeader(http.StatusFound)
			case "/search/color/" + fixtureHash:
				http.Redirect(w, request, attacker.URL+"/result", http.StatusFound)
			}
		}))
		defer origin.Close()

		session := uploadSession(t, origin)
		_, err := session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
		require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
		require.EqualError(t, err, "ascii2d redirected outside its origin")
		require.NotContains(t, err.Error(), attacker.URL)
	})
}

func TestSessionSearchParsesColorAndBOVWFromOneUpload(t *testing.T) {
	var uploadCount atomic.Int32
	var modesMu sync.Mutex
	var modes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "ascii2d_session", Value: "fixture-session", Path: "/"})
			_, _ = io.WriteString(w, uploadFormFixture())
		case "/search/file":
			uploadCount.Add(1)
			_, _ = io.Copy(io.Discard, request.Body)
			w.Header().Set("Location", "/search/color/"+fixtureHash)
			w.WriteHeader(http.StatusFound)
		case "/search/color/" + fixtureHash, "/search/bovw/" + fixtureHash:
			modesMu.Lock()
			modes = append(modes, request.URL.Path)
			modesMu.Unlock()
			_, _ = io.WriteString(w, resultFixture())
		default:
			t.Errorf("unexpected request path: %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := newClient(t, server)
	session, err := client.Upload(context.Background(), loadSnapshot(t, []byte("RIFF\x04\x00\x00\x00WEBPVP8 ")))
	require.NoError(t, err)

	var color, bovw reversesearch.ProviderResponse
	var colorErr, bovwErr error
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		color, colorErr = session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
	}()
	go func() {
		defer group.Done()
		bovw, bovwErr = session.Search(context.Background(), reversesearch.ProviderASCII2DBOVW)
	}()
	group.Wait()
	require.NoError(t, colorErr)
	require.NoError(t, bovwErr)
	require.Equal(t, int32(1), uploadCount.Load())
	require.ElementsMatch(t, []string{"/search/color/" + fixtureHash, "/search/bovw/" + fixtureHash}, modes)
	for provider, response := range map[reversesearch.Provider]reversesearch.ProviderResponse{
		reversesearch.ProviderASCII2DColor: color,
		reversesearch.ProviderASCII2DBOVW:  bovw,
	} {
		require.Equal(t, provider, response.Provider)
		require.Nil(t, response.Quota)
		require.Equal(t, []reversesearch.Match{{
			Rank: 1, IndexName: "pixiv", Title: "Fixture title", Author: "Fixture author",
			ExternalURLs: []string{"https://www.pixiv.net/artworks/123", "https://www.pixiv.net/users/456"},
		}}, response.Matches)
	}
}

func TestSessionSearchRejectsUnsupportedProviderAndMalformedResultStructure(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing query item", body: `<html><main><p>no item boxes</p></main></html>`},
		{name: "selector drift", body: `<html><div class="item-box">query</div><div class="item-box"><div class="info-box"><div class="detail"><a href="https://example.test/work">title</a></div></div></div></html>`},
		{name: "missing artwork link", body: `<html><div class="item-box">query</div><div class="item-box"><div class="info-box"><div class="detail-box"><small>pixiv</small></div></div></div></html>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := resultServer(t, test.body, http.StatusOK)
			defer server.Close()
			session := uploadSession(t, server)
			_, err := session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
			require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
			require.EqualError(t, err, "ascii2d returned a malformed result page")
		})
	}

	server := resultServer(t, resultFixture(), http.StatusOK)
	defer server.Close()
	session := uploadSession(t, server)
	_, err := session.Search(context.Background(), reversesearch.ProviderSauceNAO)
	require.Equal(t, reversesearch.CodeInvalidRequest, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d search mode is invalid")
}

func newASCII2DServer(t *testing.T, upload http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "ascii2d_session", Value: "fixture-session", Path: "/"})
			_, _ = io.WriteString(w, uploadFormFixture())
		case "/search/file":
			upload(w, request)
		default:
			t.Errorf("unexpected request path: %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func resultServer(t *testing.T, resultBody string, resultStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "ascii2d_session", Value: "fixture-session", Path: "/"})
			_, _ = io.WriteString(w, uploadFormFixture())
		case "/search/file":
			_, _ = io.Copy(io.Discard, request.Body)
			w.Header().Set("Location", "/search/color/"+fixtureHash)
			w.WriteHeader(http.StatusFound)
		case "/search/color/" + fixtureHash, "/search/bovw/" + fixtureHash:
			w.WriteHeader(resultStatus)
			_, _ = io.WriteString(w, resultBody)
		default:
			t.Errorf("unexpected request path: %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func uploadSession(t *testing.T, server *httptest.Server) reversesearch.ASCII2DSession {
	t.Helper()
	client := newClient(t, server)
	session, err := client.Upload(context.Background(), loadSnapshot(t, []byte("\xff\xd8\xff\xe0fixture")))
	require.NoError(t, err)
	return session
}

func newClient(t *testing.T, server *httptest.Server) *ascii2d.Client {
	t.Helper()
	client, err := ascii2d.New(ascii2d.Options{HTTPClient: server.Client(), Endpoint: server.URL})
	require.NoError(t, err)
	return client
}

func newTransportClient(t *testing.T, upload roundTripFunc) *ascii2d.Client {
	t.Helper()
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint: "https://ascii2d.invalid",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method == http.MethodGet {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(uploadFormFixture())),
					Request:    request,
				}, nil
			}
			return upload(request)
		})},
	})
	require.NoError(t, err)
	return client
}

func loadSnapshot(t *testing.T, content []byte) *reversesearch.Snapshot {
	t.Helper()
	return loadSizedSnapshot(t, content, int64(len(content)))
}

func loadSizedSnapshot(t *testing.T, header []byte, size int64) *reversesearch.Snapshot {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture-image")
	file, err := os.Create(path)
	require.NoError(t, err)
	_, err = file.Write(header)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(size))
	require.NoError(t, file.Close())
	snapshot, err := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()}).Load(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	return snapshot
}

func serverURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

func uploadFormFixture() string {
	return `<html><body><form id="file_upload" enctype="multipart/form-data" action="/search/file" method="post"><input type="hidden" name="authenticity_token" value="fixture-csrf"><input type="file" name="file"></form></body></html>`
}

func resultFixture() string {
	return `<html><body>
<div class="row item-box"><div class="image-box">query image</div></div>
<div class="row item-box">
  <div class="image-box"><img src="/thumbnail/fixture.jpg"></div>
  <div class="info-box">
    <div>1200x800 JPEG 100KB</div>
    <div class="detail-box">
      <h6><a href="https://www.pixiv.net/artworks/123">Fixture title</a></h6>
      <h6><a href="https://www.pixiv.net/users/456">Fixture author</a></h6>
      <small>pixiv</small>
    </div>
  </div>
</div>
</body></html>`
}

func errorChainText(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		for _, nested := range many.Unwrap() {
			text += "\n" + errorChainText(nested)
		}
		return text
	}
	return text + "\n" + errorChainText(errors.Unwrap(err))
}
