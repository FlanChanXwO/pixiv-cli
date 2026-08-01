package pixiv

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/testutil/socks5test"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
)

func TestHTTPClientLeavesRequestLifetimeToContext(t *testing.T) {
	client, err := HTTPClient("")
	if err != nil {
		t.Fatalf("HTTPClient returned error: %v", err)
	}
	if client.Timeout != 0 {
		t.Fatalf("timeout = %v, want zero", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("empty proxy must disable environment proxy fallback")
	}
}

func TestHTTPClientRoutesRequestsThroughSOCKS5AndSOCKS5H(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "through socks")
	}))
	t.Cleanup(target.Close)
	proxy, err := socks5test.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })

	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			client, err := HTTPClient(proxy.URL(scheme))
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Get(target.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "through socks" {
				t.Fatalf("response = %q", body)
			}
		})
	}
}

func TestHTTPClientKeepsProxyWithoutAddingTotalTimeout(t *testing.T) {
	client, err := HTTPClient("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("HTTPClient returned error: %v", err)
	}
	if client.Timeout != 0 {
		t.Fatalf("timeout = %v, want zero", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("proxy function is nil")
	}
}

func TestHTTPClientRejectsMalformedProxyWithSafeClassifiableError(t *testing.T) {
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	client, err := HTTPClient(proxy)

	if client != nil {
		t.Fatalf("HTTPClient() client = %#v, want nil", client)
	}
	if !errors.Is(err, uri.ErrInvalidProxy) {
		t.Fatalf("HTTPClient() error = %v, want errors.Is ErrInvalidProxy", err)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
			if strings.Contains(current.Error(), secret) {
				t.Fatalf("error chain leaked %q in %q", secret, current.Error())
			}
		}
	}
}
