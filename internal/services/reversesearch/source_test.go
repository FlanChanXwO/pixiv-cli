package reversesearch_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
)

func TestFileSourceCreatesPrivateStableSnapshot(t *testing.T) {
	sourceDir := t.TempDir()
	snapshotDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "image.bin")
	original := []byte("original-image-payload")
	require.NoError(t, os.WriteFile(sourcePath, original, 0o600))

	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: snapshotDir})
	snapshot, err := loader.Load(context.Background(), sourcePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })

	wantDigest := sha256.Sum256(original)
	require.Equal(t, reversesearch.SourceKindFile, snapshot.Kind())
	require.Equal(t, hex.EncodeToString(wantDigest[:]), snapshot.SHA256())
	require.Equal(t, int64(len(original)), snapshot.Size())

	files, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	info, err := files[0].Info()
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	require.NoError(t, os.WriteFile(sourcePath, []byte("changed"), 0o600))
	for range 2 {
		reader, openErr := snapshot.Open()
		require.NoError(t, openErr)
		body, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.NoError(t, reader.Close())
		require.Equal(t, original, body)
	}
}

func TestHTTPSourceFetchesPrivateAddressOnceIntoSnapshot(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("remote-image"))
	}))
	t.Cleanup(server.Close)
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{
		TempDir:    t.TempDir(),
		HTTPClient: server.Client(),
	})

	snapshot, err := loader.Load(context.Background(), server.URL+"/image")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	require.Equal(t, reversesearch.SourceKindURL, snapshot.Kind())
	wantDigest := sha256.Sum256([]byte("remote-image"))
	require.Equal(t, hex.EncodeToString(wantDigest[:]), snapshot.SHA256())
	require.Equal(t, int32(1), requests.Load())

	for range 2 {
		reader, openErr := snapshot.Open()
		require.NoError(t, openErr)
		body, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.NoError(t, reader.Close())
		require.Equal(t, []byte("remote-image"), body)
	}
	require.Equal(t, int32(1), requests.Load())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestURLSourceRejectsUserinfoUnsupportedSchemeAndMalformedHTTPWithoutFetching(t *testing.T) {
	var requests atomic.Int32
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{
		TempDir: t.TempDir(),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, nil
		})},
	})

	for _, source := range []string{
		"http://user:password@example.test/image",
		"ftp://example.test/image",
		"http://%zz",
	} {
		_, err := loader.Load(context.Background(), source)
		require.Equal(t, reversesearch.CodeInvalidSource, reversesearch.CodeOf(err), source)
		require.NotContains(t, err.Error(), source)
	}
	require.Zero(t, requests.Load())
}

func TestURLSourceRevalidatesRedirectAndFinalResponseURL(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		var finalRequests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/redirect" {
				http.Redirect(w, request, "http://user:password@"+request.Host+"/final", http.StatusFound)
				return
			}
			finalRequests.Add(1)
			_, _ = w.Write([]byte("must-not-be-read"))
		}))
		t.Cleanup(server.Close)
		loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{HTTPClient: server.Client(), TempDir: t.TempDir()})

		_, err := loader.Load(context.Background(), server.URL+"/redirect")
		require.Equal(t, reversesearch.CodeSourceReadFailed, reversesearch.CodeOf(err))
		require.EqualError(t, err, "could not fetch image source")
		require.Zero(t, finalRequests.Load())
	})

	t.Run("final response", func(t *testing.T) {
		loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{
			TempDir: t.TempDir(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				finalURL, err := url.Parse("ftp://example.test/final")
				require.NoError(t, err)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(&zeroReader{}),
					Header:     make(http.Header),
					Request:    &http.Request{URL: finalURL},
				}, nil
			})},
		})

		_, err := loader.Load(context.Background(), "http://example.test/initial")
		require.Equal(t, reversesearch.CodeInvalidSource, reversesearch.CodeOf(err))
		require.EqualError(t, err, "image source must use HTTP or HTTPS")
	})
}

func TestURLSourceCancellationRemovesPartialSnapshot(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	snapshotDir := t.TempDir()
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{HTTPClient: server.Client(), TempDir: snapshotDir})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := loader.Load(ctx, server.URL+"/slow")
		result <- err
	}()

	<-started
	cancel()
	err := <-result
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, reversesearch.CodeUnknown, reversesearch.CodeOf(err))
	files, readErr := os.ReadDir(snapshotDir)
	require.NoError(t, readErr)
	require.Empty(t, files)
}

type cancelAfterFirstReadBody struct {
	cancel context.CancelFunc
	read   bool
}

func (b *cancelAfterFirstReadBody) Read(buffer []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	n := copy(buffer, "partial")
	b.cancel()
	return n, nil
}

func (*cancelAfterFirstReadBody) Close() error { return nil }

func TestCancellationDuringSnapshotCopyRemainsAnOverallCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	snapshotDir := t.TempDir()
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{
		TempDir: snapshotDir,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &cancelAfterFirstReadBody{cancel: cancel},
				Header:     make(http.Header),
				Request:    request,
			}, nil
		})},
	})

	_, err := loader.Load(ctx, "http://example.test/image")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, reversesearch.CodeUnknown, reversesearch.CodeOf(err))
	files, readErr := os.ReadDir(snapshotDir)
	require.NoError(t, readErr)
	require.Empty(t, files)
}

func TestURLSourceHTTPFailureDoesNotReadOrExposeResponseBody(t *testing.T) {
	snapshotDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("private-upstream-body"))
	}))
	t.Cleanup(server.Close)
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{HTTPClient: server.Client(), TempDir: snapshotDir})

	_, err := loader.Load(context.Background(), server.URL+"/image")
	require.Equal(t, reversesearch.CodeSourceHTTPStatus, reversesearch.CodeOf(err))
	require.EqualError(t, err, "image source returned an unsuccessful HTTP status")
	require.NotContains(t, err.Error(), "private-upstream-body")
	files, readErr := os.ReadDir(snapshotDir)
	require.NoError(t, readErr)
	require.Empty(t, files)
}

type zeroReader struct{}

func (*zeroReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestFileSourceFollowsSymlinkAndRejectsNonRegularTarget(t *testing.T) {
	sourceDir := t.TempDir()
	target := filepath.Join(sourceDir, "target.bin")
	link := filepath.Join(sourceDir, "image-link")
	require.NoError(t, os.WriteFile(target, []byte("linked-image"), 0o600))
	require.NoError(t, os.Symlink(target, link))
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()})

	snapshot, err := loader.Load(context.Background(), link)
	require.NoError(t, err)
	require.NoError(t, snapshot.Close())

	_, err = loader.Load(context.Background(), sourceDir)
	require.Equal(t, reversesearch.CodeSourceNotRegularFile, reversesearch.CodeOf(err))
	require.EqualError(t, err, "image source must be a regular file")
	require.NotContains(t, err.Error(), sourceDir)
}

func TestExistingRegularFileWinsOverURLLikeColonSyntax(t *testing.T) {
	t.Chdir(t.TempDir())
	sourcePath := "art:work.png"
	require.NoError(t, os.WriteFile(sourcePath, []byte("image"), 0o600))
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()})

	snapshot, err := loader.Load(context.Background(), sourcePath)
	require.NoError(t, err)
	require.Equal(t, reversesearch.SourceKindFile, snapshot.Kind())
	require.NoError(t, snapshot.Close())
}

func TestSnapshotCloseRemovesPayloadAndPreventsReopen(t *testing.T) {
	snapshotDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "image.bin")
	require.NoError(t, os.WriteFile(sourcePath, []byte("payload"), 0o600))
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: snapshotDir})
	snapshot, err := loader.Load(context.Background(), sourcePath)
	require.NoError(t, err)

	require.NoError(t, snapshot.Close())
	require.NoError(t, snapshot.Close())
	files, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Empty(t, files)
	_, err = snapshot.Open()
	require.Equal(t, reversesearch.CodeSnapshotFailed, reversesearch.CodeOf(err))
	require.EqualError(t, err, "image snapshot is closed")
}
