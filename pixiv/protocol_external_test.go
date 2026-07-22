package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

type protocolRoundTripper func(*http.Request) (*http.Response, error)

func (fn protocolRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClientUsesCentralProfilesAndCatalogWithExplicitBases(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" || r.URL.Query().Get("illust_id") != "51" {
			t.Fatalf("app request=%s", r.URL.String())
		}
		if r.Header.Get("User-Agent") != "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)" || r.Header.Get("Referer") != "https://app-api.pixiv.net/" || r.Header.Get("Authorization") != "Bearer app-token" {
			t.Fatalf("app profile=%v", r.Header)
		}
		fmt.Fprint(w, `{"illust":{"id":51,"title":"safe","type":"illust","page_count":0,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`)
	}))
	defer app.Close()
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("authenticated detail unexpectedly requested Web: %s", r.URL.String())
	}))
	defer web.Close()

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, WebAPIBaseURL: web.URL, AccessToken: "app-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.IllustDetail(context.Background(), 51); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterFailuresStayTypedAndSecretFree(t *testing.T) {
	const appSecret = "app-body-secret"
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, appSecret, http.StatusBadGateway)
	}))
	defer app.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "app-token"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 51})
	assertProtocolFailure(t, err, pixiv.CodeUpstreamError, pixiv.OperationIllustRelated, pixiv.BackendAppAPI, http.StatusBadGateway, appSecret)

	const webSecret = "web-envelope-secret"
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"error":true,"message":%q}`, webSecret)
	}))
	defer web.Close()
	client, err = pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.IllustPages(context.Background(), 51)
	assertProtocolFailure(t, err, pixiv.CodeUpstreamError, pixiv.OperationIllustPages, pixiv.BackendWebAPI, 0, webSecret)

	const oauthSecret = "oauth-body-secret"
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, oauthSecret, http.StatusForbidden)
	}))
	defer oauth.Close()
	client, err = pixiv.OpenDefault(pixiv.Options{HTTPClient: oauth.Client(), OAuthBaseURL: oauth.URL, RefreshToken: "refresh-secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
	assertProtocolFailure(t, err, pixiv.CodeForbidden, pixiv.OperationIllustRecommended, pixiv.BackendOAuth, http.StatusForbidden, oauthSecret, "refresh-secret")

	const transportSecret = "transport-error-secret"
	client, err = pixiv.NewClient(pixiv.Options{
		HTTPClient: &http.Client{Transport: protocolRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed: " + transportSecret)
		})},
		AppAPIBaseURL: "https://app-api.example.invalid",
		AccessToken:   "app-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 51})
	assertProtocolFailure(t, err, pixiv.CodeUpstreamUnavailable, pixiv.OperationIllustRelated, pixiv.BackendAppAPI, 0, transportSecret)
}

func assertProtocolFailure(t *testing.T, err error, code pixiv.ErrorCode, operation pixiv.Operation, backend pixiv.Backend, status int, secrets ...string) {
	t.Helper()
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != code || typed.Operation != operation || typed.Backend != backend || typed.UpstreamStatus != status {
		t.Fatalf("err=%#v typed=%+v", err, typed)
	}
	if !errors.Is(err, protocolCodeSentinel(code)) {
		t.Fatalf("error did not match stable code sentinel: %v", err)
	}
	rendered := err.Error()
	if cause := errors.Unwrap(err); cause != nil {
		rendered += " " + cause.Error()
	}
	for _, secret := range secrets {
		if strings.Contains(rendered, secret) {
			t.Fatalf("secret leaked: %q in %q", secret, rendered)
		}
	}
}

func protocolCodeSentinel(code pixiv.ErrorCode) error {
	switch code {
	case pixiv.CodeForbidden:
		return pixiv.ErrForbidden
	case pixiv.CodeUpstreamError:
		return pixiv.ErrUpstreamError
	case pixiv.CodeUpstreamUnavailable:
		return pixiv.ErrUpstreamUnavailable
	default:
		panic("test must declare a stable error sentinel")
	}
}
