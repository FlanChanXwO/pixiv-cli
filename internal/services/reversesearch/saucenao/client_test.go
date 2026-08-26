package saucenao_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/saucenao"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (body *closeTrackingBody) Close() error {
	body.closed = true
	return nil
}

func TestPreflightRequiresAPIKeyWithoutMakingARequest(t *testing.T) {
	requests := 0
	client := saucenao.New(saucenao.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return nil, nil
		})},
	})

	err := client.Preflight(context.Background())
	require.Equal(t, reversesearch.CodeMissingCredential, reversesearch.CodeOf(err))
	require.EqualError(t, err, "SauceNAO API key is required")
	_, err = client.Search(context.Background(), nil)
	require.Equal(t, reversesearch.CodeMissingCredential, reversesearch.CodeOf(err), "credential check must precede snapshot access")
	require.Zero(t, requests)
}

func TestSearchUploadsFixedMultipartAndParsesPixivResultAndQuota(t *testing.T) {
	const apiKey = "fixture-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Empty(t, request.URL.RawQuery, "API key and fixed fields belong only in multipart form data")
		require.NoError(t, request.ParseMultipartForm(1<<20))
		require.Equal(t, apiKey, request.FormValue("api_key"))
		require.Equal(t, "2", request.FormValue("output_type"))
		require.Equal(t, "999", request.FormValue("db"))
		require.Empty(t, request.FormValue("numres"))
		require.Len(t, request.MultipartForm.Value, 3)
		require.Len(t, request.MultipartForm.File, 1)
		file, header, err := request.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		require.Equal(t, "image", header.Filename)
		body, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, []byte("fixture-image"), body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "header": {
    "status": 0,
    "results_requested": 1,
    "results_returned": 1,
    "short_remaining": 3,
    "long_remaining": 97,
    "short_limit": "4",
    "long_limit": "100"
  },
  "results": [{
    "header": {
      "similarity": "91.23",
      "index_id": 5,
      "index_name": "Index #5: Pixiv Images"
    },
    "data": {
      "ext_urls": ["https://www.pixiv.net/artworks/123"],
      "title": "Fixture title",
      "pixiv_id": "123",
      "member_name": "Fixture author",
      "member_id": "456"
    }
  }]
}`)
	}))
	t.Cleanup(server.Close)

	snapshot := loadSnapshot(t, []byte("fixture-image"))
	client := saucenao.New(saucenao.Options{APIKey: apiKey, HTTPClient: server.Client(), Endpoint: server.URL})
	response, err := client.Search(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, reversesearch.ProviderSauceNAO, response.Provider)
	require.Equal(t, &reversesearch.Quota{ShortRemaining: 3, LongRemaining: 97, ShortLimit: 4, LongLimit: 100}, response.Quota)
	require.Equal(t, []reversesearch.Match{{
		Rank: 1, Similarity: 91.23, IndexID: 5, IndexName: "Index #5: Pixiv Images",
		Title: "Fixture title", Author: "Fixture author", ArtworkID: 123, UserID: 456,
		ExternalURLs: []string{"https://www.pixiv.net/artworks/123"},
	}}, response.Matches)
}

func TestSearchMapsAPIStatusToSafeProviderError(t *testing.T) {
	const secret = "fixture-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "header": {
    "status": -2,
    "message": "invalid key fixture-api-key and private upstream detail",
    "short_remaining": 0,
    "long_remaining": 0,
    "short_limit": 4,
    "long_limit": 100
  }
}`)
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: secret, HTTPClient: server.Client(), Endpoint: server.URL})

	_, err := client.Search(context.Background(), loadSnapshot(t, []byte("private-source-body")))
	require.Equal(t, reversesearch.CodeProviderFailed, reversesearch.CodeOf(err))
	require.EqualError(t, err, "SauceNAO rejected the query")
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, err.Error(), "private upstream detail")
	require.NotContains(t, err.Error(), "private-source-body")
}

func TestSearchRejectsMissingJSONStructure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: "fixture-key", HTTPClient: server.Client(), Endpoint: server.URL})

	_, err := client.Search(context.Background(), loadSnapshot(t, []byte("fixture-image")))
	require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
	require.EqualError(t, err, "SauceNAO returned a malformed response")
}

func TestSearchParsesNonPixivAuthorWithoutInventingPixivIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "header": {"status": 0, "short_remaining": 3, "long_remaining": 97, "short_limit": 4, "long_limit": 100},
  "results": [{
    "header": {"similarity": 80.5, "index_id": "9", "index_name": "Fixture index"},
    "data": {
      "title": "Non-Pixiv title",
      "author_name": "Non-Pixiv author",
      "ext_urls": ["https://example.test/work/7"]
    }
  }]
}`)
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: "fixture-key", HTTPClient: server.Client(), Endpoint: server.URL})

	response, err := client.Search(context.Background(), loadSnapshot(t, []byte("fixture-image")))
	require.NoError(t, err)
	require.Len(t, response.Matches, 1)
	require.Equal(t, "Non-Pixiv author", response.Matches[0].Author)
	require.Zero(t, response.Matches[0].ArtworkID)
	require.Zero(t, response.Matches[0].UserID)
}

func TestSearchAcceptsAWellFormedEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "header": {"status": 0, "results_requested": 1, "results_returned": 0, "short_remaining": 3, "long_remaining": 97, "short_limit": 4, "long_limit": 100},
  "results": []
}`)
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: "fixture-key", HTTPClient: server.Client(), Endpoint: server.URL})

	response, err := client.Search(context.Background(), loadSnapshot(t, []byte("fixture-image")))
	require.NoError(t, err)
	require.NotNil(t, response.Matches)
	require.Empty(t, response.Matches)
	require.Equal(t, &reversesearch.Quota{ShortRemaining: 3, LongRemaining: 97, ShortLimit: 4, LongLimit: 100}, response.Quota)
}

func TestSearchMapsHTTPAndMalformedResponsesWithoutLeakingInputs(t *testing.T) {
	const key = "fixture-secret-key"
	const payload = "fixture-private-source"
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   reversesearch.ErrorCode
		wantError  string
	}{
		{
			name: "non 2xx", statusCode: http.StatusTooManyRequests,
			body:     "raw upstream body fixture-secret-key fixture-private-source",
			wantCode: reversesearch.CodeUpstreamHTTPStatus, wantError: "SauceNAO returned an unsuccessful HTTP status",
		},
		{
			name: "invalid JSON", statusCode: http.StatusOK,
			body:     "not-json fixture-secret-key fixture-private-source",
			wantCode: reversesearch.CodeMalformedUpstreamResponse, wantError: "SauceNAO returned a malformed response",
		},
		{
			name: "trailing JSON", statusCode: http.StatusOK,
			body:     `{"header":{"status":0,"short_remaining":3,"long_remaining":97,"short_limit":4,"long_limit":100},"results":[]} {"private":"fixture-secret-key"}`,
			wantCode: reversesearch.CodeMalformedUpstreamResponse, wantError: "SauceNAO returned a malformed response",
		},
		{
			name: "missing result structure", statusCode: http.StatusOK,
			body:     `{"header":{"status":0,"short_remaining":3,"long_remaining":97,"short_limit":4,"long_limit":100},"results":[{}]}`,
			wantCode: reversesearch.CodeMalformedUpstreamResponse, wantError: "SauceNAO returned a malformed response",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				w.WriteHeader(test.statusCode)
				_, _ = io.WriteString(w, test.body)
			}))
			t.Cleanup(server.Close)
			client := saucenao.New(saucenao.Options{APIKey: key, HTTPClient: server.Client(), Endpoint: server.URL})

			_, err := client.Search(context.Background(), loadSnapshot(t, []byte(payload)))
			require.Equal(t, test.wantCode, reversesearch.CodeOf(err))
			require.EqualError(t, err, test.wantError)
			require.NotContains(t, err.Error(), key)
			require.NotContains(t, err.Error(), payload)
			require.NotContains(t, err.Error(), test.body)
		})
	}
}

func TestMalformedResponseDoesNotLeakRawValuesThroughErrorChain(t *testing.T) {
	const secret = "fixture-secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		_, _ = io.WriteString(w, `{
  "header": {"status": 0, "short_remaining": "fixture-secret-key", "long_remaining": 97, "short_limit": 4, "long_limit": 100},
  "results": []
}`)
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: secret, HTTPClient: server.Client(), Endpoint: server.URL})

	_, err := client.Search(context.Background(), loadSnapshot(t, []byte("private-source")))
	require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
	require.NotContains(t, errorChainText(err), secret)
	require.NotContains(t, errorChainText(err), "private-source")
}

func TestSearchRejectsNonFiniteSimilarity(t *testing.T) {
	const secret = "fixture-secret-key"
	const payload = "private-source"
	for _, similarity := range []string{"NaN", "Inf", "-Inf"} {
		t.Run(similarity, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				_, _ = io.WriteString(w, `{
  "header": {"status": 0, "short_remaining": 3, "long_remaining": 97, "short_limit": 4, "long_limit": 100},
  "results": [{
    "header": {"similarity": "`+similarity+`", "index_id": 9, "index_name": "Fixture index"},
    "data": {}
  }]
}`)
			}))
			t.Cleanup(server.Close)
			client := saucenao.New(saucenao.Options{APIKey: secret, HTTPClient: server.Client(), Endpoint: server.URL})

			_, err := client.Search(context.Background(), loadSnapshot(t, []byte(payload)))
			require.Equal(t, reversesearch.CodeMalformedUpstreamResponse, reversesearch.CodeOf(err))
			require.EqualError(t, err, "SauceNAO returned a malformed response")
			require.NotContains(t, errorChainText(err), secret)
			require.NotContains(t, errorChainText(err), payload)
		})
	}
}

func TestTransportErrorDoesNotLeakMultipartThroughErrorChain(t *testing.T) {
	const secret = "fixture-secret-key"
	const payload = "private-source"
	client := saucenao.New(saucenao.Options{
		APIKey:   secret,
		Endpoint: "https://saucenao.invalid/search.php",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			return nil, errors.New(string(body))
		})},
	})

	_, err := client.Search(context.Background(), loadSnapshot(t, []byte(payload)))
	require.Equal(t, reversesearch.CodeProviderFailed, reversesearch.CodeOf(err))
	require.EqualError(t, err, "SauceNAO request failed")
	require.NotContains(t, errorChainText(err), secret)
	require.NotContains(t, errorChainText(err), payload)
}

func TestSearchPrioritizesHTTPStatusWhenUploadAlsoFails(t *testing.T) {
	const secret = "fixture-secret-key"
	const payload = "private-source"
	const responseBody = "private-upstream-body"
	body := &closeTrackingBody{Reader: strings.NewReader(responseBody)}
	client := saucenao.New(saucenao.Options{
		APIKey:   secret,
		Endpoint: "https://saucenao.invalid/search.php",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			require.NoError(t, request.Body.Close())
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       body,
				Header:     make(http.Header),
				Request:    request,
			}, nil
		})},
	})

	_, err := client.Search(context.Background(), loadSnapshot(t, []byte(payload)))
	require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, reversesearch.CodeOf(err))
	require.EqualError(t, err, "SauceNAO returned an unsuccessful HTTP status")
	require.NotContains(t, errorChainText(err), secret)
	require.NotContains(t, errorChainText(err), payload)
	require.NotContains(t, errorChainText(err), responseBody)
	require.True(t, body.closed)
}

func TestSearchPreservesContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		close(requestStarted)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	client := saucenao.New(saucenao.Options{APIKey: "fixture-key", HTTPClient: server.Client(), Endpoint: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	snapshot := loadSnapshot(t, []byte("fixture-image"))
	go func() {
		_, err := client.Search(ctx, snapshot)
		result <- err
	}()

	<-requestStarted
	cancel()
	err := <-result
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, reversesearch.CodeUnknown, reversesearch.CodeOf(err))
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

func loadSnapshot(t *testing.T, body []byte) *reversesearch.Snapshot {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image")
	require.NoError(t, os.WriteFile(path, body, 0o600))
	snapshot, err := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()}).Load(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	return snapshot
}
