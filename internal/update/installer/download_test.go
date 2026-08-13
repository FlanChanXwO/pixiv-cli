package installer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
)

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
	body, err := installer.download(context.Background(), release.ReleaseAsset{Name: checksumsAssetName, DownloadURL: canonical}, sources)
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
	_, err := installer.download(context.Background(), release.ReleaseAsset{Name: checksumsAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt"}, sources)
	if err == nil {
		t.Fatal("download() succeeded, want aggregate source failure")
	}
	for _, sourceID := range []string{"first", "second"} {
		if !strings.Contains(err.Error(), sourceID) {
			t.Fatalf("download() error = %v, missing source %q", err, sourceID)
		}
	}
}

func mustTestReleaseSources(t *testing.T, lines ...string) []source.ReleaseSource {
	t.Helper()
	sources, err := source.ParseReleaseSources([]byte(joinReleaseSourceLines(lines)))
	if err != nil {
		t.Fatalf("ParseReleaseSources() error = %v", err)
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
