package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestClientIllustDetailClassifiesMalformedTransportAndContextFailures(t *testing.T) {
	t.Parallel()

	const (
		appSuccess = `{"illust":{"id":731,"title":"safe detail","type":"illust","page_count":1,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`
		webSuccess = `{"error":false,"message":"","body":[{"width":1,"height":1,"urls":{"original":"https://i.pximg.net/a.jpg"}}]}`
	)
	tests := []struct {
		name       string
		failure    string
		backend    pixiv.Backend
		operation  pixiv.Operation
		code       pixiv.ErrorCode
		retryable  bool
		contextErr error
	}{
		{name: "app empty", failure: "app-empty", backend: pixiv.BackendAppAPI, operation: pixiv.OperationIllustDetail, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "app invalid json", failure: "app-invalid-json", backend: pixiv.BackendAppAPI, operation: pixiv.OperationIllustDetail, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "app type mismatch", failure: "app-type-mismatch", backend: pixiv.BackendAppAPI, operation: pixiv.OperationIllustDetail, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "app network", failure: "app-network", backend: pixiv.BackendAppAPI, operation: pixiv.OperationIllustDetail, code: pixiv.CodeUpstreamUnavailable, retryable: true},
		{name: "app canceled", failure: "app-canceled", backend: pixiv.BackendAppAPI, operation: pixiv.OperationIllustDetail, code: pixiv.CodeUpstreamUnavailable, contextErr: context.Canceled},
		{name: "web empty", failure: "web-empty", backend: pixiv.BackendWebAPI, operation: pixiv.OperationIllustPages, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "web invalid json", failure: "web-invalid-json", backend: pixiv.BackendWebAPI, operation: pixiv.OperationIllustPages, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "web type mismatch", failure: "web-type-mismatch", backend: pixiv.BackendWebAPI, operation: pixiv.OperationIllustPages, code: pixiv.CodeMalformedUpstreamResponse},
		{name: "web network", failure: "web-network", backend: pixiv.BackendWebAPI, operation: pixiv.OperationIllustPages, code: pixiv.CodeUpstreamUnavailable, retryable: true},
		{name: "web deadline", failure: "web-deadline", backend: pixiv.BackendWebAPI, operation: pixiv.OperationIllustPages, code: pixiv.CodeUpstreamUnavailable, contextErr: context.DeadlineExceeded},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				isApp := request.URL.Host == "app.invalid"
				if isApp {
					switch test.failure {
					case "app-empty":
						return testHTTPResponse(request, http.StatusOK, ""), nil
					case "app-invalid-json":
						return testHTTPResponse(request, http.StatusOK, "{"), nil
					case "app-type-mismatch":
						return testHTTPResponse(request, http.StatusOK, `{"illust":{"id":"not-a-number"}}`), nil
					case "app-network":
						return nil, errors.New("transport-canary https://app.invalid/path?secret=query-canary")
					case "app-canceled":
						return nil, context.Canceled
					default:
						return testHTTPResponse(request, http.StatusOK, appSuccess), nil
					}
				}

				switch test.failure {
				case "web-empty":
					return testHTTPResponse(request, http.StatusOK, ""), nil
				case "web-invalid-json":
					return testHTTPResponse(request, http.StatusOK, "{"), nil
				case "web-type-mismatch":
					return testHTTPResponse(request, http.StatusOK, `{"error":false,"body":[{"width":"not-a-number"}]}`), nil
				case "web-network":
					return nil, errors.New("transport-canary https://web.invalid/path?secret=query-canary")
				case "web-deadline":
					return nil, context.DeadlineExceeded
				default:
					return testHTTPResponse(request, http.StatusOK, webSuccess), nil
				}
			})
			client, err := pixiv.NewClient(pixiv.Options{
				HTTPClient:    &http.Client{Transport: transport},
				AppAPIBaseURL: "https://app.invalid",
				WebAPIBaseURL: "https://web.invalid",
				AccessToken:   "test-token",
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			detail, err := client.IllustDetail(context.Background(), 731)
			if err == nil {
				t.Fatal("IllustDetail() error = nil, want classified failure")
			}
			if detail != nil {
				t.Errorf("IllustDetail() detail = %#v, want nil on enrichment failure", detail)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) {
				t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
			}
			if typed.Code != test.code || typed.Backend != test.backend || typed.Operation != test.operation || typed.Retryable != test.retryable || typed.UpstreamStatus != 0 || typed.IllustID != 731 {
				t.Errorf("typed error = %#v, want code=%s backend=%s operation=%s retryable=%t status=0", typed, test.code, test.backend, test.operation, test.retryable)
			}
			if test.contextErr != nil && !errors.Is(err, test.contextErr) {
				t.Errorf("errors.Is(err, %v) = false, want preserved context cause", test.contextErr)
			}
			if strings.Contains(fmt.Sprintf("%+v %v", err, errors.Unwrap(err)), "transport-canary") || strings.Contains(fmt.Sprintf("%+v %v", err, errors.Unwrap(err)), "query-canary") {
				t.Errorf("public error leaked transport canary: %v / %v", err, errors.Unwrap(err))
			}
		})
	}
}

func testHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestClientIllustDetailRoutesAnonymousAccessExplicitly(t *testing.T) {
	t.Parallel()

	t.Run("fallback enabled uses only Web detail and pages", func(t *testing.T) {
		t.Parallel()

		var appRequests atomic.Int32
		var webRequests atomic.Int32
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host == "app.invalid" {
				appRequests.Add(1)
				return testHTTPResponse(request, http.StatusInternalServerError, "unexpected app request"), nil
			}
			webRequests.Add(1)
			if got := request.Header.Get("Authorization"); got != "" {
				t.Errorf("anonymous Web Authorization = %q, want empty", got)
			}
			switch request.URL.Path {
			case "/ajax/illust/731":
				return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":{"id":"731","title":"anonymous detail","illustType":"0","pageCount":"1","userId":"99","userName":"web artist","bookmarkCount":"12","viewCount":"34","xRestrict":"1","aiType":"2","createDate":"2025-03-04T05:06:07+09:00","width":"1200","height":"800","tags":{"tags":[{"tag":"創作","translation":{"en":"original"}}]},"urls":{"regular":"https://i.pximg.net/regular.jpg"}}}`), nil
			case "/ajax/illust/731/pages":
				return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":[{"width":"1200","height":"800","urls":{"thumb_mini":"https://i.pximg.net/thumb.jpg","small":"https://i.pximg.net/small.jpg","regular":"https://i.pximg.net/regular.jpg","original":"https://i.pximg.net/original.png"}}]}`), nil
			default:
				return testHTTPResponse(request, http.StatusNotFound, "unexpected path"), nil
			}
		})
		client, err := pixiv.NewClient(pixiv.Options{
			HTTPClient:         &http.Client{Transport: transport},
			AppAPIBaseURL:      "https://app.invalid",
			WebAPIBaseURL:      "https://web.invalid",
			WebFallbackEnabled: true,
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}

		detail, err := client.IllustDetail(context.Background(), 731)
		if err != nil {
			t.Fatalf("IllustDetail() error = %v", err)
		}
		if detail.Illust.ID != 731 || detail.Illust.Title != "anonymous detail" || detail.Illust.User.ID != 99 || detail.Illust.TotalBookmarks != 12 || detail.Illust.TotalView != 34 {
			t.Errorf("anonymous detail = %#v, want complete Web mapping", detail.Illust)
		}
		if detail.Illust.Type != "illust" || detail.Illust.XRestrict != 1 || detail.Illust.AIType != 2 || detail.Illust.CreateDate != "2025-03-04T05:06:07+09:00" || detail.Illust.Width != 1200 || detail.Illust.Height != 800 {
			t.Errorf("anonymous metadata = %#v, want complete Web metadata", detail.Illust)
		}
		if len(detail.Illust.Tags) != 1 || detail.Illust.Tags[0].Name != "創作" || detail.Illust.Tags[0].TranslatedName != "original" {
			t.Errorf("anonymous tags = %#v, want translated Web tags", detail.Illust.Tags)
		}
		if len(detail.Illust.MetaPages) != 1 || detail.Illust.MetaPages[0].Extension != "png" || detail.Illust.MetaPages[0].ImageURLs.Original != "https://i.pximg.net/original.png" {
			t.Errorf("anonymous pages = %#v, want complete Web page", detail.Illust.MetaPages)
		}
		if appRequests.Load() != 0 || webRequests.Load() != 2 {
			t.Errorf("backend requests = App %d, Web %d; want App 0, Web 2", appRequests.Load(), webRequests.Load())
		}
	})

	t.Run("fallback disabled rejects locally", func(t *testing.T) {
		t.Parallel()

		var requests atomic.Int32
		client, err := pixiv.NewClient(pixiv.Options{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				return testHTTPResponse(request, http.StatusInternalServerError, "unexpected request"), nil
			})},
			AccessToken:        "   ",
			WebFallbackEnabled: false,
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}

		_, err = client.IllustDetail(context.Background(), 731)
		var typed *pixiv.Error
		if !errors.As(err, &typed) {
			t.Fatalf("IllustDetail() error = %v, want *pixiv.Error", err)
		}
		if typed.Code != pixiv.CodeUnauthorized || typed.Operation != pixiv.OperationIllustDetail || typed.Backend != "" || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 731 {
			t.Errorf("typed error = %#v, want local unauthorized detail error", typed)
		}
		if !errors.Is(err, pixiv.ErrUnauthorized) {
			t.Error("errors.Is(err, ErrUnauthorized) = false")
		}
		if requests.Load() != 0 {
			t.Errorf("network requests = %d, want 0", requests.Load())
		}
	})

	t.Run("anonymous Web failure reports detail operation", func(t *testing.T) {
		t.Parallel()

		var appRequests atomic.Int32
		client, err := pixiv.NewClient(pixiv.Options{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host == "app.invalid" {
					appRequests.Add(1)
				}
				return testHTTPResponse(request, http.StatusForbidden, "anonymous failure canary"), nil
			})},
			AppAPIBaseURL:      "https://app.invalid",
			WebAPIBaseURL:      "https://web.invalid",
			WebFallbackEnabled: true,
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}

		_, err = client.IllustDetail(context.Background(), 731)
		var typed *pixiv.Error
		if !errors.As(err, &typed) {
			t.Fatalf("IllustDetail() error = %v, want *pixiv.Error", err)
		}
		if typed.Code != pixiv.CodeForbidden || typed.Backend != pixiv.BackendWebAPI || typed.Operation != pixiv.OperationIllustDetail || typed.IllustID != 731 {
			t.Errorf("typed error = %#v, want anonymous Web detail metadata", typed)
		}
		if appRequests.Load() != 0 {
			t.Errorf("App requests = %d, want 0", appRequests.Load())
		}
	})
}

func TestClientIllustDetailMapsWebEnrichmentFailureWithoutLeakingSecrets(t *testing.T) {
	t.Parallel()

	const (
		accessToken  = "access-token-canary"
		refreshToken = "refresh-token-canary"
		cookie       = "PHPSESSID=cookie-canary"
		queryURL     = "https://example.invalid/callback?oauth_code=query-canary"
		proxyURL     = "http://proxy-user:proxy-password-canary@proxy.invalid:7890"
		responseBody = "response-body-canary"
	)
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Errorf("App Authorization = %q, want injected bearer", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"illust":{"id":731,"title":"safe detail","type":"illust","page_count":1,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`)
	}))
	defer appServer.Close()

	var webRequests atomic.Int32
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Web Authorization = %q, want empty", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("Web Cookie = %q, want empty", got)
		}
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "Authorization: Bearer %s Cookie: %s refresh_token=%s url=%s proxy=%s body=%s", accessToken, cookie, refreshToken, queryURL, proxyURL, responseBody)
	}))
	defer webServer.Close()

	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:    appServer.Client(),
		AppAPIBaseURL: appServer.URL,
		WebAPIBaseURL: webServer.URL,
		AccessToken:   "  " + accessToken + "  ",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.IllustDetail(context.Background(), 731)
	if err == nil {
		t.Fatal("IllustDetail() error = nil, want Web enrichment error")
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.Code != pixiv.CodeUpstreamError || typed.Operation != pixiv.OperationIllustPages || typed.Backend != pixiv.BackendWebAPI || !typed.Retryable || typed.UpstreamStatus != http.StatusBadGateway || typed.IllustID != 731 {
		t.Errorf("typed error = %#v, want Web pages upstream_error", typed)
	}
	if webRequests.Load() != 1 {
		t.Errorf("Web requests = %d, want 1", webRequests.Load())
	}
	if !errors.Is(err, pixiv.ErrUpstreamError) {
		t.Error("errors.Is(err, ErrUpstreamError) = false")
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatal("errors.Unwrap(err) = nil, want safe cause")
	}
	outputs := []string{err.Error(), fmt.Sprint(err), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), unwrapped.Error()}
	canaries := []string{accessToken, refreshToken, cookie, queryURL, "query-canary", proxyURL, "proxy-password-canary", responseBody}
	for _, output := range outputs {
		for _, canary := range canaries {
			if strings.Contains(output, canary) {
				t.Errorf("public error output leaked canary %q: %q", canary, output)
			}
		}
	}
}

func TestClientIllustDetailMapsAppHTTPFailuresWithoutWebFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		code      pixiv.ErrorCode
		retryable bool
		sentinel  error
	}{
		{name: "bad request", status: http.StatusBadRequest, code: pixiv.CodeInvalidArgument, sentinel: pixiv.ErrInvalidArgument},
		{name: "unauthorized", status: http.StatusUnauthorized, code: pixiv.CodeUnauthorized, sentinel: pixiv.ErrUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, code: pixiv.CodeForbidden, sentinel: pixiv.ErrForbidden},
		{name: "not found", status: http.StatusNotFound, code: pixiv.CodeArtworkUnavailable, sentinel: pixiv.ErrArtworkUnavailable},
		{name: "rate limited", status: http.StatusTooManyRequests, code: pixiv.CodeRateLimited, retryable: true, sentinel: pixiv.ErrRateLimited},
		{name: "internal error", status: http.StatusInternalServerError, code: pixiv.CodeUpstreamError, retryable: true, sentinel: pixiv.ErrUpstreamError},
		{name: "bad gateway", status: http.StatusBadGateway, code: pixiv.CodeUpstreamError, retryable: true, sentinel: pixiv.ErrUpstreamError},
		{name: "service unavailable", status: http.StatusServiceUnavailable, code: pixiv.CodeUpstreamUnavailable, retryable: true, sentinel: pixiv.ErrUpstreamUnavailable},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, code: pixiv.CodeUpstreamUnavailable, retryable: true, sentinel: pixiv.ErrUpstreamUnavailable},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var appRequests atomic.Int32
			appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				appRequests.Add(1)
				w.WriteHeader(test.status)
				fmt.Fprint(w, `{"error":"app-http-body-secret-canary Authorization: Bearer app-secret-canary refresh_token=refresh-secret-canary"}`)
			}))
			defer appServer.Close()

			var webRequests atomic.Int32
			webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				webRequests.Add(1)
				http.Error(w, "unexpected web request", http.StatusInternalServerError)
			}))
			defer webServer.Close()

			client, err := pixiv.NewClient(pixiv.Options{
				HTTPClient:    appServer.Client(),
				AppAPIBaseURL: appServer.URL,
				WebAPIBaseURL: webServer.URL,
				AccessToken:   "app-test-token",
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			_, err = client.IllustDetail(context.Background(), 731)
			if err == nil {
				t.Fatalf("IllustDetail() error = nil, want HTTP %d error", test.status)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) {
				t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
			}
			if typed.Code != test.code || typed.Operation != pixiv.OperationIllustDetail || typed.Backend != pixiv.BackendAppAPI || typed.Retryable != test.retryable || typed.UpstreamStatus != test.status || typed.IllustID != 731 {
				t.Errorf("typed error = %#v, want code=%s backend=app_api status=%d retryable=%t illust_id=731", typed, test.code, test.status, test.retryable)
			}
			if !errors.Is(err, test.sentinel) {
				t.Errorf("errors.Is(err, %v) = false", test.sentinel)
			}
			for _, output := range []string{err.Error(), fmt.Sprintf("%+v", err), errors.Unwrap(err).Error()} {
				for _, canary := range []string{"app-http-body-secret-canary", "app-secret-canary", "refresh-secret-canary"} {
					if strings.Contains(output, canary) {
						t.Errorf("public App HTTP error leaked canary %q: %q", canary, output)
					}
				}
			}
			if appRequests.Load() != 1 {
				t.Errorf("App requests = %d, want 1", appRequests.Load())
			}
			if webRequests.Load() != 0 {
				t.Errorf("Web requests = %d, want 0", webRequests.Load())
			}
		})
	}
}

func TestClientIllustDetailMapsWebEnvelopeErrorsSafely(t *testing.T) {
	t.Parallel()

	const envelopeMessage = "envelope-secret-canary Authorization: Bearer token-canary Cookie: cookie-canary refresh_token=refresh-canary https://callback.invalid/?code=query-canary proxy-password-canary"
	tests := []struct {
		name        string
		token       string
		fallback    bool
		failurePath string
		code        pixiv.ErrorCode
		operation   pixiv.Operation
		retryable   bool
		sentinel    error
	}{
		{name: "anonymous detail unavailable", fallback: true, failurePath: "/ajax/illust/731", code: pixiv.CodeArtworkUnavailable, operation: pixiv.OperationIllustDetail, sentinel: pixiv.ErrArtworkUnavailable},
		{name: "authenticated pages rejected", token: "app-token", failurePath: "/ajax/illust/731/pages", code: pixiv.CodeUpstreamError, operation: pixiv.OperationIllustPages, retryable: true, sentinel: pixiv.ErrUpstreamError},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host == "app.invalid" {
					return testHTTPResponse(request, http.StatusOK, `{"illust":{"id":731,"title":"safe","type":"illust","page_count":1,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`), nil
				}
				if request.URL.Path == test.failurePath {
					return testHTTPResponse(request, http.StatusOK, fmt.Sprintf(`{"error":true,"message":%q,"body":null}`, envelopeMessage)), nil
				}
				return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":{}}`), nil
			})
			client, err := pixiv.NewClient(pixiv.Options{
				HTTPClient:         &http.Client{Transport: transport},
				AppAPIBaseURL:      "https://app.invalid",
				WebAPIBaseURL:      "https://web.invalid",
				AccessToken:        test.token,
				WebFallbackEnabled: test.fallback,
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			_, err = client.IllustDetail(context.Background(), 731)
			var typed *pixiv.Error
			if !errors.As(err, &typed) {
				t.Fatalf("IllustDetail() error = %v, want *pixiv.Error", err)
			}
			if typed.Code != test.code || typed.Backend != pixiv.BackendWebAPI || typed.Operation != test.operation || typed.Retryable != test.retryable || typed.UpstreamStatus != 0 || typed.IllustID != 731 {
				t.Errorf("typed error = %#v, want code=%s operation=%s retryable=%t", typed, test.code, test.operation, test.retryable)
			}
			if !errors.Is(err, test.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false")
			}
			for _, output := range []string{err.Error(), fmt.Sprintf("%+v", err), errors.Unwrap(err).Error()} {
				for _, canary := range []string{"envelope-secret-canary", "token-canary", "cookie-canary", "refresh-canary", "query-canary", "proxy-password-canary"} {
					if strings.Contains(output, canary) {
						t.Errorf("public envelope error leaked canary %q: %q", canary, output)
					}
				}
			}
		})
	}
}

func TestClientIllustDetailPreservesAnonymousPagesFailureStage(t *testing.T) {
	t.Parallel()

	var appRequests atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "app.invalid" {
			appRequests.Add(1)
			return testHTTPResponse(request, http.StatusInternalServerError, "unexpected App request"), nil
		}
		switch request.URL.Path {
		case "/ajax/illust/731":
			return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":{"id":"731","title":"anonymous detail","pageCount":"1"}}`), nil
		case "/ajax/illust/731/pages":
			return testHTTPResponse(request, http.StatusBadGateway, "pages-body-secret-canary"), nil
		default:
			return testHTTPResponse(request, http.StatusNotFound, "unexpected path"), nil
		}
	})
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:         &http.Client{Transport: transport},
		AppAPIBaseURL:      "https://app.invalid",
		WebAPIBaseURL:      "https://web.invalid",
		WebFallbackEnabled: true,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), 731)
	if err == nil {
		t.Fatal("IllustDetail() error = nil, want anonymous pages failure")
	}
	if detail != nil {
		t.Errorf("IllustDetail() detail = %#v, want no partial result", detail)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("IllustDetail() error = %v, want *pixiv.Error", err)
	}
	if typed.Code != pixiv.CodeUpstreamError || typed.Backend != pixiv.BackendWebAPI || typed.Operation != pixiv.OperationIllustPages || !typed.Retryable || typed.UpstreamStatus != http.StatusBadGateway || typed.IllustID != 731 {
		t.Errorf("typed error = %#v, want anonymous Web pages metadata", typed)
	}
	if appRequests.Load() != 0 {
		t.Errorf("App requests = %d, want 0", appRequests.Load())
	}
	for _, output := range []string{err.Error(), fmt.Sprintf("%+v", err), errors.Unwrap(err).Error()} {
		if strings.Contains(output, "pages-body-secret-canary") {
			t.Errorf("public error leaked pages body: %q", output)
		}
	}
}

func TestClientIllustDetailRejectsAppResponseWithoutIllust(t *testing.T) {
	t.Parallel()

	var webRequests atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "app.invalid" {
			return testHTTPResponse(request, http.StatusOK, `{}`), nil
		}
		webRequests.Add(1)
		return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":[]}`), nil
	})
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:    &http.Client{Transport: transport},
		AppAPIBaseURL: "https://app.invalid",
		WebAPIBaseURL: "https://web.invalid",
		AccessToken:   "app-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), 731)
	if detail != nil {
		t.Errorf("IllustDetail() detail = %#v, want nil", detail)
	}
	assertMalformedDetailError(t, err, pixiv.BackendAppAPI)
	if webRequests.Load() != 0 {
		t.Errorf("Web requests = %d, want 0 after malformed App detail", webRequests.Load())
	}
}

func TestClientIllustDetailRejectsAppIllustWithoutRequiredID(t *testing.T) {
	t.Parallel()

	var webRequests atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "app.invalid" {
			return testHTTPResponse(request, http.StatusOK, `{"illust":{}}`), nil
		}
		webRequests.Add(1)
		return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":[]}`), nil
	})
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:    &http.Client{Transport: transport},
		AppAPIBaseURL: "https://app.invalid",
		WebAPIBaseURL: "https://web.invalid",
		AccessToken:   "app-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), 731)
	if detail != nil {
		t.Errorf("IllustDetail() detail = %#v, want nil", detail)
	}
	assertMalformedDetailError(t, err, pixiv.BackendAppAPI)
	if webRequests.Load() != 0 {
		t.Errorf("Web requests = %d, want 0 after malformed App illust", webRequests.Load())
	}
}

func TestClientIllustDetailRejectsWebEnvelopeWithoutBody(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/ajax/illust/731":
			return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":""}`), nil
		case "/ajax/illust/731/pages":
			return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":[]}`), nil
		default:
			return testHTTPResponse(request, http.StatusNotFound, "unexpected path"), nil
		}
	})
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:         &http.Client{Transport: transport},
		WebAPIBaseURL:      "https://web.invalid",
		WebFallbackEnabled: true,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), 731)
	if detail != nil {
		t.Errorf("IllustDetail() detail = %#v, want nil", detail)
	}
	assertMalformedDetailError(t, err, pixiv.BackendWebAPI)
}

func TestClientIllustDetailRejectsWebBodyWithoutRequiredID(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/ajax/illust/731":
			return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":{}}`), nil
		case "/ajax/illust/731/pages":
			return testHTTPResponse(request, http.StatusOK, `{"error":false,"message":"","body":[]}`), nil
		default:
			return testHTTPResponse(request, http.StatusNotFound, "unexpected path"), nil
		}
	})
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:         &http.Client{Transport: transport},
		WebAPIBaseURL:      "https://web.invalid",
		WebFallbackEnabled: true,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), 731)
	if detail != nil {
		t.Errorf("IllustDetail() detail = %#v, want nil", detail)
	}
	assertMalformedDetailError(t, err, pixiv.BackendWebAPI)
}

func TestClientIllustDetailDistinguishesMissingAndEmptyWebPagesBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		webBody   string
		wantError bool
	}{
		{name: "missing body is malformed", webBody: `{"error":false,"message":""}`, wantError: true},
		{name: "empty array is valid", webBody: `{"error":false,"message":"","body":[]}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host == "app.invalid" {
					return testHTTPResponse(request, http.StatusOK, `{"illust":{"id":731,"title":"safe","type":"illust","page_count":0,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`), nil
				}
				return testHTTPResponse(request, http.StatusOK, test.webBody), nil
			})
			client, err := pixiv.NewClient(pixiv.Options{
				HTTPClient:    &http.Client{Transport: transport},
				AppAPIBaseURL: "https://app.invalid",
				WebAPIBaseURL: "https://web.invalid",
				AccessToken:   "app-token",
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			detail, err := client.IllustDetail(context.Background(), 731)
			if !test.wantError {
				if err != nil {
					t.Fatalf("IllustDetail() error = %v, want valid empty pages", err)
				}
				if detail == nil || detail.Illust.MetaPages == nil || len(detail.Illust.MetaPages) != 0 {
					t.Errorf("meta pages = %#v, want non-nil empty slice", detail)
				}
				return
			}
			if detail != nil {
				t.Errorf("IllustDetail() detail = %#v, want nil", detail)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %v, want *pixiv.Error", err)
			}
			if typed.Code != pixiv.CodeMalformedUpstreamResponse || typed.Backend != pixiv.BackendWebAPI || typed.Operation != pixiv.OperationIllustPages || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 731 {
				t.Errorf("typed error = %#v, want malformed Web pages error", typed)
			}
		})
	}
}

func assertMalformedDetailError(t *testing.T, err error, backend pixiv.Backend) {
	t.Helper()
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v, want *pixiv.Error", err)
	}
	if typed.Code != pixiv.CodeMalformedUpstreamResponse || typed.Backend != backend || typed.Operation != pixiv.OperationIllustDetail || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 731 {
		t.Errorf("typed error = %#v, want malformed detail for %s", typed, backend)
	}
	if !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
		t.Error("errors.Is(err, ErrMalformedUpstreamResponse) = false")
	}
}

func TestClientIllustDetailRejectsInvalidIDWithTypedError(t *testing.T) {
	t.Parallel()

	requests := 0
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("network must not be reached")
	})}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.IllustDetail(context.Background(), 0)
	if err == nil {
		t.Fatal("IllustDetail() error = nil, want invalid argument")
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationIllustDetail || typed.Backend != "" || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 0 {
		t.Errorf("typed error = %#v, want local invalid_argument detail error", typed)
	}
	if !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Errorf("errors.Is(err, ErrInvalidArgument) = false")
	}
	if errors.Unwrap(err) == nil {
		t.Error("errors.Unwrap(err) = nil, want safe cause")
	}
	if requests != 0 {
		t.Errorf("network requests = %d, want 0", requests)
	}

	// 这些值属于调用方持久化或分支判断的稳定 wire contract。
	wantCodes := map[pixiv.ErrorCode]string{
		pixiv.CodeInvalidArgument:           "invalid_argument",
		pixiv.CodeArtworkUnavailable:        "artwork_unavailable",
		pixiv.CodeUnauthorized:              "unauthorized",
		pixiv.CodeForbidden:                 "forbidden",
		pixiv.CodeUnsupported:               "unsupported",
		pixiv.CodeRateLimited:               "rate_limited",
		pixiv.CodeUpstreamError:             "upstream_error",
		pixiv.CodeUpstreamUnavailable:       "upstream_unavailable",
		pixiv.CodeMalformedUpstreamResponse: "malformed_upstream_response",
	}
	for got, want := range wantCodes {
		if string(got) != want {
			t.Errorf("error code = %q, want %q", got, want)
		}
	}
	if pixiv.BackendAppAPI != "app_api" || pixiv.BackendWebAPI != "web_api" || pixiv.BackendOAuth != "oauth" || pixiv.BackendResource != "resource" {
		t.Errorf("backend wire values changed")
	}
	if pixiv.OperationIllustDetail != "illust_detail" || pixiv.OperationIllustPages != "illust_pages" {
		t.Errorf("operation wire values changed")
	}
	if !errors.Is(pixiv.ErrUnsupported, pixiv.ErrUnsupported) {
		t.Error("unsupported sentinel does not match itself")
	}
}
