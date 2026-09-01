package assembly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/ascii2d"
)

func TestNewRejectsInvalidProxy(t *testing.T) {
	if _, err := New(Options{Proxy: "not a proxy"}); err == nil {
		t.Fatal("accepted an invalid reverse-search proxy")
	}
}

func TestNewWithClientSeparatesBrowserASCII2DFromStandardClients(t *testing.T) {
	var requests []string
	standardClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.String())
		switch {
		case request.Method == http.MethodGet && request.URL.String() == "https://source.invalid/image.png":
			return assemblyResponse(request, http.StatusOK, "fixture-image"), nil
		case request.Method == http.MethodPost && request.URL.String() == "https://saucenao.com/search.php":
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
			return assemblyResponse(request, http.StatusOK, `{"header":{"status":0,"short_remaining":1,"long_remaining":1,"short_limit":2,"long_limit":2},"results":[]}`), nil
		default:
			return nil, errors.New("unexpected standard reverse-search request")
		}
	})}

	var captured ascii2d.Options
	standardProxy := "http://standard.invalid"
	ascii2dProxy := "http://proxy.invalid"
	searcher, err := newWithClientWithASCII2DFactory(
		Options{Proxy: standardProxy, ASCII2DProxy: &ascii2dProxy, UserAgent: "fixture-user-agent", SauceNAOKey: "fixture-key"},
		standardClient,
		func(options ascii2d.Options) (reversesearch.ASCII2DClient, error) {
			captured = options
			return assemblyASCII2DStub{}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct reverse-searcher: %v", err)
	}

	if captured.HTTPClient != nil {
		t.Fatal("ascii2d received the standard HTTP client instead of its browser transport")
	}
	if captured.ProxyURL != ascii2dProxy {
		t.Fatalf("ascii2d proxy = %q, want %q", captured.ProxyURL, ascii2dProxy)
	}
	if captured.UserAgent != "fixture-user-agent" {
		t.Fatalf("ascii2d user-agent = %q, want %q", captured.UserAgent, "fixture-user-agent")
	}

	if _, err := searcher.Search(context.Background(), reversesearch.Request{
		Source:   "https://source.invalid/image.png",
		Provider: reversesearch.ProviderSauceNAO,
	}); err != nil {
		t.Fatalf("search through standard source/SauceNAO clients: %v", err)
	}
	wantRequests := []string{
		"GET https://source.invalid/image.png",
		"POST https://saucenao.com/search.php",
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("standard client requests = %v, want %v", requests, wantRequests)
	}
}

type assemblyASCII2DStub struct{}

func (assemblyASCII2DStub) Preflight(context.Context) error { return nil }

func (assemblyASCII2DStub) Upload(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
	return nil, errors.New("unexpected ascii2d upload")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func assemblyResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
