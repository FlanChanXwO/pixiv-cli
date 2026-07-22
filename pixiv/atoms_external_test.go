package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestClientExportsRemainingReadAtoms(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/illust/detail", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("illust_id") != "731" {
			t.Fatalf("illust_id = %q", r.URL.Query().Get("illust_id"))
		}
		_, _ = w.Write([]byte(`{"illust":{"id":731,"page_count":1,"width":10,"height":20,"meta_single_page":{"original_image_url":"https://img/page.png"}}}`))
	})
	mux.HandleFunc("/v2/illust/related", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("illust_id") != "731" {
			t.Fatalf("illust_id = %q", r.URL.Query().Get("illust_id"))
		}
		_, _ = w.Write([]byte(`{"illusts":[{"id":9}],"next_url":null}`))
	})
	mux.HandleFunc("/v1/trending-tags/illust", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"trend_tags":[{"tag":"cat","translated_name":"猫","illust":{"id":9}}]}`))
	})
	mux.HandleFunc("/v1/ugoira/metadata", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ugoira_metadata":{"zip_urls":{"medium":"https://app/medium.zip"},"frames":[{"file":"000000.jpg","delay":60}]}}`))
	})
	mux.HandleFunc("/ajax/illust/731/ugoira_meta", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := client.IllustPages(context.Background(), 731)
	if err != nil || len(pages) != 1 || pages[0].ImageURLs.Original != "https://img/page.png" {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
	related, err := client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
	if err != nil || len(related.Illusts) != 1 || related.Illusts[0].ID != 9 {
		t.Fatalf("related=%+v err=%v", related, err)
	}
	trending, err := client.TrendingTagsIllust(context.Background())
	if err != nil || len(trending.TrendTags) != 1 || trending.TrendTags[0].Tag != "cat" {
		t.Fatalf("trending=%+v err=%v", trending, err)
	}
	ugoira, err := client.UgoiraMetadata(context.Background(), 731)
	if err != nil {
		t.Fatal(err)
	}
	if ugoira.UgoiraMetadata.ZipURLs.Medium != "https://app/medium.zip" || ugoira.UgoiraMetadata.ZipURLs.Original != "" || ugoira.UgoiraMetadata.DownloadURL != "https://app/medium.zip" || ugoira.UgoiraMetadata.DownloadQuality != pixiv.UgoiraZipQualityMedium || len(ugoira.UgoiraMetadata.Frames) != 1 || ugoira.UgoiraMetadata.Frames[0].File != "000000.jpg" {
		t.Fatalf("ugoira=%+v", ugoira)
	}
	wire, _ := json.Marshal(ugoira)
	if strings.Contains(string(wire), `"original"`) || !strings.Contains(string(wire), `"download_url":"https://app/medium.zip"`) || !strings.Contains(string(wire), `"download_quality":"medium"`) {
		t.Fatalf("wire=%s", wire)
	}
}

func TestIllustPagesUsesAppDetailWithoutWebEnrichment(t *testing.T) {
	t.Parallel()

	const illustID int64 = 731
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/illust/detail" || r.URL.Query().Get("illust_id") != "731" {
			t.Fatalf("App request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"illust":{"id":731,"page_count":2,"meta_pages":[
			{"page_index":0,"width":100,"height":200,"extension":"png","image_urls":{"original":"https://img/731_p0.png"}},
			{"page_index":1,"width":300,"height":400,"extension":"jpg","image_urls":{"original":"https://img/731_p1.jpg"}}
		]}}`))
	}))
	t.Cleanup(appServer.Close)

	var webRequests atomic.Int32
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(webServer.Close)

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: appServer.Client(), AppAPIBaseURL: appServer.URL, WebAPIBaseURL: webServer.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := client.IllustPages(context.Background(), illustID)
	if err != nil {
		t.Fatalf("IllustPages() error = %v", err)
	}
	if len(pages) != 2 || pages[0].ImageURLs.Original != "https://img/731_p0.png" || pages[1].ImageURLs.Original != "https://img/731_p1.jpg" {
		t.Fatalf("pages = %#v", pages)
	}
	if webRequests.Load() != 0 {
		t.Fatalf("Web requests = %d, want 0", webRequests.Load())
	}
}

func TestIllustPagesDerivesSingleAppPage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" || r.URL.Query().Get("illust_id") != "732" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"illust":{"id":732,"page_count":1,"width":80,"height":90,"image_urls":{"large":"https://img/large.jpg"},"meta_single_page":{"original_image_url":"https://img/732_p0.webp"}}}`))
	}))
	t.Cleanup(server.Close)

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := client.IllustPages(context.Background(), 732)
	if err != nil {
		t.Fatalf("IllustPages() error = %v", err)
	}
	if len(pages) != 1 || pages[0].PageIndex != 0 || pages[0].Extension != "webp" || pages[0].ImageURLs.Original != "https://img/732_p0.webp" {
		t.Fatalf("pages = %#v", pages)
	}
}

func TestRemainingReadAtomsRejectLocallyAndNeverFallbackFromApp(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/v1/ugoira/metadata" {
			http.Error(w, `secret-token-in-body`, http.StatusBadGateway)
			return
		}
		t.Fatalf("unexpected fallback %s", r.URL.Path)
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "secret-access"})
	result, err := client.UgoiraMetadata(context.Background(), 731)
	if result != nil {
		t.Fatalf("partial=%+v", result)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationUgoiraMetadata || typed.Backend != pixiv.BackendAppAPI || typed.IllustID != 731 || typed.UpstreamStatus != http.StatusBadGateway {
		t.Fatalf("err=%#v", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	for _, rendered := range []string{err.Error(), errors.Unwrap(err).Error()} {
		if strings.Contains(rendered, "secret-token-in-body") || strings.Contains(rendered, "secret-access") {
			t.Fatalf("secret leaked: %s", rendered)
		}
	}

	calls = 0
	anon, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	_, err = anon.TrendingTagsIllust(context.Background())
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeUnauthorized || typed.Operation != pixiv.OperationTrendingTagsIllust || typed.Backend != "" {
		t.Fatalf("err=%#v", err)
	}
	_, err = anon.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeUnauthorized || typed.Operation != pixiv.OperationIllustRelated || typed.IllustID != 731 {
		t.Fatalf("err=%#v", err)
	}
	if calls != 0 {
		t.Fatalf("anonymous app-only calls=%d", calls)
	}
}

func TestAnonymousUgoiraUsesRealWebFields(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/illust/731/ugoira_meta" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"error":false,"body":{"src":"medium","originalSrc":"original","frames":[{"file":"0.jpg","delay":10}]}}`))
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	got, err := client.UgoiraMetadata(context.Background(), 731)
	if err != nil || got.UgoiraMetadata.ZipURLs.Medium != "medium" || got.UgoiraMetadata.ZipURLs.Original != "original" || got.UgoiraMetadata.DownloadURL != "original" || got.UgoiraMetadata.DownloadQuality != pixiv.UgoiraZipQualityOriginal {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestIllustRelatedCursorIsBoundToIllustAndUsesOnlyOffset(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v2/illust/related" || r.URL.Query().Get("illust_id") != "731" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{"illusts":[{"id":1}],"next_url":"https://evil.invalid/private?offset=30&token=secret"}`))
		case 2:
			if r.URL.Query().Get("offset") != "30" {
				t.Fatalf("offset=%q", r.URL.Query().Get("offset"))
			}
			_, _ = w.Write([]byte(`{"illusts":[],"next_url":null}`))
		default:
			t.Fatalf("calls=%d", calls)
		}
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	first, err := client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	_, err = client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 732, Cursor: first.NextCursor})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationIllustRelated || typed.IllustID != 732 || calls != 1 {
		t.Fatalf("err=%#v calls=%d", err, calls)
	}
	last, err := client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731, Cursor: first.NextCursor})
	if err != nil || last.NextCursor != "" || last.Illusts == nil || calls != 2 {
		t.Fatalf("last=%+v err=%v calls=%d", last, err, calls)
	}
}

func TestUgoiraAppMetadataProvidesMediumDownloadWithoutWeb(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/ugoira/metadata":
			_, _ = w.Write([]byte(`{"ugoira_metadata":{"zip_urls":{"medium":"app-medium"},"frames":[{"file":"0.jpg","delay":10}]}}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	result, err := client.UgoiraMetadata(context.Background(), 731)
	if err != nil || result == nil || result.UgoiraMetadata.DownloadURL != "app-medium" || result.UgoiraMetadata.DownloadQuality != pixiv.UgoiraZipQualityMedium {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestRemainingReadAtomsValidateIDsAndAuthorizationBeforeNetwork(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	auth, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	checks := []struct {
		operation pixiv.Operation
		call      func() error
	}{
		{pixiv.OperationIllustPages, func() error { _, err := auth.IllustPages(context.Background(), 0); return err }},
		{pixiv.OperationIllustRelated, func() error {
			_, err := auth.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{})
			return err
		}},
		{pixiv.OperationUgoiraMetadata, func() error { _, err := auth.UgoiraMetadata(context.Background(), 0); return err }},
	}
	for _, check := range checks {
		err := check.call()
		var typed *pixiv.Error
		if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != check.operation || typed.Backend != "" || typed.IllustID != 0 {
			t.Fatalf("operation=%s err=%#v", check.operation, err)
		}
	}
	anon, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL})
	_, err := anon.IllustPages(context.Background(), 731)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeUnauthorized || typed.Operation != pixiv.OperationIllustPages || typed.IllustID != 731 || typed.Backend != "" {
		t.Fatalf("err=%#v", err)
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}
