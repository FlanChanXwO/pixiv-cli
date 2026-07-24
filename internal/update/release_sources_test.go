package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReleaseSourcesAcceptsPathAndQueryTemplates(t *testing.T) {
	t.Parallel()

	sources, err := parseReleaseSources([]byte("" +
		"prefix|https://proxy.example/{url}|https://proxy.example/{url}\n" +
		"query|https://proxy.example/api?target={url_query}|https://proxy.example/download?target={url_query}\n"))
	if err != nil {
		t.Fatalf("parseReleaseSources() error = %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("source count = %d, want 2", len(sources))
	}

	canonical := "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt?part=1"
	if got, err := sources[0].assetURL(canonical); err != nil || got != "https://proxy.example/"+canonical {
		t.Fatalf("prefix assetURL() = %q, %v", got, err)
	}
	if got, err := sources[1].assetURL(canonical); err != nil || got != "https://proxy.example/download?target=https%3A%2F%2Fgithub.com%2FFlanChanXwO%2Fpixiv-cli%2Freleases%2Fdownload%2Fv1.2.3%2Fchecksums.txt%3Fpart%3D1" {
		t.Fatalf("query assetURL() = %q, %v", got, err)
	}
}

func TestParseReleaseSourcesRejectsAmbiguousOrUnsafeEntries(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"duplicate identifier": "a|{url}|{url}\na|{url}|{url}\n",
		"both placeholders":    "a|https://proxy.example/{url}{url_query}|{url}\n",
		"missing placeholder":  "a|https://proxy.example/api|{url}\n",
		"invalid field count":  "a|{url}\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseReleaseSources([]byte(content)); err == nil {
				t.Fatal("parseReleaseSources() succeeded, want error")
			}
		})
	}
}

func TestReleaseSourceSelectorChoosesFastestValidAPIResponse(t *testing.T) {
	t.Parallel()

	slowStarted := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowStarted <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(slow.Close)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-slowStarted:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.2.3","draft":false,"prerelease":false}]`)
	}))
	t.Cleanup(fast.Close)

	sources := mustTestReleaseSources(t,
		"slow|"+slow.URL+"?target={url_query}|"+slow.URL+"?target={url_query}",
		"fast|"+fast.URL+"?target={url_query}|"+fast.URL+"?target={url_query}",
	)
	selector := newReleaseSourceSelector(sources, fast.Client())
	ordered, err := selector.ordered(context.Background(), releaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if err != nil {
		t.Fatalf("ordered() error = %v", err)
	}
	if got := ordered[0].id; got != "fast" {
		t.Fatalf("first selected source = %q, want fast", got)
	}
}

func TestReleaseSourceSelectorSkipsInvalidAPIResponse(t *testing.T) {
	t.Parallel()

	invalidDone := make(chan struct{}, 1)
	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invalidDone <- struct{}{}
		fmt.Fprint(w, "not GitHub releases JSON")
	}))
	t.Cleanup(invalid.Close)
	valid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-invalidDone:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(valid.Close)

	sources := mustTestReleaseSources(t,
		"invalid|"+invalid.URL+"?target={url_query}|"+invalid.URL+"?target={url_query}",
		"valid|"+valid.URL+"?target={url_query}|"+valid.URL+"?target={url_query}",
	)
	ordered, err := newReleaseSourceSelector(sources, valid.Client()).ordered(context.Background(), releaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if err != nil {
		t.Fatalf("ordered() error = %v", err)
	}
	if got := ordered[0].id; got != "valid" {
		t.Fatalf("first selected source = %q, want valid", got)
	}
}

func TestReleaseSourceSelectorReturnsParentCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	sources := mustTestReleaseSources(t, "blocked|"+server.URL+"?target={url_query}|"+server.URL+"?target={url_query}")
	context, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newReleaseSourceSelector(sources, server.Client()).ordered(context, releaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if !errors.Is(err, context.Err()) {
		t.Fatalf("ordered() error = %v, want canceled context error", err)
	}
}

func TestReleaseAssetDownloadRetriesRemainingSourcesAfterPreferredSourceFails(t *testing.T) {
	t.Parallel()

	const canonical = "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt"
	preferred := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(preferred.Close)
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "verified checksums")
	}))
	t.Cleanup(fallback.Close)

	sources := mustTestReleaseSources(t,
		"preferred|-|"+preferred.URL+"?target={url_query}",
		"fallback|-|"+fallback.URL+"?target={url_query}",
	)
	installer := NewReleaseInstaller(ReleaseInstallerOptions{HTTPClient: preferred.Client()}).(*releaseInstaller)
	body, err := installer.download(context.Background(), ReleaseAsset{Name: checksumsAssetName, DownloadURL: canonical}, sources)
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if got := string(body); got != "verified checksums" {
		t.Fatalf("downloaded body = %q, want fallback body", got)
	}
}

func TestReleaseAssetDownloadReportsEverySourceFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	sources := mustTestReleaseSources(t,
		"first|-|"+server.URL+"?target={url_query}",
		"second|-|"+server.URL+"?target={url_query}",
	)
	installer := NewReleaseInstaller(ReleaseInstallerOptions{HTTPClient: server.Client()}).(*releaseInstaller)
	_, err := installer.download(context.Background(), ReleaseAsset{Name: checksumsAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt"}, sources)
	if err == nil {
		t.Fatal("download() succeeded, want aggregate source failure")
	}
	for _, sourceID := range []string{"first", "second"} {
		if !strings.Contains(err.Error(), sourceID) {
			t.Fatalf("download() error = %v, missing source %q", err, sourceID)
		}
	}
}

func TestGitHubReleaseClientUsesSelectedRouteButCachesCanonicalAPIURL(t *testing.T) {
	t.Parallel()

	const canonical = "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases"
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("target"); got != canonical {
			t.Fatalf("proxy target = %q, want %q", got, canonical)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.2.3","draft":false,"prerelease":false}]`)
	}))
	t.Cleanup(proxy.Close)

	cacheDir := t.TempDir()
	client, err := NewGitHubReleaseClient(ReleaseClientOptions{APIBaseURL: "https://api.github.com", HTTPClient: proxy.Client(), CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}
	client.sourceSelector = newReleaseSourceSelector(mustTestReleaseSources(t, "proxy|"+proxy.URL+"?target={url_query}|"+proxy.URL+"?target={url_query}"), proxy.Client())
	result, err := client.Check(context.Background(), ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Release == nil || result.Release.TagName != "v1.2.3" {
		t.Fatalf("Check() release = %#v, want v1.2.3", result.Release)
	}
	cache, err := os.ReadFile(filepath.Join(cacheDir, releaseCacheFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cache), canonical) || strings.Contains(string(cache), proxy.URL) {
		t.Fatalf("cache must retain canonical API URL, got:\n%s", cache)
	}
}

func mustTestReleaseSources(t *testing.T, lines ...string) []releaseSource {
	t.Helper()
	sources, err := parseReleaseSources([]byte(joinReleaseSourceLines(lines)))
	if err != nil {
		t.Fatalf("parseReleaseSources() error = %v", err)
	}
	return sources
}

func joinReleaseSourceLines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line + "\n"
	}
	return result
}
