package pixiv

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPClientKeepsDefaultTimeout(t *testing.T) {
	client, err := HTTPClient("")
	if err != nil {
		t.Fatalf("HTTPClient returned error: %v", err)
	}
	if client.Timeout != 60*time.Second {
		t.Fatalf("timeout = %v, want 60s", client.Timeout)
	}
}

func TestHTTPClientKeepsProxyAndDefaultTimeout(t *testing.T) {
	client, err := HTTPClient("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("HTTPClient returned error: %v", err)
	}
	if client.Timeout != 60*time.Second {
		t.Fatalf("timeout = %v, want 60s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("proxy function is nil")
	}
}
