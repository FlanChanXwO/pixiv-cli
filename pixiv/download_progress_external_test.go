package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestDownloadProgressReportsKnownAggregateAndResourceMetadata(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "5")
			return
		}
		_, _ = io.WriteString(w, "he")
		_, _ = io.WriteString(w, "llo")
	}))
	defer server.Close()
	client := newResourceDownloadClient(t, server)
	var mu sync.Mutex
	events := make([]pixiv.DownloadProgress, 0)
	result, err := client.DownloadAllWith(context.Background(), []string{server.URL + "/resource/file.bin"}, pixiv.DownloadOptions{
		DownloadPath: t.TempDir(),
		Progress: func(event pixiv.DownloadProgress) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		},
	})
	if err != nil || result.Items[0].Err != nil {
		t.Fatalf("download result=%#v error=%v", result, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) < 2 {
		t.Fatalf("progress events=%#v", events)
	}
	last := events[len(events)-1]
	if !last.TotalBytesKnown || last.TotalBytes != 5 || last.TotalBytesTransferred != 5 ||
		!last.ResourceTotalKnown || last.ResourceBytesTransferred != 5 || last.SourceIndex != 0 ||
		last.Page != 0 || last.DestinationPath != result.Items[0].Result.Files[0].Path ||
		last.CompletedResources != 1 || last.TotalResources != 1 {
		t.Fatalf("last progress=%+v", last)
	}
}

func TestDownloadProgressKeepsResourceBytesWhenHEADHasNoLength(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.WriteString(w, "hello")
	}))
	defer server.Close()
	client := newResourceDownloadClient(t, server)
	var mu sync.Mutex
	events := make([]pixiv.DownloadProgress, 0)
	_, err := client.DownloadAllWith(context.Background(), []string{server.URL + "/resource/file.bin"}, pixiv.DownloadOptions{
		DownloadPath: t.TempDir(),
		Progress: func(event pixiv.DownloadProgress) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	last := events[len(events)-1]
	if last.TotalBytesKnown || last.ResourceTotalKnown || last.ResourceBytesTransferred != 5 || last.TotalBytesTransferred != 5 {
		t.Fatalf("unknown-length progress=%+v", last)
	}
}

func TestDownloadProgressIncludesValidatedPartialAfterCancellationAndResume(t *testing.T) {
	var rangedGets atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "10")
			w.Header().Set("Etag", `"v1"`)
			return
		}
		w.Header().Set("Etag", `"v1"`)
		if r.Header.Get("Range") == "bytes=5-" {
			rangedGets.Add(1)
			w.Header().Set("Content-Length", "5")
			w.Header().Set("Content-Range", "bytes 5-9/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "world")
			return
		}
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "hello")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newResourceDownloadClient(t, server)
	directory := t.TempDir()
	canceled, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, err := client.DownloadAllWith(canceled, []string{server.URL + "/resource/file.bin"}, pixiv.DownloadOptions{
		DownloadPath: directory,
		Progress: func(event pixiv.DownloadProgress) {
			if event.ResourceBytesTransferred >= 5 {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) || len(first.Items) != 1 {
		t.Fatalf("canceled download result=%#v error=%v", first, err)
	}

	var resumedInitial []pixiv.DownloadProgress
	second, err := client.DownloadAllWith(context.Background(), []string{server.URL + "/resource/file.bin"}, pixiv.DownloadOptions{
		DownloadPath: directory,
		Progress:     func(event pixiv.DownloadProgress) { resumedInitial = append(resumedInitial, event) },
	})
	if err != nil || second.Items[0].Err != nil || second.Items[0].Result == nil {
		t.Fatalf("resumed download result=%#v error=%v", second, err)
	}
	if len(resumedInitial) == 0 || resumedInitial[0].ResourceBytesTransferred != 5 || rangedGets.Load() != 1 {
		t.Fatalf("resume progress=%#v ranged gets=%d", resumedInitial, rangedGets.Load())
	}
	body, err := os.ReadFile(second.Items[0].Result.Files[0].Path)
	if err != nil || string(body) != "helloworld" {
		t.Fatalf("resumed file=%q error=%v", body, err)
	}
}
