package ascii2d_test

import (
	"context"
	"encoding/json"
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

func TestUploadAcceptsSameHostHTTPRedirectForHTTPSEndpoint(t *testing.T) {
	client := newTransportClient(t, func(request *http.Request) (*http.Response, error) {
		_, err := io.Copy(io.Discard, request.Body)
		require.NoError(t, err)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {"http://ascii2d.invalid/search/color/" + fixtureHash}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})

	_, err := client.Upload(context.Background(), loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture")))
	require.NoError(t, err)
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

func TestSessionSearchIgnoresAdvertisementItemBoxes(t *testing.T) {
	server := resultServer(t, resultFixtureWithAdvertisement(), http.StatusOK)
	defer server.Close()

	session := uploadSession(t, server)
	response, err := session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
	require.NoError(t, err)
	require.Equal(t, []reversesearch.Match{{
		Rank: 1, IndexName: "pixiv", Title: "Fixture title", Author: "Fixture author",
		ExternalURLs: []string{"https://www.pixiv.net/artworks/123", "https://www.pixiv.net/users/456"},
	}}, response.Matches)
}

func TestSessionSearchParsesExternalResultDetails(t *testing.T) {
	server := resultServer(t, resultFixtureWithExternalResult(), http.StatusOK)
	defer server.Close()

	session := uploadSession(t, server)
	response, err := session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
	require.NoError(t, err)
	require.Equal(t, []reversesearch.Match{{
		Rank: 1, IndexName: "Dlsite", Title: "External title",
		ExternalURLs: []string{"https://example.test/external"},
	}}, response.Matches)
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

func resultFixtureWithAdvertisement() string {
	const advertisement = `<div class="row item-box"><div class="p-t-1 hidden-md-up"><div class="gray-link">advertisement</div></div></div>`
	return strings.Replace(resultFixture(), `<div class="row item-box">
  <div class="image-box">`, advertisement+`
<div class="row item-box">
  <div class="image-box">`, 1)
}

func resultFixtureWithExternalResult() string {
	return `<html><body>
<div class="row item-box"><div class="image-box">query image</div></div>
<div class="row item-box">
  <div class="image-box"><img src="/thumbnail/external.jpg"></div>
  <div class="info-box">
    <div class="detail-box gray-link">
      <strong class="info-header">登録された詳細</strong>
      <div class="external">External title<a target="_blank" rel="noopener" href="https://example.test/external">Dlsite</a></div>
    </div>
  </div>
</div>
</body></html>`
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

func TestUploadMapsChallengeAndSolverFailuresToStableCodes(t *testing.T) {
	tests := []struct {
		name        string
		solverBody  string
		solverState bool
		wantCode    reversesearch.ErrorCode
		wantCause   error
	}{
		{
			name:     "challenge without configured solver",
			wantCode: reversesearch.CodeChallengeRequired,
		},
		{
			name:        "solver response is malformed",
			solverState: true,
			solverBody:  `{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[]}}`,
			wantCode:    reversesearch.CodeMalformedSolverResponse,
			wantCause:   ascii2d.ErrMalformedSolverResponse,
		},
		{
			name:        "solver reports failure",
			solverState: true,
			solverBody:  `{"status":"error","message":"private solver diagnostic"}`,
			wantCode:    reversesearch.CodeSolverFailed,
			wantCause:   ascii2d.ErrSolverFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asciiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Cf-Mitigated", "challenge")
				writer.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(writer, "private ascii challenge body")
			}))
			t.Cleanup(asciiServer.Close)

			options := ascii2d.Options{Endpoint: asciiServer.URL, HTTPClient: asciiServer.Client()}
			if test.solverState {
				solverServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					var payload struct {
						Command string `json:"cmd"`
					}
					if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
						t.Errorf("decode solver request: %v", err)
						return
					}
					switch payload.Command {
					case "sessions.create":
						writeJSON(t, writer, map[string]any{"status": "ok"})
					case "request.get":
						writer.Header().Set("Content-Type", "application/json")
						writer.WriteHeader(http.StatusOK)
						_, _ = io.WriteString(writer, test.solverBody)
					default:
						t.Errorf("unexpected solver command %q", payload.Command)
						writer.WriteHeader(http.StatusBadRequest)
					}
				}))
				t.Cleanup(solverServer.Close)
				options.FlareSolverr = &ascii2d.FlareSolverrOptions{URL: solverServer.URL}
			}

			client, err := ascii2d.New(options)
			require.NoError(t, err)
			_, err = client.Upload(context.Background(), loadSnapshot(t, []byte("\xff\xd8\xff\xe0fixture")))
			require.Equal(t, test.wantCode, reversesearch.CodeOf(err))
			if test.wantCause != nil {
				require.ErrorIs(t, err, test.wantCause)
			}
			require.NotContains(t, errorChainText(err), "private ascii challenge body")
			require.NotContains(t, errorChainText(err), "private solver diagnostic")
		})
	}
}

func TestNewMapsInvalidSolverConfigurationToStableCode(t *testing.T) {
	_, err := ascii2d.New(ascii2d.Options{
		Endpoint:   "https://ascii2d.invalid",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") })},
		FlareSolverr: &ascii2d.FlareSolverrOptions{
			URL: "file:///private/solver",
		},
	})
	require.Equal(t, reversesearch.CodeSolverUnavailable, reversesearch.CodeOf(err))
	require.ErrorIs(t, err, ascii2d.ErrSolverUnavailable)
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

func TestUploadClassifiesCloudflareChallengeHeaderBeforeReadingBody(t *testing.T) {
	const privateResponse = "private cloudflare challenge response"
	responseBody := &closeTrackingBody{Reader: strings.NewReader(privateResponse)}
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint: "https://ascii2d.invalid",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Cf-Mitigated": []string{"challenge"}},
				Body:       responseBody,
				Request:    request,
			}, nil
		})},
	})
	require.NoError(t, err)

	_, err = client.Upload(context.Background(), loadSnapshot(t, []byte("\xff\xd8\xff\xe0fixture")))
	require.Equal(t, reversesearch.CodeChallengeRequired, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d challenge requires solver recovery")
	require.True(t, responseBody.closed.Load())
	require.NotContains(t, errorChainText(err), privateResponse)
}

func TestUploadClassifiesCloudflareHTMLChallengeWithoutArbitraryTruncation(t *testing.T) {
	responseBody := &closeTrackingBody{Reader: strings.NewReader(strings.Repeat("ordinary prefix ", 5000) + `<html><head><title>Just a moment...</title></head><body>Checking your browser before accessing ascii2d</body></html>`)}
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint: "https://ascii2d.invalid",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body:       responseBody,
				Request:    request,
			}, nil
		})},
	})
	require.NoError(t, err)

	_, err = client.Upload(context.Background(), loadSnapshot(t, []byte("\xff\xd8\xff\xe0fixture")))
	require.Equal(t, reversesearch.CodeChallengeRequired, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d challenge requires solver recovery")
	require.True(t, responseBody.closed.Load())
}

func TestUploadClassifiesCloudflareChallengeHeaderEvenWithSuccessfulStatus(t *testing.T) {
	const privateResponse = "private cloudflare challenge response"
	responseBody := &closeTrackingBody{Reader: strings.NewReader(privateResponse)}
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint: "https://ascii2d.invalid",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cf-Mitigated": []string{"challenge"}},
				Body:       responseBody,
				Request:    request,
			}, nil
		})},
	})
	require.NoError(t, err)

	_, err = client.Upload(context.Background(), loadSnapshot(t, []byte("\xff\xd8\xff\xe0fixture")))
	require.Equal(t, reversesearch.CodeChallengeRequired, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d challenge requires solver recovery")
	require.True(t, responseBody.closed.Load())
	require.NotContains(t, errorChainText(err), privateResponse)
}

const (
	recoveredSolverUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	solverProxyFixture       = "http://solver-proxy.invalid:8080"
)

type solverFixture struct {
	server     *httptest.Server
	createCall atomic.Int32
	getCall    atomic.Int32
	mu         sync.Mutex
	getURLs    []string
}

func newSolverFixture(t *testing.T) *solverFixture {
	t.Helper()
	fixture := &solverFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1" {
			t.Errorf("solver endpoint path = %q, want /v1", request.URL.Path)
		}
		var payload struct {
			Command string `json:"cmd"`
			Session string `json:"session"`
			URL     string `json:"url"`
			Proxy   *struct {
				URL string `json:"url"`
			} `json:"proxy"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode solver request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		switch payload.Command {
		case "sessions.create":
			fixture.createCall.Add(1)
			if payload.Proxy == nil || payload.Proxy.URL != solverProxyFixture {
				t.Errorf("solver create proxy = %#v, want %q", payload.Proxy, solverProxyFixture)
			}
			if payload.URL != "" {
				t.Errorf("solver create unexpectedly contains protected URL %q", payload.URL)
			}
			writeJSON(t, writer, map[string]any{"status": "ok", "message": "Session created."})
		case "request.get":
			fixture.getCall.Add(1)
			if payload.Proxy != nil {
				t.Errorf("solver get unexpectedly contains proxy %#v", payload.Proxy)
			}
			if payload.Session == "" {
				t.Errorf("solver get session is empty")
			}
			fixture.mu.Lock()
			fixture.getURLs = append(fixture.getURLs, payload.URL)
			fixture.mu.Unlock()
			writeJSON(t, writer, map[string]any{
				"status": "ok",
				"solution": map[string]any{
					"userAgent": recoveredSolverUserAgent,
					"cookies":   []map[string]string{{"name": "cf_clearance", "value": "clearance-fixture"}},
				},
			})
		case "sessions.destroy":
			writeJSON(t, writer, map[string]any{"status": "ok", "message": "Session destroyed."})
		default:
			t.Errorf("unexpected solver command %q", payload.Command)
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode solver response: %v", err)
	}
}

type uploadObservation struct {
	Token   string
	Cookie  string
	Agent   string
	Payload []byte
}

func observeUpload(request *http.Request) (uploadObservation, error) {
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		return uploadObservation{}, err
	}
	file, _, err := request.FormFile("file")
	if err != nil {
		return uploadObservation{}, err
	}
	defer file.Close()
	payload, err := io.ReadAll(file)
	if err != nil {
		return uploadObservation{}, err
	}
	return uploadObservation{
		Token:   request.FormValue("authenticity_token"),
		Cookie:  request.Header.Get("Cookie"),
		Agent:   request.Header.Get("User-Agent"),
		Payload: payload,
	}, nil
}

func loadReplaySnapshot(t *testing.T, payload []byte) (*reversesearch.Snapshot, string) {
	t.Helper()
	sourceDir := t.TempDir()
	snapshotDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "fixture.jpg")
	require.NoError(t, os.WriteFile(sourcePath, payload, 0o600))
	snapshot, err := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: snapshotDir}).Load(context.Background(), sourcePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	entries, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	return snapshot, filepath.Join(snapshotDir, entries[0].Name())
}

func newSolverRecoveryClient(t *testing.T, asciiServer *httptest.Server, solver *solverFixture) *ascii2d.Client {
	t.Helper()
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint:   asciiServer.URL,
		HTTPClient: asciiServer.Client(),
		FlareSolverr: &ascii2d.FlareSolverrOptions{
			URL:      solver.server.URL,
			ProxyURL: solverProxyFixture,
		},
	})
	require.NoError(t, err)
	return client
}

func solverFixtureURLs(fixture *solverFixture) []string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]string(nil), fixture.getURLs...)
}

func TestUploadReplaysWithFreshSessionAfterUploadChallenge(t *testing.T) {
	initialPayload := []byte("\xff\xd8\xff\xe0first-payload")
	replayPayload := []byte("\xff\xd8\xff\xe0reopened-payload")
	snapshot, snapshotPath := loadReplaySnapshot(t, initialPayload)
	solver := newSolverFixture(t)
	var homeCalls atomic.Int32
	var uploadCalls atomic.Int32
	var resultCalls atomic.Int32
	var observationsMu sync.Mutex
	var observations []uploadObservation
	asciiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			call := homeCalls.Add(1)
			if call == 1 {
				http.SetCookie(writer, &http.Cookie{Name: "ascii2d_session", Value: "stale-session", Path: "/"})
				_, _ = io.WriteString(writer, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", "stale-csrf"))
				return
			}
			if request.Header.Get("User-Agent") != recoveredSolverUserAgent {
				t.Errorf("recovery home User-Agent = %q, want solver UA", request.Header.Get("User-Agent"))
			}
			if strings.Contains(request.Header.Get("Cookie"), "stale-session") {
				t.Errorf("recovery home reused stale session cookie: %q", request.Header.Get("Cookie"))
			}
			clearance, err := request.Cookie("cf_clearance")
			if err != nil || clearance.Value != "clearance-fixture" {
				t.Errorf("recovery home cf_clearance = %#v, want solver clearance", clearance)
			}
			http.SetCookie(writer, &http.Cookie{Name: "ascii2d_session", Value: "fresh-session", Path: "/"})
			_, _ = io.WriteString(writer, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", "fresh-csrf"))
		case "/search/file":
			call := uploadCalls.Add(1)
			observation, err := observeUpload(request)
			if err != nil {
				t.Errorf("observe upload %d: %v", call, err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			observationsMu.Lock()
			observations = append(observations, observation)
			observationsMu.Unlock()
			if call == 1 {
				if err := os.WriteFile(snapshotPath, replayPayload, 0o600); err != nil {
					t.Errorf("replace snapshot for replay: %v", err)
				}
				writer.Header().Set("Cf-Mitigated", "challenge")
				writer.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(writer, "private challenge body")
				return
			}
			if observation.Token != "fresh-csrf" {
				t.Errorf("replay CSRF = %q, want fresh-csrf", observation.Token)
			}
			if observation.Agent != recoveredSolverUserAgent {
				t.Errorf("replay User-Agent = %q, want solver UA", observation.Agent)
			}
			if !strings.Contains(observation.Cookie, "ascii2d_session=fresh-session") || !strings.Contains(observation.Cookie, "cf_clearance=clearance-fixture") {
				t.Errorf("replay cookies = %q, want fresh session and clearance", observation.Cookie)
			}
			if strings.Contains(observation.Cookie, "stale-session") {
				t.Errorf("replay reused stale session cookie: %q", observation.Cookie)
			}
			if string(observation.Payload) != string(replayPayload) {
				t.Errorf("replay payload = %q, want reopened snapshot payload %q", observation.Payload, replayPayload)
			}
			writer.Header().Set("Location", "/search/color/"+fixtureHash)
			writer.WriteHeader(http.StatusFound)
		case "/search/color/" + fixtureHash:
			resultCalls.Add(1)
			if request.Header.Get("User-Agent") != recoveredSolverUserAgent {
				t.Errorf("result User-Agent = %q, want solver UA", request.Header.Get("User-Agent"))
			}
			_, _ = io.WriteString(writer, resultFixture())
		default:
			t.Errorf("unexpected ascii2d request path: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer asciiServer.Close()

	client := newSolverRecoveryClient(t, asciiServer, solver)
	session, err := client.Upload(context.Background(), snapshot)
	require.NoError(t, err)
	require.NotNil(t, session)
	_, err = session.Search(context.Background(), reversesearch.ProviderASCII2DColor)
	require.NoError(t, err)
	require.Equal(t, int32(2), homeCalls.Load())
	require.Equal(t, int32(2), uploadCalls.Load())
	require.Equal(t, int32(1), resultCalls.Load())
	require.Equal(t, int32(1), solver.createCall.Load())
	require.Equal(t, int32(1), solver.getCall.Load())
	require.Equal(t, []string{asciiServer.URL + "/search/file"}, solverFixtureURLs(solver))

	observationsMu.Lock()
	defer observationsMu.Unlock()
	require.Len(t, observations, 2)
	require.Equal(t, "stale-csrf", observations[0].Token)
	require.Equal(t, initialPayload, observations[0].Payload)
}

func TestUploadRecoversWhenInitialHomepageIsChallenged(t *testing.T) {
	snapshot := loadSnapshot(t, []byte("\xff\xd8\xff\xe0homepage-challenge"))
	solver := newSolverFixture(t)
	var homeCalls atomic.Int32
	var uploadCalls atomic.Int32
	asciiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			call := homeCalls.Add(1)
			if call == 1 {
				writer.Header().Set("Cf-Mitigated", "challenge")
				writer.WriteHeader(http.StatusForbidden)
				return
			}
			if request.Header.Get("User-Agent") != recoveredSolverUserAgent {
				t.Errorf("recovery home User-Agent = %q, want solver UA", request.Header.Get("User-Agent"))
			}
			http.SetCookie(writer, &http.Cookie{Name: "ascii2d_session", Value: "fresh-session", Path: "/"})
			_, _ = io.WriteString(writer, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", "fresh-csrf"))
		case "/search/file":
			uploadCalls.Add(1)
			observation, err := observeUpload(request)
			if err != nil {
				t.Errorf("observe recovered upload: %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			if observation.Token != "fresh-csrf" || observation.Agent != recoveredSolverUserAgent {
				t.Errorf("recovered upload = %+v, want fresh CSRF and solver UA", observation)
			}
			writer.Header().Set("Location", "/search/color/"+fixtureHash)
			writer.WriteHeader(http.StatusFound)
		default:
			t.Errorf("unexpected ascii2d request path: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer asciiServer.Close()

	client := newSolverRecoveryClient(t, asciiServer, solver)
	session, err := client.Upload(context.Background(), snapshot)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, int32(2), homeCalls.Load())
	require.Equal(t, int32(1), uploadCalls.Load())
	require.Equal(t, int32(1), solver.createCall.Load())
	require.Equal(t, int32(1), solver.getCall.Load())
	require.Equal(t, []string{asciiServer.URL}, solverFixtureURLs(solver))
}

func TestUploadStopsAfterOneRecoveryReplayAndInvalidatesSolverState(t *testing.T) {
	snapshot := loadSnapshot(t, []byte("\xff\xd8\xff\xe0replay-once"))
	solver := newSolverFixture(t)
	var homeCalls atomic.Int32
	var uploadCalls atomic.Int32
	asciiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			call := homeCalls.Add(1)
			if call == 1 {
				_, _ = io.WriteString(writer, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", "stale-csrf"))
				return
			}
			_, _ = io.WriteString(writer, strings.ReplaceAll(uploadFormFixture(), "fixture-csrf", "fresh-csrf"))
		case "/search/file":
			uploadCalls.Add(1)
			if _, err := observeUpload(request); err != nil {
				t.Errorf("observe upload: %v", err)
			}
			writer.Header().Set("Cf-Mitigated", "challenge")
			writer.WriteHeader(http.StatusForbidden)
		default:
			t.Errorf("unexpected ascii2d request path: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer asciiServer.Close()

	client := newSolverRecoveryClient(t, asciiServer, solver)
	_, err := client.Upload(context.Background(), snapshot)
	require.ErrorIs(t, err, ascii2d.ErrSolverFailed)
	require.Equal(t, reversesearch.CodeSolverFailed, reversesearch.CodeOf(err))
	require.Equal(t, int32(2), homeCalls.Load())
	require.Equal(t, int32(2), uploadCalls.Load(), "challenge recovery must not loop beyond one replay")
	require.Equal(t, int32(1), solver.createCall.Load())
	require.Equal(t, int32(1), solver.getCall.Load())
	require.Equal(t, []string{asciiServer.URL + "/search/file"}, solverFixtureURLs(solver))
}
