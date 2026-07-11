package update_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/update"
)

func TestGitHubReleaseClientSelectsLatestStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/FlanChanXwO/pixiv-cli/releases" {
			t.Fatalf("request path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %q", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"tag_name":"v9.0.0","draft":true,"prerelease":false},
			{"tag_name":"v2.0.0-rc.1","draft":false,"prerelease":true},
			{"tag_name":"v1.0.0","draft":false,"prerelease":false},
			{"tag_name":"v1.0.1","draft":false,"prerelease":false}
		]`)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Release == nil {
		t.Fatal("Check() returned no release")
	}
	if result.Release.TagName != "v1.0.1" {
		t.Fatalf("Check() tag = %q, want %q", result.Release.TagName, "v1.0.1")
	}
}

func TestGitHubReleaseClientIncludesPrereleasesOnlyWhenRequested(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"tag_name":"v1.0.0","draft":false,"prerelease":false},
			{"tag_name":"v1.1.0-rc.2","draft":false,"prerelease":false},
			{"tag_name":"v1.1.0-rc.10","draft":false,"prerelease":false},
			{"tag_name":"v0.9.0","draft":false,"prerelease":true}
		]`)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	stable, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("stable Check() error = %v", err)
	}
	if stable.Release == nil || stable.Release.TagName != "v1.0.0" {
		t.Fatalf("stable Check() release = %#v, want v1.0.0", stable.Release)
	}

	withPrerelease, err := client.Check(context.Background(), update.ReleaseCheckOptions{IncludePrerelease: true})
	if err != nil {
		t.Fatalf("prerelease Check() error = %v", err)
	}
	if withPrerelease.Release == nil || withPrerelease.Release.TagName != "v1.1.0-rc.10" {
		t.Fatalf("prerelease Check() release = %#v, want v1.1.0-rc.10", withPrerelease.Release)
	}
}

func TestGitHubReleaseClientRejectsNonSemVerPublishedRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"latest","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	_, err = client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err == nil {
		t.Fatal("Check() error = nil, want invalid published tag error")
	}
}

func TestGitHubReleaseClientSkipsGitHubPrereleaseBeforeTagValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"tag_name":"v1.0.0","draft":false,"prerelease":false},
			{"tag_name":"nightly","draft":false,"prerelease":true}
		]`)
	}))
	defer server.Close()
	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	stable, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("stable Check() error = %v", err)
	}
	if stable.Release == nil || stable.Release.TagName != "v1.0.0" {
		t.Fatalf("stable Check() release = %#v, want v1.0.0", stable.Release)
	}

	_, err = client.Check(context.Background(), update.ReleaseCheckOptions{IncludePrerelease: true})
	if err == nil || !strings.Contains(err.Error(), "nightly") {
		t.Fatalf("prerelease Check() error = %v, want visible nightly tag error", err)
	}
}

func TestGitHubReleaseClientPreservesBuildMetadataWithoutChangingPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"tag_name":"v1.2.3+build.7","draft":false,"prerelease":false},
			{"tag_name":"v1.2.3+build.8","draft":false,"prerelease":false}
		]`)
	}))
	defer server.Close()
	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Release == nil {
		t.Fatal("Check() returned no release")
	}
	if result.Release.TagName != "v1.2.3+build.7" || result.Release.Version != "1.2.3+build.7" {
		t.Fatalf("Check() release = %#v, want first equal-precedence build tag and Version with metadata", result.Release)
	}
}

func TestGitHubReleaseClientFollowsNextPagesAndRevalidatesTheirETags(t *testing.T) {
	requestCount := map[string]int{}
	var requestMu sync.Mutex
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCount[r.URL.RequestURI()]++
		count := requestCount[r.URL.RequestURI()]
		requestMu.Unlock()

		switch r.URL.Query().Get("page") {
		case "":
			if count == 1 {
				w.Header().Set("ETag", `"page-one"`)
				w.Header().Set("Link", "<"+server.URL+"/repos/FlanChanXwO/pixiv-cli/releases?page=2>; rel=\"next\"")
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
				return
			}
			if got := r.Header.Get("If-None-Match"); got != `"page-one"` {
				t.Errorf("page one If-None-Match = %q, want page ETag", got)
			}
			w.WriteHeader(http.StatusNotModified)
		case "2":
			if count == 1 {
				w.Header().Set("ETag", `"page-two"`)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `[{"tag_name":"v2.0.0","draft":false,"prerelease":false}]`)
				return
			}
			if got := r.Header.Get("If-None-Match"); got != `"page-two"` {
				t.Errorf("page two If-None-Match = %q, want page ETag", got)
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Errorf("unexpected page %q", r.URL.RawQuery)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	for check := range 2 {
		result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
		if err != nil {
			t.Fatalf("Check() #%d error = %v", check+1, err)
		}
		if result.Release == nil || result.Release.TagName != "v2.0.0" {
			t.Fatalf("Check() #%d release = %#v, want v2.0.0 from next page", check+1, result.Release)
		}
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if requestCount["/repos/FlanChanXwO/pixiv-cli/releases"] != 2 || requestCount["/repos/FlanChanXwO/pixiv-cli/releases?page=2"] != 2 {
		t.Fatalf("request counts = %#v, want every cached page revalidated", requestCount)
	}
}

func TestGitHubReleaseClientOrdersSemVerNumbersBeyondUint64(t *testing.T) {
	const aboveUint64 = "18446744073709551616"
	tests := []struct {
		name        string
		releases    string
		options     update.ReleaseCheckOptions
		wantTagName string
	}{
		{
			name: "major",
			releases: `[
				{"tag_name":"v18446744073709551615.0.0","draft":false,"prerelease":false},
				{"tag_name":"v` + aboveUint64 + `.0.0","draft":false,"prerelease":false}
			]`,
			wantTagName: "v" + aboveUint64 + ".0.0",
		},
		{
			name: "minor and patch",
			releases: `[
				{"tag_name":"v1.18446744073709551615.18446744073709551615","draft":false,"prerelease":false},
				{"tag_name":"v1.18446744073709551616.0","draft":false,"prerelease":false}
			]`,
			wantTagName: "v1." + aboveUint64 + ".0",
		},
		{
			name: "numeric prerelease identifier",
			releases: `[
				{"tag_name":"v1.0.0-18446744073709551615","draft":false,"prerelease":false},
				{"tag_name":"v1.0.0-18446744073709551616","draft":false,"prerelease":false}
			]`,
			options:     update.ReleaseCheckOptions{IncludePrerelease: true},
			wantTagName: "v1.0.0-" + aboveUint64,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, test.releases)
			}))
			defer server.Close()
			client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
				APIBaseURL: server.URL,
				CacheDir:   t.TempDir(),
			})
			if err != nil {
				t.Fatalf("NewGitHubReleaseClient() error = %v", err)
			}
			result, err := client.Check(context.Background(), test.options)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if result.Release == nil || result.Release.TagName != test.wantTagName {
				t.Fatalf("Check() release = %#v, want %q", result.Release, test.wantTagName)
			}
		})
	}
}

func TestGitHubReleaseClientRejectsUnsafeNextPages(t *testing.T) {
	tests := []struct {
		name string
		link func(serverURL string) string
		want string
	}{
		{
			name: "cross origin",
			link: func(string) string {
				return `<https://example.invalid/repos/FlanChanXwO/pixiv-cli/releases?page=2>; rel="next"`
			},
			want: "not on the GitHub API origin",
		},
		{
			name: "different GitHub API endpoint",
			link: func(serverURL string) string {
				return "<" + serverURL + "/user>; rel=\"next\""
			},
			want: "not the GitHub Releases endpoint",
		},
		{
			name: "pagination loop",
			link: func(serverURL string) string {
				return "<" + serverURL + "/repos/FlanChanXwO/pixiv-cli/releases>; rel=\"next\""
			},
			want: "pagination loop",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Link", test.link(server.URL))
				fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
			}))
			defer server.Close()

			client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
				APIBaseURL: server.URL,
				CacheDir:   t.TempDir(),
			})
			if err != nil {
				t.Fatalf("NewGitHubReleaseClient() error = %v", err)
			}
			_, err = client.Check(context.Background(), update.ReleaseCheckOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Check() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestGitHubReleaseClientDoesNotRequestAfterCacheReadContextExpiry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()
	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}
	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
		t.Fatalf("initial Check() error = %v", err)
	}

	_, err = client.Check(&expiresDuringCacheReadContext{Context: context.Background()}, update.ReleaseCheckOptions{})
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("Check() error = %v, want cache-read context deadline", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want no request after cache read context expiry", requests)
	}
}

type expiresDuringCacheReadContext struct {
	context.Context
	checks int
}

func (c *expiresDuringCacheReadContext) Err() error {
	c.checks++
	if c.checks >= 2 {
		return context.DeadlineExceeded
	}
	return nil
}

func TestGitHubReleaseClientUsesETagAndAtomicallyPersistedCache(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("first If-None-Match = %q, want empty", got)
			}
			w.Header().Set("ETag", `"releases-v1"`)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{"tag_name":"v9.0.0","draft":true,"prerelease":false},
				{"tag_name":"v1.0.0","draft":false,"prerelease":false}
			]`)
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"releases-v1"` {
				t.Fatalf("second If-None-Match = %q, want cache ETag", got)
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   cacheDir,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("read cache directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "github-releases.json" {
		t.Fatalf("cache entries = %#v, want only github-releases.json", entries)
	}
	cacheBytes, err := os.ReadFile(filepath.Join(cacheDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if !json.Valid(cacheBytes) {
		t.Fatalf("cache is not complete JSON: %q", cacheBytes)
	}
	if strings.Contains(string(cacheBytes), "v9.0.0") {
		t.Fatalf("cache contains draft release: %s", cacheBytes)
	}

	now = now.Add(time.Minute)
	result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("ETag Check() error = %v", err)
	}
	if result.Release == nil || result.Release.TagName != "v1.0.0" {
		t.Fatalf("ETag Check() release = %#v, want v1.0.0", result.Release)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

func TestGitHubReleaseClientReturnsCacheWriteFailures(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache-file")
	if err := os.WriteFile(cacheDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("create cache path fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   cacheDir,
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	_, err = client.Check(context.Background(), update.ReleaseCheckOptions{})
	if err == nil {
		t.Fatal("Check() error = nil, want cache write failure")
	}
	if !strings.Contains(err.Error(), "GitHub Releases cache") {
		t.Fatalf("Check() error = %q, want visible cache failure", err)
	}
}

func TestGitHubReleaseClientThrottlesAutomaticChecksFor24Hours(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			w.Header().Set("ETag", `"releases-v1"`)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"releases-v1"` {
				t.Fatalf("If-None-Match = %q, want cache ETag", got)
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   cacheDir,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}
	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
		t.Fatalf("initial Check() error = %v", err)
	}

	now = now.Add(24*time.Hour - time.Nanosecond)
	throttled, err := client.Check(context.Background(), update.ReleaseCheckOptions{Automatic: true})
	if err != nil {
		t.Fatalf("throttled automatic Check() error = %v", err)
	}
	if !throttled.Throttled {
		t.Fatal("automatic Check() Throttled = false, want true before 24 hours")
	}
	if requests != 1 {
		t.Fatalf("request count before 24 hours = %d, want 1", requests)
	}

	now = now.Add(time.Nanosecond)
	refreshed, err := client.Check(context.Background(), update.ReleaseCheckOptions{Automatic: true})
	if err != nil {
		t.Fatalf("refreshed automatic Check() error = %v", err)
	}
	if refreshed.Throttled {
		t.Fatal("automatic Check() Throttled = true, want network refresh at 24 hours")
	}
	if requests != 2 {
		t.Fatalf("request count at 24 hours = %d, want 2", requests)
	}
}

func TestGitHubReleaseClientUsesThreeSecondDeadlineOnlyForAutomaticChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"releases-v1"`)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	transport := &deadlineRecordingTransport{next: http.DefaultTransport}
	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	started := time.Now()
	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{Automatic: true}); err != nil {
		t.Fatalf("automatic Check() error = %v", err)
	}
	if !transport.hasDeadline {
		t.Fatal("automatic request has no deadline")
	}
	if transport.deadline.Before(started.Add(3*time.Second-100*time.Millisecond)) || transport.deadline.After(started.Add(3*time.Second+100*time.Millisecond)) {
		t.Fatalf("automatic request deadline = %s, want about three seconds after %s", transport.deadline, started)
	}

	transport.reset()
	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
		t.Fatalf("explicit Check() error = %v", err)
	}
	if transport.hasDeadline {
		t.Fatalf("explicit request unexpectedly has deadline %s", transport.deadline)
	}
}

func TestGitHubReleaseClientCancelsSlowAutomaticCheckAfterThreeSeconds(t *testing.T) {
	requestCanceled := make(chan time.Time, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		requestCanceled <- time.Now()
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}

	started := time.Now()
	_, err = client.Check(context.Background(), update.ReleaseCheckOptions{Automatic: true})
	if err == nil {
		t.Fatal("slow automatic Check() error = nil, want context deadline error")
	}
	elapsed := time.Since(started)
	if elapsed < 3*time.Second-500*time.Millisecond || elapsed > 3*time.Second+time.Second {
		t.Fatalf("slow automatic Check() elapsed = %s, want about three seconds", elapsed)
	}
	select {
	case canceledAt := <-requestCanceled:
		if canceledAt.Before(started) {
			t.Fatalf("server observed cancellation at %s before request start %s", canceledAt, started)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe automatic request cancellation")
	}
}

func TestGitHubReleaseClientAtomicallyReplacesCacheForConcurrentReaders(t *testing.T) {
	cacheDir := t.TempDir()
	tags := []string{"v1.0.0", "v1.0.1", "v1.0.2", "v1.0.3", "v1.0.4"}
	var requestMu sync.Mutex
	requestIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		tag := tags[requestIndex%len(tags)]
		requestIndex++
		requestMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"tag_name":%q,"draft":false,"prerelease":false}]`, tag)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{
		APIBaseURL: server.URL,
		CacheDir:   cacheDir,
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}
	if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
		t.Fatalf("initial Check() error = %v", err)
	}

	stopReaders := make(chan struct{})
	readerDone := make(chan struct{})
	readerErrors := make(chan error, 1)
	cachePath := filepath.Join(cacheDir, "github-releases.json")
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReaders:
				return
			default:
			}
			cacheBytes, err := os.ReadFile(cachePath)
			if err != nil {
				select {
				case readerErrors <- fmt.Errorf("read cache while replacing it: %w", err):
				case <-stopReaders:
				}
				return
			}
			if !json.Valid(cacheBytes) {
				select {
				case readerErrors <- fmt.Errorf("read incomplete cache JSON: %q", cacheBytes):
				case <-stopReaders:
				}
				return
			}
		}
	}()
	for range tags {
		if _, err := client.Check(context.Background(), update.ReleaseCheckOptions{}); err != nil {
			close(stopReaders)
			<-readerDone
			t.Fatalf("Check() while readers are active: %v", err)
		}
	}
	close(stopReaders)
	<-readerDone
	select {
	case err := <-readerErrors:
		t.Fatal(err)
	default:
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("read cache directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "github-releases.json" {
		t.Fatalf("cache entries after replacements = %#v, want only github-releases.json", entries)
	}
}

type deadlineRecordingTransport struct {
	next        http.RoundTripper
	hasDeadline bool
	deadline    time.Time
}

func (t *deadlineRecordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.deadline, t.hasDeadline = request.Context().Deadline()
	return t.next.RoundTrip(request)
}

func (t *deadlineRecordingTransport) reset() {
	t.hasDeadline = false
	t.deadline = time.Time{}
}
