package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadWithDirectResourceRevalidatesCache(t *testing.T) {
	t.Chdir(t.TempDir())
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == `"image-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"image-v1"`)
		_, _ = io.WriteString(w, "image")
	}))
	t.Cleanup(server.Close)
	client := newResourceDownloadClient(t, server)
	directory := t.TempDir()

	first, err := client.DownloadWith(context.Background(), server.URL+"/resource/image.png", pixiv.DownloadOptions{DownloadPath: directory})
	require.NoError(t, err)
	require.Len(t, first.Files, 1)
	assert.Equal(t, pixiv.DownloadSourceResource, first.SourceKind)
	assert.Equal(t, pixiv.ResourceDownloadCacheMiss, first.Files[0].CacheState)
	assert.Equal(t, filepath.Join(directory, "image.png"), first.Files[0].Path)

	second, err := client.Download(context.Background(), server.URL+"/resource/image.png")
	// Download uses ./downloads, so it intentionally is not the same cache entry.
	require.NoError(t, err)
	assert.Equal(t, pixiv.ResourceDownloadCacheMiss, second.Files[0].CacheState)

	third, err := client.DownloadWith(context.Background(), server.URL+"/resource/image.png", pixiv.DownloadOptions{DownloadPath: directory})
	require.NoError(t, err)
	assert.Equal(t, pixiv.ResourceDownloadCacheRevalidated, third.Files[0].CacheState)
	assert.EqualValues(t, 3, requests.Load())
	_, err = client.DownloadWith(context.Background(), server.URL+"/resource/image.png", pixiv.DownloadOptions{
		DownloadPath: directory, FilenameTemplate: "{id}",
	})
	require.Error(t, err)
}

func TestDownloadAllWithPreservesInputOrderAndItemFailures(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bad.jpg") {
			http.Error(w, "upstream failure", http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, strings.TrimPrefix(r.URL.Path, "/resource/"))
	}))
	t.Cleanup(server.Close)
	client := newResourceDownloadClient(t, server)
	result, err := client.DownloadAllWith(context.Background(), []string{
		server.URL + "/resource/first.jpg",
		server.URL + "/resource/bad.jpg",
		server.URL + "/resource/last.jpg",
	}, pixiv.DownloadOptions{DownloadPath: t.TempDir(), Concurrency: 2})
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	require.NotNil(t, result.Items[0].Result)
	assert.Equal(t, "first.jpg", filepath.Base(result.Items[0].Result.Files[0].Path))
	require.Error(t, result.Items[1].Err)
	assert.True(t, result.Items[1].Attempted)
	assert.Nil(t, result.Items[1].Result)
	require.NotNil(t, result.Items[2].Result)
	assert.Equal(t, "last.jpg", filepath.Base(result.Items[2].Result.Files[0].Path))
	invalid, err := client.DownloadAllWith(context.Background(), []string{"not a source"}, pixiv.DownloadOptions{DownloadPath: t.TempDir()})
	require.NoError(t, err)
	assert.False(t, invalid.Items[0].Attempted)
	require.Error(t, invalid.Items[0].Err)
}

func TestDownloadWithAcceptsPIDAndArtworkURL(t *testing.T) {
	var resourceHits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/illust/detail":
			if got := r.URL.Query().Get("illust_id"); got != "42" {
				http.Error(w, "unexpected illust", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `{"illust":{"id":42,"title":"work","type":"illust","page_count":1,"user":{"id":7,"name":"artist"},"tags":[],"image_urls":{},"meta_single_page":{"original_image_url":%q},"meta_pages":[]}}`, "https://"+r.Host+"/resource/42.jpg")
		case "/resource/42.jpg":
			resourceHits.Add(1)
			_, _ = io.WriteString(w, "image")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access",
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: parsed.Host, PathPrefixes: []string{"/resource/"}}}},
	})
	require.NoError(t, err)
	for _, source := range []string{"42", "https://www.pixiv.net/en/artworks/42?from=share"} {
		result, err := client.DownloadWith(context.Background(), source, pixiv.DownloadOptions{DownloadPath: t.TempDir()})
		require.NoError(t, err)
		assert.Equal(t, pixiv.DownloadSourceArtwork, result.SourceKind)
		assert.EqualValues(t, 42, result.IllustID)
		assert.Equal(t, "work", result.Title)
		require.Len(t, result.Files, 1)
	}
	assert.EqualValues(t, 2, resourceHits.Load())
}

func TestArtworkArchiveRecordsOnlyCompletedArtworkAndWritesSidecars(t *testing.T) {
	var resourceHits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/illust/detail":
			_, _ = fmt.Fprintf(w, `{"illust":{"id":42,"title":"work","type":"illust","page_count":1,"create_date":"2026-08-01T00:00:00Z","user":{"id":7,"name":"artist"},"tags":[{"name":"tag"}],"image_urls":{},"meta_single_page":{"original_image_url":%q},"meta_pages":[]}}`, "https://"+r.Host+"/resource/42.jpg")
		case "/resource/42.jpg":
			resourceHits.Add(1)
			_, _ = io.WriteString(w, "image")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access", ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: parsed.Host, PathPrefixes: []string{"/resource/"}}}}})
	require.NoError(t, err)
	directory := t.TempDir()
	archive := filepath.Join(t.TempDir(), "state", "archive.sqlite")
	options := pixiv.DownloadOptions{DownloadPath: directory, ArchivePath: archive, WriteMetadata: true}
	first, err := client.DownloadWith(context.Background(), "42", options)
	require.NoError(t, err)
	require.False(t, first.Skipped)
	require.Len(t, first.Files, 1)
	sidecarBody, err := os.ReadFile(first.Files[0].Path + ".json")
	require.NoError(t, err)
	var sidecar struct {
		Artifact string `json:"artifact"`
		Illust   struct {
			ID int64 `json:"id"`
		} `json:"illust"`
	}
	require.NoError(t, json.Unmarshal(sidecarBody, &sidecar))
	assert.Equal(t, filepath.Base(first.Files[0].Path), sidecar.Artifact)
	assert.EqualValues(t, 42, sidecar.Illust.ID)
	assert.NotContains(t, string(sidecarBody), "access")
	second, err := client.DownloadWith(context.Background(), "42", options)
	require.NoError(t, err)
	assert.True(t, second.Skipped)
	assert.EqualValues(t, 1, resourceHits.Load(), "archive must skip before resource retrieval")
}

func TestDirectResourceFilterIsRejectedBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := newResourceDownloadClient(t, server)
	filter, err := pixiv.CompileIllustFilter("bookmarkCount >= 1")
	require.NoError(t, err)
	_, err = client.DownloadWith(context.Background(), server.URL+"/resource/file.jpg", pixiv.DownloadOptions{DownloadPath: t.TempDir(), Filter: filter})
	require.Error(t, err)
	assert.Zero(t, calls.Load())
}

func TestResourceDownloadRetriesRetryableFailuresOnly(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, "image")
	}))
	t.Cleanup(server.Close)
	client := newResourceDownloadClient(t, server)
	result, err := client.DownloadWith(context.Background(), server.URL+"/resource/file.jpg", pixiv.DownloadOptions{DownloadPath: t.TempDir(), RetryPolicy: &pixiv.RetryPolicy{Retries: 3, InitialDelay: time.Millisecond}})
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.EqualValues(t, 3, attempts.Load())
}

func TestResourceDownloadUsesValidRetryAfterForRateLimit(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, "image")
	}))
	t.Cleanup(server.Close)
	client := newResourceDownloadClient(t, server)
	result, err := client.DownloadWith(context.Background(), server.URL+"/resource/file.jpg", pixiv.DownloadOptions{
		DownloadPath: t.TempDir(), RetryPolicy: &pixiv.RetryPolicy{Retries: 1, InitialDelay: time.Hour},
	})
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.EqualValues(t, 2, attempts.Load())
}

func newResourceDownloadClient(t *testing.T, server *httptest.Server) *pixiv.Client {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: server.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
			Host: parsed.Host, PathPrefixes: []string{"/resource/"},
		}}},
	})
	require.NoError(t, err)
	return client
}
