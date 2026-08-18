package release

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
)

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
	cachePath := filepath.Join(cacheDir, CacheFilename)
	client, err := NewGitHubReleaseClient(ReleaseClientOptions{
		APIBaseURL: "https://api.github.com",
		HTTPClient: proxy.Client(),
		Cache:      diskReleaseCache{path: cachePath},
	})
	if err != nil {
		t.Fatalf("NewGitHubReleaseClient() error = %v", err)
	}
	client.sourceSelector = source.NewReleaseSourceSelector(mustTestReleaseSources(t, "proxy|"+proxy.URL+"?target={url_query}|"+proxy.URL+"?target={url_query}"), proxy.Client())
	result, err := client.Check(context.Background(), ReleaseCheckOptions{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Release == nil || result.Release.TagName != "v1.2.3" {
		t.Fatalf("Check() release = %#v, want v1.2.3", result.Release)
	}
	cache, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cache), canonical) || strings.Contains(string(cache), proxy.URL) {
		t.Fatalf("cache must retain canonical API URL, got:\n%s", cache)
	}
}

type diskReleaseCache struct {
	path string
}

func (c diskReleaseCache) Read(_ context.Context) ([]byte, bool, error) {
	cacheBytes, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return cacheBytes, err == nil, err
}

func (c diskReleaseCache) Write(_ context.Context, data []byte) error {
	return os.WriteFile(c.path, data, 0o600)
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
