package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestClientExportsRemainingReadAtoms(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/ajax/illust/731/pages", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("web Authorization = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatalf("web Cookie = %q", got)
		}
		_, _ = w.Write([]byte(`{"error":false,"body":[{"urls":{"original":"https://img/page.png"},"width":10,"height":20}]}`))
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
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("web Authorization = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatalf("web Cookie = %q", got)
		}
		_, _ = w.Write([]byte(`{"error":false,"body":{"src":"https://web/medium.zip","originalSrc":"https://web/original.zip","frames":[{"file":"different.jpg","delay":1}]}}`))
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
	if ugoira.UgoiraMetadata.ZipURLs.Medium != "https://app/medium.zip" || ugoira.UgoiraMetadata.ZipURLs.Original != "https://web/original.zip" || len(ugoira.UgoiraMetadata.Frames) != 1 || ugoira.UgoiraMetadata.Frames[0].File != "000000.jpg" {
		t.Fatalf("ugoira=%+v", ugoira)
	}
	wire, _ := json.Marshal(ugoira)
	if string(wire) != `{"ugoira_metadata":{"zip_urls":{"medium":"https://app/medium.zip","original":"https://web/original.zip"},"frames":[{"file":"000000.jpg","delay":60}]}}` {
		t.Fatalf("wire=%s", wire)
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
	if err != nil || got.UgoiraMetadata.ZipURLs.Medium != "medium" || got.UgoiraMetadata.ZipURLs.Original != "original" {
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

func TestUgoiraWebEnrichmentFailureReturnsNoPartialResult(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/ugoira/metadata":
			_, _ = w.Write([]byte(`{"ugoira_metadata":{"zip_urls":{"medium":"app-medium"},"frames":[{"file":"0.jpg","delay":10}]}}`))
		case "/ajax/illust/731/ugoira_meta":
			_, _ = w.Write([]byte(`{"error":false,"body":{"src":"web-medium","frames":[{"file":"0.jpg","delay":10}]}}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	result, err := client.UgoiraMetadata(context.Background(), 731)
	if result != nil {
		t.Fatalf("partial=%+v", result)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeMalformedUpstreamResponse || typed.Backend != pixiv.BackendWebAPI || typed.Operation != pixiv.OperationUgoiraMetadata || typed.IllustID != 731 {
		t.Fatalf("err=%#v", err)
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
