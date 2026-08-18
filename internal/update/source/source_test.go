package source_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseReleaseSourcesAcceptsPathAndQueryTemplates(t *testing.T) {
	t.Parallel()

	sources, err := source.ParseReleaseSources([]byte("" +
		"prefix|https://proxy.example/{url}|https://proxy.example/{url}\n" +
		"query|https://proxy.example/api?target={url_query}|https://proxy.example/download?target={url_query}\n"))
	if err != nil {
		t.Fatalf("ParseReleaseSources() error = %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("source count = %d, want 2", len(sources))
	}

	canonical := "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt?part=1"
	if got, err := sources[0].AssetURL(canonical); err != nil || got != "https://proxy.example/"+canonical {
		t.Fatalf("prefix AssetURL() = %q, %v", got, err)
	}
	if got, err := sources[1].AssetURL(canonical); err != nil || got != "https://proxy.example/download?target=https%3A%2F%2Fgithub.com%2FFlanChanXwO%2Fpixiv-cli%2Freleases%2Fdownload%2Fv1.2.3%2Fchecksums.txt%3Fpart%3D1" {
		t.Fatalf("query AssetURL() = %q, %v", got, err)
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
			if _, err := source.ParseReleaseSources([]byte(content)); err == nil {
				t.Fatal("ParseReleaseSources() succeeded, want error")
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
	selector := source.NewReleaseSourceSelector(sources, fast.Client())
	ordered, err := selector.Ordered(context.Background(), source.ReleaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if err != nil {
		t.Fatalf("Ordered() error = %v", err)
	}
	if got := ordered[0].ID(); got != "fast" {
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
	ordered, err := source.NewReleaseSourceSelector(sources, valid.Client()).Ordered(context.Background(), source.ReleaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if err != nil {
		t.Fatalf("Ordered() error = %v", err)
	}
	if got := ordered[0].ID(); got != "valid" {
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

	_, err := source.NewReleaseSourceSelector(sources, server.Client()).Ordered(context, source.ReleaseSourceAPI, "https://api.github.com/repos/FlanChanXwO/pixiv-cli/releases")
	if !errors.Is(err, context.Err()) {
		t.Fatalf("Ordered() error = %v, want canceled context error", err)
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
