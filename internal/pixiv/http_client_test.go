package pixiv

import (
	"net/http"
	"testing"
)

func TestHTTPClientLeavesRequestLifetimeToContext(t *testing.T) {
	client, err := HTTPClient("")
	if err != nil {
		t.Fatalf("HTTPClient returned error: %v", err)
	}
	if client.Timeout != 0 {
		t.Fatalf("timeout = %v, want zero", client.Timeout)
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
